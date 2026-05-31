// Package agent coordinates the BPF datapath lifecycle: policy reloads and
// event log draining.
package agent

import (
	"context"
	"errors"
	"fmt"
	"log"
	"os"
	"sync"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/handler"
	"github.com/qf/qf/agent/internal/loader"
)

// eventSource is the read side of the BPF ring buffer. Implemented by
// *loader.EventReader in production; mockable in tests.
type eventSource interface {
	Read() (loader.LogEvent, error)
	Close() error
}

// Agent coordinates the BPF datapath: applies policy bundles and drains the
// event ring buffer. Safe for concurrent use.
type Agent struct {
	policy    *handler.PolicyHandler
	log       *log.Logger
	newReader func() (eventSource, error)
}

// New creates an Agent backed by a loaded BPF program. If logger is nil,
// output goes to os.Stderr.
func New(l *loader.Loader, logger *log.Logger) *Agent {
	if logger == nil {
		logger = log.New(os.Stderr, "[qf] ", log.LstdFlags)
	}
	a := &Agent{
		policy: handler.NewPolicyHandler(l),
		log:    logger,
	}
	a.newReader = func() (eventSource, error) {
		return l.NewEventReader()
	}
	return a
}

// Reload compiles bundle and atomically pushes new rules into the BPF
// datapath. On failure the previous generation stays active.
func (a *Agent) Reload(bundle *qfv1.PolicyBundle) (*handler.ApplyResult, error) {
	return a.policy.Apply(bundle)
}

// PolicyStatus returns the result of the most recent successful Reload, or
// nil if no bundle has been applied yet.
func (a *Agent) PolicyStatus() *handler.ApplyResult {
	return a.policy.Current()
}

// Start drains BPF log events until ctx is cancelled. Blocks until the event
// goroutine has fully stopped. Returns nil on normal shutdown.
func (a *Agent) Start(ctx context.Context) error {
	er, err := a.newReader()
	if err != nil {
		return fmt.Errorf("event reader: %w", err)
	}

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		a.drainEvents(er)
	}()

	<-ctx.Done()
	er.Close()
	wg.Wait()
	return nil
}

func (a *Agent) drainEvents(src eventSource) {
	for {
		ev, err := src.Read()
		if err != nil {
			if !errors.Is(err, os.ErrClosed) {
				a.log.Printf("event read error: %v", err)
			}
			return
		}
		a.log.Printf("evt rule=%.4x src=%s:%d dst=%s:%d proto=%d action=%d ct=%d",
			ev.RuleID[:4], ev.SrcIP, ev.SrcPort, ev.DstIP, ev.DstPort,
			ev.Proto, ev.Action, ev.CTState)
	}
}
