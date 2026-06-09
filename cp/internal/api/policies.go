package api

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/auth"
	polpkg "github.com/qf/qf/cp/internal/policy"
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
	Name        string               `json:"name"`
	Description string               `json:"description"`
	Priority    int32                `json:"priority"`
	Selector    json.RawMessage      `json:"selector"`
	Rules       []createRuleRequest  `json:"rules"`
}

// PolicyDetailResponse includes the policy and its rules.
type PolicyDetailResponse struct {
	PolicyResponse
	Rules []RuleResponse `json:"rules"`
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
	q        *storegen.Queries
	db       *pgxpool.Pool
	compiler *polpkg.RulesetCompiler
	cascade  *polpkg.CascadeRecompiler // nil = push disabled
	tenantID pgtype.UUID
}

func registerPolicies(r chi.Router, q *storegen.Queries, db *pgxpool.Pool, compiler *polpkg.RulesetCompiler, cascade *polpkg.CascadeRecompiler, tenantID pgtype.UUID) {
	h := &policyHandler{q: q, db: db, compiler: compiler, cascade: cascade, tenantID: tenantID}
	rw := auth.RequireRole("admin", "editor")
	r.Get("/", h.list)
	r.With(rw).Post("/", h.create)
	r.Get("/{id}", h.get)
	r.With(rw).Put("/{id}", h.update)
	r.With(rw).Delete("/{id}", h.delete)
	r.Route("/{id}/rules", func(r chi.Router) {
		r.Get("/", h.listRules)
		r.With(rw).Post("/", h.createRule)
		r.Get("/{ruleID}", h.getRule)
		r.With(rw).Put("/{ruleID}", h.updateRule)
		r.With(rw).Delete("/{ruleID}", h.deleteRule)
	})
	r.With(rw).Post("/{id}/preview", h.preview)
	r.Get("/{id}/versions", h.listVersions)
	r.With(rw).Post("/{id}/versions/{v}/revert", h.revertVersion)
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
	rules, _ := h.q.ListRulesByPolicy(r.Context(), policyUUID)
	writeJSON(w, http.StatusOK, toPolicyDetailResponse(policy, rules))
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

	// Capture current state for audit log before, and save old selector for cascade.
	var oldSelectorRaw []byte
	if cur, err := h.q.GetPolicy(r.Context(), storegen.GetPolicyParams{ID: policyUUID, TenantID: tenantUUID}); err == nil {
		oldSelectorRaw = cur.Selector
		curRules, _ := h.q.ListRulesByPolicy(r.Context(), policyUUID)
		if b, jerr := json.Marshal(toPolicyDetailResponse(cur, curRules)); jerr == nil {
			SetAuditBefore(r.Context(), b)
		}
	}

	tx, err := h.db.Begin(r.Context())
	if err != nil {
		apiError(w, http.StatusInternalServerError, "begin tx: "+err.Error())
		return
	}
	defer tx.Rollback(r.Context()) //nolint:errcheck

	qtx := h.q.WithTx(tx)

	policy, err := qtx.UpdatePolicy(r.Context(), storegen.UpdatePolicyParams{
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

	// Sync rules: delete all existing, recreate from request body (within tx).
	existing, _ := qtx.ListRulesByPolicy(r.Context(), policyUUID)
	for _, er := range existing {
		if err := qtx.DeleteRule(r.Context(), er.ID); err != nil {
			apiError(w, http.StatusInternalServerError, "delete rule: "+err.Error())
			return
		}
	}
	for _, rr := range req.Rules {
		if rr.Direction == "" {
			rr.Direction = "ingress"
		}
		if rr.Action == "" {
			rr.Action = "deny"
		}
		if _, err := qtx.CreateRule(r.Context(), storegen.CreateRuleParams{
			PolicyID:  policyUUID,
			Name:      rr.Name,
			Priority:  rr.Priority,
			Direction: rr.Direction,
			Match:     rawJSONOrEmpty(rr.Match),
			State:     rr.State,
			Action:    rr.Action,
			Log:       rr.Log,
			Silent:    rr.Silent,
		}); err != nil {
			apiError(w, http.StatusInternalServerError, "create rule: "+err.Error())
			return
		}
	}

	if err := tx.Commit(r.Context()); err != nil {
		apiError(w, http.StatusInternalServerError, "commit tx: "+err.Error())
		return
	}

	savedRules, _ := h.q.ListRulesByPolicy(r.Context(), policyUUID)

	// Write version snapshot asynchronously (best-effort).
	go func() {
		content, err := snapshotPolicy(policy, savedRules)
		if err != nil {
			return
		}
		actor := ""
		if c := auth.ClaimsFromCtx(r.Context()); c != nil {
			actor = c.Email
		}
		h.q.InsertConfigVersion(r.Context(), storegen.InsertConfigVersionParams{ //nolint:errcheck
			TenantID:   tenantUUID,
			EntityType: "policy",
			EntityID:   policyUUID,
			Content:    content,
			CreatedBy:  actor,
		})
	}()

	h.dispatchCascadeWithOldSelector(r, policyUUID, oldSelectorRaw)
	writeJSON(w, http.StatusOK, toPolicyDetailResponse(policy, savedRules))
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
	h.dispatchCascade(r, policyUUID)
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

// dispatchCascade triggers a non-blocking policy recompile+push for all matching hosts.
func (h *policyHandler) dispatchCascade(_ *http.Request, policyID pgtype.UUID) {
	if h.cascade == nil {
		return
	}
	tenantStr := uuidToStr(h.tenantID)
	policyStr := uuidToStr(policyID)
	go func() {
		if err := h.cascade.OnPolicyChanged(context.Background(), tenantStr, policyStr); err != nil {
			slog.Warn("cascade: policy push failed", "policy", policyStr, "err", err)
		}
	}()
}

// dispatchCascadeWithOldSelector recompiles for union(old selector hosts, new selector hosts).
func (h *policyHandler) dispatchCascadeWithOldSelector(_ *http.Request, policyID pgtype.UUID, oldSelectorRaw []byte) {
	if h.cascade == nil {
		return
	}
	tenantStr := uuidToStr(h.tenantID)
	policyStr := uuidToStr(policyID)
	go func() {
		if err := h.cascade.OnPolicyChangedWithOldSelector(context.Background(), tenantStr, policyStr, oldSelectorRaw); err != nil {
			slog.Warn("cascade: policy push failed", "policy", policyStr, "err", err)
		}
	}()
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
	if req.State != nil && *req.State == "related" {
		apiError(w, http.StatusUnprocessableEntity, "state=related is not implemented in the BPF datapath; rules with this state will never match")
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
	h.dispatchCascade(r, policyUUID)
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
	if req.State != nil && *req.State == "related" {
		apiError(w, http.StatusUnprocessableEntity, "state=related is not implemented in the BPF datapath; rules with this state will never match")
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
	h.dispatchCascade(r, policyUUID)
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
	h.dispatchCascade(r, policyUUID)
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

func toPolicyDetailResponse(p storegen.Policy, rules []storegen.Rule) PolicyDetailResponse {
	ruleResps := make([]RuleResponse, 0, len(rules))
	for _, r := range rules {
		ruleResps = append(ruleResps, toRuleResponse(r))
	}
	return PolicyDetailResponse{PolicyResponse: toPolicyResponse(p), Rules: ruleResps}
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

// ── P4-BE-02: Policy dry-run preview ─────────────────────────────────────────

type previewRequest struct {
	Rules []polpkg.DryRunRuleSpec `json:"rules"`
}

func (h *policyHandler) preview(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.q.GetPolicy(r.Context(), storegen.GetPolicyParams{
		ID:       policyUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusNotFound, "policy not found")
		return
	}

	var req previewRequest
	if !decodeJSON(w, r, &req) {
		return
	}

	tenantIDStr := uuidToStr(tenantUUID)
	policyIDStr := uuidToStr(policyUUID)

	result, err := h.compiler.DryRunPreview(r.Context(), tenantIDStr, policyIDStr, req.Rules)
	if err != nil {
		apiError(w, http.StatusInternalServerError, "preview: "+err.Error())
		return
	}
	writeJSON(w, http.StatusOK, result)
}

// ── P4-BE-03: Policy version history ─────────────────────────────────────────

type versionResponse struct {
	ID        string          `json:"id"`
	Version   int32           `json:"version"`
	Content   json.RawMessage `json:"content"`
	CreatedBy string          `json:"created_by"`
	CreatedAt time.Time       `json:"created_at"`
}

func (h *policyHandler) listVersions(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}
	if _, err := h.q.GetPolicy(r.Context(), storegen.GetPolicyParams{
		ID:       policyUUID,
		TenantID: tenantUUID,
	}); err != nil {
		apiError(w, http.StatusNotFound, "policy not found")
		return
	}

	versions, err := h.q.ListConfigVersions(r.Context(), storegen.ListConfigVersionsParams{
		TenantID:   tenantUUID,
		EntityType: "policy",
		EntityID:   policyUUID,
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "list versions: "+err.Error())
		return
	}

	resp := make([]versionResponse, 0, len(versions))
	for _, v := range versions {
		vr := versionResponse{
			ID:        uuidToStr(v.ID),
			Version:   v.Version,
			Content:   json.RawMessage(v.Content),
			CreatedBy: v.CreatedBy,
		}
		if v.CreatedAt.Valid {
			vr.CreatedAt = v.CreatedAt.Time
		}
		resp = append(resp, vr)
	}
	writeJSON(w, http.StatusOK, resp)
}

func (h *policyHandler) revertVersion(w http.ResponseWriter, r *http.Request) {
	tenantUUID, ok := tenantFromRequest(w, r)
	if !ok {
		return
	}
	policyUUID, ok := uuidParam(w, r, "id")
	if !ok {
		return
	}

	vStr := chi.URLParam(r, "v")
	vNum, err := strconv.Atoi(vStr)
	if err != nil || vNum < 1 {
		apiError(w, http.StatusBadRequest, "invalid version")
		return
	}

	// Load the target version snapshot.
	snap, err := h.q.GetConfigVersion(r.Context(), storegen.GetConfigVersionParams{
		TenantID:   tenantUUID,
		EntityType: "policy",
		EntityID:   policyUUID,
		Version:    int32(vNum),
	})
	if err != nil {
		apiError(w, http.StatusNotFound, "version not found")
		return
	}

	// Decode snapshot to extract policy + rules.
	var snapshot policySnapshot
	if err := json.Unmarshal(snap.Content, &snapshot); err != nil {
		apiError(w, http.StatusInternalServerError, "corrupt snapshot: "+err.Error())
		return
	}

	// Overwrite policy fields.
	policy, err := h.q.UpdatePolicy(r.Context(), storegen.UpdatePolicyParams{
		ID:          policyUUID,
		TenantID:    tenantUUID,
		Name:        snapshot.Policy.Name,
		Description: snapshot.Policy.Description,
		Priority:    snapshot.Policy.Priority,
		Selector:    rawJSONOrEmpty(snapshot.Policy.Selector),
	})
	if err != nil {
		apiError(w, http.StatusInternalServerError, "revert policy: "+err.Error())
		return
	}

	// Replace rules: delete all, recreate from snapshot.
	existing, _ := h.q.ListRulesByPolicy(r.Context(), policyUUID)
	for _, er := range existing {
		h.q.DeleteRule(r.Context(), er.ID) //nolint:errcheck
	}
	for _, sr := range snapshot.Rules {
		h.q.CreateRule(r.Context(), storegen.CreateRuleParams{ //nolint:errcheck
			PolicyID:  policyUUID,
			Name:      sr.Name,
			Priority:  sr.Priority,
			Direction: sr.Direction,
			Match:     rawJSONOrEmpty(sr.Match),
			State:     sr.State,
			Action:    sr.Action,
			Log:       sr.Log,
			Silent:    sr.Silent,
		})
	}

	// Write new version = copy of vN.
	actor := ""
	if c := auth.ClaimsFromCtx(r.Context()); c != nil {
		actor = c.Email
	}
	h.q.InsertConfigVersion(r.Context(), storegen.InsertConfigVersionParams{ //nolint:errcheck
		TenantID:   tenantUUID,
		EntityType: "policy",
		EntityID:   policyUUID,
		Content:    snap.Content,
		CreatedBy:  actor,
	})

	writeJSON(w, http.StatusOK, toPolicyResponse(policy))
}

// policySnapshot is the JSON shape stored in config_versions.content.
type policySnapshot struct {
	Policy struct {
		Name        string          `json:"name"`
		Description string          `json:"description"`
		Priority    int32           `json:"priority"`
		Selector    json.RawMessage `json:"selector"`
	} `json:"policy"`
	Rules []struct {
		Name      string          `json:"name"`
		Priority  int32           `json:"priority"`
		Direction string          `json:"direction"`
		Match     json.RawMessage `json:"match"`
		State     *string         `json:"state,omitempty"`
		Action    string          `json:"action"`
		Log       bool            `json:"log"`
		Silent    bool            `json:"silent"`
	} `json:"rules"`
}

// snapshotPolicy builds a config_versions snapshot for a policy.
func snapshotPolicy(p storegen.Policy, rules []storegen.Rule) ([]byte, error) {
	type ruleSnap struct {
		Name      string          `json:"name"`
		Priority  int32           `json:"priority"`
		Direction string          `json:"direction"`
		Match     json.RawMessage `json:"match"`
		State     *string         `json:"state,omitempty"`
		Action    string          `json:"action"`
		Log       bool            `json:"log"`
		Silent    bool            `json:"silent"`
	}
	snap := struct {
		Policy struct {
			Name        string          `json:"name"`
			Description string          `json:"description"`
			Priority    int32           `json:"priority"`
			Selector    json.RawMessage `json:"selector"`
		} `json:"policy"`
		Rules []ruleSnap `json:"rules"`
	}{}
	snap.Policy.Name = p.Name
	snap.Policy.Description = p.Description
	snap.Policy.Priority = p.Priority
	snap.Policy.Selector = jsonOrNull(p.Selector)
	for _, r := range rules {
		snap.Rules = append(snap.Rules, ruleSnap{
			Name:      r.Name,
			Priority:  r.Priority,
			Direction: r.Direction,
			Match:     jsonOrNull(r.Match),
			State:     r.State,
			Action:    r.Action,
			Log:       r.Log,
			Silent:    r.Silent,
		})
	}
	return json.Marshal(snap)
}
