package policy

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"

	storegen "github.com/qf/qf/cp/internal/store/gen"
	"github.com/jackc/pgx/v5/pgtype"
)

// Selector mirrors the k8s-style label selector stored in policy.selector JSONB.
type Selector struct {
	MatchLabels      map[string]string          `json:"matchLabels,omitempty"`
	MatchExpressions []LabelSelectorRequirement `json:"matchExpressions,omitempty"`
}

// LabelSelectorRequirement is one matchExpressions entry.
type LabelSelectorRequirement struct {
	Key      string   `json:"key"`
	Operator string   `json:"operator"` // In | NotIn | Exists | DoesNotExist
	Values   []string `json:"values,omitempty"`
}

// ParseSelector decodes the JSONB selector bytes from a policy row.
// An empty or null selector matches all hosts.
func ParseSelector(raw []byte) (Selector, error) {
	var s Selector
	if len(raw) == 0 || string(raw) == "null" || string(raw) == "{}" {
		return s, nil
	}
	return s, json.Unmarshal(raw, &s)
}

// MatchesLabels reports whether labels satisfy sel.
// Empty selector (no matchLabels, no matchExpressions) matches everything.
func MatchesLabels(sel Selector, labels map[string]string) bool {
	for k, v := range sel.MatchLabels {
		if labels[k] != v {
			return false
		}
	}
	for _, expr := range sel.MatchExpressions {
		if !evalExpression(expr, labels) {
			return false
		}
	}
	return true
}

func evalExpression(expr LabelSelectorRequirement, labels map[string]string) bool {
	val, exists := labels[expr.Key]
	switch expr.Operator {
	case "In":
		return exists && slices.Contains(expr.Values, val)
	case "NotIn":
		return !exists || !slices.Contains(expr.Values, val)
	case "Exists":
		return exists
	case "DoesNotExist":
		return !exists
	default:
		return false
	}
}

// SelectorMatcher resolves a selector to matching hosts using the DB.
type SelectorMatcher struct {
	queries *storegen.Queries
}

// NewSelectorMatcher creates a SelectorMatcher.
func NewSelectorMatcher(queries *storegen.Queries) *SelectorMatcher {
	return &SelectorMatcher{queries: queries}
}

// ResolveHosts returns all active hosts in the tenant that match sel.
func (sm *SelectorMatcher) ResolveHosts(ctx context.Context, tenantID string, sel Selector) ([]storegen.Host, error) {
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, fmt.Errorf("selector: invalid tenant_id: %w", err)
	}

	hosts, err := sm.queries.ListHosts(ctx, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("selector: list hosts: %w", err)
	}

	var matched []storegen.Host
	for _, h := range hosts {
		if h.Status != "active" {
			continue
		}
		labels, err := parseLabels(h.Labels)
		if err != nil {
			continue
		}
		if MatchesLabels(sel, labels) {
			matched = append(matched, h)
		}
	}
	return matched, nil
}

// ResolveHostIDs returns just the host UUIDs as strings.
func (sm *SelectorMatcher) ResolveHostIDs(ctx context.Context, tenantID string, sel Selector) ([]string, error) {
	hosts, err := sm.ResolveHosts(ctx, tenantID, sel)
	if err != nil {
		return nil, err
	}
	ids := make([]string, len(hosts))
	for i, h := range hosts {
		ids[i] = pgUUIDToStr(h.ID)
	}
	return ids, nil
}

func parseLabels(raw []byte) (map[string]string, error) {
	var m map[string]string
	if len(raw) == 0 {
		return m, nil
	}
	return m, json.Unmarshal(raw, &m)
}

func pgUUIDToStr(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%08x-%04x-%04x-%04x-%012x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}
