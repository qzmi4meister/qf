package auth

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"golang.org/x/crypto/bcrypt"
)

type Handler struct {
	q        *storegen.Queries
	secret   []byte
	tenantID pgtype.UUID
}

func NewHandler(q *storegen.Queries, secret []byte, tenantID pgtype.UUID) *Handler {
	return &Handler{q: q, secret: secret, tenantID: tenantID}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}

type meResponse struct {
	ID       string `json:"id"`
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

	user, err := h.q.GetUserByEmail(r.Context(), storegen.GetUserByEmailParams{
		TenantID: h.tenantID,
		Email:    req.Email,
	})
	if err != nil || user.Status != "active" {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if user.PasswordHash == nil {
		http.Error(w, "invalid credentials", http.StatusUnauthorized)
		return
	}
	if err := bcrypt.CompareHashAndPassword([]byte(*user.PasswordHash), []byte(req.Password)); err != nil {
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

	access, err := IssueAccessToken(h.secret, userIDStr, tenantIDStr, user.Email, ur.Role)
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

	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(meResponse{
		ID:       userIDStr,
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
	userIDStr, err := ParseRefreshToken(h.secret, c.Value)
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
	ur, err := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{
		UserID:   user.ID,
		TenantID: h.tenantID,
	})
	if err != nil {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	tenantIDStr := uuidStr(h.tenantID)
	access, err := IssueAccessToken(h.secret, userIDStr, tenantIDStr, user.Email, ur.Role)
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
		Email:    user.Email,
		Role:     ur.Role,
		TenantID: tenantIDStr,
	})
}

func (h *Handler) Logout(w http.ResponseWriter, r *http.Request) {
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
		Email:    c.Email,
		Role:     c.Role,
		TenantID: c.TenantID,
	})
}

func uuidStr(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}
