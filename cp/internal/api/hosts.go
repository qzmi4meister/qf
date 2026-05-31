package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// HostResponse is the JSON shape returned for a host.
type HostResponse struct {
	ID                string            `json:"id"`
	TenantID          string            `json:"tenant_id"`
	Hostname          string            `json:"hostname"`
	Labels            map[string]string `json:"labels"`
	Status            string            `json:"status"`
	CurrentGeneration int32             `json:"current_generation"`
	LastHeartbeatAt   *time.Time        `json:"last_heartbeat_at,omitempty"`
	AgentVersion      *string           `json:"agent_version,omitempty"`
	KernelVersion     *string           `json:"kernel_version,omitempty"`
	CreatedAt         time.Time         `json:"created_at"`
	UpdatedAt         time.Time         `json:"updated_at"`
}

type createHostRequest struct {
	Hostname string            `json:"hostname"`
	Labels   map[string]string `json:"labels"`
	Status   string            `json:"status"`
}

type patchHostRequest struct {
	Labels *map[string]string `json:"labels,omitempty"`
	Status *string            `json:"status,omitempty"`
}

type hostHandler struct {
	q *storegen.Queries
}

func registerHosts(r chi.Router, q *storegen.Queries) {
	h := &hostHandler{q: q}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.patch)
}

func (h *hostHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hosts, err := h.q.ListHosts(r.Context(), tenantUUID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list hosts: "+err.Error())
		return
	}
	resp := make([]HostResponse, 0, len(hosts))
	for _, h := range hosts {
		resp = append(resp, toHostResponse(h))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *hostHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	var req createHostRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Hostname == "" {
		apiError(w, http.StatusBadRequest, "hostname required")
		return
	}
	if req.Status == "" {
		req.Status = "pending"
	}
	labelsJSON, err := json.Marshal(labelsOrEmpty(req.Labels))
	if err != nil {
		apiError(w, http.StatusBadRequest, "invalid labels")
		return
	}
	host, err := h.q.CreateHost(r.Context(), storegen.CreateHostParams{
		TenantID: tenantUUID,
		Hostname: req.Hostname,
		Labels:   labelsJSON,
		Status:   req.Status,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "create host: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toHostResponse(host))
}

func (h *hostHandler) get(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	host, err := h.q.GetHost(r.Context(), storegen.GetHostParams{
		ID:       hostUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		apiError(w, http.StatusNotFound, "host not found")
		return
	}
	writeJSON(w, http.StatusOK, toHostResponse(host))
}

func (h *hostHandler) patch(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var req patchHostRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	var host storegen.Host
	var err error

	if req.Labels != nil {
		labelsJSON, jerr := json.Marshal(labelsOrEmpty(*req.Labels))
		if jerr != nil {
			apiError(w, http.StatusBadRequest, "invalid labels")
			return
		}
		host, err = h.q.UpdateHostLabels(r.Context(), storegen.UpdateHostLabelsParams{
			ID:       hostUUID,
			TenantID: tenantUUID,
			Labels:   labelsJSON,
		})
		if err != nil {
			apiError(w, http.StatusInternalServerError, "update labels: "+err.Error())
			return
		}
	}

	if req.Status != nil {
		host, err = h.q.UpdateHostStatus(r.Context(), storegen.UpdateHostStatusParams{
			ID:       hostUUID,
			TenantID: tenantUUID,
			Status:   *req.Status,
		})
		if err != nil {
			apiError(w, http.StatusInternalServerError, "update status: "+err.Error())
			return
		}
	}

	if req.Labels == nil && req.Status == nil {
		// Nothing to update — return current state.
		host, err = h.q.GetHost(r.Context(), storegen.GetHostParams{
			ID:       hostUUID,
			TenantID: tenantUUID,
		})
		if err != nil {
			apiError(w, http.StatusNotFound, "host not found")
			return
		}
	}

	writeJSON(w, http.StatusOK, toHostResponse(host))
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toHostResponse(h storegen.Host) HostResponse {
	resp := HostResponse{
		ID:                uuidToStr(h.ID),
		TenantID:          uuidToStr(h.TenantID),
		Hostname:          h.Hostname,
		Status:            h.Status,
		CurrentGeneration: h.CurrentGeneration,
		AgentVersion:      h.AgentVersion,
		KernelVersion:     h.KernelVersion,
	}
	if h.CreatedAt.Valid {
		resp.CreatedAt = h.CreatedAt.Time
	}
	if h.UpdatedAt.Valid {
		resp.UpdatedAt = h.UpdatedAt.Time
	}
	if h.LastHeartbeatAt.Valid {
		t := h.LastHeartbeatAt.Time
		resp.LastHeartbeatAt = &t
	}
	if len(h.Labels) > 0 {
		_ = json.Unmarshal(h.Labels, &resp.Labels)
	}
	if resp.Labels == nil {
		resp.Labels = map[string]string{}
	}
	return resp
}

func tenantFromRequest(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	raw := r.Header.Get("X-Tenant-ID")
	if raw == "" {
		apiError(w, http.StatusBadRequest, "X-Tenant-ID header required")
		return pgtype.UUID{}, false
	}
	var u pgtype.UUID
	if err := u.Scan(raw); err != nil {
		apiError(w, http.StatusBadRequest, "invalid X-Tenant-ID")
		return pgtype.UUID{}, false
	}
	return u, true
}

func uuidParam(w http.ResponseWriter, r *http.Request, param string) (pgtype.UUID, bool) {
	raw := chi.URLParam(r, param)
	var u pgtype.UUID
	if err := u.Scan(raw); err != nil {
		apiError(w, http.StatusBadRequest, fmt.Sprintf("invalid %s", param))
		return pgtype.UUID{}, false
	}
	return u, true
}

func uuidToStr(u pgtype.UUID) string {
	if !u.Valid {
		return ""
	}
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func labelsOrEmpty(m map[string]string) map[string]string {
	if m == nil {
		return map[string]string{}
	}
	return m
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	json.NewEncoder(w).Encode(v)
}

func apiError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		apiError(w, http.StatusBadRequest, "invalid JSON: "+err.Error())
		return false
	}
	return true
}
