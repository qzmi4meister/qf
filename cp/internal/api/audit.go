package api

import (
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

type auditHandler struct {
	q *storegen.Queries
}

func registerAuditLog(r chi.Router, q *storegen.Queries) {
	h := &auditHandler{q: q}
	r.Get("/", h.list)
}

// GET /audit-log?actor_type=&actor_id=&object_type=&object_id=&start=&end=&limit=
func (h *auditHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	p := storegen.ListAuditLogParams{
		TenantID: tenantUUID,
		Column2:  r.URL.Query().Get("actor_type"),
		Column3:  parseUUIDParam(r, "actor_id"),
		Column4:  r.URL.Query().Get("object_type"),
		Column5:  parseUUIDParam(r, "object_id"),
		Column6:  parseTimestampParam(r, "start"),
		Column7:  parseTimestampParam(r, "end"),
		Limit:    int32(parseLimit(r)),
	}
	rows, err := h.q.ListAuditLog(r.Context(), p)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list audit log: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toAuditLogResponses(rows))
}

// ── response type ─────────────────────────────────────────────────────────

type AuditLogResponse struct {
	ID            string        `json:"id"`
	TenantID      string        `json:"tenant_id"`
	ActorType     string        `json:"actor_type"`
	ActorID       string        `json:"actor_id,omitempty"`
	ActorUsername string        `json:"actor_username,omitempty"`
	Action        string        `json:"action"`
	ObjectType    string        `json:"object_type"`
	ObjectID      string        `json:"object_id,omitempty"`
	Before        jsonRawOrNull `json:"before"`
	After         jsonRawOrNull `json:"after"`
	CreatedAt     time.Time     `json:"created_at"`
}

// jsonRawOrNull serialises a []byte as a raw JSON value (or null).
type jsonRawOrNull []byte

func (j jsonRawOrNull) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func toAuditLogResponses(rows []storegen.ListAuditLogRow) []AuditLogResponse {
	out := make([]AuditLogResponse, 0, len(rows))
	for _, row := range rows {
		a := AuditLogResponse{
			ID:         uuidToStr(row.ID),
			TenantID:   uuidToStr(row.TenantID),
			ActorType:  row.ActorType,
			ActorID:    uuidToStr(row.ActorID),
			Action:     row.Action,
			ObjectType: row.ObjectType,
			ObjectID:   uuidToStr(row.ObjectID),
			Before:     jsonRawOrNull(row.Before),
			After:      jsonRawOrNull(row.After),
		}
		if row.ActorUsername != nil {
			a.ActorUsername = *row.ActorUsername
		}
		if row.CreatedAt.Valid {
			a.CreatedAt = row.CreatedAt.Time
		}
		out = append(out, a)
	}
	return out
}
