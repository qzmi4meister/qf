package auth

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

type apiTokenResponse struct {
	ID          string     `json:"id"`
	Name        string     `json:"name"`
	Role        string     `json:"role"`
	CreatedBy   string     `json:"created_by"`
	ExpiresAt   *time.Time `json:"expires_at,omitempty"`
	LastUsedAt  *time.Time `json:"last_used_at,omitempty"`
	CreatedAt   time.Time  `json:"created_at"`
	PlainToken  string     `json:"token,omitempty"` // only on creation
}

func tokenToResponse(t storegen.ApiToken, plain string) apiTokenResponse {
	r := apiTokenResponse{
		ID:        uuidStr(t.ID),
		Name:      t.Name,
		Role:      t.Role,
		CreatedBy: uuidStr(t.CreatedBy),
		CreatedAt: t.CreatedAt.Time,
		PlainToken: plain,
	}
	if t.ExpiresAt.Valid {
		v := t.ExpiresAt.Time
		r.ExpiresAt = &v
	}
	if t.LastUsedAt.Valid {
		v := t.LastUsedAt.Time
		r.LastUsedAt = &v
	}
	return r
}

// RegisterAPITokens registers API token management routes.
func RegisterAPITokens(r chi.Router, q *storegen.Queries, tenantID pgtype.UUID) {
	h := &apiTokensHandler{q: q, tenantID: tenantID}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Delete("/{id}", h.delete)
}

type apiTokensHandler struct {
	q        *storegen.Queries
	tenantID pgtype.UUID
}

func (h *apiTokensHandler) list(w http.ResponseWriter, r *http.Request) {
	tokens, err := h.q.ListAPITokens(r.Context(), h.tenantID)
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	out := make([]apiTokenResponse, 0, len(tokens))
	for _, t := range tokens {
		out = append(out, tokenToResponse(t, ""))
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(out)
}

type createAPITokenRequest struct {
	Name      string     `json:"name"`
	Role      string     `json:"role"`
	ExpiresAt *time.Time `json:"expires_at,omitempty"`
}

func (h *apiTokensHandler) create(w http.ResponseWriter, r *http.Request) {
	var req createAPITokenRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Name == "" {
		http.Error(w, "bad request", http.StatusBadRequest)
		return
	}
	if !validRole(req.Role) {
		http.Error(w, "invalid role", http.StatusBadRequest)
		return
	}

	rawBytes := make([]byte, 32)
	if _, err := rand.Read(rawBytes); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	plain := hex.EncodeToString(rawBytes)
	hash := sha256Hex(plain)

	claims := ClaimsFromCtx(r.Context())
	var createdBy pgtype.UUID
	if claims != nil {
		createdBy.Scan(claims.UserID) //nolint:errcheck
	}

	var expiresAt pgtype.Timestamptz
	if req.ExpiresAt != nil {
		expiresAt = pgtype.Timestamptz{Time: *req.ExpiresAt, Valid: true}
	}

	tok, err := h.q.CreateAPIToken(r.Context(), storegen.CreateAPITokenParams{
		TenantID:  h.tenantID,
		Name:      req.Name,
		TokenHash: hash,
		Role:      req.Role,
		CreatedBy: createdBy,
		ExpiresAt: expiresAt,
	})
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusCreated)
	json.NewEncoder(w).Encode(tokenToResponse(tok, plain))
}

func (h *apiTokensHandler) delete(w http.ResponseWriter, r *http.Request) {
	uid, err := parseUUID(chi.URLParam(r, "id"))
	if err != nil {
		http.Error(w, "bad id", http.StatusBadRequest)
		return
	}
	if err := h.q.DeleteAPIToken(r.Context(), storegen.DeleteAPITokenParams{ID: uid, TenantID: h.tenantID}); err != nil {
		http.Error(w, "not found", http.StatusNotFound)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
