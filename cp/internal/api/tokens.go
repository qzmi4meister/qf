package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qf/qf/cp/internal/pki"
)

// TokenResponse is the JSON shape for a bootstrap token.
// PlainToken is only populated on creation.
type TokenResponse struct {
	ID            string            `json:"id"`
	TenantID      string            `json:"tenant_id"`
	Type          string            `json:"type"`
	TargetHostID  *string           `json:"target_host_id,omitempty"`
	LabelTemplate map[string]string `json:"label_template,omitempty"`
	MaxUses       int               `json:"max_uses"`
	UsesCount     int               `json:"uses_count"`
	ExpiresAt     time.Time         `json:"expires_at"`
	PlainToken    string            `json:"token,omitempty"` // set only on creation
}

type createTokenRequest struct {
	Type          string            `json:"type"` // single_host | bulk
	TargetHostID  string            `json:"target_host_id,omitempty"`
	LabelTemplate map[string]string `json:"label_template,omitempty"`
	TTLSeconds    int               `json:"ttl_seconds"`
	MaxUses       int               `json:"max_uses"`
}

type tokHandler struct {
	store *pki.TokenStore
}

func registerTokens(r chi.Router, store *pki.TokenStore) {
	h := &tokHandler{store: store}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Delete("/{id}", h.delete)
}

func (h *tokHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		apiError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	tokens, err := h.store.ListTokens(r.Context(), tenantID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list tokens: "+err.Error())
		return
	}
	resp := make([]TokenResponse, 0, len(tokens))
	for _, t := range tokens {
		resp = append(resp, toTokenResponse(t, ""))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *tokHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantID := r.Header.Get("X-Tenant-ID")
	if tenantID == "" {
		apiError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return
	}
	var req createTokenRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.TTLSeconds <= 0 {
		req.TTLSeconds = 3600
	}
	if req.MaxUses <= 0 {
		req.MaxUses = 1
	}
	ttl := time.Duration(req.TTLSeconds) * time.Second

	var plain string
	var err error
	switch req.Type {
	case "single_host":
		if req.TargetHostID == "" {
			apiError(w, http.StatusBadRequest, "target_host_id required for single_host token")
			return
		}
		plain, err = h.store.CreateSingleHostToken(r.Context(), tenantID, req.TargetHostID, ttl, req.MaxUses)
	case "bulk":
		plain, err = h.store.CreateBulkToken(r.Context(), tenantID, req.LabelTemplate, ttl, req.MaxUses)
	default:
		apiError(w, http.StatusBadRequest, "type must be single_host or bulk")
		return
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "create token: "+err.Error())
		return
	}

	// Fetch the created token record to return full response.
	tokens, err := h.store.ListTokens(r.Context(), tenantID)
	if err != nil {
		// Return minimal response on list failure.
		writeJSON(w, http.StatusCreated, map[string]string{"token": plain})
		return
	}
	// Return last created token (most recent).
	if len(tokens) == 0 {
		writeJSON(w, http.StatusCreated, map[string]string{"token": plain})
		return
	}
	resp := toTokenResponse(tokens[len(tokens)-1], plain)
	writeJSON(w, http.StatusCreated, resp)
}

func (h *tokHandler) delete(w http.ResponseWriter, r *http.Request) {
	id := chi.URLParam(r, "id")
	if id == "" {
		apiError(w, http.StatusBadRequest, "id required")
		return
	}
	if err := h.store.DeleteToken(r.Context(), id); err != nil {
		apiError(w, http.StatusInternalServerError, "delete token: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func toTokenResponse(t *pki.BootstrapToken, plain string) TokenResponse {
	return TokenResponse{
		ID:            t.ID,
		TenantID:      t.TenantID,
		Type:          t.Type,
		TargetHostID:  t.TargetHostID,
		LabelTemplate: t.LabelTemplate,
		MaxUses:       t.MaxUses,
		UsesCount:     t.UsesCount,
		ExpiresAt:     t.ExpiresAt,
		PlainToken:    plain,
	}
}
