package auth

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"golang.org/x/crypto/bcrypt"
)

type userResponse struct {
	ID          string     `json:"id"`
	Email       string     `json:"email"`
	Status      string     `json:"status"`
	Role        string     `json:"role,omitempty"`
	IsOIDC      bool       `json:"is_oidc"`
	LastLoginAt *time.Time `json:"last_login_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
}

func userToResponse(u storegen.User, role string) userResponse {
	r := userResponse{
		ID:        uuidStr(u.ID),
		Email:     u.Email,
		Status:    u.Status,
		Role:      role,
		IsOIDC:    u.OidcSubject != nil,
		CreatedAt: u.CreatedAt.Time,
	}
	if u.LastLoginAt.Valid {
		t := u.LastLoginAt.Time
		r.LastLoginAt = &t
	}
	return r
}

// RegisterUsers registers user management routes. Requires Admin role on mutating routes.
func RegisterUsers(r chi.Router, q *storegen.Queries, tenantID pgtype.UUID) {
	h := &usersHandler{q: q, tenantID: tenantID}
	r.Get("/", h.list)
	r.Post("/", RequireRole("admin")(http.HandlerFunc(h.create)).ServeHTTP)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", RequireRole("admin")(http.HandlerFunc(h.patch)).ServeHTTP)
	r.Delete("/{id}", RequireRole("admin")(http.HandlerFunc(h.delete)).ServeHTTP)
	r.Get("/{id}/roles", h.getRole)
	r.Put("/{id}/roles", RequireRole("admin")(http.HandlerFunc(h.putRole)).ServeHTTP)
}

type usersHandler struct {
	q        *storegen.Queries
	tenantID pgtype.UUID
}

func (h *usersHandler) list(w http.ResponseWriter, r *http.Request) {
	users, err := h.q.ListUsers(r.Context(), h.tenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]userResponse, 0, len(users))
	for _, u := range users {
		ur, _ := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{UserID: u.ID, TenantID: h.tenantID})
		out = append(out, userToResponse(u, ur.Role))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

func (h *usersHandler) get(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	u, err := h.q.GetUser(r.Context(), storegen.GetUserParams{ID: uid, TenantID: h.tenantID})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	ur, _ := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{UserID: u.ID, TenantID: h.tenantID})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userToResponse(u, ur.Role))
}

type createUserRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
	Role     string `json:"role"`
}

func (h *usersHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Email == "" || req.Password == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !validRole(req.Role) {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(req.Password), bcrypt.DefaultCost)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	hashStr := string(hash)
	u, err := h.q.CreateUser(r.Context(), storegen.CreateUserParams{
		TenantID:     h.tenantID,
		Email:        req.Email,
		PasswordHash: &hashStr,
		OidcSubject:  nil,
	})
	if err != nil {
		http.Error(w, "conflict or internal error", http.StatusConflict)
		return
	}
	if err := h.q.UpsertUserRole(r.Context(), storegen.UpsertUserRoleParams{
		UserID: u.ID, TenantID: h.tenantID, Role: req.Role,
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(userToResponse(u, req.Role))
}

type patchUserRequest struct {
	Password *string `json:"password,omitempty"`
	Status   *string `json:"status,omitempty"`
}

func (h *usersHandler) patch(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var req patchUserRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	var u storegen.User
	if req.Password != nil {
		hash, err := bcrypt.GenerateFromPassword([]byte(*req.Password), bcrypt.DefaultCost)
		if err != nil {
			http.Error(w, "internal error", http.StatusInternalServerError)
			return
		}
		hashStr := string(hash)
		u, err = h.q.UpdateUserPassword(r.Context(), storegen.UpdateUserPasswordParams{
			ID: uid, TenantID: h.tenantID, PasswordHash: &hashStr,
		})
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	if req.Status != nil {
		u, err = h.q.UpdateUserStatus(r.Context(), storegen.UpdateUserStatusParams{
			ID: uid, TenantID: h.tenantID, Status: *req.Status,
		})
		if err != nil {
			http.Error(w, "not found", http.StatusNotFound)
			return
		}
	}
	if u.ID == (pgtype.UUID{}) {
		u, _ = h.q.GetUser(r.Context(), storegen.GetUserParams{ID: uid, TenantID: h.tenantID})
	}
	ur, _ := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{UserID: u.ID, TenantID: h.tenantID})
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(userToResponse(u, ur.Role))
}

func (h *usersHandler) delete(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteUser(r.Context(), storegen.DeleteUserParams{ID: uid, TenantID: h.tenantID}); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

type roleResponse struct {
	UserID string `json:"user_id"`
	Role   string `json:"role"`
}

func (h *usersHandler) getRole(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	ur, err := h.q.GetUserRole(r.Context(), storegen.GetUserRoleParams{UserID: uid, TenantID: h.tenantID})
	if err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roleResponse{UserID: chi.URLParam(r, "id"), Role: ur.Role})
}

type putRoleRequest struct {
	Role string `json:"role"`
}

func (h *usersHandler) putRole(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	var req putRoleRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || !validRole(req.Role) {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if err := h.q.UpsertUserRole(r.Context(), storegen.UpsertUserRoleParams{
		UserID: uid, TenantID: h.tenantID, Role: req.Role,
	}); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(roleResponse{UserID: chi.URLParam(r, "id"), Role: req.Role})
}

func validRole(r string) bool {
	switch r {
	case "admin", "editor", "operator", "auditor":
		return true
	}
	return false
}

func parseUUID(s string) (pgtype.UUID, error) {
	var u pgtype.UUID
	err := u.Scan(s)
	return u, err
}
