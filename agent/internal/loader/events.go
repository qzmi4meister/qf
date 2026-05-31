package loader

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"net"
	"time"

	"github.com/cilium/ebpf/ringbuf"
)

// LogEvent is a decoded ring-buffer event emitted by the BPF datapath.
// Emitted on ACTION_DENY, ACTION_LOG, or when log_enabled is set on a rule.
type LogEvent struct {
	TimestampNs uint64
	RuleID      [16]byte // UUID: hi=RuleIdHi, lo=RuleIdLo
	SrcIP       net.IP
	DstIP       net.IP
	SrcPort     uint16
	DstPort     uint16
	PktSize     uint16
	TCPFlags    uint8
	Proto       uint8
	Direction   uint8
	Action      uint8
	CTState     uint8
}

// logEventWire mirrors struct log_event in common.h for binary decoding.
// 44 bytes, no padding (verified against C struct layout).
type logEventWire struct {
	TsNs      uint64
	RuleIdHi  uint64
	RuleIdLo  uint64
	SrcIP     uint32 // __be32 — decode via decodeIP4
	DstIP     uint32 // __be32 — decode via decodeIP4
	SrcPort   uint16
	DstPort   uint16
	PktSize   uint16
	TCPFlags  uint16 // stored as __u16 in BPF struct; only low 8 bits used
	Proto     uint8
	Direction uint8
	Action    uint8
	CTState   uint8
}

const logEventSize = 44 // sizeof(struct log_event)

// EventReader reads log events from the qf_events ring buffer.
type EventReader struct {
	rd *ringbuf.Reader
}

// NewEventReader creates an EventReader attached to the qf_events ring buffer.
func NewEventReader(objs *TcFilterObjects) (*EventReader, error) {
	rd, err := ringbuf.NewReader(objs.QfEvents)
	if err != nil {
		return nil, fmt.Errorf("ringbuf reader: %w", err)
	}
	return &EventReader{rd: rd}, nil
}

// Close releases the reader. Interrupts any blocked Read call.
func (er *EventReader) Close() error {
	return er.rd.Close()
}

// SetDeadline sets a deadline for future Read calls.
// A zero time.Time removes the deadline.
func (er *EventReader) SetDeadline(t time.Time) {
	er.rd.SetDeadline(t)
}

// Read blocks until a log event is available and returns it decoded.
// Returns os.ErrDeadlineExceeded if the deadline set via SetDeadline elapses.
// Returns os.ErrClosed if Close has been called.
func (er *EventReader) Read() (LogEvent, error) {
	rec, err := er.rd.Read()
	if err != nil {
		return LogEvent{}, err
	}
	return decodeLogEvent(rec.RawSample)
}

// Loader convenience wrapper.
func (l *Loader) NewEventReader() (*EventReader, error) {
	return NewEventReader(&l.objs)
}

// ── decoding ─────────────────────────────────────────────────────────────

func decodeLogEvent(raw []byte) (LogEvent, error) {
	if len(raw) < logEventSize {
		return LogEvent{}, fmt.Errorf("short log_event: %d < %d bytes", len(raw), logEventSize)
	}
	var w logEventWire
	if err := binary.Read(bytes.NewReader(raw[:logEventSize]), binary.LittleEndian, &w); err != nil {
		return LogEvent{}, fmt.Errorf("decode log_event: %w", err)
	}
	var id [16]byte
	binary.BigEndian.PutUint64(id[:8], w.RuleIdHi)
	binary.BigEndian.PutUint64(id[8:], w.RuleIdLo)
	return LogEvent{
		TimestampNs: w.TsNs,
		RuleID:      id,
		SrcIP:       decodeIP4(w.SrcIP),
		DstIP:       decodeIP4(w.DstIP),
		SrcPort:     w.SrcPort,
		DstPort:     w.DstPort,
		PktSize:     w.PktSize,
		TCPFlags:    uint8(w.TCPFlags),
		Proto:       w.Proto,
		Direction:   w.Direction,
		Action:      w.Action,
		CTState:     w.CTState,
	}, nil
}
