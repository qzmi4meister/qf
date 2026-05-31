// Package policy compiles proto PolicyBundle messages into BPF-ready rule sets.
package policy

import (
	"encoding/hex"
	"fmt"
	"net"
	"strings"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/loader"
)

// CompileResult holds BPF-ready rules and config derived from a PolicyBundle.
type CompileResult struct {
	Rules  []loader.RuleSpec
	Config loader.Config
	// IPSets maps IPSet id → CIDR list for sets spilled from inline rule_match.
	// id is assigned sequentially starting from 1 per bundle compilation.
	IPSets map[uint32][]loader.CIDR4
	// Warnings lists rules skipped due to Phase 1 limitations (e.g. IPv6-only CIDRs).
	Warnings []string
}

// compileState tracks IPSet allocation across a single CompileBundle call.
type compileState struct {
	nextID uint32
	ipsets map[uint32][]loader.CIDR4
}

func (s *compileState) allocIPSet(cidrs []loader.CIDR4) uint32 {
	s.nextID++
	if s.ipsets == nil {
		s.ipsets = make(map[uint32][]loader.CIDR4)
	}
	s.ipsets[s.nextID] = cidrs
	return s.nextID
}

// CompileBundle converts a PolicyBundle received from the control-plane into
// BPF-ready rules and agent config.
//
// Rules are emitted in bundle order (CP has already sorted them); each rule
// receives a priority equal to its array index so PushRules preserves CP ordering.
//
// IPv6 CIDRs are silently dropped (Phase 1: BPF rule engine is IPv4-only). If
// all CIDRs on one side of a rule become empty after filtering, the rule is
// skipped and a warning is added to CompileResult.Warnings.
//
// When a CIDR list exceeds loader.IPSetInlineMax entries, the CIDRs are spilled
// into CompileResult.IPSets (keyed by a freshly allocated id) and the rule
// references that id instead of inline CIDRs.
func CompileBundle(bundle *qfv1.PolicyBundle) (CompileResult, error) {
	if bundle == nil {
		return CompileResult{}, fmt.Errorf("nil bundle")
	}

	res := CompileResult{
		Config: loader.DefaultConfig,
	}

	// Config from bundle defaults.
	if a := bundle.GetDefaultIngressAction(); a != qfv1.Action_ACTION_UNSPECIFIED {
		res.Config.DefaultIngress = uint8(a)
	}
	if a := bundle.GetDefaultEgressAction(); a != qfv1.Action_ACTION_UNSPECIFIED {
		res.Config.DefaultEgress = uint8(a)
	}
	res.Config.ConntrackEnabled = bundle.GetRequiresConntrack()

	state := &compileState{}

	// Compile rules in bundle order.
	for i, er := range bundle.GetRules() {
		spec, warn, err := compileRule(er, int32(i), state)
		if err != nil {
			return CompileResult{}, fmt.Errorf("rule[%d] %q: %w", i, er.GetRuleId(), err)
		}
		if warn != "" {
			res.Warnings = append(res.Warnings,
				fmt.Sprintf("rule[%d] %q: %s (skipped)", i, er.GetRuleId(), warn))
			continue
		}
		res.Rules = append(res.Rules, spec)
	}
	res.IPSets = state.ipsets
	return res, nil
}

func compileRule(er *qfv1.EffectiveRule, priority int32, state *compileState) (loader.RuleSpec, string, error) {
	if er == nil {
		return loader.RuleSpec{}, "", fmt.Errorf("nil EffectiveRule")
	}

	action := uint8(er.GetAction())
	if action == 0 {
		return loader.RuleSpec{}, "", fmt.Errorf("action unspecified")
	}
	direction := uint8(er.GetDirection())
	if direction == 0 {
		return loader.RuleSpec{}, "", fmt.Errorf("direction unspecified")
	}

	protocol := uint8(er.GetMatch().GetProtocol())
	if protocol == 0 {
		protocol = loader.ProtoAny
	}
	cs := uint8(er.GetState())
	if cs == 0 {
		cs = loader.CTNone
	}

	id, err := parseUUID(er.GetRuleId())
	if err != nil {
		return loader.RuleSpec{}, "", fmt.Errorf("parse rule_id: %w", err)
	}

	srcCIDRs, srcIPSetID, warn := compileCIDRs(er.GetMatch().GetSrcCidrs(), "src_cidrs", state)
	if warn != "" {
		return loader.RuleSpec{}, warn, nil
	}
	dstCIDRs, dstIPSetID, warn := compileCIDRs(er.GetMatch().GetDstCidrs(), "dst_cidrs", state)
	if warn != "" {
		return loader.RuleSpec{}, warn, nil
	}

	srcPorts, err := compilePorts(er.GetMatch().GetSrcPorts())
	if err != nil {
		return loader.RuleSpec{}, "", fmt.Errorf("src_ports: %w", err)
	}
	dstPorts, err := compilePorts(er.GetMatch().GetDstPorts())
	if err != nil {
		return loader.RuleSpec{}, "", fmt.Errorf("dst_ports: %w", err)
	}

	spec := loader.RuleSpec{
		ID:           id,
		Priority:     priority,
		Action:       action,
		LogEnabled:   er.GetLogEnabled(),
		LogRateLimit: uint16(er.GetLogRateLimitPerSec()),
		Match: loader.RuleMatch{
			Protocol:   protocol,
			Direction:  direction,
			State:      cs,
			SrcCIDRs:   srcCIDRs,
			DstCIDRs:   dstCIDRs,
			SrcPorts:   srcPorts,
			DstPorts:   dstPorts,
			SrcIPSetID: srcIPSetID,
			DstIPSetID: dstIPSetID,
		},
	}
	return spec, "", nil
}

// compileCIDRs filters IPv6 prefixes and handles IPSet spill.
//
//   - Empty input → (nil, 0, "") — match any.
//   - All-IPv6 input → ("", 0, warning) — caller skips rule.
//   - ≤ IPSetInlineMax IPv4 CIDRs → (cidrs, 0, "") — inline.
//   - > IPSetInlineMax IPv4 CIDRs → (nil, allocatedID, "") — spill to IPSet.
func compileCIDRs(cidrs []*qfv1.CIDR, field string, state *compileState) ([]loader.CIDR4, uint32, string) {
	if len(cidrs) == 0 {
		return nil, 0, "" // empty = match any
	}
	var out []loader.CIDR4
	for _, c := range cidrs {
		ip := net.IP(c.GetIp())
		if ip.To4() == nil {
			continue // IPv6 — skip for Phase 1
		}
		ones := int(c.GetPrefixLen())
		_, ipNet, err := net.ParseCIDR(fmt.Sprintf("%s/%d", ip.String(), ones))
		if err != nil {
			continue // malformed CIDR — skip
		}
		o, _ := ipNet.Mask.Size()
		out = append(out, loader.CIDR4{IP: ipNet.IP, Ones: o})
	}
	if len(out) == 0 {
		return nil, 0, fmt.Sprintf("%s: all %d CIDR(s) are IPv6 — not enforceable in Phase 1", field, len(cidrs))
	}
	if len(out) > loader.IPSetInlineMax {
		return nil, state.allocIPSet(out), ""
	}
	return out, 0, ""
}

func compilePorts(ports []*qfv1.PortRange) ([]loader.PortRange, error) {
	if len(ports) > 8 {
		return nil, fmt.Errorf("too many port ranges: %d > 8", len(ports))
	}
	out := make([]loader.PortRange, 0, len(ports))
	for _, p := range ports {
		s, e := p.GetStart(), p.GetEnd()
		if s > 65535 || e > 65535 {
			return nil, fmt.Errorf("port out of range: [%d, %d]", s, e)
		}
		if s > e {
			return nil, fmt.Errorf("port range start %d > end %d", s, e)
		}
		out = append(out, loader.PortRange{Start: uint16(s), End: uint16(e)})
	}
	return out, nil
}

func parseUUID(s string) ([16]byte, error) {
	clean := strings.ReplaceAll(s, "-", "")
	b, err := hex.DecodeString(clean)
	if err != nil || len(b) != 16 {
		if err == nil {
			err = fmt.Errorf("expected 16 bytes, got %d", len(b))
		}
		return [16]byte{}, fmt.Errorf("invalid UUID %q: %w", s, err)
	}
	var id [16]byte
	copy(id[:], b)
	return id, nil
}
