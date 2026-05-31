package policy

import (
	"context"
	"fmt"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// BundleUpdate is a recompiled bundle ready to push to an agent stream.
type BundleUpdate struct {
	TenantID string
	HostID   string
	Bundle   *qfv1.PolicyBundle
}

// Dispatcher delivers recompiled bundles to active agent streams.
// Implemented by agentsrv (GRPC-05); stubbed until then.
type Dispatcher interface {
	Dispatch(update BundleUpdate)
}

// CascadeRecompiler handles policy change propagation.
type CascadeRecompiler struct {
	queries    *storegen.Queries
	builder    *BundleBuilder
	selector   *SelectorMatcher
	dispatcher Dispatcher
}

// NewCascadeRecompiler creates a CascadeRecompiler.
func NewCascadeRecompiler(queries *storegen.Queries, builder *BundleBuilder, dispatcher Dispatcher) *CascadeRecompiler {
	return &CascadeRecompiler{
		queries:    queries,
		builder:    builder,
		selector:   NewSelectorMatcher(queries),
		dispatcher: dispatcher,
	}
}

// OnObjectGroupChanged recompiles and dispatches bundles for all hosts
// whose effective ruleset references ogID.
func (cr *CascadeRecompiler) OnObjectGroupChanged(ctx context.Context, tenantID, ogID string) error {
	hostIDs, err := cr.affectedHosts(ctx, tenantID, ogID)
	if err != nil {
		return fmt.Errorf("cascade: find affected hosts: %w", err)
	}
	if len(hostIDs) == 0 {
		return nil
	}
	slog.Info("cascade: recompiling", "og_id", ogID, "hosts", len(hostIDs))
	return cr.recompileAndDispatch(ctx, tenantID, hostIDs)
}

// OnPolicyChanged recompiles bundles for all hosts matching policy's selector.
func (cr *CascadeRecompiler) OnPolicyChanged(ctx context.Context, tenantID, policyID string) error {
	var tenantUUID, policyUUID pgtype.UUID
	if err := tenantUUID.Scan(tenantID); err != nil {
		return err
	}
	if err := policyUUID.Scan(policyID); err != nil {
		return err
	}
	policy, err := cr.queries.GetPolicy(ctx, storegen.GetPolicyParams{
		ID: policyUUID, TenantID: tenantUUID,
	})
	if err != nil {
		return fmt.Errorf("cascade: get policy: %w", err)
	}
	sel, err := ParseSelector(policy.Selector)
	if err != nil {
		return fmt.Errorf("cascade: parse selector: %w", err)
	}
	hostIDs, err := cr.selector.ResolveHostIDs(ctx, tenantID, sel)
	if err != nil {
		return fmt.Errorf("cascade: resolve hosts: %w", err)
	}
	slog.Info("cascade: policy changed", "policy_id", policyID, "hosts", len(hostIDs))
	return cr.recompileAndDispatch(ctx, tenantID, hostIDs)
}

// affectedHosts finds all hosts whose effective ruleset references ogID.
// Path: og → rule_objectgroup_refs → rules → policies → selector → hosts.
func (cr *CascadeRecompiler) affectedHosts(ctx context.Context, tenantID, ogID string) ([]string, error) {
	var ogUUID, tenantUUID pgtype.UUID
	if err := ogUUID.Scan(ogID); err != nil {
		return nil, err
	}
	if err := tenantUUID.Scan(tenantID); err != nil {
		return nil, err
	}

	ruleIDs, err := cr.queries.ListRuleIDsByObjectGroup(ctx, ogUUID)
	if err != nil {
		return nil, fmt.Errorf("list rules by og: %w", err)
	}

	// Collect unique policy IDs.
	policySet := make(map[string]struct{})
	for _, rid := range ruleIDs {
		rule, err := cr.queries.GetRule(ctx, rid)
		if err != nil {
			slog.Warn("cascade: get rule failed", "rule_id", pgUUIDToStr(rid), "err", err)
			continue
		}
		policySet[pgUUIDToStr(rule.PolicyID)] = struct{}{}
	}

	// For each policy, resolve affected hosts.
	hostSet := make(map[string]struct{})
	for policyID := range policySet {
		var policyUUID pgtype.UUID
		if err := policyUUID.Scan(policyID); err != nil {
			continue
		}
		policy, err := cr.queries.GetPolicy(ctx, storegen.GetPolicyParams{
			ID: policyUUID, TenantID: tenantUUID,
		})
		if err != nil {
			slog.Warn("cascade: get policy failed", "policy_id", policyID, "err", err)
			continue
		}
		sel, err := ParseSelector(policy.Selector)
		if err != nil {
			continue
		}
		ids, err := cr.selector.ResolveHostIDs(ctx, tenantID, sel)
		if err != nil {
			continue
		}
		for _, id := range ids {
			hostSet[id] = struct{}{}
		}
	}

	hosts := make([]string, 0, len(hostSet))
	for id := range hostSet {
		hosts = append(hosts, id)
	}
	return hosts, nil
}

// recompileAndDispatch builds a new bundle for each host and dispatches it.
func (cr *CascadeRecompiler) recompileAndDispatch(ctx context.Context, tenantID string, hostIDs []string) error {
	var errs []error
	for _, hostID := range hostIDs {
		bundle, err := cr.builder.GetBundle(ctx, tenantID, hostID)
		if err != nil {
			slog.Error("cascade: build bundle failed", "host", hostID, "err", err)
			errs = append(errs, err)
			continue
		}
		if cr.dispatcher != nil {
			cr.dispatcher.Dispatch(BundleUpdate{
				TenantID: tenantID,
				HostID:   hostID,
				Bundle:   bundle,
			})
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("cascade: %d hosts failed recompile (first: %w)", len(errs), errs[0])
	}
	return nil
}
