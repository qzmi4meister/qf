package handler

import (
	"errors"
	"net"
	"strings"
	"sync"
	"testing"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/loader"
)

// ── mock ──────────────────────────────────────────────────────────────────────

type mockApplier struct {
	mu         sync.Mutex
	configs    []loader.Config
	ruleSets   [][]loader.RuleSpec
	ipsets     map[uint32][]loader.CIDR4
	configErr  error
	pushErr    error
	ipsetErr   error
}

func (m *mockApplier) SetConfig(cfg loader.Config) error {
	if m.configErr != nil {
		return m.configErr
	}
	m.mu.Lock()
	m.configs = append(m.configs, cfg)
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) PushRules(rules []loader.RuleSpec) error {
	if m.pushErr != nil {
		return m.pushErr
	}
	cp := make([]loader.RuleSpec, len(rules))
	copy(cp, rules)
	m.mu.Lock()
	m.ruleSets = append(m.ruleSets, cp)
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) ClearIPSets() error {
	if m.ipsetErr != nil {
		return m.ipsetErr
	}
	m.mu.Lock()
	m.ipsets = make(map[uint32][]loader.CIDR4)
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) PushIPSet(id uint32, cidrs []loader.CIDR4) error {
	if m.ipsetErr != nil {
		return m.ipsetErr
	}
	m.mu.Lock()
	if m.ipsets == nil {
		m.ipsets = make(map[uint32][]loader.CIDR4)
	}
	m.ipsets[id] = cidrs
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) lastConfig() loader.Config {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.configs) == 0 {
		return loader.Config{}
	}
	return m.configs[len(m.configs)-1]
}

func (m *mockApplier) lastRules() []loader.RuleSpec {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.ruleSets) == 0 {
		return nil
	}
	return m.ruleSets[len(m.ruleSets)-1]
}

func (m *mockApplier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.ruleSets)
}

// ── helpers ──────────────────────────────────────────────────────────────────

const testUUID = "550e8400-e29b-41d4-a716-446655440000"

func validBundle(gen int64) *qfv1.PolicyBundle {
	return &qfv1.PolicyBundle{
		Generation:           gen,
		DefaultIngressAction: qfv1.Action_ACTION_ALLOW,
		DefaultEgressAction:  qfv1.Action_ACTION_DENY,
		RequiresConntrack:    true,
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_ALLOW,
				Match:     &qfv1.RuleMatch{},
			},
		},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestPolicyHandler_Apply_Success(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	ar, err := h.Apply(validBundle(42))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar.Generation != 42 {
		t.Errorf("Generation: want 42, got %d", ar.Generation)
	}
	if m.callCount() != 1 {
		t.Errorf("PushRules call count: want 1, got %d", m.callCount())
	}
	if len(m.lastRules()) != 1 {
		t.Errorf("rules pushed: want 1, got %d", len(m.lastRules()))
	}
}

func TestPolicyHandler_Apply_ConfigPushed(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	_, err := h.Apply(validBundle(1))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cfg := m.lastConfig()
	if !cfg.ConntrackEnabled {
		t.Error("ConntrackEnabled: want true")
	}
	if cfg.DefaultIngress != loader.ActionAllow {
		t.Errorf("DefaultIngress: want %d, got %d", loader.ActionAllow, cfg.DefaultIngress)
	}
	if cfg.DefaultEgress != loader.ActionDeny {
		t.Errorf("DefaultEgress: want %d, got %d", loader.ActionDeny, cfg.DefaultEgress)
	}
}

func TestPolicyHandler_Apply_UpdatesCurrent(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	if h.Current() != nil {
		t.Fatal("Current should be nil before any Apply")
	}
	_, err := h.Apply(validBundle(7))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	cur := h.Current()
	if cur == nil {
		t.Fatal("Current should not be nil after Apply")
	}
	if cur.Generation != 7 {
		t.Errorf("Current.Generation: want 7, got %d", cur.Generation)
	}
}

func TestPolicyHandler_Apply_CompileError(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	bad := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_UNSPECIFIED, // invalid
				Action:    qfv1.Action_ACTION_ALLOW,
				Match:     &qfv1.RuleMatch{},
			},
		},
	}
	_, err := h.Apply(bad)
	if err == nil {
		t.Fatal("want error for invalid bundle")
	}
	if m.callCount() != 0 {
		t.Error("PushRules must not be called on compile error")
	}
	if h.Current() != nil {
		t.Error("Current must stay nil on error")
	}
}

func TestPolicyHandler_Apply_SetConfigError(t *testing.T) {
	wantErr := errors.New("config write failed")
	m := &mockApplier{configErr: wantErr}
	h := NewPolicyHandler(m)

	_, err := h.Apply(validBundle(1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
	if m.callCount() != 0 {
		t.Error("PushRules must not be called after SetConfig error")
	}
}

func TestPolicyHandler_Apply_PushRulesError(t *testing.T) {
	wantErr := errors.New("bpf map full")
	m := &mockApplier{pushErr: wantErr}
	h := NewPolicyHandler(m)

	_, err := h.Apply(validBundle(1))
	if !errors.Is(err, wantErr) {
		t.Fatalf("want %v, got %v", wantErr, err)
	}
	if h.Current() != nil {
		t.Error("Current must not be updated on PushRules error")
	}
}

func TestPolicyHandler_Apply_IPv6Warning(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_DENY,
				Match: &qfv1.RuleMatch{
					SrcCidrs: []*qfv1.CIDR{
						{Ip: net.ParseIP("2001:db8::1").To16(), PrefixLen: 128},
					},
				},
			},
		},
	}
	ar, err := h.Apply(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(ar.Warnings) == 0 {
		t.Error("want warning for IPv6-only rule")
	}
	if len(m.lastRules()) != 0 {
		t.Error("IPv6-only rule must be skipped (not pushed)")
	}
}

func TestPolicyHandler_Apply_GenerationAdvances(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	for _, gen := range []int64{1, 2, 3} {
		ar, err := h.Apply(validBundle(gen))
		if err != nil {
			t.Fatalf("gen %d: %v", gen, err)
		}
		if ar.Generation != gen {
			t.Errorf("gen %d: got %d", gen, ar.Generation)
		}
	}
	if h.Current().Generation != 3 {
		t.Errorf("Current.Generation: want 3, got %d", h.Current().Generation)
	}
	if m.callCount() != 3 {
		t.Errorf("PushRules call count: want 3, got %d", m.callCount())
	}
}

func TestPolicyHandler_Apply_Concurrent(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	var wg sync.WaitGroup
	for i := int64(0); i < 10; i++ {
		wg.Add(1)
		go func(gen int64) {
			defer wg.Done()
			h.Apply(validBundle(gen)) //nolint:errcheck
		}(i)
	}
	wg.Wait()
	// No race; Current() returns some valid result.
	if h.Current() == nil {
		t.Error("Current should not be nil after concurrent applies")
	}
}

func TestPolicyHandler_Apply_IPSetSpill(t *testing.T) {
	m := &mockApplier{}
	h := NewPolicyHandler(m)

	// Build a bundle with 9 src CIDRs — should trigger IPSet spill.
	cidrs := make([]*qfv1.CIDR, 9)
	for i := range cidrs {
		cidrs[i] = &qfv1.CIDR{Ip: net.IP{10, 0, byte(i), 0}, PrefixLen: 24}
	}
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_DENY,
				Match:     &qfv1.RuleMatch{SrcCidrs: cidrs},
			},
		},
	}
	ar, err := h.Apply(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar == nil {
		t.Fatal("nil ApplyResult")
	}
	// ClearIPSets was called.
	if m.ipsets == nil {
		t.Error("ClearIPSets should have been called (ipsets map initialised)")
	}
	// Exactly one IPSet pushed.
	if len(m.ipsets) != 1 {
		t.Errorf("want 1 IPSet pushed, got %d", len(m.ipsets))
	}
	// The pushed IPSet has 9 CIDRs.
	for id, c := range m.ipsets {
		if len(c) != 9 {
			t.Errorf("ipset[%d]: want 9 CIDRs, got %d", id, len(c))
		}
	}
	// Rule references the IPSet, not inline CIDRs.
	if len(m.lastRules()) != 1 {
		t.Fatalf("want 1 rule pushed")
	}
	rule := m.lastRules()[0]
	if rule.Match.SrcIPSetID == 0 {
		t.Error("SrcIPSetID should be non-zero in pushed rule")
	}
	if len(rule.Match.SrcCIDRs) != 0 {
		t.Error("SrcCIDRs should be empty when IPSet is used")
	}
}

func TestPolicyHandler_Apply_IPSetClearError(t *testing.T) {
	clearErr := errors.New("clear failed")
	m := &mockApplier{ipsetErr: clearErr}
	h := NewPolicyHandler(m)

	_, err := h.Apply(validBundle(1))
	if !errors.Is(err, clearErr) {
		t.Fatalf("want clearErr, got %v", err)
	}
}

func TestMakeBundleApplied_Success(t *testing.T) {
	ar := &ApplyResult{Generation: 5, DurationMs: 12}
	msg := MakeBundleApplied(5, ar, nil)
	if !msg.Success {
		t.Error("want Success=true")
	}
	if msg.Generation != 5 {
		t.Errorf("Generation: want 5, got %d", msg.Generation)
	}
	if msg.DurationMs != 12 {
		t.Errorf("DurationMs: want 12, got %d", msg.DurationMs)
	}
	if msg.ErrorMessage != "" {
		t.Errorf("ErrorMessage: want empty, got %q", msg.ErrorMessage)
	}
}

func TestMakeBundleApplied_Failure(t *testing.T) {
	applyErr := errors.New("compile failed: bad uuid")
	msg := MakeBundleApplied(3, nil, applyErr)
	if msg.Success {
		t.Error("want Success=false")
	}
	if msg.Generation != 3 {
		t.Errorf("Generation: want 3, got %d", msg.Generation)
	}
	if !strings.Contains(msg.ErrorMessage, "bad uuid") {
		t.Errorf("ErrorMessage: want error text, got %q", msg.ErrorMessage)
	}
}
