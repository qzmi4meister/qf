package ingest

import (
	"context"
	"fmt"
	"net/netip"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// connectBench opens a pgxpool using QF_BENCH_DSN.
// Skips the benchmark if the env var is not set.
func connectBench(b *testing.B) *pgxpool.Pool {
	b.Helper()
	dsn := os.Getenv("QF_BENCH_DSN")
	if dsn == "" {
		b.Skip("QF_BENCH_DSN not set; export it or run make bench-ingest on a host with PostgreSQL access")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, dsn)
	if err != nil {
		b.Fatalf("pgxpool.New: %v", err)
	}
	if err := pool.Ping(ctx); err != nil {
		pool.Close()
		b.Fatalf("DB ping: %v", err)
	}
	b.Cleanup(pool.Close)
	return pool
}

// ensureTodayPartition creates today's log_events partition if it does not exist.
// log_events is PARTITION BY RANGE (created_at) with daily partitions.
func ensureTodayPartition(b *testing.B, pool *pgxpool.Pool) {
	b.Helper()
	today := time.Now().UTC().Truncate(24 * time.Hour)
	tomorrow := today.AddDate(0, 0, 1)
	name := "log_events_" + today.Format("2006_01_02")
	sql := fmt.Sprintf(
		"CREATE TABLE IF NOT EXISTS %s PARTITION OF log_events FOR VALUES FROM ('%s') TO ('%s')",
		name, today.Format("2006-01-02"), tomorrow.Format("2006-01-02"),
	)
	if _, err := pool.Exec(context.Background(), sql); err != nil {
		b.Fatalf("create today's partition: %v", err)
	}
}

var (
	fixedTenantID = mustParseUUID("00000000-0000-0000-0000-000000000001")
	fixedHostID   = mustParseUUID("00000000-0000-0000-0000-000000000002")
	fixedRuleID   = mustParseUUID("00000000-0000-0000-0000-000000000003")
	fixedPolicyID = mustParseUUID("00000000-0000-0000-0000-000000000004")
	addrSrc       = func() *netip.Addr { a := netip.MustParseAddr("192.168.1.1"); return &a }()
	addrDst       = func() *netip.Addr { a := netip.MustParseAddr("10.0.0.2"); return &a }()
)

func mustParseUUID(s string) pgtype.UUID {
	var u pgtype.UUID
	if err := u.Scan(s); err != nil {
		panic(err)
	}
	return u
}

// makeRow builds one InsertLogEventsBatchParams row.
// i is used to vary src_port so rows are not identical.
func makeRow(i int) storegen.InsertLogEventsBatchParams {
	sp := int32(1024 + i%60000)
	dp := int32(80)
	return storegen.InsertLogEventsBatchParams{
		TenantID:  fixedTenantID,
		HostID:    fixedHostID,
		RuleID:    fixedRuleID,
		PolicyID:  fixedPolicyID,
		Direction: "ingress",
		Action:    "deny",
		Protocol:  6,
		SrcIp:     addrSrc,
		DstIp:     addrDst,
		SrcPort:   &sp,
		DstPort:   &dp,
		CreatedAt: pgtype.Timestamptz{Time: time.Now(), Valid: true},
	}
}

// makeBatch pre-builds a slice of n rows.
func makeBatch(n int) []storegen.InsertLogEventsBatchParams {
	rows := make([]storegen.InsertLogEventsBatchParams, n)
	for i := range rows {
		rows[i] = makeRow(i)
	}
	return rows
}

// makeProtoMsg builds a *qfv1.LogEvents with n events.
func makeProtoMsg(n int) *qfv1.LogEvents {
	now := timestamppb.Now()
	events := make([]*qfv1.LogEvent, n)
	for i := range events {
		events[i] = &qfv1.LogEvent{
			Ts:        now,
			RuleId:    "00000000-0000-0000-0000-000000000003",
			PolicyId:  "00000000-0000-0000-0000-000000000004",
			Direction: qfv1.Direction_DIRECTION_INGRESS,
			Action:    qfv1.Action_ACTION_DENY,
			Protocol:  6,
			SrcIp:     []byte{192, 168, 1, 1},
			DstIp:     []byte{10, 0, 0, 2},
			SrcPort:   uint32(1024 + i%60000),
			DstPort:   80,
		}
	}
	return &qfv1.LogEvents{Events: events}
}

// waitDrain blocks until logCh is empty or the deadline passes.
func waitDrain(b *testing.B, ing *Ingester, timeout time.Duration) {
	b.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if len(ing.logCh) == 0 {
			return
		}
		time.Sleep(50 * time.Millisecond)
	}
	b.Logf("waitDrain: channel not drained within %s; residual=%d", timeout, len(ing.logCh))
}

// ── Direct COPY benchmarks ────────────────────────────────────────────────
//
// Bypass the ingester channel and measure raw PostgreSQL COPY throughput.
// One b.N iteration = one COPY call inserting batchSize rows.

func benchDirectCOPY(b *testing.B, batchSize int) {
	b.Helper()
	pool := connectBench(b)
	ensureTodayPartition(b, pool)
	q := storegen.New(pool)
	ctx := context.Background()
	batch := makeBatch(batchSize)

	b.SetBytes(int64(batchSize))
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		if _, err := q.InsertLogEventsBatch(ctx, batch); err != nil {
			b.Fatalf("InsertLogEventsBatch: %v", err)
		}
	}
	b.StopTimer()

	total := int64(b.N) * int64(batchSize)
	b.ReportMetric(float64(total)/b.Elapsed().Seconds(), "rows/s")
	b.ReportMetric(b.Elapsed().Seconds()/float64(b.N)*1000, "ms/batch")
}

// BenchmarkIngestDirectCOPY_100rows — raw COPY with 100-row batches.
func BenchmarkIngestDirectCOPY_100rows(b *testing.B) { benchDirectCOPY(b, 100) }

// BenchmarkIngestDirectCOPY_500rows — raw COPY with 500-row batches.
func BenchmarkIngestDirectCOPY_500rows(b *testing.B) { benchDirectCOPY(b, 500) }

// BenchmarkIngestDirectCOPY_2000rows — raw COPY at maxBatch=2000 (ingester's flush size).
func BenchmarkIngestDirectCOPY_2000rows(b *testing.B) { benchDirectCOPY(b, maxBatch) }

// ── Ingester throughput benchmarks ───────────────────────────────────────
//
// Full pipeline: IngestLogEvents → logCh → logWorker → InsertLogEventsBatch.
// One b.N iteration = one IngestLogEvents call with eventsPerMsg events.
// Reports events/s (producer rate) and peak channel queue depth.
//
// Note: when eventsPerMsg > chanBuf (10000), events are dropped by the
// non-blocking channel put in IngestLogEvents. Peak queue depth ≥ chanBuf
// indicates saturation.

func benchIngestThroughput(b *testing.B, eventsPerMsg int) {
	b.Helper()
	pool := connectBench(b)
	ensureTodayPartition(b, pool)
	q := storegen.New(pool)
	msg := makeProtoMsg(eventsPerMsg)

	ctx, cancel := context.WithCancel(context.Background())
	b.Cleanup(cancel)

	ing := New(q, nil)
	go ing.Start(ctx)

	var peakQueue int

	b.ResetTimer()
	for range b.N {
		ing.IngestLogEvents(fixedTenantID, fixedHostID, msg)
		if d := len(ing.logCh); d > peakQueue {
			peakQueue = d
		}
	}
	b.StopTimer()

	waitDrain(b, ing, 30*time.Second)

	total := int64(b.N) * int64(eventsPerMsg)
	b.ReportMetric(float64(total)/b.Elapsed().Seconds(), "events/s")
	b.ReportMetric(float64(peakQueue), "peak_queue_depth")
}

// BenchmarkIngestThroughput_1k — 1000 events per IngestLogEvents call.
func BenchmarkIngestThroughput_1k(b *testing.B) { benchIngestThroughput(b, 1_000) }

// BenchmarkIngestThroughput_5k — 5000 events per call.
func BenchmarkIngestThroughput_5k(b *testing.B) { benchIngestThroughput(b, 5_000) }

// BenchmarkIngestThroughput_10k — 10000 events per call (= chanBuf; channel hits capacity).
func BenchmarkIngestThroughput_10k(b *testing.B) { benchIngestThroughput(b, 10_000) }

// BenchmarkIngestThroughput_50k — 50000 events per call (5× chanBuf; heavy drop rate).
func BenchmarkIngestThroughput_50k(b *testing.B) { benchIngestThroughput(b, 50_000) }
