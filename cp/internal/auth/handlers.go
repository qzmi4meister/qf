package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"golang.org/x/crypto/bcrypt"
)

// dummyHash is a pre-computed bcrypt hash used for constant-time comparison
// when the user does not exist, preventing username enumeration via timing.
var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("qf-dummy-password-do-not-use"), bcrypt.DefaultCost)

type Handler struct {
	q        *storegen.Queries
	secret   []byte
	tenantID pgtype.UUID
}

func NewHandler(q *storegen.Queries, secret []byte, tenantID pgtype.UUID) *Handler {
	return &Handler{q: q, secret: secret, tenantID: tenantID}
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type meResponse struct {
	ID       string `json:"id"`
	Username string `json:"username"`
	Email    string `json:"email"`
	Role     string `json:"role"`
	TenantID string `json:"tenant_id"`
}

func (h *Handler) Login(w http.ResponseWriter, r *http.Request) {
	var req loginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}

	user, err := h.q.GetUserByUsername(r.Context(), storegen.GetUserByUsernameParams{
		TenantID: h.tenantID,
		Username: req.Username,
	})
	if err != nil || user.Status != "active" {
		// Constant-time: run dummy bcrypt to prevent username enumeration via timing.
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password)) //nolint:errcheck
		h.logAuditAfter("user", "", "auth.login_failed", "user", "",
			[]byte(`{"attempted_username":"`+req.Username+`"}`))
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if user.PasswordHash == nil {
		bcrypt.CompareHashAndPassword(dummyHash, []byte(req.Password)) //nolint:errcheck
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
		h.logAudit("user", uuidStr(user.ID), "auth.login_failed", "user", uuidStr(user.ID))
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	ur, err := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{
		UserID:   user.ID,
		TenantID: h.tenantID,
	})
	if err != nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}

	userIDStr := uuidStr(user.ID)
	tenantIDStr := uuidStr(h.tenantID)

	access, err := IssueAccessToken(h.secret, userIDStr, tenantIDStr, user.Username, user.Email, ur.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	refresh, err := IssueRefreshToken(h.secret, userIDStr, user.TokenVersion)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	_ = h.q.UpdateUserLastLogin(r.Context(), user.ID)
	h.logAudit("user", userIDStr, "auth.login", "user", userIDStr)

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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meResponse{
		ID:       userIDStr,
		Username: user.Username,
		Email:    user.Email,
		Role:     ur.Role,
		TenantID: tenantIDStr,
	})
}

func (h *Handler) Refresh(w http.ResponseWriter, r *http.Request) {
	c, err := r.Cookie(refreshCookie)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	userIDStr, claimTV, err := ParseRefreshToken(h.secret, c.Value)
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(userIDStr); err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	user, err := h.q.GetUser(r.Context(), storegen.GetUserParams{
		ID:       uid,
		TenantID: h.tenantID,
	})
	if err != nil || user.Status != "active" {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	if claimTV != user.TokenVersion {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	ur, err := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{
		UserID:   user.ID,
		TenantID: h.tenantID,
	})
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantIDStr := uuidStr(h.tenantID)
	access, err := IssueAccessToken(h.secret, userIDStr, tenantIDStr, user.Username, user.Email, ur.Role)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	http.SetCookie(w, &http.Cookie{
		Name:     accessCookie,
		Value:    access,
		Path:     "/",
		HttpOnly: true,
		SameSite: http.SameSiteLaxMode,
		MaxAge:   int(accessTokenTTL.Seconds()),
	})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meResponse{
		ID:       userIDStr,
		Username: user.Username,
		Email:    user.Email,
		Role:     ur.Role,
		TenantID: tenantIDStr,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
	if c := ClaimsFromCtx(r.Context()); c != nil {
		h.logAudit("user", c.UserID, "auth.logout", "user", c.UserID)
		var uid pgtype.UUID
		if err := uid.Scan(c.UserID); err == nil {
			_ = h.q.BumpUserTokenVersion(r.Context(), uid)
		}
	}
	http.SetCookie(w, &http.Cookie{Name: accessCookie, Value: "", MaxAge: -1, Path: "/"})
	http.SetCookie(w, &http.Cookie{Name: refreshCookie, Value: "", MaxAge: -1, Path: "/auth/refresh"})
	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) Me(w http.ResponseWriter, r *http.Request) {
	c := ClaimsFromCtx(r.Context())
	if c == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meResponse{
		ID:       c.UserID,
		Username: c.Username,
		Email:    c.Email,
		Role:     c.Role,
		TenantID: c.TenantID,
	})
}

func (h *Handler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	c := ClaimsFromCtx(r.Context())
	if c == nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	var req struct {
		CurrentPassword string `json:"current_password"`
		NewPassword     string `json:"new_password"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if len(req.NewPassword) < 8 {
		http.Error(w, "new password must be at least 8 characters", http.StatusBadRequest)
		return
	}

	var uid pgtype.UUID
	if err := uid.Scan(c.UserID); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	user, err := h.q.GetUser(r.Context(), storegen.GetUserParams{ID: uid, TenantID: h.tenantID})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if user.PasswordHash == nil {
		http.Error(w, "password change not supported for OIDC accounts", http.StatusBadRequest)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.CurrentPassword)); err != nil {
		http.Error(w, "current password incorrect", http.StatusUnauthorized)
		return
	}

	hash, err := bcrypt.GenerateFromPassword([]byte(req.NewPassword), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	hashStr := string(hash)
	if _, err := h.q.UpdateUserPassword(r.Context(), storegen.UpdateUserPasswordParams{
		ID:           uid,
		TenantID:     h.tenantID,
		PasswordHash: &hashStr,
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (h *Handler) logAudit(actorType, actorID, action, objectType, objectID string) {
	h.logAuditAfter(actorType, actorID, action, objectType, objectID, nil)
}

func (h *Handler) logAuditAfter(actorType, actorID, action, objectType, objectID string, after []byte) {
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		var aID, oID pgtype.UUID
		_ = aID.Scan(actorID)
		_ = oID.Scan(objectID)
		_, _ = h.q.InsertAuditLog(ctx, storegen.InsertAuditLogParams{
			TenantID:   h.tenantID,
			ActorType:  actorType,
			ActorID:    aID,
			Action:     action,
			ObjectType: objectType,
			ObjectID:   oID,
			After:      after,
		})
	}()
}

func uuidStr(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
