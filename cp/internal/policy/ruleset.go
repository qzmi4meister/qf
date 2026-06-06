package policy

import (
	"cmp"
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// RuleMatchSpec is the JSONB shape stored in rules.match.
// ObjectGroup refs are IDs; inline lists are used for small static sets.
type RuleMatchSpec struct {
	Protocol string `json:"protocol,omitempty"` // tcp|udp|icmp|icmpv6|any

	// ObjectGroup IDs (resolved by Resolver).
	SrcIPSetID   string `json:"src_ip_set_id,omitempty"`
	DstIPSetID   string `json:"dst_ip_set_id,omitempty"`
	SrcPortSetID string `json:"src_port_set_id,omitempty"`
	DstPortSetID string `json:"dst_port_set_id,omitempty"`
	SrcHostSetID string `json:"src_host_set_id,omitempty"`
	DstHostSetID string `json:"dst_host_set_id,omitempty"`

	// Inline lists (used when no ObjectGroup is needed).
	SrcCIDRs []string `json:"src_cidrs,omitempty"`
	DstCIDRs []string `json:"dst_cidrs,omitempty"`
	SrcPorts []string `json:"src_ports,omitempty"`
	DstPorts []string `json:"dst_ports,omitempty"`

	TCPFlagsMask  uint32 `json:"tcp_flags_mask,omitempty"`
	TCPFlagsMatch uint32 `json:"tcp_flags_match,omitempty"`
}

// ResolvedRule is one fully-resolved rule ready for bundle assembly.
type ResolvedRule struct {
	Rule     storegen.Rule
	Policy   storegen.Policy
	Match    RuleMatchSpec
	SrcCIDRs []*qfv1.CIDR
	DstCIDRs []*qfv1.CIDR
	SrcPorts []*qfv1.PortRange
	DstPorts []*qfv1.PortRange
}

// EffectiveRuleset is the complete resolved policy for one host.
type EffectiveRuleset struct {
	Rules          []ResolvedRule
	DefaultIngress string // "allow" | "deny"
	DefaultEgress  string
}

// RulesetCompiler builds the effective ruleset for a host.
type RulesetCompiler struct {
	queries  *storegen.Queries
	selector *SelectorMatcher
	resolver *Resolver
}

// NewRulesetCompiler creates a RulesetCompiler.
func NewRulesetCompiler(queries *storegen.Queries) *RulesetCompiler {
	return &RulesetCompiler{
		queries:  queries,
		selector: NewSelectorMatcher(queries),
		resolver: NewResolver(queries),
	}
}

// Compile builds the effective ruleset for hostID in tenantID.
func (c *RulesetCompiler) Compile(ctx context.Context, tenantID, hostID string) (*EffectiveRuleset, error) {
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, fmt.Errorf("compiler: invalid tenant_id: %w", err)
	}

	// Load default policy first.
	dp, err := c.queries.GetDefaultPolicy(ctx, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("compiler: get default policy: %w", err)
	}

	// Load all policies, keep those whose selector matches the host.
	policies, err := c.queries.ListPolicies(ctx, tenantUUID)
	if err != nil {
		return nil, fmt.Errorf("compiler: list policies: %w", err)
	}

	var hostLabels map[string]string
	{
		var hostUUID pgtype.UUID
		if err := hostUUID.Scan(hostID); err != nil {
			return nil, fmt.Errorf("compiler: invalid host_id: %w", err)
		}
		h, err := c.queries.GetHost(ctx, storegen.GetHostParams{ID: hostUUID, TenantID: tenantUUID})
		if err != nil {
			return nil, fmt.Errorf("compiler: get host: %w", err)
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
		rules, err := c.resolveRulesForPolicy(ctx, tenantID, p)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, rules...)
	}

	// Sort: (policy.priority ASC, rule.priority ASC, rule.id ASC).
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

func (c *RulesetCompiler) resolveRulesForPolicy(ctx context.Context, tenantID string, p storegen.Policy) ([]ResolvedRule, error) {
	var policyUUID pgtype.UUID
	if err := policyUUID.Scan(pgUUIDToStr(p.ID)); err != nil {
		return nil, err
	}
	rules, err := c.queries.ListRulesByPolicy(ctx, policyUUID)
	if err != nil {
		return nil, fmt.Errorf("compiler: list rules for policy %s: %w", pgUUIDToStr(p.ID), err)
	}

	var resolved []ResolvedRule
	for _, r := range rules {
		rr, err := c.resolveRule(ctx, tenantID, p, r)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, rr)
	}
	return resolved, nil
}

func (c *RulesetCompiler) resolveRule(ctx context.Context, tenantID string, p storegen.Policy, r storegen.Rule) (ResolvedRule, error) {
	var ms RuleMatchSpec
	if len(r.Match) > 0 {
		if err := json.Unmarshal(r.Match, &ms); err != nil {
			return ResolvedRule{}, fmt.Errorf("compiler: parse rule match %s: %w", pgUUIDToStr(r.ID), err)
		}
	}

	rr := ResolvedRule{Rule: r, Policy: p, Match: ms}

	// Resolve src CIDRs: ObjectGroup or inline.
	if ms.SrcIPSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.SrcIPSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.SrcCIDRs = g.CIDRs
	} else if ms.SrcHostSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.SrcHostSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.SrcCIDRs = g.CIDRs
	} else {
		rr.SrcCIDRs, _ = parseCIDRList(ms.SrcCIDRs)
	}

	// Resolve dst CIDRs.
	if ms.DstIPSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.DstIPSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.DstCIDRs = g.CIDRs
	} else if ms.DstHostSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.DstHostSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.DstCIDRs = g.CIDRs
	} else {
		rr.DstCIDRs, _ = parseCIDRList(ms.DstCIDRs)
	}

	// Resolve src ports.
	if ms.SrcPortSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.SrcPortSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.SrcPorts = g.Ports
	} else {
		rr.SrcPorts, _ = parsePortList(ms.SrcPorts)
	}

	// Resolve dst ports.
	if ms.DstPortSetID != "" {
		g, err := c.resolver.Resolve(ctx, tenantID, ms.DstPortSetID)
		if err != nil {
			return ResolvedRule{}, err
		}
		rr.DstPorts = g.Ports
	} else {
		rr.DstPorts, _ = parsePortList(ms.DstPorts)
	}

	return rr, nil
}

func parseCIDRList(ss []string) ([]*qfv1.CIDR, error) {
	out := make([]*qfv1.CIDR, 0, len(ss))
	for _, s := range ss {
		c, err := parseCIDR(s)
		if err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, nil
}

func parsePortList(ss []string) ([]*qfv1.PortRange, error) {
	out := make([]*qfv1.PortRange, 0, len(ss))
	for _, s := range ss {
		p, err := parsePortRange(s)
		if err != nil {
			return nil, err
		}
		out = append(out, p)
	}
	return out, nil
}

// protoProtocol maps string protocol names to proto enum values.
func protoProtocol(s string) qfv1.Protocol {
	switch strings.ToLower(s) {
	case "tcp":
		return qfv1.Protocol_PROTOCOL_TCP
	case "udp":
		return qfv1.Protocol_PROTOCOL_UDP
	case "icmp":
		return qfv1.Protocol_PROTOCOL_ICMP
	case "icmpv6":
		return qfv1.Protocol_PROTOCOL_ICMPV6
	default:
		return qfv1.Protocol_PROTOCOL_ANY
	}
}

// protoAction maps string action names to proto enum values.
func protoAction(s string) qfv1.Action {
	switch s {
	case "allow":
		return qfv1.Action_ACTION_ALLOW
	case "deny":
		return qfv1.Action_ACTION_DENY
	case "log":
		return qfv1.Action_ACTION_LOG
	default:
		return qfv1.Action_ACTION_DENY
	}
}

// protoDirection maps string direction names to proto enum values.
func protoDirection(s string) qfv1.Direction {
	if s == "egress" {
		return qfv1.Direction_DIRECTION_EGRESS
	}
	return qfv1.Direction_DIRECTION_INGRESS
}

// protoState maps string conntrack state names to proto enum values.
func protoState(s string) qfv1.ConntrackState {
	switch s {
	case "new":
		return qfv1.ConntrackState_CONNTRACK_STATE_NEW
	case "established":
		return qfv1.ConntrackState_CONNTRACK_STATE_ESTABLISHED
	case "related":
		return qfv1.ConntrackState_CONNTRACK_STATE_RELATED
	case "invalid":
		return qfv1.ConntrackState_CONNTRACK_STATE_INVALID
	default:
		return qfv1.ConntrackState_CONNTRACK_STATE_NONE
	}
}
