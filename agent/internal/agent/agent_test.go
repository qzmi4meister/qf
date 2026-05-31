package agent

import (
	"bytes"
	"context"
	"errors"
	"io"
	"log"
	"os"
	"sync"
	"testing"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/handler"
	"github.com/qf/qf/agent/internal/loader"
)

// ── mock applier ──────────────────────────────────────────────────────────────

type mockApplier struct {
	mu      sync.Mutex
	configs []loader.Config
	rules   [][]loader.RuleSpec
}

func (m *mockApplier) SetConfig(cfg loader.Config) error {
	m.mu.Lock()
	m.configs = append(m.configs, cfg)
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) PushRules(rules []loader.RuleSpec) error {
	cp := make([]loader.RuleSpec, len(rules))
	copy(cp, rules)
	m.mu.Lock()
	m.rules = append(m.rules, cp)
	m.mu.Unlock()
	return nil
}

func (m *mockApplier) ClearIPSets() error                            { return nil }
func (m *mockApplier) PushIPSet(_ uint32, _ []loader.CIDR4) error    { return nil }

func (m *mockApplier) callCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.rules)
}

// ── mock event sources ────────────────────────────────────────────────────────

// blockingEventSource delivers buffered events then blocks until Close.
type blockingEventSource struct {
	mu     sync.Mutex
	events []loader.LogEvent
	pos    int
	closed chan struct{}
}

func newBlockingSource(events ...loader.LogEvent) *blockingEventSource {
	return &blockingEventSource{events: events, closed: make(chan struct{})}
}

func (s *blockingEventSource) Read() (loader.LogEvent, error) {
	s.mu.Lock()
	if s.pos < len(s.events) {
		ev := s.events[s.pos]
		s.pos++
		s.mu.Unlock()
		return ev, nil
	}
	s.mu.Unlock()
	<-s.closed
	return loader.LogEvent{}, os.ErrClosed
}

func (s *blockingEventSource) Close() error {
	select {
	case <-s.closed:
	default:
		close(s.closed)
	}
	return nil
}

// errorEventSource returns one error then blocks until Close.
type errorEventSource struct {
	errCh  chan error
	closed chan struct{}
}

func newErrorSource(err error) *errorEventSource {
	e := &errorEventSource{
		errCh:  make(chan error, 1),
		closed: make(chan struct{}),
	}
	e.errCh <- err
	return e
}

func (e *errorEventSource) Read() (loader.LogEvent, error) {
	select {
	case err := <-e.errCh:
		return loader.LogEvent{}, err
	case <-e.closed:
		return loader.LogEvent{}, os.ErrClosed
	}
}

func (e *errorEventSource) Close() error {
	select {
	case <-e.closed:
	default:
		close(e.closed)
	}
	return nil
}

// ── log capture ───────────────────────────────────────────────────────────────

type logBuf struct {
	mu   sync.Mutex
	data bytes.Buffer
}

func (b *logBuf) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.data.Write(p)
}

func (b *logBuf) lines() int {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Count(b.data.Bytes(), []byte{'\n'})
}

// ── helpers ──────────────────────────────────────────────────────────────────

const testUUID = "550e8400-e29b-41d4-a716-446655440000"

func newTestAgent(applier handler.RuleApplier, src eventSource) *Agent {
	return &Agent{
		policy: handler.NewPolicyHandler(applier),
		log:    log.New(io.Discard, "", 0),
		newReader: func() (eventSource, error) {
			return src, nil
		},
	}
}

func validBundle(gen int64) *qfv1.PolicyBundle {
	return &qfv1.PolicyBundle{
		Generation:           gen,
		DefaultIngressAction: qfv1.Action_ACTION_ALLOW,
		Rules: []*qfv1.EffectiveRule{
			{
				RuleId:    testUUID,
				Direction: qfv1.Direction_DIRECTION_INGRESS,
				Action:    qfv1.Action_ACTION_ALLOW,
				Match:     &qfv1.RuleMatch{},
			},
		},
	}
}

// ── tests ─────────────────────────────────────────────────────────────────────

func TestAgent_Reload(t *testing.T) {
	m := &mockApplier{}
	a := newTestAgent(m, newBlockingSource())

	ar, err := a.Reload(validBundle(5))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ar.Generation != 5 {
		t.Errorf("Generation: want 5, got %d", ar.Generation)
	}
	if m.callCount() != 1 {
		t.Errorf("PushRules call count: want 1, got %d", m.callCount())
	}
}

func TestAgent_PolicyStatus_NilBeforeReload(t *testing.T) {
	m := &mockApplier{}
	a := newTestAgent(m, newBlockingSource())
	if a.PolicyStatus() != nil {
		t.Fatal("PolicyStatus should be nil before first Reload")
	}
}

func TestAgent_PolicyStatus_AfterReload(t *testing.T) {
	m := &mockApplier{}
	a := newTestAgent(m, newBlockingSource())
	a.Reload(validBundle(3)) //nolint:errcheck
	st := a.PolicyStatus()
	if st == nil {
		t.Fatal("PolicyStatus should not be nil after Reload")
	}
	if st.Generation != 3 {
		t.Errorf("Generation: want 3, got %d", st.Generation)
	}
}

func TestAgent_Reload_MultipleGenerations(t *testing.T) {
	m := &mockApplier{}
	a := newTestAgent(m, newBlockingSource())

	for _, gen := range []int64{1, 2, 3} {
		if _, err := a.Reload(validBundle(gen)); err != nil {
			t.Fatalf("gen %d: %v", gen, err)
		}
	}
	if a.PolicyStatus().Generation != 3 {
		t.Errorf("final generation: want 3, got %d", a.PolicyStatus().Generation)
	}
	if m.callCount() != 3 {
		t.Errorf("PushRules call count: want 3, got %d", m.callCount())
	}
}

func TestAgent_Start_ContextCancel(t *testing.T) {
	m := &mockApplier{}
	a := newTestAgent(m, newBlockingSource())

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return after context cancel")
	}
}

func TestAgent_Start_DrainsEvents(t *testing.T) {
	m := &mockApplier{}
	events := []loader.LogEvent{
		{Proto: loader.ProtoTCP, Action: loader.ActionAllow},
		{Proto: loader.ProtoUDP, Action: loader.ActionDeny},
	}
	src := newBlockingSource(events...)

	buf := &logBuf{}
	a := &Agent{
		policy:    handler.NewPolicyHandler(m),
		log:       log.New(buf, "", 0),
		newReader: func() (eventSource, error) { return src, nil },
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()
	<-done

	if buf.lines() < 2 {
		t.Errorf("expected ≥2 log lines for 2 events, got %d", buf.lines())
	}
}

func TestAgent_Start_NonFatalReadError(t *testing.T) {
	// Non-os.ErrClosed error should be logged; goroutine exits; Start returns
	// after ctx cancel.
	m := &mockApplier{}
	readErr := errors.New("ring buffer overflow")
	src := newErrorSource(readErr)

	buf := &logBuf{}
	a := &Agent{
		policy:    handler.NewPolicyHandler(m),
		log:       log.New(buf, "", 0),
		newReader: func() (eventSource, error) { return src, nil },
	}

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- a.Start(ctx) }()

	// Give the event goroutine time to read and log the error before cancel.
	time.Sleep(50 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Start returned error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("Start did not return")
	}
	if buf.lines() == 0 {
		t.Error("expected error to be logged")
	}
}

func TestAgent_Start_ReaderFactoryError(t *testing.T) {
	m := &mockApplier{}
	factoryErr := errors.New("no ring buffer")
	a := &Agent{
		policy: handler.NewPolicyHandler(m),
		log:    log.New(io.Discard, "", 0),
		newReader: func() (eventSource, error) {
			return nil, factoryErr
		},
	}

	ctx := context.Background()
	err := a.Start(ctx)
	if !errors.Is(err, factoryErr) {
		t.Fatalf("want factory error, got %v", err)
	}
}
