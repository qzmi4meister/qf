// Package ingest receives telemetry from gRPC handlers and bulk-inserts it
// into PostgreSQL. Each event type has its own buffered channel and worker
// goroutine; workers flush when the batch reaches maxBatch rows or flushInterval
// elapses (whichever comes first).
package ingest

import (
	"context"
	"log/slog"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

const (
	maxBatch      = 2000
	flushInterval = time.Second
	chanBuf       = 10_000
	chanBufSmall  = 1_000
)

// Ingester fans telemetry into per-type bulk-insert workers.
type Ingester struct {
	q         *storegen.Queries
	logCh     chan storegen.InsertLogEventsBatchParams
	flowCh    chan storegen.InsertFlowEventsBatchParams
	counterCh chan storegen.InsertCounterSnapshotsBatchParams
	systemCh  chan storegen.InsertSystemEventParams
}

// New creates an Ingester backed by q. Call Start to launch workers.
func New(q *storegen.Queries) *Ingester {
	return &Ingester{
		q:         q,
		logCh:     make(chan storegen.InsertLogEventsBatchParams, chanBuf),
		flowCh:    make(chan storegen.InsertFlowEventsBatchParams, chanBuf),
		counterCh: make(chan storegen.InsertCounterSnapshotsBatchParams, chanBuf),
		systemCh:  make(chan storegen.InsertSystemEventParams, chanBufSmall),
	}
}

// Start launches worker goroutines and blocks until ctx is cancelled.
func (ing *Ingester) Start(ctx context.Context) {
	var wg sync.WaitGroup
	wg.Add(4)
	go func() { defer wg.Done(); ing.logWorker(ctx) }()
	go func() { defer wg.Done(); ing.flowWorker(ctx) }()
	go func() { defer wg.Done(); ing.counterWorker(ctx) }()
	go func() { defer wg.Done(); ing.systemWorker(ctx) }()
	wg.Wait()
}

// ── Public ingest methods ─────────────────────────────────────────────────

// IngestLogEvents converts a proto LogEvents message and queues the rows.
// tenantID and hostID come from the mTLS stream context.
func (ing *Ingester) IngestLogEvents(tenantID, hostID pgtype.UUID, msg *qfv1.LogEvents) {
	for _, ev := range msg.Events {
		p := protoLogEventToParams(tenantID, hostID, ev)
		select {
		case ing.logCh <- p:
		default:
			slog.Warn("ingest: log channel full, dropping event")
		}
	}
}

// IngestFlowEvents converts a proto FlowEvents message and queues the rows.
func (ing *Ingester) IngestFlowEvents(tenantID, hostID pgtype.UUID, msg *qfv1.FlowEvents) {
	for _, fv := range msg.Flows {
		p := protoFlowEventToParams(tenantID, hostID, fv)
		select {
		case ing.flowCh <- p:
		default:
			slog.Warn("ingest: flow channel full, dropping event")
		}
	}
}

// IngestCounterUpdate converts a proto CounterUpdate message and queues rows.
func (ing *Ingester) IngestCounterUpdate(tenantID, hostID pgtype.UUID, msg *qfv1.CounterUpdate) {
	ts := protoTsToTimestamptz(msg.Ts)
	for _, rc := range msg.Counters {
		p := storegen.InsertCounterSnapshotsBatchParams{
			TenantID: tenantID,
			HostID:   hostID,
			RuleID:   parseUUID(rc.RuleId),
			PolicyID: pgtype.UUID{}, // not in proto; left null
			Packets:  int64(rc.Packets),
			Bytes:    int64(rc.Bytes),
			Ts:       ts,
		}
		select {
		case ing.counterCh <- p:
		default:
			slog.Warn("ingest: counter channel full, dropping snapshot")
		}
	}
}

// IngestSystemEvent converts a proto SystemEvent and queues it.
func (ing *Ingester) IngestSystemEvent(tenantID, hostID pgtype.UUID, msg *qfv1.SystemEvent) {
	p := protoSystemEventToParams(tenantID, hostID, msg)
	select {
	case ing.systemCh <- p:
	default:
		slog.Warn("ingest: system event channel full, dropping")
	}
}

// ── Workers ───────────────────────────────────────────────────────────────

func (ing *Ingester) logWorker(ctx context.Context) {
	var batch []storegen.InsertLogEventsBatchParams
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		n, err := ing.q.InsertLogEventsBatch(ctx, batch)
		if err != nil {
			slog.Error("ingest: log flush failed", "err", err, "rows", len(batch))
		} else {
			slog.Debug("ingest: log flush", "rows", n)
		}
		batch = batch[:0]
	}

	for {
		select {
		case p, ok := <-ing.logCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, p)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-tick.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (ing *Ingester) flowWorker(ctx context.Context) {
	var batch []storegen.InsertFlowEventsBatchParams
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		n, err := ing.q.InsertFlowEventsBatch(ctx, batch)
		if err != nil {
			slog.Error("ingest: flow flush failed", "err", err, "rows", len(batch))
		} else {
			slog.Debug("ingest: flow flush", "rows", n)
		}
		batch = batch[:0]
	}

	for {
		select {
		case p, ok := <-ing.flowCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, p)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-tick.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (ing *Ingester) counterWorker(ctx context.Context) {
	var batch []storegen.InsertCounterSnapshotsBatchParams
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()

	flush := func() {
		if len(batch) == 0 {
			return
		}
		n, err := ing.q.InsertCounterSnapshotsBatch(ctx, batch)
		if err != nil {
			slog.Error("ingest: counter flush failed", "err", err, "rows", len(batch))
		} else {
			slog.Debug("ingest: counter flush", "rows", n)
		}
		batch = batch[:0]
	}

	for {
		select {
		case p, ok := <-ing.counterCh:
			if !ok {
				flush()
				return
			}
			batch = append(batch, p)
			if len(batch) >= maxBatch {
				flush()
			}
		case <-tick.C:
			flush()
		case <-ctx.Done():
			flush()
			return
		}
	}
}

func (ing *Ingester) systemWorker(ctx context.Context) {
	tick := time.NewTicker(flushInterval)
	defer tick.Stop()

	for {
		select {
		case p, ok := <-ing.systemCh:
			if !ok {
				return
			}
			if _, err := ing.q.InsertSystemEvent(ctx, p); err != nil {
				slog.Error("ingest: system event insert failed", "err", err)
			}
		case <-tick.C:
			// nothing to flush — system events are inserted one by one
		case <-ctx.Done():
			return
		}
	}
}
