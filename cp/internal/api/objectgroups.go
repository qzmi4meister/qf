package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/qf/qf/cp/internal/policy"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// ObjectGroupResponse is the JSON shape for an object group.
type ObjectGroupResponse struct {
	ID         string          `json:"id"`
	TenantID   string          `json:"tenant_id"`
	Type       string          `json:"type"`
	Name       string          `json:"name"`
	Spec       json.RawMessage `json:"spec"`
	ResolvedAt *time.Time      `json:"resolved_at,omitempty"`
	CreatedAt  time.Time       `json:"created_at"`
	UpdatedAt  time.Time       `json:"updated_at"`
}

type createObjectGroupRequest struct {
	Type string          `json:"type"` // ipset | portset | hostset
	Name string          `json:"name"`
	Spec json.RawMessage `json:"spec"`
}

type updateObjectGroupRequest struct {
	Spec json.RawMessage `json:"spec"`
}

type ogHandler struct {
	q       *storegen.Queries
	cascade *policy.CascadeRecompiler
}

func registerObjectGroups(r chi.Router, q *storegen.Queries, cascade *policy.CascadeRecompiler) {
	h := &ogHandler{q: q, cascade: cascade}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
}

func (h *ogHandler) dispatchCascade(tenantID, ogID string) {
	if h.cascade == nil {
		return
	}
	go func() {
		if err := h.cascade.OnObjectGroupChanged(context.Background(), tenantID, ogID); err != nil {
			slog.Warn("cascade: objectgroup push failed", "og", ogID, "err", err)
		}
	}()
}

func (h *ogHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	var ogs []storegen.ObjectGroup
	var err error

	if t := r.URL.Query().Get("type"); t != "" {
		ogs, err = h.q.ListObjectGroupsByType(r.Context(), storegen.ListObjectGroupsByTypeParams{
			TenantID: tenantUUID,
			Type:     t,
		})
	} else {
		ogs, err = h.q.ListObjectGroups(r.Context(), tenantUUID)
	}
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list objectgroups: "+err.Error())
		return
	}
	resp := make([]ObjectGroupResponse, 0, len(ogs))
	for _, og := range ogs {
		resp = append(resp, toOGResponse(og))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *ogHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	var req createObjectGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}
	switch req.Type {
	case "ipset", "portset", "hostset":
	default:
		apiError(w, http.StatusBadRequest, "type must be ipset, portset, or hostset")
		return
	}
	og, err := h.q.CreateObjectGroup(r.Context(), storegen.CreateObjectGroupParams{
		TenantID: tenantUUID,
		Type:     req.Type,
		Name:     req.Name,
		Spec:     rawJSONOrEmpty(req.Spec),
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "create objectgroup: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toOGResponse(og))
}

func (h *ogHandler) get(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	ogUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	og, err := h.q.GetObjectGroup(r.Context(), storegen.GetObjectGroupParams{
		ID:       ogUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		apiError(w, http.StatusNotFound, "objectgroup not found")
		return
	}
	writeJSON(w, http.StatusOK, toOGResponse(og))
}

func (h *ogHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	ogUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var req updateObjectGroupRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	og, err := h.q.UpdateObjectGroup(r.Context(), storegen.UpdateObjectGroupParams{
		ID:       ogUUID,
		TenantID: tenantUUID,
		Spec:     rawJSONOrEmpty(req.Spec),
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "update objectgroup: "+err.Error())
		return
	}
	h.dispatchCascade(uuidToStr(tenantUUID), uuidToStr(ogUUID))
	writeJSON(w, http.StatusOK, toOGResponse(og))
}

func (h *ogHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	ogUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.q.DeleteObjectGroup(r.Context(), storegen.DeleteObjectGroupParams{
		ID:       ogUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusInternalServerError, "delete objectgroup: "+err.Error())
		return
	}
	h.dispatchCascade(uuidToStr(tenantUUID), uuidToStr(ogUUID))
	w.WriteHeader(http.StatusNoContent)
}

func toOGResponse(og storegen.ObjectGroup) ObjectGroupResponse {
	resp := ObjectGroupResponse{
		ID:       uuidToStr(og.ID),
		TenantID: uuidToStr(og.TenantID),
		Type:     og.Type,
		Name:     og.Name,
		Spec:     jsonOrNull(og.Spec),
	}
	if og.CreatedAt.Valid {
		resp.CreatedAt = og.CreatedAt.Time
	}
	if og.UpdatedAt.Valid {
		resp.UpdatedAt = og.UpdatedAt.Time
	}
	if og.ResolvedAt.Valid {
		t := og.ResolvedAt.Time
		resp.ResolvedAt = &t
	}
	return resp
}
