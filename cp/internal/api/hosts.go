package api

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/auth"
	"github.com/qf/qf/cp/internal/policy"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// HostResponse is the JSON shape returned for a host.
type HostResponse struct {
	ID                 string            `json:"id"`
	TenantID           string            `json:"tenant_id"`
	Hostname           string            `json:"hostname"`
	Labels             map[string]string `json:"labels"`
	Status             string            `json:"status"`
	CurrentGeneration  int32             `json:"current_generation"`
	LastHeartbeatAt    *time.Time        `json:"last_heartbeat_at,omitempty"`
	AgentVersion       *string           `json:"agent_version,omitempty"`
	KernelVersion      *string           `json:"kernel_version,omitempty"`
	FlowEventsEnabled  bool              `json:"flow_events_enabled"`
	CreatedAt          time.Time         `json:"created_at"`
	UpdatedAt          time.Time         `json:"updated_at"`
}

type createHostRequest struct {
	Hostname string            `json:"hostname"`
	Labels   map[string]string `json:"labels"`
	Status   string            `json:"status"`
}

type patchHostRequest struct {
	Labels             *map[string]string `json:"labels,omitempty"`
	Status             *string            `json:"status,omitempty"`
	FlowEventsEnabled  *bool              `json:"flow_events_enabled,omitempty"`
}

type hostHandler struct {
	q            *storegen.Queries
	cascade      *policy.CascadeRecompiler
	compiler     *policy.RulesetCompiler
	disconnector Disconnector
}

func registerHosts(r chi.Router, q *storegen.Queries, cascade *policy.CascadeRecompiler, compiler *policy.RulesetCompiler, disconnector Disconnector) {
	h := &hostHandler{q: q, cascade: cascade, compiler: compiler, disconnector: disconnector}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Patch("/{id}", h.patch)
	r.Delete("/{id}", h.delete)
	r.Get("/{id}/ruleset", h.getRuleset)
}

// RulesetRuleItem is one resolved rule in the effective ruleset response.
type RulesetRuleItem struct {
	RuleID     string   `json:"rule_id"`
	RuleName   string   `json:"rule_name"`
	PolicyID   string   `json:"policy_id"`
	PolicyName string   `json:"policy_name"`
	Priority   int32    `json:"priority"`
	Direction  string   `json:"direction"`
	Action     string   `json:"action"`
	Protocol   string   `json:"protocol,omitempty"`
	SrcCIDRs   []string `json:"src_cidrs,omitempty"`
	DstCIDRs   []string `json:"dst_cidrs,omitempty"`
	SrcPorts   []string `json:"src_ports,omitempty"`
	DstPorts   []string `json:"dst_ports,omitempty"`
}

// RulesetResponse is the effective ruleset for a host.
type RulesetResponse struct {
	HostID         string            `json:"host_id"`
	DefaultIngress string            `json:"default_ingress"`
	DefaultEgress  string            `json:"default_egress"`
	Rules          []RulesetRuleItem `json:"rules"`
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
		req.Status = "enrolling"
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

	// Capture current state for audit log before.
	if cur, err := h.q.GetHost(r.Context(), storegen.GetHostParams{ID: hostUUID, TenantID: tenantUUID}); err == nil {
		if b, jerr := json.Marshal(toHostResponse(cur)); jerr == nil {
			SetAuditBefore(r.Context(), b)
		}
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
		if h.cascade != nil {
			tenantStr := uuidToStr(tenantUUID)
			hostStr := uuidToStr(hostUUID)
			go func() {
				if err := h.cascade.OnHostLabelsChanged(context.Background(), tenantStr, hostStr); err != nil {
					slog.Warn("cascade: host labels push failed", "host", hostStr, "err", err)
				}
			}()
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

	if req.FlowEventsEnabled != nil {
		if err = h.q.UpdateHostFlowEnabled(r.Context(), storegen.UpdateHostFlowEnabledParams{
			ID:                hostUUID,
			TenantID:          tenantUUID,
			FlowEventsEnabled: *req.FlowEventsEnabled,
		}); err != nil {
			apiError(w, http.StatusInternalServerError, "update flow_events_enabled: "+err.Error())
			return
		}
		if h.disconnector != nil {
			go h.disconnector.Disconnect(uuidToStr(hostUUID), "config-updated", 1000)
		}
		host, err = h.q.GetHost(r.Context(), storegen.GetHostParams{
			ID:       hostUUID,
			TenantID: tenantUUID,
		})
		if err != nil {
			apiError(w, http.StatusInternalServerError, "get host: "+err.Error())
			return
		}
	}

	if req.Labels == nil && req.Status == nil && req.FlowEventsEnabled == nil {
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

func (h *hostHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.q.DeleteHost(r.Context(), storegen.DeleteHostParams{
		ID:       hostUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusInternalServerError, "delete host: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── helpers ───────────────────────────────────────────────────────────────────

func toHostResponse(h storegen.Host) HostResponse {
	resp := HostResponse{
		ID:                 uuidToStr(h.ID),
		TenantID:           uuidToStr(h.TenantID),
		Hostname:           h.Hostname,
		Status:             h.Status,
		CurrentGeneration:  h.CurrentGeneration,
		AgentVersion:       h.AgentVersion,
		KernelVersion:      h.KernelVersion,
		FlowEventsEnabled:  h.FlowEventsEnabled,
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
	// Prefer tenant ID from JWT claims (browser cookie auth).
	if c := auth.ClaimsFromCtx(r.Context()); c != nil && c.TenantID != "" {
		var u pgtype.UUID
		if err := u.Scan(c.TenantID); err == nil {
			return u, true
		}
	}
	// Fall back to explicit header (API token clients).
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

func (h *hostHandler) getRuleset(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	hostUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	tenantID := uuidToStr(tenantUUID)
	hostID := uuidToStr(hostUUID)

	rs, err := h.compiler.Compile(r.Context(), tenantID, hostID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, err.Error())
		return
	}

	items := make([]RulesetRuleItem, 0, len(rs.Rules))
	for _, rr := range rs.Rules {
		item := RulesetRuleItem{
			RuleID:     uuidToStr(rr.Rule.ID),
			RuleName:   rr.Rule.Name,
			PolicyID:   uuidToStr(rr.Policy.ID),
			PolicyName: rr.Policy.Name,
			Priority:   rr.Rule.Priority,
			Direction:  rr.Rule.Direction,
			Action:     rr.Rule.Action,
			Protocol:   rr.Match.Protocol,
			SrcCIDRs:   cidrSliceToStr(rr.SrcCIDRs),
			DstCIDRs:   cidrSliceToStr(rr.DstCIDRs),
			SrcPorts:   portRangeSliceToStr(rr.SrcPorts),
			DstPorts:   portRangeSliceToStr(rr.DstPorts),
		}
		items = append(items, item)
	}

	writeJSON(w, http.StatusOK, RulesetResponse{
		HostID:         hostID,
		DefaultIngress: rs.DefaultIngress,
		DefaultEgress:  rs.DefaultEgress,
		Rules:          items,
	})
}

func cidrSliceToStr(cidrs []*qfv1.CIDR) []string {
	if len(cidrs) == 0 {
		return nil
	}
	out := make([]string, 0, len(cidrs))
	for _, c := range cidrs {
		ip := net.IP(c.Ip)
		out = append(out, fmt.Sprintf("%s/%d", ip.String(), c.PrefixLen))
	}
	return out
}

func portRangeSliceToStr(ports []*qfv1.PortRange) []string {
	if len(ports) == 0 {
		return nil
	}
	out := make([]string, 0, len(ports))
	for _, p := range ports {
		if p.Start == p.End {
			out = append(out, fmt.Sprintf("%d", p.Start))
		} else {
			out = append(out, fmt.Sprintf("%d-%d", p.Start, p.End))
		}
	}
	return out
}
