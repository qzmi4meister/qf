// Package handler wires compiled policy bundles into the BPF datapath.
package handler

import (
	"fmt"
	"sync"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/loader"
	"github.com/qf/qf/agent/internal/policy"
)

// RuleApplier is the subset of loader.Loader used by PolicyHandler.
// Implemented by *loader.Loader in production; mockable in tests.
type RuleApplier interface {
	PushRules(rules []loader.RuleSpec) error
	SetConfig(cfg loader.Config) error
	ClearIPSets() error
	PushIPSet(id uint32, cidrs []loader.CIDR4) error
}

// ApplyResult holds the outcome of a successful Apply call.
type ApplyResult struct {
	Generation int64
	Warnings   []string
	DurationMs uint32
	AppliedAt  time.Time
}

// PolicyHandler applies PolicyBundle messages to the BPF datapath.
// Safe for concurrent use.
type PolicyHandler struct {
	mu      sync.RWMutex
	applier RuleApplier
	current *ApplyResult
}

// NewPolicyHandler creates a PolicyHandler backed by the given RuleApplier.
func NewPolicyHandler(a RuleApplier) *PolicyHandler {
	return &PolicyHandler{applier: a}
}

// Apply compiles bundle and atomically pushes rules + config into the BPF
// datapath. On success it updates the current result; on failure the previous
// generation remains active.
func (h *PolicyHandler) Apply(bundle *qfv1.PolicyBundle) (*ApplyResult, error) {
	start := time.Now()

	res, err := policy.CompileBundle(bundle)
	if err != nil {
		return nil, fmt.Errorf("compile bundle: %w", err)
	}
	if err := h.applier.ClearIPSets(); err != nil {
		return nil, fmt.Errorf("clear ipsets: %w", err)
	}
	for id, cidrs := range res.IPSets {
		if err := h.applier.PushIPSet(id, cidrs); err != nil {
			return nil, fmt.Errorf("push ipset %d: %w", id, err)
		}
	}
	if err := h.applier.SetConfig(res.Config); err != nil {
		return nil, fmt.Errorf("set config: %w", err)
	}
	if err := h.applier.PushRules(res.Rules); err != nil {
		return nil, fmt.Errorf("push rules: %w", err)
	}

	ar := &ApplyResult{
		Generation: bundle.GetGeneration(),
		Warnings:   res.Warnings,
		DurationMs: uint32(time.Since(start).Milliseconds()),
		AppliedAt:  time.Now(),
	}
	h.mu.Lock()
	h.current = ar
	h.mu.Unlock()
	return ar, nil
}

// Current returns the result of the most recent successful Apply, or nil if
// no bundle has been applied yet.
func (h *PolicyHandler) Current() *ApplyResult {
	h.mu.RLock()
	defer h.mu.RUnlock()
	return h.current
}

// MakeBundleApplied builds the BundleApplied proto message to send back to CP
// after an Apply call. Pass applyErr=nil when Apply succeeded.
func MakeBundleApplied(gen int64, ar *ApplyResult, applyErr error) *qfv1.BundleApplied {
	if applyErr != nil {
		return &qfv1.BundleApplied{
			Generation:   gen,
			Success:      false,
			ErrorMessage: applyErr.Error(),
		}
	}
	return &qfv1.BundleApplied{
		Generation: ar.Generation,
		Success:    true,
		DurationMs: ar.DurationMs,
	}
}
