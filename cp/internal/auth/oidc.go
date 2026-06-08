package auth

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	gooidc "github.com/coreos/go-oidc/v3/oidc"
	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"golang.org/x/oauth2"
)

const oidcStateCookie = "qf_oidc_state"

// OIDCConfig holds OIDC provider configuration. Zero-value = OIDC disabled.
type OIDCConfig struct {
	Issuer       string
	ClientID     string
	ClientSecret string
	RedirectURL  string
}

// OIDCConfigFromEnv reads QF_OIDC_* env vars. Returns zero-value if any missing.
func OIDCConfigFromEnv() OIDCConfig {
	issuer := os.Getenv("QF_OIDC_ISSUER")
	clientID := os.Getenv("QF_OIDC_CLIENT_ID")
	clientSecret := os.Getenv("QF_OIDC_CLIENT_SECRET")
	redirectURL := os.Getenv("QF_OIDC_REDIRECT_URL")
	if issuer == "" || clientID == "" || clientSecret == "" {
		return OIDCConfig{}
	}
	if redirectURL == "" {
		redirectURL = "/auth/oidc/callback"
	}
	return OIDCConfig{
		Issuer:       issuer,
		ClientID:     clientID,
		ClientSecret: clientSecret,
		RedirectURL:  redirectURL,
	}
}

func (c OIDCConfig) Enabled() bool {
	return c.Issuer != ""
}

// String returns a safe representation of OIDCConfig with ClientSecret masked.
// Prevents accidental secret leakage via fmt.Sprintf("%v", cfg) or slog attrs.
func (c OIDCConfig) String() string {
	secret := "<not set>"
	if c.ClientSecret != "" {
		secret = "<masked>"
	}
	return fmt.Sprintf("OIDCConfig{Issuer:%q ClientID:%q ClientSecret:%s RedirectURL:%q}",
		c.Issuer, c.ClientID, secret, c.RedirectURL)
}

// OIDCHandler handles OIDC login redirect and callback.
type OIDCHandler struct {
	q        *storegen.Queries
	secret   []byte
	tenantID pgtype.UUID
	provider *gooidc.Provider
	verifier *gooidc.IDTokenVerifier
	oauth2   oauth2.Config
}

// NewOIDCHandler initialises the OIDC provider. Returns nil if cfg disabled.
func NewOIDCHandler(ctx context.Context, q *storegen.Queries, secret []byte, tenantID pgtype.UUID, cfg OIDCConfig) (*OIDCHandler, error) {
	if !cfg.Enabled() {
		return nil, nil
	}
	provider, err := gooidc.NewProvider(ctx, cfg.Issuer)
	if err != nil {
		return nil, fmt.Errorf("oidc provider %s: %w", cfg.Issuer, err)
	}
	verifier := provider.Verifier(&gooidc.Config{ClientID: cfg.ClientID})
	return &OIDCHandler{
		q:        q,
		secret:   secret,
		tenantID: tenantID,
		provider: provider,
		verifier: verifier,
		oauth2: oauth2.Config{
			ClientID:     cfg.ClientID,
			ClientSecret: cfg.ClientSecret,
			RedirectURL:  cfg.RedirectURL,
			Endpoint:     provider.Endpoint(),
			Scopes:       []string{gooidc.ScopeOpenID, "email", "profile"},
		},
	}, nil
}

// Login redirects to the OIDC provider authorization endpoint.
func (h *OIDCHandler) Login(w http.ResponseWriter, r *http.Request) {
	stateBytes := make([]byte, 16)
	if _, err := rand.Read(stateBytes); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	state := hex.EncodeToString(stateBytes)

	http.SetCookie(w, &http.Cookie{
		Name:     oidcStateCookie,
		Value:    state,
		Path:     "/auth/oidc",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   300,
	})

	http.Redirect(w, r, h.oauth2.AuthCodeURL(state), http.StatusFound)
}

// Callback handles the authorization code callback from the OIDC provider.
func (h *OIDCHandler) Callback(w http.ResponseWriter, r *http.Request) {
	// Verify state
	stateCookie, err := r.Cookie(oidcStateCookie)
	if err != nil || r.URL.Query().Get("state") != stateCookie.Value {
		http.Error(w, "invalid state", http.StatusBadRequest)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: oidcStateCookie, Value: "", MaxAge: -1, Path: "/auth/oidc"})

	// Exchange code
	oauth2Token, err := h.oauth2.Exchange(r.Context(), r.URL.Query().Get("code"))
	if err != nil {
		http.Error(w, "code exchange failed", http.StatusBadRequest)
		return
	}

	// Extract and verify ID token
	rawIDToken, ok := oauth2Token.Extra("id_token").(string)
	if !ok {
		http.Error(w, "missing id_token", http.StatusBadRequest)
		return
	}
	idToken, err := h.verifier.Verify(r.Context(), rawIDToken)
	if err != nil {
		http.Error(w, "id_token verification failed", http.StatusUnauthorized)
		return
	}

	var claims struct {
		Email             string `json:"email"`
		Subject           string `json:"sub"`
		PreferredUsername string `json:"preferred_username"`
	}
	if err := idToken.Claims(&claims); err != nil || claims.Email == "" {
		http.Error(w, "missing email claim", http.StatusBadRequest)
		return
	}
	// Fallback: use email local part if preferred_username not provided
	username := claims.PreferredUsername
	if username == "" {
		username = splitEmailLocal(claims.Email)
	}

	// Lookup or create user
	user, role, err := h.findOrCreateOIDCUser(r.Context(), claims.Email, username, claims.Subject)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	userIDStr := uuidStr(user.ID)
	tenantIDStr := uuidStr(h.tenantID)

	access, err := IssueAccessToken(h.secret, userIDStr, tenantIDStr, user.Username, user.Email, role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	refresh, err := IssueRefreshToken(h.secret, userIDStr)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	_ = h.q.UpdateUserLastLogin(r.Context(), user.ID)

	http.SetCookie(w, &http.Cookie{
		Name:     accessCookie,
		Value:    access,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(accessTokenTTL.Seconds()),
	})
	http.SetCookie(w, &http.Cookie{
		Name:     refreshCookie,
		Value:    refresh,
		Path:     "/auth/refresh",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(refreshTokenTTL.Seconds()),
	})

	http.Redirect(w, r, "/app", http.StatusFound)
}

// OIDCEnabled returns a JSON response indicating whether OIDC is configured.
func OIDCEnabled(enabled bool) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]bool{"enabled": enabled})
	}
}

func (h *OIDCHandler) findOrCreateOIDCUser(ctx context.Context, email, username, subject string) (storegen.User, string, error) {
	user, err := h.q.GetUserByEmail(ctx, storegen.GetUserByEmailParams{
		TenantID: h.tenantID,
		Email:    email,
	})
	if err == nil {
		if user.Status != "active" {
			return storegen.User{}, "", fmt.Errorf("user disabled")
		}
		if user.OidcSubject == nil {
			if err := h.q.UpdateUserOIDCSubject(ctx, storegen.UpdateUserOIDCSubjectParams{
				ID:          user.ID,
				TenantID:    h.tenantID,
				OidcSubject: &subject,
			}); err == nil {
				user.OidcSubject = &subject
			}
		}
		ur, _ := h.q.GetUserRole(ctx, storegen.GetUserRoleParams{UserID: user.ID, TenantID: h.tenantID})
		return user, ur.Role, nil
	}

	// New user — ensure unique username
	username = uniqueUsername(ctx, h.q, h.tenantID, username)
	user, err = h.q.CreateUser(ctx, storegen.CreateUserParams{
		TenantID:     h.tenantID,
		Email:        email,
		Username:     username,
		PasswordHash: nil,
		OidcSubject:  &subject,
	})
	if err != nil {
		return storegen.User{}, "", fmt.Errorf("create oidc user: %w", err)
	}
	if err := h.q.UpsertUserRole(ctx, storegen.UpsertUserRoleParams{
		UserID: user.ID, TenantID: h.tenantID, Role: "auditor",
	}); err != nil {
		return storegen.User{}, "", fmt.Errorf("set oidc user role: %w", err)
	}
	return user, "auditor", nil
}

func splitEmailLocal(email string) string {
	if email == "" {
		return "user"
	}
	for i, c := range email {
		if c == '@' && i > 0 {
			return email[:i]
		}
	}
	return email
}

// uniqueUsername returns username, appending -N if already taken.
func uniqueUsername(ctx context.Context, q *storegen.Queries, tenantID pgtype.UUID, base string) string {
	candidate := base
	for n := 1; n <= 99; n++ {
		_, err := q.GetUserByUsername(ctx, storegen.GetUserByUsernameParams{
			TenantID: tenantID,
			Username: candidate,
		})
		if err != nil {
			return candidate // not found = available
		}
		candidate = fmt.Sprintf("%s-%d", base, n)
	}
	return candidate
}
