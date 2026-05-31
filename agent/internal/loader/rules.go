package loader

import (
	"encoding/binary"
	"fmt"
	"net"
	"sort"
)

// BPF constant mirrors — keep in sync with agent/bpf/common.h.
const (
	ProtoAny    uint8 = 1
	ProtoTCP    uint8 = 2
	ProtoUDP    uint8 = 3
	ProtoICMP   uint8 = 4
	ProtoICMPv6 uint8 = 5

	ActionAllow uint8 = 1
	ActionDeny  uint8 = 2
	ActionLog   uint8 = 3

	DirIngress uint8 = 1
	DirEgress  uint8 = 2

	CTNone        uint8 = 1
	CTNew         uint8 = 2
	CTEstablished uint8 = 3
	CTRelated     uint8 = 4
	CTInvalid     uint8 = 5

	MaxRules     = 2048 // qf_rules / qf_rule_counters map capacity
	EvalMaxRules = 64   // max rules evaluated per packet (BPF verifier bound)
)

// CIDR4 is an IPv4 prefix (Phase 1; IPv6 not yet supported by rule engine).
type CIDR4 struct {
	IP   net.IP
	Ones int // prefix length 0–32
}

// ParseCIDR4 parses "a.b.c.d/prefix" notation into a CIDR4.
func ParseCIDR4(s string) (CIDR4, error) {
	_, ipNet, err := net.ParseCIDR(s)
	if err != nil {
		return CIDR4{}, err
	}
	if ipNet.IP.To4() == nil {
		return CIDR4{}, fmt.Errorf("IPv6 CIDR not supported in Phase 1: %s", s)
	}
	ones, _ := ipNet.Mask.Size()
	return CIDR4{IP: ipNet.IP, Ones: ones}, nil
}

// PortRange is an inclusive [Start, End] port range.
type PortRange struct {
	Start, End uint16
}

// Port returns a PortRange matching exactly one port.
func Port(p uint16) PortRange { return PortRange{p, p} }

// RuleMatch holds per-packet match criteria for a rule.
type RuleMatch struct {
	Protocol   uint8 // Proto* constant; ProtoAny matches all
	Direction  uint8 // DirIngress or DirEgress
	State      uint8 // CT* predicate; 0 is normalised to CTNone (stateless)
	SrcCIDRs   []CIDR4
	DstCIDRs   []CIDR4
	SrcPorts   []PortRange
	DstPorts   []PortRange
	SrcIPSetID uint32 // 0 = no IPSet; >0 = LPM trie id (spilled from inline)
	DstIPSetID uint32
}

// RuleSpec is the user-facing rule. Priority controls evaluation order
// (lower value = evaluated first). Rules with equal Priority are ordered
// by insertion position after sorting.
type RuleSpec struct {
	ID           [16]byte // UUID stored in ring-buffer events for correlation
	Priority     int32
	Action       uint8
	LogEnabled   bool
	LogRateLimit uint16
	Match        RuleMatch
}

// PushRules atomically replaces the active ruleset in the BPF maps.
// Rules are sorted by Priority before writing. The function validates all
// rules before touching any map — a single invalid rule aborts the whole push.
//
// Note: rules beyond EvalMaxRules index are written to the map but will not be
// evaluated per-packet (BPF verifier jump-complexity bound). Pass ≤ EvalMaxRules
// rules to guarantee all are reachable.
func PushRules(objs *TcFilterObjects, rules []RuleSpec) error {
	if len(rules) > MaxRules {
		return fmt.Errorf("rule count %d exceeds map capacity %d", len(rules), MaxRules)
	}

	sorted := make([]RuleSpec, len(rules))
	copy(sorted, rules)
	sort.SliceStable(sorted, func(i, j int) bool {
		return sorted[i].Priority < sorted[j].Priority
	})

	// Validate and convert all rules before writing any map entry.
	entries := make([]TcFilterRuleEntry, len(sorted))
	for i, r := range sorted {
		e, err := toRuleEntry(r)
		if err != nil {
			return fmt.Errorf("rule[%d] priority=%d: %w", i, r.Priority, err)
		}
		entries[i] = e
	}

	// Write rule entries first, then publish the new count.
	// BPF eval loop reads count as its upper bound, so new entries are
	// invisible until count is updated.
	for i, e := range entries {
		if err := objs.QfRules.Put(uint32(i), e); err != nil {
			return fmt.Errorf("write rule[%d]: %w", i, err)
		}
	}
	count := uint32(len(entries))
	if err := objs.QfRuleCount.Put(uint32(0), count); err != nil {
		return fmt.Errorf("write rule_count: %w", err)
	}
	return nil
}

// PushRules is the Loader convenience wrapper.
func (l *Loader) PushRules(rules []RuleSpec) error {
	return PushRules(&l.objs, rules)
}

// toRuleEntry converts a RuleSpec to the BPF wire representation.
func toRuleEntry(r RuleSpec) (TcFilterRuleEntry, error) {
	if err := validateRule(r); err != nil {
		return TcFilterRuleEntry{}, err
	}

	e := TcFilterRuleEntry{
		Action:             r.Action,
		LogRateLimitPerSec: r.LogRateLimit,
		RuleIdHi:           binary.BigEndian.Uint64(r.ID[:8]),
		RuleIdLo:           binary.BigEndian.Uint64(r.ID[8:]),
	}
	if r.LogEnabled {
		e.LogEnabled = 1
	}

	m := &e.Match
	m.Protocol  = r.Match.Protocol
	m.Direction = r.Match.Direction
	m.State     = r.Match.State
	if m.State == 0 {
		m.State = CTNone
	}

	for i, c := range r.Match.SrcCIDRs {
		enc, err := encodeCIDR4(c)
		if err != nil {
			return TcFilterRuleEntry{}, fmt.Errorf("src_cidr[%d]: %w", i, err)
		}
		m.SrcCidrs[i] = enc
	}
	m.N_srcCidrs = uint8(len(r.Match.SrcCIDRs))

	for i, c := range r.Match.DstCIDRs {
		enc, err := encodeCIDR4(c)
		if err != nil {
			return TcFilterRuleEntry{}, fmt.Errorf("dst_cidr[%d]: %w", i, err)
		}
		m.DstCidrs[i] = enc
	}
	m.N_dstCidrs = uint8(len(r.Match.DstCIDRs))

	for i, p := range r.Match.SrcPorts {
		m.SrcPorts[i] = struct{ Start, End uint16 }{p.Start, p.End}
	}
	m.N_srcPorts = uint8(len(r.Match.SrcPorts))

	for i, p := range r.Match.DstPorts {
		m.DstPorts[i] = struct{ Start, End uint16 }{p.Start, p.End}
	}
	m.N_dstPorts = uint8(len(r.Match.DstPorts))

	m.SrcIpsetId = r.Match.SrcIPSetID
	m.DstIpsetId = r.Match.DstIPSetID

	return e, nil
}

func validateRule(r RuleSpec) error {
	switch r.Action {
	case ActionAllow, ActionDeny, ActionLog:
	default:
		return fmt.Errorf("unknown action %d", r.Action)
	}
	switch r.Match.Protocol {
	case ProtoAny, ProtoTCP, ProtoUDP, ProtoICMP, ProtoICMPv6:
	default:
		return fmt.Errorf("unknown protocol %d", r.Match.Protocol)
	}
	switch r.Match.Direction {
	case DirIngress, DirEgress:
	default:
		return fmt.Errorf("unknown direction %d", r.Match.Direction)
	}
	if len(r.Match.SrcCIDRs) > IPSetInlineMax {
		return fmt.Errorf("too many src CIDRs: %d > %d (use IPSet for large lists)", len(r.Match.SrcCIDRs), IPSetInlineMax)
	}
	if len(r.Match.DstCIDRs) > IPSetInlineMax {
		return fmt.Errorf("too many dst CIDRs: %d > %d (use IPSet for large lists)", len(r.Match.DstCIDRs), IPSetInlineMax)
	}
	if len(r.Match.SrcPorts) > 8 {
		return fmt.Errorf("too many src port ranges: %d > 8", len(r.Match.SrcPorts))
	}
	if len(r.Match.DstPorts) > 8 {
		return fmt.Errorf("too many dst port ranges: %d > 8", len(r.Match.DstPorts))
	}
	for i, p := range r.Match.SrcPorts {
		if p.Start > p.End {
			return fmt.Errorf("src_port[%d]: start %d > end %d", i, p.Start, p.End)
		}
	}
	for i, p := range r.Match.DstPorts {
		if p.Start > p.End {
			return fmt.Errorf("dst_port[%d]: start %d > end %d", i, p.Start, p.End)
		}
	}
	return nil
}

// encodeCIDR4 converts a CIDR4 to the BPF __be32 map representation.
func encodeCIDR4(c CIDR4) (struct{ Addr, Mask uint32 }, error) {
	if c.IP.To4() == nil {
		return struct{ Addr, Mask uint32 }{}, fmt.Errorf("IPv6 not supported: %v", c.IP)
	}
	if c.Ones < 0 || c.Ones > 32 {
		return struct{ Addr, Mask uint32 }{}, fmt.Errorf("invalid prefix length %d", c.Ones)
	}
	mask := net.CIDRMask(c.Ones, 32)
	return struct{ Addr, Mask uint32 }{
		Addr: encodeIP4(c.IP),
		Mask: binary.LittleEndian.Uint32(mask),
	}, nil
}
