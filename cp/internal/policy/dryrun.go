package policy

import (
	"cmp"
	"context"
	"encoding/json"
	"slices"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// DryRunRuleSpec is a rule as submitted in a preview request (no DB IDs required).
type DryRunRuleSpec struct {
	ID        string  `json:"id,omitempty"` // existing rule UUID; empty = new
	Name      string  `json:"name"`
	Priority  int32   `json:"priority"`
	Direction string  `json:"direction"`
	Match     json.RawMessage `json:"match"`
	State     *string `json:"state,omitempty"`
	Action    string  `json:"action"`
	Log       bool    `json:"log"`
	Silent    bool    `json:"silent"`
}

// HostDiff describes what changes on one host after the policy patch is applied.
type HostDiff struct {
	ID       string   `json:"id"`
	Hostname string   `json:"hostname"`
	Added    []string `json:"added"`   // rule IDs appearing after but not before
	Removed  []string `json:"removed"` // rule IDs appearing before but not after
	Changed  []string `json:"changed"` // rule IDs in both but with different action/direction
}

// PreviewResult is the dry-run response.
type PreviewResult struct {
	AffectedCount int        `json:"affected_count"`
	Hosts         []HostDiff `json:"hosts"`
}

// CompileWithOverride compiles the effective ruleset for hostID, replacing
// the rules of overridePolicyID with overrideRules (without touching the DB).
func (c *RulesetCompiler) CompileWithOverride(
	ctx context.Context,
	tenantID, hostID, overridePolicyID string,
	overrideRules []DryRunRuleSpec,
) (*EffectiveRuleset, error) {
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, err
	}

	dp, err := c.queries.GetDefaultPolicy(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}

	policies, err := c.queries.ListPolicies(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}

	var hostLabels map[string]string
	{
		var hostUUID pgtype.UUID
		if err := hostUUID.Scan(hostID); err != nil {
			return nil, err
		}
		h, err := c.queries.GetHost(ctx, storegen.GetHostParams{ID: hostUUID, TenantID: tenantUUID})
		if err != nil {
			return nil, err
		}
		hostLabels, _ = parseLabels(h.Labels)
	}

	var resolved []ResolvedRule
	for _, p := range policies {
		sel, err := ParseSelector(p.Selector)
		if err != nil {
			continue
		}
		if !MatchesLabels(sel, hostLabels) {
			continue
		}
		if pgUUIDToStr(p.ID) == overridePolicyID {
			// Use in-memory override rules instead of DB.
			rules, err := c.resolveOverrideRules(ctx, tenantID, p, overrideRules)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, rules...)
		} else {
			rules, err := c.resolveRulesForPolicy(ctx, tenantID, p)
			if err != nil {
				return nil, err
			}
			resolved = append(resolved, rules...)
		}
	}

	slices.SortFunc(resolved, func(a, b ResolvedRule) int {
		if n := cmp.Compare(a.Policy.Priority, b.Policy.Priority); n != 0 {
			return n
		}
		if n := cmp.Compare(a.Rule.Priority, b.Rule.Priority); n != 0 {
			return n
		}
		return cmp.Compare(pgUUIDToStr(a.Rule.ID), pgUUIDToStr(b.Rule.ID))
	})

	return &EffectiveRuleset{
		Rules:          resolved,
		DefaultIngress: dp.DefaultIngressAction,
		DefaultEgress:  dp.DefaultEgressAction,
	}, nil
}

func (c *RulesetCompiler) resolveOverrideRules(
	ctx context.Context,
	tenantID string,
	p storegen.Policy,
	specs []DryRunRuleSpec,
) ([]ResolvedRule, error) {
	var resolved []ResolvedRule
	for _, spec := range specs {
		// Build a synthetic storegen.Rule from the spec.
		var ruleID pgtype.UUID
		if spec.ID != "" {
			ruleID.Scan(spec.ID) //nolint:errcheck
		}
		dir := spec.Direction
		if dir == "" {
			dir = "ingress"
		}
		action := spec.Action
		if action == "" {
			action = "deny"
		}
		r := storegen.Rule{
			ID:        ruleID,
			PolicyID:  p.ID,
			Name:      spec.Name,
			Priority:  spec.Priority,
			Direction: dir,
			Match:     spec.Match,
			State:     spec.State,
			Action:    action,
			Log:       spec.Log,
			Silent:    spec.Silent,
		}
		rr, err := c.resolveRule(ctx, tenantID, p, r)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, rr)
	}
	return resolved, nil
}

// DryRunPreview computes what changes on all tenant hosts when overridePolicyID's
// rules are replaced with overrideRules.
func (c *RulesetCompiler) DryRunPreview(
	ctx context.Context,
	tenantID, overridePolicyID string,
	overrideRules []DryRunRuleSpec,
) (*PreviewResult, error) {
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, err
	}
	hosts, err := c.queries.ListHosts(ctx, tenantUUID)
	if err != nil {
		return nil, err
	}

	result := &PreviewResult{Hosts: []HostDiff{}}

	for _, h := range hosts {
		hostID := pgUUIDToStr(h.ID)

		before, err := c.Compile(ctx, tenantID, hostID)
		if err != nil {
			continue
		}
		after, err := c.CompileWithOverride(ctx, tenantID, hostID, overridePolicyID, overrideRules)
		if err != nil {
			continue
		}

		diff := diffRulesets(before, after)
		if len(diff.Added) == 0 && len(diff.Removed) == 0 && len(diff.Changed) == 0 {
			continue
		}
		diff.ID = hostID
		diff.Hostname = h.Hostname
		result.Hosts = append(result.Hosts, diff)
	}
	result.AffectedCount = len(result.Hosts)
	return result, nil
}

func diffRulesets(before, after *EffectiveRuleset) HostDiff {
	type ruleKey struct {
		id     string
		action string
		dir    string
	}
	beforeMap := make(map[string]ruleKey, len(before.Rules))
	for _, r := range before.Rules {
		id := pgUUIDToStr(r.Rule.ID)
		beforeMap[id] = ruleKey{id: id, action: r.Rule.Action, dir: r.Rule.Direction}
	}
	afterMap := make(map[string]ruleKey, len(after.Rules))
	for _, r := range after.Rules {
		id := pgUUIDToStr(r.Rule.ID)
		afterMap[id] = ruleKey{id: id, action: r.Rule.Action, dir: r.Rule.Direction}
	}

	var added, removed, changed []string
	for id, ak := range afterMap {
		bk, exists := beforeMap[id]
		if !exists {
			added = append(added, id)
		} else if ak != bk {
			changed = append(changed, id)
		}
	}
	for id := range beforeMap {
		if _, exists := afterMap[id]; !exists {
			removed = append(removed, id)
		}
	}
	slices.Sort(added)
	slices.Sort(removed)
	slices.Sort(changed)
	return HostDiff{Added: added, Removed: removed, Changed: changed}
}
