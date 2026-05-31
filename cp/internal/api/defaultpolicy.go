package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// DefaultPolicyResponse is the JSON shape for default policy.
type DefaultPolicyResponse struct {
	ID                   string    `json:"id"`
	TenantID             string    `json:"tenant_id"`
	DefaultIngressAction string    `json:"default_ingress_action"`
	DefaultEgressAction  string    `json:"default_egress_action"`
	UpdatedAt            time.Time `json:"updated_at"`
}

type putDefaultPolicyRequest struct {
	DefaultIngressAction string `json:"default_ingress_action"`
	DefaultEgressAction  string `json:"default_egress_action"`
}

type dpHandler struct {
	q *storegen.Queries
}

func registerDefaultPolicy(r chi.Router, q *storegen.Queries) {
	h := &dpHandler{q: q}
	r.Get("/", h.get)
	r.Put("/", h.put)
}

func (h *dpHandler) get(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	dp, err := h.q.GetDefaultPolicy(r.Context(), tenantUUID)
	if err != nil {
		apiError(w, http.StatusNotFound, "default policy not found")
		return
	}
	writeJSON(w, http.StatusOK, toDPResponse(dp))
}

func (h *dpHandler) put(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	var req putDefaultPolicyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.DefaultIngressAction == "" {
		req.DefaultIngressAction = "deny"
	}
	if req.DefaultEgressAction == "" {
		req.DefaultEgressAction = "deny"
	}
	dp, err := h.q.UpsertDefaultPolicy(r.Context(), storegen.UpsertDefaultPolicyParams{
		TenantID:             tenantUUID,
		DefaultIngressAction: req.DefaultIngressAction,
		DefaultEgressAction:  req.DefaultEgressAction,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "upsert default policy: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toDPResponse(dp))
}

func toDPResponse(dp storegen.DefaultPolicy) DefaultPolicyResponse {
	resp := DefaultPolicyResponse{
		ID:                   uuidToStr(dp.ID),
		TenantID:             uuidToStr(dp.TenantID),
		DefaultIngressAction: dp.DefaultIngressAction,
		DefaultEgressAction:  dp.DefaultEgressAction,
	}
	if dp.UpdatedAt.Valid {
		resp.UpdatedAt = dp.UpdatedAt.Time
	}
	return resp
}
