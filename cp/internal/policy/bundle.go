package policy

import (
	"context"
	"fmt"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/pki"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// BundleBuilder compiles and signs PolicyBundles.
// Implements agentsrv.BundleProvider.
type BundleBuilder struct {
	compiler *RulesetCompiler
	signer   *pki.BundleSigner
	queries  *storegen.Queries
}

// NewBundleBuilder creates a BundleBuilder.
func NewBundleBuilder(queries *storegen.Queries, signer *pki.BundleSigner) *BundleBuilder {
	return &BundleBuilder{
		compiler: NewRulesetCompiler(queries),
		signer:   signer,
		queries:  queries,
	}
}

// Build compiles the effective ruleset for hostID and returns a signed bundle
// with the given generation number.
func (b *BundleBuilder) Build(ctx context.Context, tenantID, hostID string, generation int64) (*qfv1.PolicyBundle, error) {
	rs, err := b.compiler.Compile(ctx, tenantID, hostID)
	if err != nil {
		return nil, fmt.Errorf("bundle: compile: %w", err)
	}

	bundle := buildProtoBundle(rs, generation)

	if b.signer != nil {
		if err := b.signer.SignBundle(bundle); err != nil {
			return nil, fmt.Errorf("bundle: sign: %w", err)
		}
	}
	return bundle, nil
}

// GetBundle implements agentsrv.BundleProvider.
// It looks up the host's server-side generation and builds a fresh bundle.
func (b *BundleBuilder) GetBundle(ctx context.Context, tenantID, hostID string) (*qfv1.PolicyBundle, error) {
	var tenantUUID, hostUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, fmt.Errorf("bundle: tenant uuid: %w", err)
	}
	if err := hostUUID.Scan(hostID); err != nil {
		return nil, fmt.Errorf("bundle: host uuid: %w", err)
	}
	host, err := b.queries.GetHost(ctx, storegen.GetHostParams{
		ID:       hostUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		return nil, fmt.Errorf("bundle: get host: %w", err)
	}
	// Use server generation + 1; the agent is behind so we push the next generation.
	gen := int64(host.CurrentGeneration) + 1
	return b.Build(ctx, tenantID, hostID, gen)
}

// buildProtoBundle converts an EffectiveRuleset into a PolicyBundle proto.
func buildProtoBundle(rs *EffectiveRuleset, generation int64) *qfv1.PolicyBundle {
	rules := make([]*qfv1.EffectiveRule, 0, len(rs.Rules))
	requiresCT := false

	for _, rr := range rs.Rules {
		state := protoState(nullableString(rr.Rule.State))
		if state != qfv1.ConntrackState_CONNTRACK_STATE_NONE &&
			state != qfv1.ConntrackState_CONNTRACK_STATE_UNSPECIFIED {
			requiresCT = true
		}

		action := protoAction(rr.Rule.Action)
		logEnabled := rr.Rule.Log
		// Auto-enable log for DENY (unless silent) and for LOG action.
		if action == qfv1.Action_ACTION_LOG ||
			(action == qfv1.Action_ACTION_DENY && !rr.Rule.Silent) {
			logEnabled = true
		}

		rules = append(rules, &qfv1.EffectiveRule{
			RuleId:         pgUUIDToStr(rr.Rule.ID),
			PolicyId:       pgUUIDToStr(rr.Policy.ID),
			RuleName:       rr.Rule.Name,
			PolicyName:     rr.Policy.Name,
			PolicyPriority: rr.Policy.Priority,
			RulePriority:   rr.Rule.Priority,
			Direction:      protoDirection(rr.Rule.Direction),
			Match: &qfv1.RuleMatch{
				Protocol:      protoProtocol(rr.Match.Protocol),
				SrcCidrs:      rr.SrcCIDRs,
				DstCidrs:      rr.DstCIDRs,
				SrcPorts:      rr.SrcPorts,
				DstPorts:      rr.DstPorts,
				TcpFlagsMatch: rr.Match.TCPFlagsMatch,
				TcpFlagsMask:  rr.Match.TCPFlagsMask,
			},
			State:      state,
			Action:     action,
			LogEnabled: logEnabled,
		})
	}

	return &qfv1.PolicyBundle{
		Generation:           generation,
		Rules:                rules,
		DefaultIngressAction: protoAction(rs.DefaultIngress),
		DefaultEgressAction:  protoAction(rs.DefaultEgress),
		RequiresConntrack:    requiresCT,
	}
}

// nullableString returns the value or "" if the pointer is nil.
func nullableString(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}
