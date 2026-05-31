package policy

import (
	"net"
	"strings"
	"testing"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/loader"
)

// ── helpers ──────────────────────────────────────────────────────────────────

func ipv4CIDR(addr string, ones uint32) *qfv1.CIDR {
	ip := net.ParseIP(addr).To4()
	return &qfv1.CIDR{Ip: ip, PrefixLen: ones}
}

func ipv6CIDR(addr string, ones uint32) *qfv1.CIDR {
	ip := net.ParseIP(addr).To16()
	return &qfv1.CIDR{Ip: ip, PrefixLen: ones}
}

func portRange(start, end uint32) *qfv1.PortRange {
	return &qfv1.PortRange{Start: start, End: end}
}

const testUUID = "550e8400-e29b-41d4-a716-446655440000"

func mustParseUUID(t *testing.T, s string) [16]byte {
	t.Helper()
	id, err := parseUUID(s)
	if err != nil {
		t.Fatalf("parseUUID(%q): %v", s, err)
	}
	return id
}

func basicRule() *qfv1.EffectiveRule {
	return &qfv1.EffectiveRule{
		RuleId:    testUUID,
		Direction: qfv1.Direction_DIRECTION_INGRESS,
		Action:    qfv1.Action_ACTION_ALLOW,
		Match:     &qfv1.RuleMatch{},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestCompileBundle_Nil(t *testing.T) {
	_, err := CompileBundle(nil)
	if err == nil {
		t.Fatal("want error for nil bundle")
	}
}

func TestCompileBundle_Empty(t *testing.T) {
	res, err := CompileBundle(&qfv1.PolicyBundle{})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 0 {
		t.Fatalf("want 0 rules, got %d", len(res.Rules))
	}
	if len(res.Warnings) != 0 {
		t.Fatalf("want no warnings, got %v", res.Warnings)
	}
	// default actions come from DefaultConfig
	if res.Config.DefaultIngress != loader.ActionAllow {
		t.Errorf("DefaultIngress: want %d, got %d", loader.ActionAllow, res.Config.DefaultIngress)
	}
	if res.Config.DefaultEgress != loader.ActionAllow {
		t.Errorf("DefaultEgress: want %d, got %d", loader.ActionAllow, res.Config.DefaultEgress)
	}
}

func TestCompileBundle_Config(t *testing.T) {
	bundle := &qfv1.PolicyBundle{
		DefaultIngressAction: qfv1.Action_ACTION_DENY,
		DefaultEgressAction:  qfv1.Action_ACTION_ALLOW,
		RequiresConntrack:    true,
	}
	res, err := CompileBundle(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Config.DefaultIngress != loader.ActionDeny {
		t.Errorf("DefaultIngress: want %d, got %d", loader.ActionDeny, res.Config.DefaultIngress)
	}
	if res.Config.DefaultEgress != loader.ActionAllow {
		t.Errorf("DefaultEgress: want %d, got %d", loader.ActionAllow, res.Config.DefaultEgress)
	}
	if !res.Config.ConntrackEnabled {
		t.Error("ConntrackEnabled: want true")
	}
}

func TestCompileBundle_ConfigUnspecifiedKeepsDefault(t *testing.T) {
	// ACTION_UNSPECIFIED must not override DefaultConfig values.
	bundle := &qfv1.PolicyBundle{
		DefaultIngressAction: qfv1.Action_ACTION_UNSPECIFIED,
		DefaultEgressAction:  qfv1.Action_ACTION_UNSPECIFIED,
	}
	res, err := CompileBundle(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Config.DefaultIngress != loader.DefaultConfig.DefaultIngress {
		t.Errorf("DefaultIngress: want %d, got %d", loader.DefaultConfig.DefaultIngress, res.Config.DefaultIngress)
	}
	if res.Config.DefaultEgress != loader.DefaultConfig.DefaultEgress {
		t.Errorf("DefaultEgress: want %d, got %d", loader.DefaultConfig.DefaultEgress, res.Config.DefaultEgress)
	}
}

func TestCompileBundle_BasicRule(t *testing.T) {
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:             testUUID,
				Direction:          qfv1.Direction_DIRECTION_INGRESS,
				Action:             qfv1.Action_ACTION_DENY,
				LogEnabled:         true,
				LogRateLimitPerSec: 100,
				Match: &qfv1.RuleMatch{
					Protocol: qfv1.Protocol_PROTOCOL_TCP,
					SrcCidrs: []*qfv1.CIDR{ipv4CIDR("10.0.0.0", 8)},
					DstPorts: []*qfv1.PortRange{portRange(80, 443)},
				},
			},
		},
	}
	res, err := CompileBundle(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(res.Rules))
	}
	r := res.Rules[0]

	wantID := mustParseUUID(t, testUUID)
	if r.ID != wantID {
		t.Errorf("ID mismatch")
	}
	if r.Priority != 0 {
		t.Errorf("Priority: want 0, got %d", r.Priority)
	}
	if r.Action != loader.ActionDeny {
		t.Errorf("Action: want %d, got %d", loader.ActionDeny, r.Action)
	}
	if !r.LogEnabled {
		t.Error("LogEnabled: want true")
	}
	if r.LogRateLimit != 100 {
		t.Errorf("LogRateLimit: want 100, got %d", r.LogRateLimit)
	}
	if r.Match.Protocol != loader.ProtoTCP {
		t.Errorf("Protocol: want %d, got %d", loader.ProtoTCP, r.Match.Protocol)
	}
	if r.Match.Direction != loader.DirIngress {
		t.Errorf("Direction: want %d, got %d", loader.DirIngress, r.Match.Direction)
	}
	if len(r.Match.SrcCIDRs) != 1 {
		t.Fatalf("SrcCIDRs: want 1, got %d", len(r.Match.SrcCIDRs))
	}
	if r.Match.SrcCIDRs[0].Ones != 8 {
		t.Errorf("SrcCIDRs[0].Ones: want 8, got %d", r.Match.SrcCIDRs[0].Ones)
	}
	if len(r.Match.DstPorts) != 1 {
		t.Fatalf("DstPorts: want 1, got %d", len(r.Match.DstPorts))
	}
	if r.Match.DstPorts[0].Start != 80 || r.Match.DstPorts[0].End != 443 {
		t.Errorf("DstPorts[0]: want [80,443], got [%d,%d]", r.Match.DstPorts[0].Start, r.Match.DstPorts[0].End)
	}
}

func TestCompileBundle_PriorityOrder(t *testing.T) {
	rules := make([]*qfv1.EffectiveRule, 3)
	for i := range rules {
		r := basicRule()
		r.RuleId = testUUID[:35] + string(rune('0'+i))
		rules[i] = r
	}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: rules})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 3 {
		t.Fatalf("want 3 rules, got %d", len(res.Rules))
	}
	for i, r := range res.Rules {
		if r.Priority != int32(i) {
			t.Errorf("rule[%d].Priority: want %d, got %d", i, i, r.Priority)
		}
	}
}

func TestCompileBundle_IPv6AllSkipped(t *testing.T) {
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_ALLOW,
				Match: &qfv1.RuleMatch{
					SrcCidrs: []*qfv1.CIDR{
						ipv6CIDR("2001:db8::1", 128),
						ipv6CIDR("fe80::1", 64),
					},
				},
			},
		},
	}
	res, err := CompileBundle(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 0 {
		t.Fatalf("want rule skipped, got %d rules", len(res.Rules))
	}
	if len(res.Warnings) != 1 {
		t.Fatalf("want 1 warning, got %d: %v", len(res.Warnings), res.Warnings)
	}
	if !strings.Contains(res.Warnings[0], "IPv6") {
		t.Errorf("warning should mention IPv6: %q", res.Warnings[0])
	}
}

func TestCompileBundle_IPv4Mixed(t *testing.T) {
	// IPv4 + IPv6 in same CIDR list → only IPv4 kept, no skip.
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_ALLOW,
				Match: &qfv1.RuleMatch{
					SrcCidrs: []*qfv1.CIDR{
						ipv4CIDR("10.0.0.0", 8),
						ipv6CIDR("2001:db8::1", 128),
					},
				},
			},
		},
	}
	res, err := CompileBundle(bundle)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(res.Rules))
	}
	if len(res.Warnings) != 0 {
		t.Errorf("want no warnings, got %v", res.Warnings)
	}
	if len(res.Rules[0].Match.SrcCIDRs) != 1 {
		t.Errorf("SrcCIDRs: want 1 (only IPv4), got %d", len(res.Rules[0].Match.SrcCIDRs))
	}
}

func TestCompileBundle_ActionUnspecified(t *testing.T) {
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_UNSPECIFIED,
				Match:     &qfv1.RuleMatch{},
			},
		},
	}
	_, err := CompileBundle(bundle)
	if err == nil {
		t.Fatal("want error for unspecified action")
	}
}

func TestCompileBundle_DirectionUnspecified(t *testing.T) {
	bundle := &qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_UNSPECIFIED,
				Action:    qfv1.Action_ACTION_ALLOW,
				Match:     &qfv1.RuleMatch{},
			},
		},
	}
	_, err := CompileBundle(bundle)
	if err == nil {
		t.Fatal("want error for unspecified direction")
	}
}

func TestCompileBundle_ProtocolUnspecifiedBecomesAny(t *testing.T) {
	r := basicRule()
	r.Match = &qfv1.RuleMatch{Protocol: qfv1.Protocol_PROTOCOL_UNSPECIFIED}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Rules[0].Match.Protocol != loader.ProtoAny {
		t.Errorf("Protocol: want ProtoAny(%d), got %d", loader.ProtoAny, res.Rules[0].Match.Protocol)
	}
}

func TestCompileBundle_StateMapping(t *testing.T) {
	cases := []struct {
		proto   qfv1.ConntrackState
		want    uint8
		name    string
	}{
		{qfv1.ConntrackState_CONNTRACK_STATE_UNSPECIFIED, loader.CTNone, "unspecified→CTNone"},
		{qfv1.ConntrackState_CONNTRACK_STATE_NONE, loader.CTNone, "none→CTNone"},
		{qfv1.ConntrackState_CONNTRACK_STATE_NEW, loader.CTNew, "new→CTNew"},
		{qfv1.ConntrackState_CONNTRACK_STATE_ESTABLISHED, loader.CTEstablished, "established→CTEstablished"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := basicRule()
			r.State = tc.proto
			res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if res.Rules[0].Match.State != tc.want {
				t.Errorf("State: want %d, got %d", tc.want, res.Rules[0].Match.State)
			}
		})
	}
}

func TestCompileBundle_PortTooMany(t *testing.T) {
	ports := make([]*qfv1.PortRange, 9)
	for i := range ports {
		ports[i] = portRange(uint32(i*100), uint32(i*100+99))
	}
	r := basicRule()
	r.Match = &qfv1.RuleMatch{DstPorts: ports}
	_, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err == nil {
		t.Fatal("want error for 9 port ranges")
	}
}

func TestCompileBundle_PortStartGTEnd(t *testing.T) {
	r := basicRule()
	r.Match = &qfv1.RuleMatch{DstPorts: []*qfv1.PortRange{portRange(443, 80)}}
	_, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err == nil {
		t.Fatal("want error for start > end")
	}
}

func TestCompileBundle_PortOutOfRange(t *testing.T) {
	r := basicRule()
	r.Match = &qfv1.RuleMatch{DstPorts: []*qfv1.PortRange{portRange(0, 70000)}}
	_, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err == nil {
		t.Fatal("want error for port > 65535")
	}
}

func TestCompileBundle_InvalidUUID(t *testing.T) {
	r := basicRule()
	r.RuleId = "not-a-uuid"
	_, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err == nil {
		t.Fatal("want error for invalid UUID")
	}
}

func TestCompileBundle_NilRule(t *testing.T) {
	bundle := &qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{nil}}
	_, err := CompileBundle(bundle)
	if err == nil {
		t.Fatal("want error for nil rule")
	}
}

func TestCompileBundle_MultipleWarnings(t *testing.T) {
	// Two all-IPv6 rules → both skipped, both warned.
	mkIPv6Rule := func(id string) *qfv1.EffectiveRule {
		r := basicRule()
		r.RuleId = id
		r.Match = &qfv1.RuleMatch{
			SrcCidrs: []*qfv1.CIDR{ipv6CIDR("::1", 128)},
		}
		return r
	}
	id1 := "550e8400-e29b-41d4-a716-446655440001"
	id2 := "550e8400-e29b-41d4-a716-446655440002"
	res, err := CompileBundle(&qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{mkIPv6Rule(id1), mkIPv6Rule(id2)},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 0 {
		t.Fatalf("want 0 rules, got %d", len(res.Rules))
	}
	if len(res.Warnings) != 2 {
		t.Fatalf("want 2 warnings, got %d: %v", len(res.Warnings), res.Warnings)
	}
}

func TestCompileBundle_IPSetSpill_Src(t *testing.T) {
	// 9 src CIDRs > IPSetInlineMax(8) → spill to IPSet.
	cidrs := make([]*qfv1.CIDR, 9)
	for i := range cidrs {
		cidrs[i] = &qfv1.CIDR{Ip: net.IP{10, 0, byte(i), 0}, PrefixLen: 24}
	}
	r := basicRule()
	r.Match = &qfv1.RuleMatch{SrcCidrs: cidrs}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 1 {
		t.Fatalf("want 1 rule, got %d", len(res.Rules))
	}
	rule := res.Rules[0]
	if len(rule.Match.SrcCIDRs) != 0 {
		t.Errorf("SrcCIDRs should be empty after spill, got %d", len(rule.Match.SrcCIDRs))
	}
	if rule.Match.SrcIPSetID == 0 {
		t.Error("SrcIPSetID should be non-zero after spill")
	}
	if len(res.IPSets) == 0 {
		t.Fatal("want IPSets populated")
	}
	ipset := res.IPSets[rule.Match.SrcIPSetID]
	if len(ipset) != 9 {
		t.Errorf("IPSet size: want 9, got %d", len(ipset))
	}
}

func TestCompileBundle_IPSetSpill_Dst(t *testing.T) {
	cidrs := make([]*qfv1.CIDR, 10)
	for i := range cidrs {
		cidrs[i] = &qfv1.CIDR{Ip: net.IP{192, 168, byte(i), 0}, PrefixLen: 24}
	}
	r := basicRule()
	r.Match = &qfv1.RuleMatch{DstCidrs: cidrs}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rule := res.Rules[0]
	if rule.Match.DstIPSetID == 0 {
		t.Error("DstIPSetID should be non-zero after spill")
	}
	if len(rule.Match.DstCIDRs) != 0 {
		t.Error("DstCIDRs should be empty after spill")
	}
}

func TestCompileBundle_IPSetSpill_BothSides(t *testing.T) {
	mkCIDRs := func(n int, base byte) []*qfv1.CIDR {
		out := make([]*qfv1.CIDR, n)
		for i := range out {
			out[i] = &qfv1.CIDR{Ip: net.IP{10, base, byte(i), 0}, PrefixLen: 24}
		}
		return out
	}
	r := basicRule()
	r.Match = &qfv1.RuleMatch{SrcCidrs: mkCIDRs(9, 1), DstCidrs: mkCIDRs(9, 2)}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rule := res.Rules[0]
	if rule.Match.SrcIPSetID == 0 || rule.Match.DstIPSetID == 0 {
		t.Error("both sides should have IPSet IDs")
	}
	if rule.Match.SrcIPSetID == rule.Match.DstIPSetID {
		t.Error("src and dst IPSet IDs should be distinct")
	}
	if len(res.IPSets) != 2 {
		t.Errorf("want 2 IPSets, got %d", len(res.IPSets))
	}
}

func TestCompileBundle_IPSetNoSpill_ExactlyInline(t *testing.T) {
	// Exactly 8 CIDRs → inline, no IPSet.
	cidrs := make([]*qfv1.CIDR, 8)
	for i := range cidrs {
		cidrs[i] = &qfv1.CIDR{Ip: net.IP{10, 0, byte(i), 0}, PrefixLen: 24}
	}
	r := basicRule()
	r.Match = &qfv1.RuleMatch{SrcCidrs: cidrs}
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	rule := res.Rules[0]
	if rule.Match.SrcIPSetID != 0 {
		t.Error("SrcIPSetID should be 0 for ≤8 CIDRs")
	}
	if len(rule.Match.SrcCIDRs) != 8 {
		t.Errorf("SrcCIDRs: want 8, got %d", len(rule.Match.SrcCIDRs))
	}
	if len(res.IPSets) != 0 {
		t.Errorf("want no IPSets, got %d", len(res.IPSets))
	}
}

func TestCompileBundle_IPSetSpill_MultipleRules(t *testing.T) {
	// Two rules both spilling → each gets a distinct IPSet ID.
	mkRule := func(uuidSuffix string) *qfv1.EffectiveRule {
		cidrs := make([]*qfv1.CIDR, 9)
		for i := range cidrs {
			cidrs[i] = &qfv1.CIDR{Ip: net.IP{10, 0, byte(i), 0}, PrefixLen: 24}
		}
		r := basicRule()
		r.RuleId = "550e8400-e29b-41d4-a716-44665544" + uuidSuffix
		r.Match = &qfv1.RuleMatch{SrcCidrs: cidrs}
		return r
	}
	res, err := CompileBundle(&qfv1.PolicyBundle{
		Rules: []*qfv1.EffectiveRule{mkRule("0000"), mkRule("0001")},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(res.Rules) != 2 {
		t.Fatalf("want 2 rules, got %d", len(res.Rules))
	}
	id0 := res.Rules[0].Match.SrcIPSetID
	id1 := res.Rules[1].Match.SrcIPSetID
	if id0 == 0 || id1 == 0 {
		t.Error("both rules should have IPSet IDs")
	}
	if id0 == id1 {
		t.Error("rules should have distinct IPSet IDs")
	}
	if len(res.IPSets) != 2 {
		t.Errorf("want 2 IPSets, got %d", len(res.IPSets))
	}
}

func TestCompileBundle_EmptyCIDRMeansAny(t *testing.T) {
	r := basicRule()
	r.Match = &qfv1.RuleMatch{} // no src/dst CIDRs
	res, err := CompileBundle(&qfv1.PolicyBundle{Rules: []*qfv1.EffectiveRule{r}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Rules[0].Match.SrcCIDRs != nil {
		t.Error("empty src CIDRs should be nil (match-any)")
	}
	if res.Rules[0].Match.DstCIDRs != nil {
		t.Error("empty dst CIDRs should be nil (match-any)")
	}
}
