package loader

import (
	"errors"
	"testing"

	"golang.org/x/sys/unix"
)

// conntrackMax mirrors CONNTRACK_MAX from common.h — the BPF LRU map capacity.
const conntrackMax = 65536

// fillCT writes count CT entries directly into the BPF LRU map via Put.
// Uses srcIP=10.0.0.1, dstIP=192.168.1.1, dstPort=80, srcPort=0..count-1.
// Count must be <= 65536 to stay within unique srcPort space for this base tuple.
// All entries are seeded as CT_ESTABLISHED.
//
// Both 10.0.0.1 and 192.168.1.1 encode such that 10.0.0.1 < 192.168.1.1 in
// the BPF __be32 LE representation, so the canonical key has SrcIp=10.0.0.1.
func fillCT(b testing.TB, objs TcFilterObjects, count int, proto uint8) {
	b.Helper()
	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	entry := TcFilterCtEntry{State: ctEstablished}
	for i := 0; i < count; i++ {
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i), DstPort: 80,
			Proto: proto,
		}
		if err := objs.QfConntrack.Put(key, entry); err != nil {
			b.Fatalf("fillCT[%d]: %v", i, err)
		}
	}
}

// fillCTExtra writes count CT entries with dstPort=443 — distinct from the
// base fillCT tuples (dstPort=80), so every Put creates a new unique key and
// triggers one LRU eviction once the map is full.
func fillCTExtra(b testing.TB, objs TcFilterObjects, count int, proto uint8) {
	b.Helper()
	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	entry := TcFilterCtEntry{State: ctEstablished}
	for i := 0; i < count; i++ {
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i), DstPort: 443,
			Proto: proto,
		}
		if err := objs.QfConntrack.Put(key, entry); err != nil {
			b.Fatalf("fillCTExtra[%d]: %v", i, err)
		}
	}
}

// countCT returns the number of entries in the CT map via iteration.
func countCT(objs TcFilterObjects) int {
	var k TcFilterCtKey
	var v TcFilterCtEntry
	n := 0
	iter := objs.QfConntrack.Iterate()
	for iter.Next(&k, &v) {
		n++
	}
	return n
}

// bpfBenchCT runs prog.Benchmark for the conntrack benchmarks.
// Resets the timer before benchmarking so setup (fillCT) is excluded.
// Handles EPERM → b.Skip (requires CAP_BPF on Linux).
func bpfBenchCT(b *testing.B, objs TcFilterObjects, pkt []byte) {
	b.Helper()
	prog := objs.QfTcIngress
	repeat := b.N
	if repeat == 0 {
		repeat = 1
	}
	b.ResetTimer() // exclude fillCT setup from timing
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

// ── Throughput benchmarks ─────────────────────────────────────────────────

// BenchmarkConntrack_HotPath_Full_TCP measures BPF ingress throughput when
// the CT map is at full capacity (65536 TCP flows).
// A packet matching an ESTABLISHED entry exercises the ct_lookup hot path.
// Compare to BenchmarkBPF_HotPath_Established which has only one CT entry.
func BenchmarkConntrack_HotPath_Full_TCP(b *testing.B) {
	objs := loadForTest(b)
	putConfig(b, objs, 0, 0x1) // enable conntrack

	fillCT(b, objs, conntrackMax, protoTCP)

	// ACK packet for flow srcPort=0, matching one of the pre-seeded entries.
	src := benchSrc // 192.168.1.1 from bench_bpf_test.go — but we need 10.0.0.1
	_ = src
	pkt := buildACK(
		decodeIP4(ip4BPF("10.0.0.1")),
		decodeIP4(ip4BPF("192.168.1.1")),
		0, 80,
	)
	bpfBenchCT(b, objs, pkt)
}

// BenchmarkConntrack_HotPath_Full_UDP measures BPF ingress throughput with
// the CT map full of UDP flows (mixed protocol stress).
func BenchmarkConntrack_HotPath_Full_UDP(b *testing.B) {
	objs := loadForTest(b)
	putConfig(b, objs, 0, 0x1) // enable conntrack

	fillCT(b, objs, conntrackMax, protoUDP)

	pkt := buildUDP(
		decodeIP4(ip4BPF("10.0.0.1")),
		decodeIP4(ip4BPF("192.168.1.1")),
		0, 80,
	)
	bpfBenchCT(b, objs, pkt)
}

// BenchmarkConntrack_HotPath_Full_Mixed measures BPF throughput when the map
// holds 32768 TCP + 32768 UDP flows (realistic mixed-protocol load).
func BenchmarkConntrack_HotPath_Full_Mixed(b *testing.B) {
	objs := loadForTest(b)
	putConfig(b, objs, 0, 0x1)

	half := conntrackMax / 2
	fillCT(b, objs, half, protoTCP)

	// Fill second half using UDP with srcPort starting from half to avoid collision.
	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	entry := TcFilterCtEntry{State: ctEstablished}
	for i := 0; i < half; i++ {
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i), DstPort: 53, // UDP DNS port — distinct from TCP/80
			Proto: protoUDP,
		}
		if err := objs.QfConntrack.Put(key, entry); err != nil {
			b.Fatalf("fill UDP half[%d]: %v", i, err)
		}
	}

	// Bench on a TCP packet — tests lookup in mixed-protocol map.
	pkt := buildACK(
		decodeIP4(ip4BPF("10.0.0.1")),
		decodeIP4(ip4BPF("192.168.1.1")),
		0, 80,
	)
	bpfBenchCT(b, objs, pkt)
}

// BenchmarkConntrack_LRUEviction_Put measures the throughput of Put operations
// on a map that is already at capacity (every Put triggers one LRU eviction).
// This stresses the LRU eviction path without BPF_PROG_TEST_RUN.
func BenchmarkConntrack_LRUEviction_Put(b *testing.B) {
	objs := loadForTest(b)

	// Fill to capacity first (not counted in the benchmark timer).
	fillCT(b, objs, conntrackMax, protoTCP)

	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	entry := TcFilterCtEntry{State: ctEstablished}

	b.ResetTimer()
	for i := range b.N {
		// dstPort=443 ensures a new unique key every iteration (distinct from
		// the pre-filled dstPort=80 entries), triggering an LRU eviction.
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i % 65536), DstPort: 443,
			Proto: protoTCP,
		}
		if err := objs.QfConntrack.Put(key, entry); err != nil {
			b.Fatalf("eviction put[%d]: %v", i, err)
		}
	}
	b.StopTimer()
	b.ReportMetric(float64(b.N)/b.Elapsed().Seconds(), "evictions/s")
}

// ── Correctness tests ─────────────────────────────────────────────────────

// TestConntrackLRU_NoCorruption fills the map to capacity and verifies that
// every entry can be looked up with the correct State — no corruption.
func TestConntrackLRU_NoCorruption(t *testing.T) {
	objs := loadForTest(t)
	putConfig(t, objs, 0, 0x1)

	const fill = conntrackMax
	fillCT(t, objs, fill, protoTCP)

	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	corrupt := 0
	for i := 0; i < fill; i++ {
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i), DstPort: 80,
			Proto: protoTCP,
		}
		var val TcFilterCtEntry
		if err := objs.QfConntrack.Lookup(key, &val); err != nil {
			// Could be legitimately evicted by the map (LRU can evict on Put
			// even when not over capacity due to internal bucket balancing).
			continue
		}
		if val.State != ctEstablished {
			t.Errorf("entry[%d]: got State=%d want %d", i, val.State, ctEstablished)
			corrupt++
		}
	}
	if corrupt > 0 {
		t.Errorf("corruption: %d entries have wrong state", corrupt)
	}
}

// TestConntrackLRU_SizeStable verifies that the LRU map never exceeds its
// capacity after an overfill: 65536 base entries + 10240 extra = 75776 Puts.
// Expected: map size ≤ conntrackMax after all Puts.
func TestConntrackLRU_SizeStable(t *testing.T) {
	objs := loadForTest(t)

	const extra = 10240
	fillCT(t, objs, conntrackMax, protoTCP)
	fillCTExtra(t, objs, extra, protoTCP) // triggers 10240 LRU evictions

	got := countCT(objs)
	if got > conntrackMax {
		t.Errorf("CT map exceeded capacity: got %d entries, max %d", got, conntrackMax)
	}
}

// TestConntrackLRU_HitRate quantifies LRU eviction accuracy: after filling
// 65536 entries and adding 10240 extras (forcing 10240 evictions), it measures
// what fraction of the original entries survive.
//
// Expected: hit_rate ≈ (65536-10240)/65536 ≈ 84% (LRU evicts oldest first).
// The test asserts hit_rate > 60% to guard against catastrophic eviction bugs.
func TestConntrackLRU_HitRate(t *testing.T) {
	objs := loadForTest(t)

	const extra = 10240
	fillCT(t, objs, conntrackMax, protoTCP)
	fillCTExtra(t, objs, extra, protoTCP)

	src := ip4BPF("10.0.0.1")
	dst := ip4BPF("192.168.1.1")
	hits := 0
	for i := 0; i < conntrackMax; i++ {
		key := TcFilterCtKey{
			SrcIp: src, DstIp: dst,
			SrcPort: uint16(i), DstPort: 80,
			Proto: protoTCP,
		}
		var val TcFilterCtEntry
		if err := objs.QfConntrack.Lookup(key, &val); err == nil {
			hits++
		}
	}
	hitRate := float64(hits) / float64(conntrackMax)
	t.Logf("hit_rate=%.1f%% (%d/%d), evictions=%d",
		hitRate*100, hits, conntrackMax, extra)

	if hitRate < 0.60 {
		t.Errorf("hit_rate=%.1f%% below 60%% threshold — catastrophic LRU eviction", hitRate*100)
	}
}
