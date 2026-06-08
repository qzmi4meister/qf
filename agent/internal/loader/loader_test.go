package loader

import (
	"encoding/binary"
	"errors"
	"net"
	"testing"
	"time"

	"github.com/cilium/ebpf/rlimit"
	"golang.org/x/sys/unix"
)

const (
	// aliases — rules.go exports the canonical names; keep short forms for
	// test table readability.
	protoAny  = ProtoAny
	protoTCP  = ProtoTCP
	protoUDP  = ProtoUDP

	actionAllow = ActionAllow
	actionDeny  = ActionDeny

	dirIngress = DirIngress
	dirEgress  = DirEgress

	ctNone        = CTNone
	ctNew         = CTNew
	ctEstablished = CTEstablished

	tcActOK   = 0
	tcActShot = 2
)

// ip4BPF encodes an IPv4 address for writing to a BPF __be32 map field.
// BPF map values are stored in host (little-endian) byte order on x86_64,
// so writing LittleEndian.Uint32(ip.To4()) produces the correct in-memory
// representation that the BPF __be32 arithmetic expects.
func ip4BPF(s string) uint32 {
	return binary.LittleEndian.Uint32(net.ParseIP(s).To4())
}

// mask4BPF returns a /prefix subnet mask for writing to a BPF __be32 field.
func mask4BPF(ones int) uint32 {
	return binary.LittleEndian.Uint32(net.CIDRMask(ones, 32))
}

// buildSYN builds a minimal TCP SYN frame (54 bytes).
func buildSYN(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := buildPkt(srcIP, dstIP, srcPort, dstPort)
	pkt[14+20+13] = 0x02 // TCP flags: SYN
	return pkt
}

// buildSYNACK builds a minimal TCP SYN-ACK frame.
func buildSYNACK(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := buildPkt(srcIP, dstIP, srcPort, dstPort)
	pkt[14+20+13] = 0x12 // TCP flags: SYN+ACK
	return pkt
}

// buildACK builds a minimal TCP ACK frame.
func buildACK(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := buildPkt(srcIP, dstIP, srcPort, dstPort)
	pkt[14+20+13] = 0x10 // TCP flags: ACK
	return pkt
}

// buildFIN builds a minimal TCP FIN-ACK frame.
func buildFIN(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := buildPkt(srcIP, dstIP, srcPort, dstPort)
	pkt[14+20+13] = 0x11 // TCP flags: FIN+ACK
	return pkt
}

// buildUDP builds a minimal Ethernet + IPv4 + UDP frame (42 bytes).
func buildUDP(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := make([]byte, 14+20+8)
	pkt[12] = 0x08; pkt[13] = 0x00 // EtherType = IPv4
	ip := pkt[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 28) // 20+8
	ip[8] = 64; ip[9] = 17 // TTL, protocol=UDP
	copy(ip[12:16], srcIP.To4())
	copy(ip[16:20], dstIP.To4())
	udp := ip[20:]
	binary.BigEndian.PutUint16(udp[0:2], srcPort)
	binary.BigEndian.PutUint16(udp[2:4], dstPort)
	binary.BigEndian.PutUint16(udp[4:6], 8) // length
	return pkt
}

// buildICMPEcho builds a minimal ICMP echo request (type=8) or reply (type=0)
// frame. id is stored in both src_port and dst_port per the CT key convention.
func buildICMPEcho(srcIP, dstIP net.IP, id uint16, isReply bool) []byte {
	pkt := make([]byte, 14+20+8)
	pkt[12] = 0x08; pkt[13] = 0x00
	ip := pkt[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 28)
	ip[8] = 64; ip[9] = 1 // TTL, protocol=ICMP
	copy(ip[12:16], srcIP.To4())
	copy(ip[16:20], dstIP.To4())
	icmp := ip[20:]
	if isReply {
		icmp[0] = 0 // ICMP_ECHOREPLY
	} else {
		icmp[0] = 8 // ICMP_ECHO
	}
	// id at bytes 4-5
	binary.BigEndian.PutUint16(icmp[4:6], id)
	return pkt
}

// buildPkt builds a minimal Ethernet + IPv4 + TCP frame (54 bytes).
func buildPkt(srcIP, dstIP net.IP, srcPort, dstPort uint16) []byte {
	pkt := make([]byte, 14+20+20)

	// Ethernet: EtherType = IPv4
	pkt[12] = 0x08
	pkt[13] = 0x00

	// IPv4
	ip := pkt[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 40)
	ip[8] = 64   // TTL
	ip[9] = 6    // protocol = TCP
	copy(ip[12:16], srcIP.To4())
	copy(ip[16:20], dstIP.To4())

	// TCP
	tcp := ip[20:]
	binary.BigEndian.PutUint16(tcp[0:2], srcPort)
	binary.BigEndian.PutUint16(tcp[2:4], dstPort)
	tcp[12] = 0x50 // data offset = 5

	return pkt
}

func loadForTest(t testing.TB) BpfObjects {
	t.Helper()
	if err := rlimit.RemoveMemlock(); err != nil {
		t.Logf("remove memlock (non-fatal): %v", err)
	}
	var raw TcFilterObjects
	if err := LoadTcFilterObjects(&raw, nil); err != nil {
		t.Fatalf("LoadTcFilterObjects: %v", err)
	}
	objs := bpfObjectsFromFull(&raw)
	t.Cleanup(func() { objs.Close() })
	return objs
}

func runIngress(t testing.TB, objs BpfObjects, src, dst string, sp, dp uint16) uint32 {
	t.Helper()
	pkt := buildPkt(net.ParseIP(src), net.ParseIP(dst), sp, dp)
	ret, _, err := objs.QfTcIngress.Test(pkt)
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		t.Fatalf("prog test run: %v", err)
	}
	return ret
}

func runEgress(t testing.TB, objs BpfObjects, src, dst string, sp, dp uint16) uint32 {
	t.Helper()
	pkt := buildPkt(net.ParseIP(src), net.ParseIP(dst), sp, dp)
	ret, _, err := objs.QfTcEgress.Test(pkt)
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		t.Fatalf("prog test run: %v", err)
	}
	return ret
}

// putRule writes a rule entry and updates qf_rule_count.
func putRule(t testing.TB, objs BpfObjects, idx uint32, entry TcFilterRuleEntry) {
	t.Helper()
	if err := objs.QfRules.Put(idx, entry); err != nil {
		t.Fatalf("put rule[%d]: %v", idx, err)
	}
	count := idx + 1
	if err := objs.QfRuleCount.Put(uint32(0), count); err != nil {
		t.Fatalf("put rule_count: %v", err)
	}
}

// putConfig writes a value to qf_config[slot].
func putConfig(t testing.TB, objs BpfObjects, slot uint32, val uint32) {
	t.Helper()
	if err := objs.QfConfig.Put(slot, val); err != nil {
		t.Fatalf("put config[%d]: %v", slot, err)
	}
}

// seedCT inserts a conntrack entry directly into qf_conntrack.
// Canonical key: lower src_ip (as raw uint32) goes in SrcIp.
func seedCT(t testing.TB, objs BpfObjects,
	srcIP, dstIP string, srcPort, dstPort uint16, proto, state uint8) {
	t.Helper()
	s, d := ip4BPF(srcIP), ip4BPF(dstIP)
	key := TcFilterCtKey{Proto: proto}
	if s <= d {
		key.SrcIp, key.DstIp = s, d
		key.SrcPort, key.DstPort = srcPort, dstPort
	} else {
		key.SrcIp, key.DstIp = d, s
		key.SrcPort, key.DstPort = dstPort, srcPort
	}
	entry := TcFilterCtEntry{State: state}
	if err := objs.QfConntrack.Put(key, entry); err != nil {
		t.Fatalf("seed conntrack: %v", err)
	}
}

// ── Tests ────────────────────────────────────────────────────────────────

// Empty ruleset: default action is allow (qf_config slots are zero → fallback).
func TestNoRules_AllowAll(t *testing.T) {
	objs := loadForTest(t)

	for _, src := range []string{"192.168.1.1", "10.0.0.1", "8.8.8.8"} {
		if got := runIngress(t, objs, src, "10.0.0.2", 1234, 80); got != tcActOK {
			t.Errorf("src=%s: got %d want %d (allow)", src, got, tcActOK)
		}
	}
}

// Single deny rule matching by src CIDR /32.
func TestDenyRule_SrcCIDR(t *testing.T) {
	objs := loadForTest(t)

	denyIP := "192.168.1.1"
	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:   protoAny,
			Direction:  dirIngress,
			N_srcCidrs: 1,
			State:      ctNone,
			SrcCidrs: [8]struct{ Addr, Mask uint32 }{
				{Addr: ip4BPF(denyIP), Mask: mask4BPF(32)},
			},
		},
	}
	putRule(t, objs, 0, rule)

	// Denied source.
	if got := runIngress(t, objs, denyIP, "10.0.0.2", 1234, 80); got != tcActShot {
		t.Errorf("denied src: got verdict %d want %d", got, tcActShot)
	}
	// Other source — not matched, default allow.
	if got := runIngress(t, objs, "10.0.0.1", "10.0.0.2", 1234, 80); got != tcActOK {
		t.Errorf("other src: got verdict %d want %d", got, tcActOK)
	}
}

// Subnet deny: /24 covers range, outside passes.
func TestDenyRule_SubnetCIDR(t *testing.T) {
	objs := loadForTest(t)

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:   protoAny,
			Direction:  dirIngress,
			N_srcCidrs: 1,
			State:      ctNone,
			SrcCidrs: [8]struct{ Addr, Mask uint32 }{
				{Addr: ip4BPF("192.168.1.0"), Mask: mask4BPF(24)},
			},
		},
	}
	putRule(t, objs, 0, rule)

	hits := []string{"192.168.1.1", "192.168.1.254"}
	for _, src := range hits {
		if got := runIngress(t, objs, src, "10.0.0.2", 1234, 80); got != tcActShot {
			t.Errorf("in-subnet %s: got %d want shot", src, got)
		}
	}
	misses := []string{"192.168.2.1", "10.0.0.1"}
	for _, src := range misses {
		if got := runIngress(t, objs, src, "10.0.0.2", 1234, 80); got != tcActOK {
			t.Errorf("out-subnet %s: got %d want ok", src, got)
		}
	}
}

// Port range matching: deny dst port 80-443.
func TestDenyRule_DstPortRange(t *testing.T) {
	objs := loadForTest(t)

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:   protoTCP,
			Direction:  dirIngress,
			N_dstPorts: 1,
			State:      ctNone,
			DstPorts: [8]struct{ Start, End uint16 }{
				{Start: 80, End: 443},
			},
		},
	}
	putRule(t, objs, 0, rule)

	inRange := []uint16{80, 200, 443}
	for _, dp := range inRange {
		if got := runIngress(t, objs, "1.2.3.4", "10.0.0.2", 1234, dp); got != tcActShot {
			t.Errorf("dport=%d: got %d want shot", dp, got)
		}
	}
	outRange := []uint16{79, 444, 8080}
	for _, dp := range outRange {
		if got := runIngress(t, objs, "1.2.3.4", "10.0.0.2", 1234, dp); got != tcActOK {
			t.Errorf("dport=%d: got %d want ok", dp, got)
		}
	}
}

// First-match-wins: allow rule before deny wins.
func TestFirstMatchWins(t *testing.T) {
	objs := loadForTest(t)

	matchBlock := struct {
		Protocol   uint8
		Direction  uint8
		N_srcCidrs uint8
		N_dstCidrs uint8
		N_srcPorts uint8
		N_dstPorts uint8
		State      uint8
		Pad        uint8
		SrcCidrs   [8]struct{ Addr, Mask uint32 }
		DstCidrs   [8]struct{ Addr, Mask uint32 }
		SrcPorts   [8]struct{ Start, End uint16 }
		DstPorts   [8]struct{ Start, End uint16 }
		SrcIpsetId uint32
		DstIpsetId uint32
	}{
		Protocol:   protoAny,
		Direction:  dirIngress,
		N_srcCidrs: 1,
		State:      ctNone,
		SrcCidrs: [8]struct{ Addr, Mask uint32 }{
			{Addr: ip4BPF("192.168.1.1"), Mask: mask4BPF(32)},
		},
	}

	// Rule 0: allow 192.168.1.1
	allow := TcFilterRuleEntry{Action: actionAllow, Match: matchBlock}
	putRule(t, objs, 0, allow)

	// Rule 1: deny 192.168.1.1 (should never be reached)
	deny := TcFilterRuleEntry{Action: actionDeny, Match: matchBlock}
	putRule(t, objs, 1, deny)

	if got := runIngress(t, objs, "192.168.1.1", "10.0.0.2", 1234, 80); got != tcActOK {
		t.Errorf("first-match allow: got %d want ok", got)
	}
}

// Protocol filter: TCP rule does not match UDP.
func TestProtocolFilter(t *testing.T) {
	objs := loadForTest(t)

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:  protoTCP,
			Direction: dirIngress,
			State:     ctNone,
		},
	}
	putRule(t, objs, 0, rule)

	// TCP packet: denied
	tcpPkt := buildPkt(net.ParseIP("1.2.3.4"), net.ParseIP("5.6.7.8"), 100, 80)
	if ret, _, err := objs.QfTcIngress.Test(tcpPkt); err != nil {
		t.Fatalf("test run: %v", err)
	} else if ret != tcActShot {
		t.Errorf("TCP: got %d want shot", ret)
	}

	// UDP packet: passes (rule is TCP-only)
	udpPkt := make([]byte, 14+20+8)
	udpPkt[12] = 0x08 // IPv4
	ip := udpPkt[14:]
	ip[0] = 0x45
	binary.BigEndian.PutUint16(ip[2:4], 28)
	ip[8] = 64
	ip[9] = 17 // UDP
	copy(ip[12:16], net.ParseIP("1.2.3.4").To4())
	copy(ip[16:20], net.ParseIP("5.6.7.8").To4())
	binary.BigEndian.PutUint16(udpPkt[34:36], 100)
	binary.BigEndian.PutUint16(udpPkt[36:38], 53)
	binary.BigEndian.PutUint16(udpPkt[38:40], 8)
	if ret, _, err := objs.QfTcIngress.Test(udpPkt); err != nil {
		t.Fatalf("test run: %v", err)
	} else if ret != tcActOK {
		t.Errorf("UDP: got %d want ok", ret)
	}
}

// CT_NONE rule matches regardless of conntrack state (stateless pass-through).
func TestCTState_NoneMatchesUntracked(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:  protoTCP,
			Direction: dirIngress,
			State:     ctNone, // stateless — no CT entry required
		},
	}
	putRule(t, objs, 0, rule)

	// No CT entry; CT_NONE rule still matches → DENY.
	if got := runIngress(t, objs, "10.0.0.1", "10.0.0.2", 1234, 80); got != tcActShot {
		t.Errorf("CT_NONE should match untracked packet: got %d want shot", got)
	}
}

// CT_NEW rule must not match an untracked packet (ct_state == 0).
func TestCTState_NewNoMatchUntracked(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:  protoAny,
			Direction: dirIngress,
			State:     ctNew, // requires CT_NEW
		},
	}
	putRule(t, objs, 0, rule)

	// No CT entry → ct_state == 0 → stateful rule does not match → default ALLOW.
	if got := runIngress(t, objs, "10.0.0.1", "10.0.0.2", 1234, 80); got != tcActOK {
		t.Errorf("CT_NEW should not match untracked packet: got %d want ok", got)
	}
}

// CT_ESTABLISHED rule matches a pre-seeded ESTABLISHED entry and passes
// flows with no CT entry.
func TestCTState_EstablishedMatchesSeeded(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled

	// Pre-seed conntrack: 10.0.0.1:1234 → 10.0.0.2:80 TCP ESTABLISHED.
	seedCT(t, objs, "10.0.0.1", "10.0.0.2", 1234, 80, protoTCP, ctEstablished)

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:  protoTCP,
			Direction: dirIngress,
			State:     ctEstablished,
		},
	}
	putRule(t, objs, 0, rule)

	// Tracked ESTABLISHED flow → rule matches → DENY.
	if got := runIngress(t, objs, "10.0.0.1", "10.0.0.2", 1234, 80); got != tcActShot {
		t.Errorf("ESTABLISHED flow: got %d want shot", got)
	}
	// Different flow, no CT entry → rule does not match → default ALLOW.
	if got := runIngress(t, objs, "10.0.0.3", "10.0.0.2", 5555, 80); got != tcActOK {
		t.Errorf("untracked flow: got %d want ok", got)
	}
}

// PushRules: deny a /24 via the high-level API; replace with allow; verify both.
func TestPushRules_ReplaceDenyWithAllow(t *testing.T) {
	objs := loadForTest(t)

	subnet, err := ParseCIDR4("192.168.10.0/24")
	if err != nil {
		t.Fatal(err)
	}

	deny := []RuleSpec{{
		Priority: 0,
		Action:   ActionDeny,
		Match: RuleMatch{
			Protocol:  ProtoAny,
			Direction: DirIngress,
			SrcCIDRs:  []CIDR4{subnet},
		},
	}}
	if err := PushRules(&objs, deny); err != nil {
		t.Fatalf("PushRules(deny): %v", err)
	}

	// In-subnet packet → DENY.
	if got := runIngress(t, objs, "192.168.10.5", "10.0.0.1", 1234, 80); got != tcActShot {
		t.Errorf("after deny push: got %d want shot", got)
	}
	// Out-of-subnet → ALLOW.
	if got := runIngress(t, objs, "192.168.11.1", "10.0.0.1", 1234, 80); got != tcActOK {
		t.Errorf("out-of-subnet after deny push: got %d want ok", got)
	}

	// Replace ruleset with a single allow rule for the same subnet.
	allow := []RuleSpec{{
		Priority: 0,
		Action:   ActionAllow,
		Match: RuleMatch{
			Protocol:  ProtoAny,
			Direction: DirIngress,
			SrcCIDRs:  []CIDR4{subnet},
		},
	}}
	if err := PushRules(&objs, allow); err != nil {
		t.Fatalf("PushRules(allow): %v", err)
	}

	// Same packet → ALLOW after ruleset replacement.
	if got := runIngress(t, objs, "192.168.10.5", "10.0.0.1", 1234, 80); got != tcActOK {
		t.Errorf("after allow push: got %d want ok", got)
	}
}

// PushRules priority ordering: lower priority number wins.
func TestPushRules_PriorityOrder(t *testing.T) {
	objs := loadForTest(t)

	subnet, _ := ParseCIDR4("10.5.0.0/16")

	rules := []RuleSpec{
		{Priority: 10, Action: ActionDeny, Match: RuleMatch{Protocol: ProtoAny, Direction: DirIngress, SrcCIDRs: []CIDR4{subnet}}},
		{Priority: 5, Action: ActionAllow, Match: RuleMatch{Protocol: ProtoAny, Direction: DirIngress, SrcCIDRs: []CIDR4{subnet}}},
	}
	if err := PushRules(&objs, rules); err != nil {
		t.Fatalf("PushRules: %v", err)
	}

	// Priority 5 (allow) evaluated before priority 10 (deny) → ALLOW.
	if got := runIngress(t, objs, "10.5.1.1", "10.0.0.1", 1234, 80); got != tcActOK {
		t.Errorf("priority order: got %d want ok (allow wins)", got)
	}
}

// PushRules validation: bad action rejected before any map write.
func TestPushRules_Validation(t *testing.T) {
	objs := loadForTest(t)

	bad := []RuleSpec{{
		Priority: 0,
		Action:   99, // invalid
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress},
	}}
	if err := PushRules(&objs, bad); err == nil {
		t.Error("expected error for invalid action, got nil")
	}
}

// ConntrackDump: after a SYN packet the flow appears in the dump.
func TestConntrackDump_TrackFlow(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled; default ALLOW → ct_update runs

	src, dst := "10.2.1.1", "10.2.1.2"
	pkt := buildSYN(net.ParseIP(src), net.ParseIP(dst), 5000, 80)
	if ret, _, err := objs.QfTcIngress.Test(pkt); err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		t.Fatalf("test run: %v", err)
	} else if ret != tcActOK {
		t.Fatalf("SYN should pass (default allow): got %d", ret)
	}

	flows, err := ConntrackDump(&objs)
	if err != nil {
		t.Fatalf("ConntrackDump: %v", err)
	}
	if len(flows) == 0 {
		t.Fatal("expected at least one conntrack entry after SYN")
	}
	f := flows[0]
	if f.State != CTNew {
		t.Errorf("state: got %d want CT_NEW (%d)", f.State, CTNew)
	}
	if f.TCPState != TCPCSSynSent {
		t.Errorf("tcp_state: got %d want TCP_CS_SYN_SENT (%d)", f.TCPState, TCPCSSynSent)
	}
}

// ConntrackLookup: finds the entry by either src→dst or dst→src direction.
func TestConntrackLookup(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1)

	src, dst := "10.3.1.1", "10.3.1.2"
	pkt := buildSYN(net.ParseIP(src), net.ParseIP(dst), 6000, 443)
	if _, _, err := objs.QfTcIngress.Test(pkt); err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		t.Fatalf("test run: %v", err)
	}

	key := ConntrackKey{
		SrcIP:    net.ParseIP(src),
		DstIP:    net.ParseIP(dst),
		SrcPort:  6000,
		DstPort:  443,
		Protocol: ProtoTCP,
	}
	e, err := ConntrackLookup(&objs, key)
	if err != nil {
		t.Fatalf("ConntrackLookup: %v", err)
	}
	if e == nil {
		t.Fatal("expected entry, got nil")
	}
	if e.State != CTNew {
		t.Errorf("state: got %d want CT_NEW", e.State)
	}

	// Reverse direction lookup must return the same entry (canonical key).
	rev := ConntrackKey{
		SrcIP: net.ParseIP(dst), DstIP: net.ParseIP(src),
		SrcPort: 443, DstPort: 6000,
		Protocol: ProtoTCP,
	}
	eRev, err := ConntrackLookup(&objs, rev)
	if err != nil {
		t.Fatalf("reverse lookup: %v", err)
	}
	if eRev == nil {
		t.Fatal("reverse direction lookup returned nil — canonical key mismatch")
	}
}

// ConntrackLookup returns nil for an unknown flow (no error).
func TestConntrackLookup_NotFound(t *testing.T) {
	objs := loadForTest(t)

	e, err := ConntrackLookup(&objs, ConntrackKey{
		SrcIP:    net.ParseIP("1.2.3.4"),
		DstIP:    net.ParseIP("5.6.7.8"),
		SrcPort:  1234,
		DstPort:  80,
		Protocol: ProtoTCP,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if e != nil {
		t.Errorf("expected nil for unknown flow, got %+v", e)
	}
}

// SetConfig default-deny: packets with no matching rule are dropped.
func TestSetConfig_DefaultDeny(t *testing.T) {
	objs := loadForTest(t)

	// Baseline: BPF array zero → default_action() fallback = ActionAllow.
	if got := runIngress(t, objs, "1.2.3.4", "5.6.7.8", 100, 80); got != tcActOK {
		t.Fatalf("pre-config baseline: got %d want ok", got)
	}

	cfg := DefaultConfig
	cfg.DefaultIngress = ActionDeny
	if err := SetConfig(&objs, cfg); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}

	if got := runIngress(t, objs, "1.2.3.4", "5.6.7.8", 100, 80); got != tcActShot {
		t.Errorf("after default-deny: got %d want shot", got)
	}
}

// GetConfig round-trips all fields through the BPF map correctly.
func TestGetConfig_RoundTrip(t *testing.T) {
	objs := loadForTest(t)

	want := Config{
		ConntrackEnabled:  true,
		FlowEventsEnabled: false,
		DefaultIngress:    ActionDeny,
		DefaultEgress:     ActionAllow,
	}
	if err := SetConfig(&objs, want); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	got, err := GetConfig(&objs)
	if err != nil {
		t.Fatalf("GetConfig: %v", err)
	}
	if got != want {
		t.Errorf("got %+v\nwant %+v", got, want)
	}
}

// ReadCounter increments by the number of matching packets; non-matching leave it zero.
func TestReadCounter_IncrOnMatch(t *testing.T) {
	objs := loadForTest(t)

	if err := PushRules(&objs, []RuleSpec{{
		Priority: 0,
		Action:   ActionDeny,
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, DstPorts: []PortRange{Port(9090)}},
	}}); err != nil {
		t.Fatal(err)
	}

	const matchCount = 3
	for i := 0; i < matchCount; i++ {
		runIngress(t, objs, "1.2.3.4", "5.6.7.8", 1234, 9090) // matching
	}
	for i := 0; i < 2; i++ {
		runIngress(t, objs, "1.2.3.4", "5.6.7.8", 1234, 80) // non-matching
	}

	c, err := ReadCounter(&objs, 0)
	if err != nil {
		t.Fatalf("ReadCounter: %v", err)
	}
	if c.Packets != matchCount {
		t.Errorf("packets: got %d want %d", c.Packets, matchCount)
	}
	// IP tot_len = 40 (20 IP + 20 TCP); pkt_ctx.pkt_size set from bpf_ntohs(ip->tot_len).
	const pktSize = 40
	if c.Bytes != matchCount*pktSize {
		t.Errorf("bytes: got %d want %d", c.Bytes, matchCount*pktSize)
	}
}

// ReadCounters covers only active rules (indexed by PushRules position).
func TestReadCounters_MultiRule(t *testing.T) {
	objs := loadForTest(t)

	if err := PushRules(&objs, []RuleSpec{
		{Priority: 0, Action: ActionDeny, Match: RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, DstPorts: []PortRange{Port(111)}}},
		{Priority: 1, Action: ActionDeny, Match: RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, DstPorts: []PortRange{Port(222)}}},
	}); err != nil {
		t.Fatal(err)
	}

	runIngress(t, objs, "1.2.3.4", "5.6.7.8", 1234, 111) // hits rule[0]
	runIngress(t, objs, "1.2.3.4", "5.6.7.8", 1234, 111) // hits rule[0]
	runIngress(t, objs, "1.2.3.4", "5.6.7.8", 1234, 222) // hits rule[1]

	cs, err := ReadCounters(&objs)
	if err != nil {
		t.Fatalf("ReadCounters: %v", err)
	}
	if len(cs) != 2 {
		t.Fatalf("len: got %d want 2", len(cs))
	}
	if cs[0].Packets != 2 {
		t.Errorf("rule[0].packets: got %d want 2", cs[0].Packets)
	}
	if cs[1].Packets != 1 {
		t.Errorf("rule[1].packets: got %d want 1", cs[1].Packets)
	}
}

// EventReader: DENY action emits a log event with correct fields.
func TestEventReader_DenyEmitsEvent(t *testing.T) {
	objs := loadForTest(t)

	if err := PushRules(&objs, []RuleSpec{{
		Priority: 0,
		Action:   ActionDeny,
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, DstPorts: []PortRange{Port(7777)}},
	}}); err != nil {
		t.Fatal(err)
	}

	er, err := NewEventReader(&objs)
	if err != nil {
		t.Fatal(err)
	}
	defer er.Close()
	er.SetDeadline(time.Now().Add(2 * time.Second))

	if got := runIngress(t, objs, "10.1.1.1", "10.1.1.2", 4321, 7777); got != tcActShot {
		t.Fatalf("packet not denied: got %d", got)
	}

	ev, err := er.Read()
	if err != nil {
		t.Skipf("ring buffer event not delivered (kernel may not support PROG_TEST_RUN ringbuf): %v", err)
	}

	if ev.Action != ActionDeny {
		t.Errorf("action: got %d want ActionDeny (%d)", ev.Action, ActionDeny)
	}
	if ev.Proto != ProtoTCP {
		t.Errorf("proto: got %d want ProtoTCP (%d)", ev.Proto, ProtoTCP)
	}
	if ev.DstPort != 7777 {
		t.Errorf("dst_port: got %d want 7777", ev.DstPort)
	}
	if ev.Direction != DirIngress {
		t.Errorf("direction: got %d want DirIngress (%d)", ev.Direction, DirIngress)
	}
	if ev.SrcIP.String() != "10.1.1.1" {
		t.Errorf("src_ip: got %s want 10.1.1.1", ev.SrcIP)
	}
	if ev.TimestampNs == 0 {
		t.Error("timestamp is zero")
	}
}

// TestIPSet_SrcMatch verifies that a rule using an IPSet (src_ipset_id) denies
// packets from addresses inside the set and allows those outside.
func TestIPSet_SrcMatch(t *testing.T) {
	objs := loadForTest(t)

	// Populate IPSet id=1 with 10.0.0.0/24.
	key := TcFilterIpsetLpmKey{
		Prefixlen: 32 + 24, // 32 (ipset_id exact) + 24 (prefix)
		IpsetId:   1,
		Addr:      ip4BPF("10.0.0.0"),
	}
	if err := objs.QfIpsets.Update(key, uint8(1), 0); err != nil {
		t.Fatalf("populate ipset: %v", err)
	}

	// Deny rule: src in ipset 1, any dst.
	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:   protoAny,
			Direction:  dirIngress,
			State:      ctNone,
			SrcIpsetId: 1,
		},
	}
	putRule(t, objs, 0, rule)

	// 10.0.0.5 is inside 10.0.0.0/24 → deny.
	if got := runIngress(t, objs, "10.0.0.5", "172.16.0.1", 1234, 80); got != tcActShot {
		t.Errorf("inside set: want TC_ACT_SHOT(%d), got %d", tcActShot, got)
	}
	// 192.168.1.1 is outside → allow (default action).
	if got := runIngress(t, objs, "192.168.1.1", "172.16.0.1", 1234, 80); got != tcActOK {
		t.Errorf("outside set: want TC_ACT_OK(%d), got %d", tcActOK, got)
	}
}

// TestIPSet_DstMatch verifies dst_ipset_id matching.
func TestIPSet_DstMatch(t *testing.T) {
	objs := loadForTest(t)

	// Populate IPSet id=2 with 203.0.113.0/24 (TEST-NET-3).
	key := TcFilterIpsetLpmKey{
		Prefixlen: 32 + 24,
		IpsetId:   2,
		Addr:      ip4BPF("203.0.113.0"),
	}
	if err := objs.QfIpsets.Update(key, uint8(1), 0); err != nil {
		t.Fatalf("populate ipset: %v", err)
	}

	rule := TcFilterRuleEntry{
		Action: actionDeny,
		Match: struct {
			Protocol   uint8
			Direction  uint8
			N_srcCidrs uint8
			N_dstCidrs uint8
			N_srcPorts uint8
			N_dstPorts uint8
			State      uint8
			Pad        uint8
			SrcCidrs   [8]struct{ Addr, Mask uint32 }
			DstCidrs   [8]struct{ Addr, Mask uint32 }
			SrcPorts   [8]struct{ Start, End uint16 }
			DstPorts   [8]struct{ Start, End uint16 }
			SrcIpsetId uint32
			DstIpsetId uint32
		}{
			Protocol:   protoAny,
			Direction:  dirIngress,
			State:      ctNone,
			DstIpsetId: 2,
		},
	}
	putRule(t, objs, 0, rule)

	// dst inside set → deny.
	if got := runIngress(t, objs, "10.0.0.1", "203.0.113.7", 5000, 80); got != tcActShot {
		t.Errorf("inside dst set: want TC_ACT_SHOT(%d), got %d", tcActShot, got)
	}
	// dst outside set → allow.
	if got := runIngress(t, objs, "10.0.0.1", "10.10.10.10", 5000, 80); got != tcActOK {
		t.Errorf("outside dst set: want TC_ACT_OK(%d), got %d", tcActOK, got)
	}
}

// TestTCPHandshake_FullFlow verifies that conntrack transitions correctly
// through all three steps of the TCP handshake:
//
//	SYN  (ingress) → CT_NEW  / TCP_CS_SYN_SENT
//	SYN-ACK (egress) → CT_NEW  / TCP_CS_SYN_RCVD
//	ACK  (ingress) → CT_ESTABLISHED / TCP_CS_ESTABLISHED
func TestTCPHandshake_FullFlow(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled; default allow → ct_update runs

	const (
		clientIP = "10.30.0.1"
		serverIP = "10.30.0.2"
		sp       = uint16(54321)
		dp       = uint16(80)
	)

	ctKey := ConntrackKey{
		SrcIP:    net.ParseIP(clientIP),
		DstIP:    net.ParseIP(serverIP),
		SrcPort:  sp,
		DstPort:  dp,
		Protocol: ProtoTCP,
	}

	skipIfNoCap := func(err error) {
		t.Helper()
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
	}

	lookupOrFail := func(step string) *ConntrackEntry {
		t.Helper()
		e, err := ConntrackLookup(&objs, ctKey)
		if err != nil {
			t.Fatalf("%s: ConntrackLookup: %v", step, err)
		}
		if e == nil {
			t.Fatalf("%s: no CT entry", step)
		}
		return e
	}

	// ── Step 1: SYN ────────────────────────────────────────────────────────
	{
		pkt := buildSYN(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
		ret, _, err := objs.QfTcIngress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("SYN ingress: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("SYN: want OK, got %d", ret)
		}
		e := lookupOrFail("SYN")
		if e.State != CTNew {
			t.Errorf("SYN: state got %d want CT_NEW(%d)", e.State, CTNew)
		}
		if e.TCPState != TCPCSSynSent {
			t.Errorf("SYN: tcp_state got %d want SYN_SENT(%d)", e.TCPState, TCPCSSynSent)
		}
	}

	// ── Step 2: SYN-ACK ────────────────────────────────────────────────────
	{
		pkt := buildSYNACK(net.ParseIP(serverIP), net.ParseIP(clientIP), dp, sp)
		ret, _, err := objs.QfTcEgress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("SYN-ACK egress: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("SYN-ACK: want OK, got %d", ret)
		}
		e := lookupOrFail("SYN-ACK")
		if e.State != CTNew {
			t.Errorf("SYN-ACK: state got %d want CT_NEW(%d)", e.State, CTNew)
		}
		if e.TCPState != TCPCSSynRcvd {
			t.Errorf("SYN-ACK: tcp_state got %d want SYN_RCVD(%d)", e.TCPState, TCPCSSynRcvd)
		}
	}

	// ── Step 3: ACK ────────────────────────────────────────────────────────
	{
		pkt := buildACK(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
		ret, _, err := objs.QfTcIngress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("ACK ingress: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("ACK: want OK, got %d", ret)
		}
		e := lookupOrFail("ACK")
		if e.State != CTEstablished {
			t.Errorf("ACK: state got %d want ESTABLISHED(%d)", e.State, CTEstablished)
		}
		if e.TCPState != TCPCSEstablished {
			t.Errorf("ACK: tcp_state got %d want TCP_CS_ESTABLISHED(%d)", e.TCPState, TCPCSEstablished)
		}
		if e.EstablishedNs == 0 {
			t.Error("ACK: EstablishedNs should be non-zero after handshake")
		}
	}
}

// TestTCPHandshake_DenyEstablished verifies that a CT_ESTABLISHED deny rule
// is evaluated against flows that completed the 3-way handshake via BPF (not
// via seedCT), and that unrelated fresh SYNs are not matched.
func TestTCPHandshake_DenyEstablished(t *testing.T) {
	const (
		clientIP = "10.31.0.1"
		serverIP = "10.31.0.2"
		sp       = uint16(44444)
		dp       = uint16(443)
	)

	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled + default allow

	skipIfNoCap := func(err error) {
		t.Helper()
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
	}

	// Complete 3WHS so flow reaches ESTABLISHED (no deny rules yet).
	for _, step := range []struct {
		label string
		pkt   []byte
		prog  interface{ Test([]byte) (uint32, []byte, error) }
	}{
		{"SYN", buildSYN(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp), objs.QfTcIngress},
		{"SYN-ACK", buildSYNACK(net.ParseIP(serverIP), net.ParseIP(clientIP), dp, sp), objs.QfTcEgress},
		{"ACK", buildACK(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp), objs.QfTcIngress},
	} {
		ret, _, err := step.prog.Test(step.pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("%s: %v", step.label, err)
		}
		if ret != tcActOK {
			t.Fatalf("%s: want OK, got %d", step.label, ret)
		}
	}

	// Push a deny rule for CT_ESTABLISHED ingress.
	if err := PushRules(&objs, []RuleSpec{{
		Priority: 0,
		Action:   ActionDeny,
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, State: CTEstablished},
	}}); err != nil {
		t.Fatalf("PushRules: %v", err)
	}

	// Data ACK on the established flow → ct_state=ESTABLISHED → rule matches → SHOT.
	pkt := buildACK(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
	ret, _, err := objs.QfTcIngress.Test(pkt)
	skipIfNoCap(err)
	if err != nil {
		t.Fatalf("data ACK: %v", err)
	}
	if ret != tcActShot {
		t.Errorf("data ACK on established flow: want SHOT (CT_ESTABLISHED deny), got %d", ret)
	}

	// Fresh SYN from a different src IP: ct_state=0 → rule no match → default allow → OK.
	synOther := buildSYN(net.ParseIP("10.31.0.99"), net.ParseIP(serverIP), 11111, dp)
	retOther, _, err := objs.QfTcIngress.Test(synOther)
	if err != nil {
		t.Fatalf("other SYN: %v", err)
	}
	if retOther != tcActOK {
		t.Errorf("fresh SYN (untracked): want OK, got %d", retOther)
	}
}

// TestTCPHandshake_DefaultDenyAllowEstablished verifies that with default
// ingress deny + allow-CT_ESTABLISHED rule:
//   - A bare SYN (no CT entry) is dropped.
//   - A packet on an ESTABLISHED flow (seeded directly) is allowed.
func TestTCPHandshake_DefaultDenyAllowEstablished(t *testing.T) {
	const (
		clientIP = "10.32.0.1"
		serverIP = "10.32.0.2"
		sp       = uint16(33333)
		dp       = uint16(8080)
	)

	objs := loadForTest(t)

	// ConntrackEnabled must be true so ct_lookup runs; DefaultIngress=Deny
	// ensures untracked SYNs are blocked.
	if err := SetConfig(&objs, Config{
		ConntrackEnabled: true,
		DefaultIngress:   ActionDeny,
		DefaultEgress:    ActionAllow,
	}); err != nil {
		t.Fatalf("SetConfig: %v", err)
	}
	if err := PushRules(&objs, []RuleSpec{{
		Priority: 0,
		Action:   ActionAllow,
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress, State: CTEstablished},
	}}); err != nil {
		t.Fatalf("PushRules: %v", err)
	}

	skipIfNoCap := func(err error) {
		t.Helper()
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
	}

	// Fresh SYN: no CT entry → ct_state=0 → CT_ESTABLISHED rule no match →
	// default deny → SHOT.
	syn := buildSYN(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
	ret, _, err := objs.QfTcIngress.Test(syn)
	skipIfNoCap(err)
	if err != nil {
		t.Fatalf("SYN: %v", err)
	}
	if ret != tcActShot {
		t.Errorf("fresh SYN: want SHOT (default deny, no CT entry), got %d", ret)
	}

	// Seed ESTABLISHED entry directly (simulates a flow that bypassed policy,
	// e.g., started before the firewall loaded).
	seedCT(t, objs, clientIP, serverIP, sp, dp, protoTCP, ctEstablished)

	// Data ACK → ct_state=ESTABLISHED → allow rule matches → OK.
	ack := buildACK(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
	ret2, _, err := objs.QfTcIngress.Test(ack)
	if err != nil {
		t.Fatalf("data ACK: %v", err)
	}
	if ret2 != tcActOK {
		t.Errorf("data ACK (established): want OK, got %d", ret2)
	}
}

// TestConntrack_DenyDoesNotWriteCT verifies that packets matched by a DENY
// rule do not create a conntrack entry (ct_update skipped when action==DENY).
func TestConntrack_DenyDoesNotWriteCT(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled

	// Deny all ingress TCP.
	if err := PushRules(&objs, []RuleSpec{{
		Priority: 0,
		Action:   ActionDeny,
		Match:    RuleMatch{Protocol: ProtoTCP, Direction: DirIngress},
	}}); err != nil {
		t.Fatalf("PushRules: %v", err)
	}

	pkt := buildSYN(net.ParseIP("10.50.0.1"), net.ParseIP("10.50.0.2"), 9000, 80)
	ret, _, err := objs.QfTcIngress.Test(pkt)
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		t.Fatalf("test run: %v", err)
	}
	if ret != tcActShot {
		t.Fatalf("want SHOT (deny rule), got %d", ret)
	}

	// CT must be empty — deny suppresses ct_update.
	flows, err := ConntrackDump(&objs)
	if err != nil {
		t.Fatalf("ConntrackDump: %v", err)
	}
	if len(flows) != 0 {
		t.Errorf("deny rule: want 0 CT entries, got %d", len(flows))
	}
}

// TestConntrack_UDPPseudoState verifies UDP conntrack pseudo-state:
// first packet creates CT_NEW; first reply promotes to CT_ESTABLISHED.
func TestConntrack_UDPPseudoState(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled + default allow

	const (
		clientIP = "10.51.0.1"
		serverIP = "10.51.0.2"
		sp       = uint16(12345)
		dp       = uint16(53)
	)
	ctKey := ConntrackKey{
		SrcIP: net.ParseIP(clientIP), DstIP: net.ParseIP(serverIP),
		SrcPort: sp, DstPort: dp,
		Protocol: ProtoUDP,
	}

	skipIfNoCap := func(err error) {
		t.Helper()
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
	}

	// Step 1: UDP request (client→server, ingress) → CT_NEW.
	{
		pkt := buildUDP(net.ParseIP(clientIP), net.ParseIP(serverIP), sp, dp)
		ret, _, err := objs.QfTcIngress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("UDP request: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("UDP request: want OK, got %d", ret)
		}
		e, err := ConntrackLookup(&objs, ctKey)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if e == nil {
			t.Fatal("UDP request: no CT entry created")
		}
		if e.State != CTNew {
			t.Errorf("UDP request: state got %d want CT_NEW(%d)", e.State, CTNew)
		}
	}

	// Step 2: UDP reply (server→client, egress) → CT_ESTABLISHED.
	{
		pkt := buildUDP(net.ParseIP(serverIP), net.ParseIP(clientIP), dp, sp)
		ret, _, err := objs.QfTcEgress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("UDP reply: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("UDP reply: want OK, got %d", ret)
		}
		e, err := ConntrackLookup(&objs, ctKey)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if e == nil {
			t.Fatal("UDP reply: CT entry vanished")
		}
		if e.State != CTEstablished {
			t.Errorf("UDP reply: state got %d want CT_ESTABLISHED(%d)", e.State, CTEstablished)
		}
	}
}

// TestConntrack_ICMPEchoReply verifies ICMP echo conntrack:
// echo request creates CT_NEW; echo reply promotes to CT_ESTABLISHED.
// The ICMP CT key uses echo identifier in both port fields so that the
// canonical key is symmetric across request and reply directions.
func TestConntrack_ICMPEchoReply(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 1) // conntrack_enabled + default allow

	const (
		clientIP = "10.52.0.1"
		serverIP = "10.52.0.2"
		echoID   = uint16(0x4242)
	)
	ctKey := ConntrackKey{
		SrcIP: net.ParseIP(clientIP), DstIP: net.ParseIP(serverIP),
		SrcPort: echoID, DstPort: echoID,
		Protocol: ProtoICMP,
	}

	skipIfNoCap := func(err error) {
		t.Helper()
		if errors.Is(err, unix.EPERM) {
			t.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
	}

	// Step 1: ICMP echo request (client→server, ingress) → CT_NEW.
	{
		pkt := buildICMPEcho(net.ParseIP(clientIP), net.ParseIP(serverIP), echoID, false)
		ret, _, err := objs.QfTcIngress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("ICMP echo request: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("ICMP echo request: want OK, got %d", ret)
		}
		e, err := ConntrackLookup(&objs, ctKey)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if e == nil {
			t.Fatal("ICMP echo request: no CT entry created")
		}
		if e.State != CTNew {
			t.Errorf("ICMP echo request: state got %d want CT_NEW(%d)", e.State, CTNew)
		}
	}

	// Step 2: ICMP echo reply (server→client, egress) → CT_ESTABLISHED.
	{
		pkt := buildICMPEcho(net.ParseIP(serverIP), net.ParseIP(clientIP), echoID, true)
		ret, _, err := objs.QfTcEgress.Test(pkt)
		skipIfNoCap(err)
		if err != nil {
			t.Fatalf("ICMP echo reply: %v", err)
		}
		if ret != tcActOK {
			t.Fatalf("ICMP echo reply: want OK, got %d", ret)
		}
		e, err := ConntrackLookup(&objs, ctKey)
		if err != nil {
			t.Fatalf("lookup: %v", err)
		}
		if e == nil {
			t.Fatal("ICMP echo reply: CT entry vanished")
		}
		if e.State != CTEstablished {
			t.Errorf("ICMP echo reply: state got %d want CT_ESTABLISHED(%d)", e.State, CTEstablished)
		}
	}
}

// TestIPSet_ClearAndPush verifies ClearIPSets + PushIPSet Go helpers.
func TestIPSet_ClearAndPush(t *testing.T) {
	objs := loadForTest(t)

	cidr, _ := ParseCIDR4("10.1.0.0/16")

	if err := PushIPSet(&objs, 1, []CIDR4{cidr}); err != nil {
		t.Fatalf("PushIPSet: %v", err)
	}

	// Verify entry present via lookup.
	lookupKey := TcFilterIpsetLpmKey{
		Prefixlen: 64,
		IpsetId:   1,
		Addr:      ip4BPF("10.1.2.3"),
	}
	var val uint8
	if err := objs.QfIpsets.Lookup(lookupKey, &val); err != nil {
		t.Fatalf("lookup after PushIPSet: %v", err)
	}

	if err := ClearIPSets(&objs); err != nil {
		t.Fatalf("ClearIPSets: %v", err)
	}

	// After clear, lookup should fail.
	if err := objs.QfIpsets.Lookup(lookupKey, &val); err == nil {
		t.Error("expected lookup to fail after ClearIPSets")
	}
}
