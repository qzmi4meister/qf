package grpcclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/qf/qf/agent/internal/handler"
	"github.com/qf/qf/agent/internal/loader"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultCounterIntervalMs = 60_000

// CounterPoller reads BPF per-rule counters on a configurable interval and
// sends CounterUpdate messages on the gRPC stream.
type CounterPoller struct {
	stream qfv1.AgentService_StreamClient
	ldr    *loader.Loader
	policy *handler.PolicyHandler

	mu         sync.Mutex
	intervalMs uint32
}

// NewCounterPoller creates a CounterPoller.
// intervalMs=0 uses defaultCounterIntervalMs (60 000).
func NewCounterPoller(
	stream qfv1.AgentService_StreamClient,
	ldr *loader.Loader,
	policy *handler.PolicyHandler,
	intervalMs uint32,
) *CounterPoller {
	if intervalMs == 0 {
		intervalMs = defaultCounterIntervalMs
	}
	return &CounterPoller{
		stream:     stream,
		ldr:        ldr,
		policy:     policy,
		intervalMs: intervalMs,
	}
}

// SetIntervalMs updates the poll interval. Takes effect on the next tick.
func (p *CounterPoller) SetIntervalMs(ms uint32) {
	if ms == 0 {
		return
	}
	p.mu.Lock()
	p.intervalMs = ms
	p.mu.Unlock()
}

// Run polls counters until ctx is cancelled or the stream errors.
func (p *CounterPoller) Run(ctx context.Context) error {
	for {
		p.mu.Lock()
		interval := time.Duration(p.intervalMs) * time.Millisecond
		p.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
			if err := p.poll(); err != nil {
				return fmt.Errorf("counter poller: %w", err)
			}
		}
	}
}

func (p *CounterPoller) poll() error {
	counters, err := p.ldr.ReadCounters()
	if err != nil {
		// BPF read errors are transient; skip this cycle.
		return nil
	}

	var rules []loader.RuleSpec
	if ar := p.policy.Current(); ar != nil {
		rules = ar.Rules
	}

	ruleCounters := make([]*qfv1.RuleCounter, 0, len(counters))
	for i, c := range counters {
		ruleID := ""
		if i < len(rules) {
			ruleID = uuidBytesToString(rules[i].ID)
		}
		ruleCounters = append(ruleCounters, &qfv1.RuleCounter{
			RuleId:  ruleID,
			Packets: c.Packets,
			Bytes:   c.Bytes,
		})
	}

	return p.stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_CounterUpdate{
			CounterUpdate: &qfv1.CounterUpdate{
				Ts:       timestamppb.Now(),
				Counters: ruleCounters,
			},
		},
	})
}
