package api

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/pubsub"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

const defaultLimit = 500
const maxLimit = 1000

type eventHandler struct {
	q   *storegen.Queries
	hub *pubsub.Hub // nil = SSE not available
}

func registerEvents(r chi.Router, q *storegen.Queries, hub *pubsub.Hub) {
	h := &eventHandler{q: q, hub: hub}
	r.Get("/{id}/events", h.listEvents)
	r.Get("/{id}/events/stream", h.streamEvents)
	r.Get("/{id}/flows", h.listFlows)
	r.Get("/{id}/counters", h.listCounters)
	r.Get("/{id}/counters/latest", h.latestCounters)
}

// GET /hosts/{id}/events/stream — SSE live tail
func (h *eventHandler) streamEvents(w http.ResponseWriter, r *http.Request) {
	if h.hub == nil {
		http.Error(w, "SSE not available", http.StatusServiceUnavailable)
		return
	}
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}

	// If Last-Event-ID is set, flush history from DB first.
	lastEventID := r.Header.Get("Last-Event-ID")
	if lastEventID != "" {
		since, err := time.Parse(time.RFC3339Nano, lastEventID)
		if err == nil {
			p := storegen.ListLogEventsParams{
				TenantID: tenantUUID,
				HostID:   hostUUID,
				Column3:  pgtype.Timestamptz{Time: since, Valid: true},
				Column4:  pgtype.Timestamptz{},
				Limit:    500,
			}
			rows, err := h.q.ListLogEvents(r.Context(), p)
			if err == nil {
				w.Header().Set("Content-Type", "text/event-stream")
				w.Header().Set("Cache-Control", "no-cache")
				w.Header().Set("X-Accel-Buffering", "no")
				flusher, _ := w.(http.Flusher)
				for _, row := range rows {
					b, _ := json.Marshal(toSingleLogEvent(row))
					id := ""
					if row.CreatedAt.Valid {
						id = row.CreatedAt.Time.UTC().Format(time.RFC3339Nano)
					}
					fmt.Fprintf(w, "id: %s\nevent: log_event\ndata: %s\n\n", id, b)
					if flusher != nil {
						flusher.Flush()
					}
				}
			}
		}
	}

	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("X-Accel-Buffering", "no")
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	hostIDStr := uuidToStr(hostUUID)
	ch, cancel := h.hub.Subscribe(hostIDStr, 64)
	defer cancel()

	for {
		select {
		case <-r.Context().Done():
			return
		case msg, open := <-ch:
			if !open {
				return
			}
			fmt.Fprintf(w, "id: %s\nevent: log_event\ndata: %s\n\n", msg.ID, msg.Data)
			flusher.Flush()
		}
	}
}

// GET /hosts/{id}/events
func (h *eventHandler) listEvents(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	p := storegen.ListLogEventsParams{
		TenantID: tenantUUID,
		HostID:   hostUUID,
		Column3:  parseTimestampParam(r, "start"),
		Column4:  parseTimestampParam(r, "end"),
		Column5:  parseUUIDParam(r, "rule_id"),
		Column6:  r.URL.Query().Get("action"),
		Limit:    int32(parseLimit(r)),
	}
	rows, err := h.q.ListLogEvents(r.Context(), p)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list events: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toLogEventResponses(rows))
}

// GET /hosts/{id}/flows
func (h *eventHandler) listFlows(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	p := storegen.ListFlowEventsParams{
		TenantID: tenantUUID,
		HostID:   hostUUID,
		Column3:  parseTimestampParam(r, "start"),
		Column4:  parseTimestampParam(r, "end"),
		Limit:    int32(parseLimit(r)),
	}
	rows, err := h.q.ListFlowEvents(r.Context(), p)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list flows: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toFlowEventResponses(rows))
}

// GET /hosts/{id}/counters
func (h *eventHandler) listCounters(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	p := storegen.ListCounterSnapshotsParams{
		TenantID: tenantUUID,
		HostID:   hostUUID,
		Column3:  parseUUIDParam(r, "rule_id"),
		Column4:  parseTimestampParam(r, "start"),
		Column5:  parseTimestampParam(r, "end"),
		Limit:    int32(parseLimit(r)),
	}
	rows, err := h.q.ListCounterSnapshots(r.Context(), p)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list counters: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toCounterResponses(rows))
}

// GET /hosts/{id}/counters/latest
func (h *eventHandler) latestCounters(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	rows, err := h.q.GetLatestCounterSnapshotsForHost(r.Context(),
		storegen.GetLatestCounterSnapshotsForHostParams{
			TenantID: tenantUUID,
			HostID:   hostUUID,
		})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "latest counters: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toLatestCounterResponses(rows))
}

// ── response types ────────────────────────────────────────────────────────

type LogEventResponse struct {
	ID         string     `json:"id"`
	HostID     string     `json:"host_id"`
	RuleID     string     `json:"rule_id,omitempty"`
	PolicyID   string     `json:"policy_id,omitempty"`
	Direction  string     `json:"direction"`
	Action     string     `json:"action"`
	Protocol   int16      `json:"protocol"`
	SrcIP      string     `json:"src_ip,omitempty"`
	SrcPort    *int32     `json:"src_port,omitempty"`
	DstIP      string     `json:"dst_ip,omitempty"`
	DstPort    *int32     `json:"dst_port,omitempty"`
	PacketSize *int32     `json:"packet_size,omitempty"`
	TCPFlags   *int32     `json:"tcp_flags,omitempty"`
	CTState    *string    `json:"ct_state,omitempty"`
	CreatedAt  time.Time  `json:"created_at"`
}

type FlowEventResponse struct {
	ID           string     `json:"id"`
	HostID       string     `json:"host_id"`
	Protocol     int16      `json:"protocol"`
	SrcIP        string     `json:"src_ip,omitempty"`
	SrcPort      *int32     `json:"src_port,omitempty"`
	DstIP        string     `json:"dst_ip,omitempty"`
	DstPort      *int32     `json:"dst_port,omitempty"`
	BytesOrig    int64      `json:"bytes_orig"`
	BytesReply   int64      `json:"bytes_reply"`
	PacketsOrig  int64      `json:"packets_orig"`
	PacketsReply int64      `json:"packets_reply"`
	FinalState   *string    `json:"final_state,omitempty"`
	StartedAt    *time.Time `json:"started_at,omitempty"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	CreatedAt    time.Time  `json:"created_at"`
}

type CounterResponse struct {
	ID       string    `json:"id"`
	HostID   string    `json:"host_id"`
	RuleID   string    `json:"rule_id"`
	PolicyID string    `json:"policy_id,omitempty"`
	Packets  int64     `json:"packets"`
	Bytes    int64     `json:"bytes"`
	Ts       time.Time `json:"ts"`
}

// ── converters ────────────────────────────────────────────────────────────

func toSingleLogEvent(r storegen.ListLogEventsRow) LogEventResponse {
	ev := LogEventResponse{
		ID:         uuidToStr(r.ID),
		HostID:     uuidToStr(r.HostID),
		RuleID:     uuidToStr(r.RuleID),
		PolicyID:   uuidToStr(r.PolicyID),
		Direction:  r.Direction,
		Action:     r.Action,
		Protocol:   r.Protocol,
		SrcPort:    r.SrcPort,
		DstPort:    r.DstPort,
		PacketSize: r.PacketSize,
		TCPFlags:   r.TcpFlags,
		CTState:    r.CtState,
	}
	if r.SrcIp != nil {
		ev.SrcIP = addrToStr(r.SrcIp)
	}
	if r.DstIp != nil {
		ev.DstIP = addrToStr(r.DstIp)
	}
	if r.CreatedAt.Valid {
		ev.CreatedAt = r.CreatedAt.Time
	}
	return ev
}

func toLogEventResponses(rows []storegen.ListLogEventsRow) []LogEventResponse {
	out := make([]LogEventResponse, 0, len(rows))
	for _, r := range rows {
		ev := LogEventResponse{
			ID:         uuidToStr(r.ID),
			HostID:     uuidToStr(r.HostID),
			RuleID:     uuidToStr(r.RuleID),
			PolicyID:   uuidToStr(r.PolicyID),
			Direction:  r.Direction,
			Action:     r.Action,
			Protocol:   r.Protocol,
			SrcPort:    r.SrcPort,
			DstPort:    r.DstPort,
			PacketSize: r.PacketSize,
			TCPFlags:   r.TcpFlags,
			CTState:    r.CtState,
		}
		if r.SrcIp != nil {
			ev.SrcIP = addrToStr(r.SrcIp)
		}
		if r.DstIp != nil {
			ev.DstIP = addrToStr(r.DstIp)
		}
		if r.CreatedAt.Valid {
			ev.CreatedAt = r.CreatedAt.Time
		}
		out = append(out, ev)
	}
	return out
}

func toFlowEventResponses(rows []storegen.ListFlowEventsRow) []FlowEventResponse {
	out := make([]FlowEventResponse, 0, len(rows))
	for _, r := range rows {
		fv := FlowEventResponse{
			ID:           uuidToStr(r.ID),
			HostID:       uuidToStr(r.HostID),
			Protocol:     r.Protocol,
			SrcPort:      r.SrcPort,
			DstPort:      r.DstPort,
			BytesOrig:    r.BytesOrig,
			BytesReply:   r.BytesReply,
			PacketsOrig:  r.PacketsOrig,
			PacketsReply: r.PacketsReply,
			FinalState:   r.FinalState,
		}
		if r.SrcIp != nil {
			fv.SrcIP = addrToStr(r.SrcIp)
		}
		if r.DstIp != nil {
			fv.DstIP = addrToStr(r.DstIp)
		}
		if r.StartedAt.Valid {
			t := r.StartedAt.Time
			fv.StartedAt = &t
		}
		if r.EndedAt.Valid {
			t := r.EndedAt.Time
			fv.EndedAt = &t
		}
		if r.CreatedAt.Valid {
			fv.CreatedAt = r.CreatedAt.Time
		}
		out = append(out, fv)
	}
	return out
}

func toCounterResponses(rows []storegen.ListCounterSnapshotsRow) []CounterResponse {
	out := make([]CounterResponse, 0, len(rows))
	for _, r := range rows {
		cr := CounterResponse{
			ID:       uuidToStr(r.ID),
			HostID:   uuidToStr(r.HostID),
			RuleID:   uuidToStr(r.RuleID),
			PolicyID: uuidToStr(r.PolicyID),
			Packets:  r.Packets,
			Bytes:    r.Bytes,
		}
		if r.Ts.Valid {
			cr.Ts = r.Ts.Time
		}
		out = append(out, cr)
	}
	return out
}

func toLatestCounterResponses(rows []storegen.GetLatestCounterSnapshotsForHostRow) []CounterResponse {
	out := make([]CounterResponse, 0, len(rows))
	for _, r := range rows {
		cr := CounterResponse{
			ID:       uuidToStr(r.ID),
			HostID:   uuidToStr(r.HostID),
			RuleID:   uuidToStr(r.RuleID),
			PolicyID: uuidToStr(r.PolicyID),
			Packets:  r.Packets,
			Bytes:    r.Bytes,
		}
		if r.Ts.Valid {
			cr.Ts = r.Ts.Time
		}
		out = append(out, cr)
	}
	return out
}

// ── param helpers ─────────────────────────────────────────────────────────

func parseTimestampParam(r *http.Request, key string) pgtype.Timestamptz {
	s := r.URL.Query().Get(key)
	if s == "" {
		return pgtype.Timestamptz{}
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return pgtype.Timestamptz{}
	}
	return pgtype.Timestamptz{Time: t, Valid: true}
}

func parseUUIDParam(r *http.Request, key string) pgtype.UUID {
	s := r.URL.Query().Get(key)
	if s == "" {
		return pgtype.UUID{}
	}
	var u pgtype.UUID
	_ = u.Scan(s)
	return u
}

func parseLimit(r *http.Request) int {
	s := r.URL.Query().Get("limit")
	if s == "" {
		return defaultLimit
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return defaultLimit
	}
	if n > maxLimit {
		return maxLimit
	}
	return n
}

func addrToStr(a *netip.Addr) string {
	if a == nil {
		return ""
	}
	return a.String()
}
