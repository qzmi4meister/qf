package grpcclient

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"time"

	"github.com/qf/qf/agent/internal/loader"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const (
	defaultBatchSize = 100
	defaultMaxAgeMs  = 5_000
)

// EventBatcher reads log events from the BPF ring buffer, batches them, and
// flushes LogEvents messages on the gRPC stream.
// Flush triggers:
//   - batch reaches batchSize events (configurable via SetBatchSize)
//   - maxAgeMs elapses since the last flush (configurable via SetMaxAgeMs)
type EventBatcher struct {
	stream  qfv1.AgentService_StreamClient
	reader  *loader.EventReader
	ldr     *loader.Loader // nil → skip suppressed-count read/reset
	diskBuf *DiskBuffer    // nil → no disk buffering on send failure

	mu        sync.RWMutex
	batchSize uint32
	maxAgeMs  uint32
}

// SetDiskBuf wires a DiskBuffer so that batches are persisted locally when
// the gRPC stream send fails (e.g. CP unreachable).
func (b *EventBatcher) SetDiskBuf(db *DiskBuffer) {
	b.diskBuf = db
}

// NewEventBatcher creates an EventBatcher.
// ldr may be nil (suppressed count will not be included).
// Zero batchSize/maxAgeMs use defaults (100 / 5000 ms).
func NewEventBatcher(
	stream qfv1.AgentService_StreamClient,
	reader *loader.EventReader,
	ldr *loader.Loader,
	batchSize, maxAgeMs uint32,
) *EventBatcher {
	if batchSize == 0 {
		batchSize = defaultBatchSize
	}
	if maxAgeMs == 0 {
		maxAgeMs = defaultMaxAgeMs
	}
	return &EventBatcher{
		stream:    stream,
		reader:    reader,
		ldr:       ldr,
		batchSize: batchSize,
		maxAgeMs:  maxAgeMs,
	}
}

// SetBatchSize updates the maximum batch size. Takes effect after the current batch flushes.
func (b *EventBatcher) SetBatchSize(n uint32) {
	if n == 0 {
		return
	}
	b.mu.Lock()
	b.batchSize = n
	b.mu.Unlock()
}

// SetMaxAgeMs updates the max batch age. Takes effect after the next flush.
func (b *EventBatcher) SetMaxAgeMs(ms uint32) {
	if ms == 0 {
		return
	}
	b.mu.Lock()
	b.maxAgeMs = ms
	b.mu.Unlock()
}

func (b *EventBatcher) getBatchSize() uint32 {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.batchSize
}

func (b *EventBatcher) getMaxAge() time.Duration {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return time.Duration(b.maxAgeMs) * time.Millisecond
}

// Run reads events and sends batches until ctx is cancelled or the stream errors.
func (b *EventBatcher) Run(ctx context.Context) error {
	type readResult struct {
		ev  loader.LogEvent
		err error
	}
	evCh := make(chan readResult, 256)
	go func() {
		for {
			ev, err := b.reader.Read()
			select {
			case evCh <- readResult{ev, err}:
			case <-ctx.Done():
				return
			}
			if err != nil {
				return
			}
		}
	}()

	var batch []*qfv1.LogEvent

	timer := time.NewTimer(b.getMaxAge())
	defer timer.Stop()

	stopTimer := func() {
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
	}

	flush := func() error {
		if len(batch) == 0 {
			return nil
		}
		var suppressed uint64
		if b.ldr != nil {
			suppressed, _ = b.ldr.ReadSuppressedCount()
			_ = b.ldr.ResetSuppressedCount()
		}
		msg := &qfv1.AgentMessage{
			Payload: &qfv1.AgentMessage_LogEvents{
				LogEvents: &qfv1.LogEvents{
					Events:          batch,
					SuppressedCount: suppressed,
				},
			},
		}
		err := b.stream.Send(msg)
		if err != nil && b.diskBuf != nil {
			_ = b.diskBuf.Write(msg)
		}
		batch = batch[:0]
		return err
	}

	for {
		select {
		case r := <-evCh:
			if r.err != nil {
				_ = flush()
				if errors.Is(r.err, os.ErrDeadlineExceeded) ||
					errors.Is(r.err, os.ErrClosed) ||
					errors.Is(r.err, io.EOF) {
					return nil
				}
				return fmt.Errorf("event batcher read: %w", r.err)
			}
			batch = append(batch, loaderEventToProto(r.ev))
			if uint32(len(batch)) >= b.getBatchSize() {
				if err := flush(); err != nil {
					return fmt.Errorf("event batcher send: %w", err)
				}
				stopTimer()
				timer.Reset(b.getMaxAge())
			}
		case <-timer.C:
			if err := flush(); err != nil {
				return fmt.Errorf("event batcher send: %w", err)
			}
			timer.Reset(b.getMaxAge())
		case <-ctx.Done():
			_ = flush()
			return nil
		}
	}
}

// ── conversion ────────────────────────────────────────────────────────────

func loaderEventToProto(ev loader.LogEvent) *qfv1.LogEvent {
	return &qfv1.LogEvent{
		// timestamppb.Now() gives wall-clock time; BPF ts_ns is CLOCK_MONOTONIC
		// and can't be used as wall-clock without computing the boot-time offset.
		Ts:         timestamppb.Now(),
		RuleId:     uuidBytesToString(ev.RuleID),
		Direction:  bpfDirToProto(ev.Direction),
		Action:     bpfActionToProto(ev.Action),
		Protocol:   bpfProtoToProto(ev.Proto),
		SrcIp:      ipToBytes(ev.SrcIP),
		SrcPort:    uint32(ev.SrcPort),
		DstIp:      ipToBytes(ev.DstIP),
		DstPort:    uint32(ev.DstPort),
		PacketSize: uint32(ev.PktSize),
		TcpFlags:       uint32(ev.TCPFlags),
		ConntrackState: bpfCtStateToProto(ev.CTState),
	}
}

func uuidBytesToString(b [16]byte) string {
	if b == ([16]byte{}) {
		return ""
	}
	return fmt.Sprintf("%x-%x-%x-%x-%x",
		b[0:4], b[4:6], b[6:8], b[8:10], b[10:16])
}

func ipToBytes(ip net.IP) []byte {
	if ip == nil {
		return nil
	}
	if v4 := ip.To4(); v4 != nil {
		return []byte(v4)
	}
	return []byte(ip)
}

func bpfDirToProto(d uint8) qfv1.Direction {
	switch d {
	case 1:
		return qfv1.Direction_DIRECTION_INGRESS
	case 2:
		return qfv1.Direction_DIRECTION_EGRESS
	}
	return qfv1.Direction_DIRECTION_UNSPECIFIED
}

func bpfActionToProto(a uint8) qfv1.Action {
	switch a {
	case 1:
		return qfv1.Action_ACTION_ALLOW
	case 2:
		return qfv1.Action_ACTION_DENY
	case 3:
		return qfv1.Action_ACTION_LOG
	}
	return qfv1.Action_ACTION_UNSPECIFIED
}

func bpfProtoToProto(p uint8) qfv1.Protocol {
	switch p {
	case 1:
		return qfv1.Protocol_PROTOCOL_ANY
	case 2:
		return qfv1.Protocol_PROTOCOL_TCP
	case 3:
		return qfv1.Protocol_PROTOCOL_UDP
	case 4:
		return qfv1.Protocol_PROTOCOL_ICMP
	case 5:
		return qfv1.Protocol_PROTOCOL_ICMPV6
	}
	return qfv1.Protocol_PROTOCOL_UNSPECIFIED
}

func bpfCtStateToProto(s uint8) qfv1.ConntrackState {
	switch s {
	case 1:
		return qfv1.ConntrackState_CONNTRACK_STATE_NONE
	case 2:
		return qfv1.ConntrackState_CONNTRACK_STATE_NEW
	case 3:
		return qfv1.ConntrackState_CONNTRACK_STATE_ESTABLISHED
	case 4:
		return qfv1.ConntrackState_CONNTRACK_STATE_RELATED
	case 5:
		return qfv1.ConntrackState_CONNTRACK_STATE_INVALID
	}
	return qfv1.ConntrackState_CONNTRACK_STATE_UNSPECIFIED
}
