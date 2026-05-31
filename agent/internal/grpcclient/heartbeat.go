package grpcclient

import (
	"context"
	"fmt"
	"sync"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultHeartbeatMs = 30_000

// HeartbeatSender sends periodic Heartbeat messages on a stream.
// Interval is configurable at runtime via SetIntervalMs (from ConfigUpdate).
type HeartbeatSender struct {
	stream     qfv1.AgentService_StreamClient
	mu         sync.Mutex
	intervalMs uint32
	// genFn returns the agent's current applied bundle generation.
	genFn func() int64
	// healthFn returns current AgentHealth; may be nil (sends empty health).
	healthFn func() *qfv1.AgentHealth
}

// NewHeartbeatSender creates a HeartbeatSender.
// intervalMs=0 uses defaultHeartbeatMs (30 000).
func NewHeartbeatSender(
	stream qfv1.AgentService_StreamClient,
	genFn func() int64,
	healthFn func() *qfv1.AgentHealth,
	intervalMs uint32,
) *HeartbeatSender {
	if intervalMs == 0 {
		intervalMs = defaultHeartbeatMs
	}
	return &HeartbeatSender{
		stream:     stream,
		intervalMs: intervalMs,
		genFn:      genFn,
		healthFn:   healthFn,
	}
}

// SetIntervalMs updates the heartbeat interval. Takes effect on the next beat.
func (h *HeartbeatSender) SetIntervalMs(ms uint32) {
	if ms == 0 {
		return
	}
	h.mu.Lock()
	h.intervalMs = ms
	h.mu.Unlock()
}

// Run sends heartbeats until ctx is cancelled or the stream returns an error.
func (h *HeartbeatSender) Run(ctx context.Context) error {
	for {
		h.mu.Lock()
		interval := time.Duration(h.intervalMs) * time.Millisecond
		h.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
			if err := h.send(); err != nil {
				return fmt.Errorf("heartbeat: %w", err)
			}
		}
	}
}

func (h *HeartbeatSender) send() error {
	var health *qfv1.AgentHealth
	if h.healthFn != nil {
		health = h.healthFn()
	}
	if health == nil {
		health = &qfv1.AgentHealth{}
	}
	hb := &qfv1.Heartbeat{
		CurrentGeneration: h.genFn(),
		Health:            health,
		Ts:                timestamppb.Now(),
	}
	return h.stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_Heartbeat{Heartbeat: hb},
	})
}
