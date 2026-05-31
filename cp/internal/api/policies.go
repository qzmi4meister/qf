package api

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// PolicyResponse is the JSON shape for a policy.
type PolicyResponse struct {
	ID             string          `json:"id"`
	TenantID       string          `json:"tenant_id"`
	Name           string          `json:"name"`
	Description    string          `json:"description"`
	Priority       int32           `json:"priority"`
	Selector       json.RawMessage `json:"selector"`
	CurrentVersion int32           `json:"current_version"`
	CreatedAt      time.Time       `json:"created_at"`
	UpdatedAt      time.Time       `json:"updated_at"`
}

// RuleResponse is the JSON shape for a rule.
type RuleResponse struct {
	ID        string          `json:"id"`
	PolicyID  string          `json:"policy_id"`
	Name      string          `json:"name"`
	Priority  int32           `json:"priority"`
	Direction string          `json:"direction"`
	Match     json.RawMessage `json:"match"`
	State     *string         `json:"state,omitempty"`
	Action    string          `json:"action"`
	Log       bool            `json:"log"`
	Silent    bool            `json:"silent"`
	CreatedAt time.Time       `json:"created_at"`
	UpdatedAt time.Time       `json:"updated_at"`
}

type createPolicyRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Priority    int32           `json:"priority"`
	Selector    json.RawMessage `json:"selector"`
}

type updatePolicyRequest struct {
	Name        string          `json:"name"`
	Description string          `json:"description"`
	Priority    int32           `json:"priority"`
	Selector    json.RawMessage `json:"selector"`
}

type createRuleRequest struct {
	Name      string          `json:"name"`
	Priority  int32           `json:"priority"`
	Direction string          `json:"direction"`
	Match     json.RawMessage `json:"match"`
	State     *string         `json:"state,omitempty"`
	Action    string          `json:"action"`
	Log       bool            `json:"log"`
	Silent    bool            `json:"silent"`
}

type updateRuleRequest struct {
	Name      string          `json:"name"`
	Priority  int32           `json:"priority"`
	Direction string          `json:"direction"`
	Match     json.RawMessage `json:"match"`
	State     *string         `json:"state,omitempty"`
	Action    string          `json:"action"`
	Log       bool            `json:"log"`
	Silent    bool            `json:"silent"`
}

type policyHandler struct {
	q *storegen.Queries
}

func registerPolicies(r chi.Router, q *storegen.Queries) {
	h := &policyHandler{q: q}
	r.Get("/", h.list)
	r.Post("/", h.create)
	r.Get("/{id}", h.get)
	r.Put("/{id}", h.update)
	r.Delete("/{id}", h.delete)
	r.Route("/{id}/rules", func(r chi.Router) {
		r.Get("/", h.listRules)
		r.Post("/", h.createRule)
		r.Get("/{ruleID}", h.getRule)
		r.Put("/{ruleID}", h.updateRule)
		r.Delete("/{ruleID}", h.deleteRule)
	})
}

// ── Policies ──────────────────────────────────────────────────────────────────

func (h *policyHandler) list(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policies, err := h.q.ListPolicies(r.Context(), tenantUUID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list policies: "+err.Error())
		return
	}
	resp := make([]PolicyResponse, 0, len(policies))
	for _, p := range policies {
		resp = append(resp, toPolicyResponse(p))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *policyHandler) create(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	var req createPolicyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}
	sel := rawJSONOrEmpty(req.Selector)
	policy, err := h.q.CreatePolicy(r.Context(), storegen.CreatePolicyParams{
		TenantID:    tenantUUID,
		Name:        req.Name,
		Description: req.Description,
		Priority:    req.Priority,
		Selector:    sel,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "create policy: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toPolicyResponse(policy))
}

func (h *policyHandler) get(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	policy, err := h.q.GetPolicy(r.Context(), storegen.GetPolicyParams{
		ID:       policyUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		apiError(w, http.StatusNotFound, "policy not found")
		return
	}
	writeJSON(w, http.StatusOK, toPolicyResponse(policy))
}

func (h *policyHandler) update(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	var req updatePolicyRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}
	policy, err := h.q.UpdatePolicy(r.Context(), storegen.UpdatePolicyParams{
		ID:          policyUUID,
		TenantID:    tenantUUID,
		Name:        req.Name,
		Description: req.Description,
		Priority:    req.Priority,
		Selector:    rawJSONOrEmpty(req.Selector),
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "update policy: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toPolicyResponse(policy))
}

func (h *policyHandler) delete(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if err := h.q.DeletePolicy(r.Context(), storegen.DeletePolicyParams{
		ID:       policyUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusInternalServerError, "delete policy: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// ── Rules ─────────────────────────────────────────────────────────────────────

func (h *policyHandler) listRules(w http.ResponseWriter, r *http.Request) {
	policyUUID, ok := h.resolvePolicy(w, r)
	if !ok {
		return
	}
	rules, err := h.q.ListRulesByPolicy(r.Context(), policyUUID)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list rules: "+err.Error())
		return
	}
	resp := make([]RuleResponse, 0, len(rules))
	for _, rule := range rules {
		resp = append(resp, toRuleResponse(rule))
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *policyHandler) createRule(w http.ResponseWriter, r *http.Request) {
	policyUUID, ok := h.resolvePolicy(w, r)
	if !ok {
		return
	}
	var req createRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}
	if req.Direction == "" {
		req.Direction = "ingress"
	}
	if req.Action == "" {
		req.Action = "deny"
	}
	rule, err := h.q.CreateRule(r.Context(), storegen.CreateRuleParams{
		PolicyID:  policyUUID,
		Name:      req.Name,
		Priority:  req.Priority,
		Direction: req.Direction,
		Match:     rawJSONOrEmpty(req.Match),
		State:     req.State,
		Action:    req.Action,
		Log:       req.Log,
		Silent:    req.Silent,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "create rule: "+err.Error())
		return
	}
	writeJSON(w, http.StatusCreated, toRuleResponse(rule))
}

func (h *policyHandler) getRule(w http.ResponseWriter, r *http.Request) {
	policyUUID, ok := h.resolvePolicy(w, r)
	if !ok {
		return
	}
	ruleUUID, ok := uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	rule, err := h.q.GetRule(r.Context(), ruleUUID)
	if err != nil || rule.PolicyID != policyUUID {
		apiError(w, http.StatusNotFound, "rule not found")
		return
	}
	writeJSON(w, http.StatusOK, toRuleResponse(rule))
}

func (h *policyHandler) updateRule(w http.ResponseWriter, r *http.Request) {
	policyUUID, ok := h.resolvePolicy(w, r)
	if !ok {
		return
	}
	ruleUUID, ok := uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	// Verify ownership.
	existing, err := h.q.GetRule(r.Context(), ruleUUID)
	if err != nil || existing.PolicyID != policyUUID {
		apiError(w, http.StatusNotFound, "rule not found")
		return
	}
	var req updateRuleRequest
	if !decodeJSON(w, r, &req) {
		return
	}
	if req.Name == "" {
		apiError(w, http.StatusBadRequest, "name required")
		return
	}
	rule, err := h.q.UpdateRule(r.Context(), storegen.UpdateRuleParams{
		ID:        ruleUUID,
		Name:      req.Name,
		Priority:  req.Priority,
		Direction: req.Direction,
		Match:     rawJSONOrEmpty(req.Match),
		State:     req.State,
		Action:    req.Action,
		Log:       req.Log,
		Silent:    req.Silent,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "update rule: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, toRuleResponse(rule))
}

func (h *policyHandler) deleteRule(w http.ResponseWriter, r *http.Request) {
	policyUUID, ok := h.resolvePolicy(w, r)
	if !ok {
		return
	}
	ruleUUID, ok := uuidParam(w, r, "ruleID")
	if !ok {
		return
	}
	existing, err := h.q.GetRule(r.Context(), ruleUUID)
	if err != nil || existing.PolicyID != policyUUID {
		apiError(w, http.StatusNotFound, "rule not found")
		return
	}
	if err := h.q.DeleteRule(r.Context(), ruleUUID); err != nil {
		apiError(w, http.StatusInternalServerError, "delete rule: "+err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// resolvePolicy verifies tenant owns the policy and returns its UUID.
func (h *policyHandler) resolvePolicy(w http.ResponseWriter, r *http.Request) (pgtype.UUID, bool) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return pgtype.UUID{}, false
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return pgtype.UUID{}, false
	}
	if _, err := h.q.GetPolicy(r.Context(), storegen.GetPolicyParams{
		ID:       policyUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusNotFound, "policy not found")
		return pgtype.UUID{}, false
	}
	return policyUUID, true
}

// ── converters ────────────────────────────────────────────────────────────────

func toPolicyResponse(p storegen.Policy) PolicyResponse {
	resp := PolicyResponse{
		ID:             uuidToStr(p.ID),
		TenantID:       uuidToStr(p.TenantID),
		Name:           p.Name,
		Description:    p.Description,
		Priority:       p.Priority,
		CurrentVersion: p.CurrentVersion,
		Selector:       jsonOrNull(p.Selector),
	}
	if p.CreatedAt.Valid {
		resp.CreatedAt = p.CreatedAt.Time
	}
	if p.UpdatedAt.Valid {
		resp.UpdatedAt = p.UpdatedAt.Time
	}
	return resp
}

func toRuleResponse(r storegen.Rule) RuleResponse {
	resp := RuleResponse{
		ID:        uuidToStr(r.ID),
		PolicyID:  uuidToStr(r.PolicyID),
		Name:      r.Name,
		Priority:  r.Priority,
		Direction: r.Direction,
		Match:     jsonOrNull(r.Match),
		State:     r.State,
		Action:    r.Action,
		Log:       r.Log,
		Silent:    r.Silent,
	}
	if r.CreatedAt.Valid {
		resp.CreatedAt = r.CreatedAt.Time
	}
	if r.UpdatedAt.Valid {
		resp.UpdatedAt = r.UpdatedAt.Time
	}
	return resp
}

func rawJSONOrEmpty(raw json.RawMessage) []byte {
	if len(raw) == 0 {
		return []byte("{}")
	}
	return []byte(raw)
}

func jsonOrNull(b []byte) json.RawMessage {
	if len(b) == 0 {
		return json.RawMessage("null")
	}
	return json.RawMessage(b)
}
