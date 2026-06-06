package loader

import (
	"errors"
	"net"
	"testing"

	"golang.org/x/sys/unix"
)

// ruleMatch is a type alias for the anonymous struct in TcFilterRuleEntry.Match.
type ruleMatch = struct {
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
}

// denyPortRule builds an ingress deny rule matching a single dst port.
func denyPortRule(port uint16) TcFilterRuleEntry {
	return TcFilterRuleEntry{
		Action: actionDeny,
		Match: ruleMatch{
			Protocol:   protoTCP,
			Direction:  dirIngress,
			N_dstPorts: 1,
			State:      ctNone,
			DstPorts:   [8]struct{ Start, End uint16 }{{Start: port, End: port}},
		},
	}
}

// seedLastMatchRules installs n rules where rule n-1 matches dst port 80.
// Rules 0..n-2 deny ports 1000..1000+n-2 (no match for test packet on port 80).
// Worst-case: full rule walk before hitting match.
func seedLastMatchRules(b *testing.B, objs TcFilterObjects, n int) {
	b.Helper()
	for i := 0; i < n-1; i++ {
		putRule(b, objs, uint32(i), denyPortRule(uint16(1000+i)))
	}
	putRule(b, objs, uint32(n-1), denyPortRule(80))
}

// seedNoMatchRules installs n rules, none matching port 80.
// Full rule walk with no early exit → default allow.
func seedNoMatchRules(b *testing.B, objs TcFilterObjects, n int) {
	b.Helper()
	for i := 0; i < n; i++ {
		putRule(b, objs, uint32(i), denyPortRule(uint16(1000+i)))
	}
}

// bpfBench runs prog b.N times via BPF_PROG_TEST_RUN (kernel-measured CPU time)
// and reports ns/pkt and pps.
//
// Uses prog.Benchmark which executes all iterations in the kernel and returns
// the total kernel-measured duration, avoiding syscall overhead per packet.
func bpfBench(b *testing.B, objs TcFilterObjects, pkt []byte, useIngress bool) {
	b.Helper()
	prog := objs.QfTcIngress
	if !useIngress {
		prog = objs.QfTcEgress
	}

	repeat := b.N
	if repeat == 0 {
		repeat = 1
	}

	_, total, err := prog.Benchmark(pkt, repeat, b.ResetTimer)
	if err != nil {
		if errors.Is(err, unix.EPERM) {
			b.Skip("BPF_PROG_TEST_RUN requires CAP_BPF")
		}
		b.Fatal(err)
	}

	nsPerPkt := float64(total.Nanoseconds()) / float64(repeat)
	b.ReportMetric(nsPerPkt, "ns/pkt")
	b.ReportMetric(1e9/nsPerPkt, "pps")
}

var (
	benchSrc = net.ParseIP("192.168.1.1")
	benchDst = net.ParseIP("10.0.0.2")
)

// BenchmarkBPF_Baseline — empty ruleset, default allow.
// Measures BPF program overhead with no rules and no conntrack entry.
func BenchmarkBPF_Baseline(b *testing.B) {
	objs := loadForTest(b)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_HotPath_Established — conntrack hit (CT_ESTABLISHED).
// No rule walk; tests the fast-path exit for known flows.
func BenchmarkBPF_HotPath_Established(b *testing.B) {
	objs := loadForTest(b)
	seedCT(b, objs, benchSrc.String(), benchDst.String(), 1234, 80, protoTCP, ctEstablished)
	pkt := buildACK(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_ColdPath_8_LastMatch — 8 rules, match at rule 7 (worst-case 8-rule walk).
func BenchmarkBPF_ColdPath_8_LastMatch(b *testing.B) {
	objs := loadForTest(b)
	seedLastMatchRules(b, objs, 8)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_ColdPath_32_LastMatch — 32 rules, match at rule 31.
func BenchmarkBPF_ColdPath_32_LastMatch(b *testing.B) {
	objs := loadForTest(b)
	seedLastMatchRules(b, objs, 32)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_ColdPath_64_LastMatch — EvalMaxRules rules, match at last.
// Absolute worst case: full loop unroll, match at position 63.
func BenchmarkBPF_ColdPath_64_LastMatch(b *testing.B) {
	objs := loadForTest(b)
	seedLastMatchRules(b, objs, EvalMaxRules)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_ColdPath_8_NoMatch — 8 rules, none match → default allow.
func BenchmarkBPF_ColdPath_8_NoMatch(b *testing.B) {
	objs := loadForTest(b)
	seedNoMatchRules(b, objs, 8)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}

// BenchmarkBPF_ColdPath_64_NoMatch — EvalMaxRules rules, none match → default allow.
// Full walk with no early exit — measures per-rule overhead precisely.
func BenchmarkBPF_ColdPath_64_NoMatch(b *testing.B) {
	objs := loadForTest(b)
	seedNoMatchRules(b, objs, EvalMaxRules)
	pkt := buildPkt(benchSrc, benchDst, 1234, 80)
	bpfBench(b, objs, pkt, true)
}
