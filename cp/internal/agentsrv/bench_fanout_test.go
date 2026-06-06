package agentsrv

import (
	"context"
	"fmt"
	"sort"
	"sync"
	"testing"
	"time"

	"google.golang.org/grpc/metadata"

	"github.com/qf/qf/cp/internal/policy"
	qfv1 "github.com/qf/qf/proto/qf/v1"
)

// stubStream implements qfv1.AgentService_StreamServer for bench use.
// Send records the arrival time into a buffered channel.
// All other methods are no-ops or block on context cancellation.
type stubStream struct {
	ctx      context.Context
	recvTime chan time.Time // cap=1; Send writes arrival time
}

func newStubStream(ctx context.Context) *stubStream {
	return &stubStream{ctx: ctx, recvTime: make(chan time.Time, 1)}
}

func (s *stubStream) Send(msg *qfv1.ServerMessage) error {
	s.recvTime <- time.Now()
	return nil
}

func (s *stubStream) Recv() (*qfv1.AgentMessage, error) {
	<-s.ctx.Done()
	return nil, context.Canceled
}

func (s *stubStream) Context() context.Context      { return s.ctx }
func (s *stubStream) SetHeader(metadata.MD) error   { return nil }
func (s *stubStream) SendHeader(metadata.MD) error  { return nil }
func (s *stubStream) SetTrailer(metadata.MD)        {}
func (s *stubStream) SendMsg(_ any) error           { return nil }
func (s *stubStream) RecvMsg(_ any) error           { return nil }

// fanOutBench dispatches a PolicyBundle to nAgents using concurrency goroutines
// and measures per-agent delivery latency.
//
// Latency = time.Since(t0) recorded inside stubStream.Send, where t0 is
// sampled just before the first dispatch goroutine starts. This captures
// the full fan-out cost: worker scheduling + registry lock + channel write.
//
// Collects one sample per agent per b.N iteration; reports p50/p95/p99/max in ms.
func fanOutBench(b *testing.B, nAgents, concurrency int) {
	b.Helper()

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	reg := NewStreamRegistry()
	stubs := make([]*stubStream, nAgents)
	hostIDs := make([]string, nAgents)
	bundle := &qfv1.PolicyBundle{Generation: 1}

	for i := range nAgents {
		hostIDs[i] = fmt.Sprintf("bench-host-%06d", i)
		stubs[i] = newStubStream(ctx)
		reg.register(hostIDs[i], stubs[i])
	}

	allLatencies := make([]int64, 0, b.N*nAgents)

	b.ResetTimer()
	for range b.N {
		t0 := time.Now()

		work := make(chan string, nAgents)
		for _, id := range hostIDs {
			work <- id
		}
		close(work)

		var wg sync.WaitGroup
		wg.Add(concurrency)
		for range concurrency {
			go func() {
				defer wg.Done()
				for id := range work {
					reg.Dispatch(policy.BundleUpdate{
						TenantID: "bench-tenant",
						HostID:   id,
						Bundle:   bundle,
					})
				}
			}()
		}
		wg.Wait()

		// Drain arrival times — all sends completed before wg.Wait() returned.
		for _, s := range stubs {
			recv := <-s.recvTime
			allLatencies = append(allLatencies, recv.Sub(t0).Nanoseconds())
		}
	}
	b.StopTimer()

	if len(allLatencies) == 0 {
		return
	}
	sort.Slice(allLatencies, func(i, j int) bool { return allLatencies[i] < allLatencies[j] })
	n := len(allLatencies)
	ms := func(ns int64) float64 { return float64(ns) / 1e6 }
	b.ReportMetric(ms(allLatencies[n/2]), "p50_ms")
	b.ReportMetric(ms(allLatencies[n*95/100]), "p95_ms")
	b.ReportMetric(ms(allLatencies[n*99/100]), "p99_ms")
	b.ReportMetric(ms(allLatencies[n-1]), "max_ms")
}

// BenchmarkFanOut_1000agents_conc1 — sequential dispatch (current prod path, 1 goroutine).
func BenchmarkFanOut_1000agents_conc1(b *testing.B) { fanOutBench(b, 1000, 1) }

// BenchmarkFanOut_1000agents_conc50 — 50 concurrent dispatcher goroutines.
func BenchmarkFanOut_1000agents_conc50(b *testing.B) { fanOutBench(b, 1000, 50) }

// BenchmarkFanOut_1000agents_conc100 — 100 concurrent dispatcher goroutines.
func BenchmarkFanOut_1000agents_conc100(b *testing.B) { fanOutBench(b, 1000, 100) }

// BenchmarkFanOut_1000agents_conc200 — 200 concurrent dispatcher goroutines.
func BenchmarkFanOut_1000agents_conc200(b *testing.B) { fanOutBench(b, 1000, 200) }
