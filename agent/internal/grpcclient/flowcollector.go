package grpcclient

import (
	"context"
	"sync"
	"time"

	"github.com/qf/qf/agent/internal/loader"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const defaultFlowIntervalMs = 10_000

// flowKey is a comparable 5-tuple map key.
// net.IP (slice) is not comparable, so fixed-size byte arrays are used.
type flowKey struct {
	srcIP   [4]byte
	dstIP   [4]byte
	srcPort uint16
	dstPort uint16
	proto   uint8
}

type flowState struct {
	firstSeen time.Time
	entry     loader.ConntrackEntry
}

// FlowEventCollector periodically scans the BPF conntrack table and emits
// FlowEvents for completed/disappeared flows.
// Disabled by default; call SetEnabled(true) or configure via ConfigUpdate.
type FlowEventCollector struct {
	stream qfv1.AgentService_StreamClient
	ldr    *loader.Loader

	mu         sync.Mutex
	enabled    bool
	intervalMs uint32

	tracked map[flowKey]flowState
}

// NewFlowEventCollector creates a FlowEventCollector.
// intervalMs=0 uses defaultFlowIntervalMs (10 000).
func NewFlowEventCollector(
	stream qfv1.AgentService_StreamClient,
	ldr *loader.Loader,
	intervalMs uint32,
) *FlowEventCollector {
	if intervalMs == 0 {
		intervalMs = defaultFlowIntervalMs
	}
	return &FlowEventCollector{
		stream:     stream,
		ldr:        ldr,
		intervalMs: intervalMs,
		tracked:    make(map[flowKey]flowState),
	}
}

// SetEnabled enables or disables flow event collection.
// When enabling, also forces ConntrackEnabled in the BPF map so the
// datapath actually populates the conntrack table.
func (fc *FlowEventCollector) SetEnabled(v bool) {
	fc.mu.Lock()
	fc.enabled = v
	fc.mu.Unlock()

	if v && fc.ldr != nil {
		if cfg, err := fc.ldr.GetConfig(); err == nil {
			cfg.ConntrackEnabled = true
			_ = fc.ldr.SetConfig(cfg)
		}
	}
}

// Run scans conntrack periodically until ctx is cancelled.
func (fc *FlowEventCollector) Run(ctx context.Context) error {
	for {
		fc.mu.Lock()
		interval := time.Duration(fc.intervalMs) * time.Millisecond
		enabled := fc.enabled
		fc.mu.Unlock()

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(interval):
			if enabled {
				// Ignore errors — flow events are best-effort telemetry.
				_ = fc.scan()
			}
		}
	}
}

func (fc *FlowEventCollector) scan() error {
	entries, err := fc.ldr.ConntrackDump()
	if err != nil {
		return nil
	}

	now := time.Now()
	current := make(map[flowKey]loader.ConntrackEntry, len(entries))
	for _, e := range entries {
		k := entryKey(e)
		current[k] = e
	}

	var events []*qfv1.FlowEvent

	// Detect completed TCP flows and disappeared flows.
	for k, st := range fc.tracked {
		if e, ok := current[k]; ok {
			if e.TCPState == loader.TCPCSClosed {
				events = append(events, makeFlowEvent(st.firstSeen, now, e))
				delete(fc.tracked, k)
			} else {
				// Update tracked entry.
				fc.tracked[k] = flowState{firstSeen: st.firstSeen, entry: e}
			}
		} else {
			// Flow disappeared from conntrack — emit with original entry.
			events = append(events, makeFlowEvent(st.firstSeen, now, st.entry))
			delete(fc.tracked, k)
		}
	}

	// Register new flows.
	for k, e := range current {
		if _, tracked := fc.tracked[k]; !tracked {
			fc.tracked[k] = flowState{firstSeen: now, entry: e}
		}
	}

	if len(events) == 0 {
		return nil
	}

	return fc.stream.Send(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_FlowEvents{
			FlowEvents: &qfv1.FlowEvents{Flows: events},
		},
	})
}

func entryKey(e loader.ConntrackEntry) flowKey {
	k := flowKey{
		srcPort: e.Key.SrcPort,
		dstPort: e.Key.DstPort,
		proto:   e.Key.Protocol,
	}
	if v4 := e.Key.SrcIP.To4(); v4 != nil {
		copy(k.srcIP[:], v4)
	}
	if v4 := e.Key.DstIP.To4(); v4 != nil {
		copy(k.dstIP[:], v4)
	}
	return k
}

func makeFlowEvent(start, end time.Time, e loader.ConntrackEntry) *qfv1.FlowEvent {
	fe := &qfv1.FlowEvent{
		TsStart:      timestamppb.New(start),
		TsEnd:        timestamppb.New(end),
		Protocol:     bpfProtoToProto(e.Key.Protocol),
		SrcPort:      uint32(e.Key.SrcPort),
		DstPort:      uint32(e.Key.DstPort),
		BytesOrig:    e.BytesFwd,
		BytesReply:   e.BytesRev,
		PacketsOrig:  e.PacketsFwd,
		PacketsReply: e.PacketsRev,
	}
	fe.SrcIp = ipToBytes(e.Key.SrcIP)
	fe.DstIp = ipToBytes(e.Key.DstIP)
	if e.TCPState == loader.TCPCSClosed {
		fe.FinalState = "TCP_CLOSED"
	}
	return fe
}
