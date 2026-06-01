// Package agent coordinates the BPF datapath lifecycle: policy reloads and
// event log draining.
package agent

import (
	"context"
	"crypto/ed25519"
	"errors"
	"fmt"
	"log"
	"log/slog"
	"net"
	"os"
	"strings"
	"sync"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"

	"github.com/qf/qf/agent/internal/grpcclient"
	"github.com/qf/qf/agent/internal/handler"
	"github.com/qf/qf/agent/internal/loader"
	"github.com/qf/qf/version"
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
	ldr       *loader.Loader // may be nil in tests
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
		ldr:    l,
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
// This is the simplified path used by tests and standalone operation.
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

// RunFullConfig holds parameters for the full gRPC-connected agent pipeline.
type RunFullConfig struct {
	GRPC      grpcclient.Config
	BundleKey ed25519.PublicKey // bundle-signing Ed25519 public key; must not be nil in production
	DiskBuf   *grpcclient.DiskBuffer
}

// RunFull runs the full gRPC-connected agent pipeline with reconnect loop.
// Blocks until ctx is cancelled or a fatal TLS error occurs.
func (a *Agent) RunFull(ctx context.Context, cfg RunFullConfig) error {
	return grpcclient.RunWithReconnect(
		ctx,
		grpcclient.DefaultBackoff(),
		func(ctx context.Context) (*grpcclient.Client, error) {
			return grpcclient.Dial(ctx, cfg.GRPC)
		},
		func(ctx context.Context, c *grpcclient.Client) error {
			return a.runSession(ctx, c, cfg)
		},
	)
}

func (a *Agent) runSession(ctx context.Context, c *grpcclient.Client, cfg RunFullConfig) error {
	stream, err := c.Stream(ctx)
	if err != nil {
		return err
	}

	currentGen := int64(0)
	if ar := a.policy.Current(); ar != nil {
		currentGen = ar.Generation
	}

	hostname, _ := os.Hostname()
	kernelBytes, _ := os.ReadFile("/proc/sys/kernel/osrelease")
	kernelVer := strings.TrimSpace(string(kernelBytes))

	netIfaces, _ := net.Interfaces()
	ifaceInfos := make([]*qfv1.InterfaceInfo, 0, len(netIfaces))
	for _, iface := range netIfaces {
		ifaceInfos = append(ifaceInfos, &qfv1.InterfaceInfo{
			Name:       iface.Name,
			AttachMode: qfv1.AttachMode_ATTACH_MODE_TCX,
		})
	}

	hs, err := grpcclient.Handshake(stream, &qfv1.Hello{
		AgentVersion:      version.Version,
		CurrentGeneration: currentGen,
		Hostname:          hostname,
		KernelVersion:     kernelVer,
		Interfaces:        ifaceInfos,
	})
	if err != nil {
		return err
	}

	if hs.Bundle != nil {
		if berr := grpcclient.HandleBundle(stream, hs.Bundle, cfg.BundleKey, a.applyFn); berr != nil {
			return berr
		}
	}

	if cfg.DiskBuf != nil {
		if rerr := cfg.DiskBuf.Replay(ctx, func(msg *qfv1.AgentMessage) error {
			return stream.Send(msg)
		}); rerr != nil && ctx.Err() != nil {
			return nil
		}
	}

	_ = grpcclient.SendSystemEvent(stream, grpcclient.EventCPConnected, qfv1.Severity_SEVERITY_INFO, "", nil)

	if currentGen == 0 {
		// First connect in this process lifetime.
		_ = grpcclient.SendSystemEvent(stream, grpcclient.EventAgentStarted,
			qfv1.Severity_SEVERITY_INFO, version.Version, nil)
	}

	er, err := a.ldr.NewEventReader()
	if err != nil {
		return fmt.Errorf("event reader: %w", err)
	}

	sctx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		er.Close()
	}()

	genFn := func() int64 {
		if ar := a.policy.Current(); ar != nil {
			return ar.Generation
		}
		return 0
	}

	hb := grpcclient.NewHeartbeatSender(stream, genFn, nil, 0)
	eb := grpcclient.NewEventBatcher(stream, er, a.ldr, 0, 0)
	if cfg.DiskBuf != nil {
		eb.SetDiskBuf(cfg.DiskBuf)
	}
	cp := grpcclient.NewCounterPoller(stream, a.ldr, a.policy, 0)
	fc := grpcclient.NewFlowEventCollector(stream, a.ldr, 0)
	cr := grpcclient.NewCertRotator(cfg.GRPC.CertFile, cfg.GRPC.KeyFile, stream.Send)

	if wc := hs.Welcome.GetConfig(); wc != nil {
		grpcclient.ApplyConfigUpdate(wc, hb, eb, cp, fc)
	}

	errCh := make(chan error, 4)
	go func() { errCh <- hb.Run(sctx) }()
	go func() { errCh <- eb.Run(sctx) }()
	go func() { errCh <- cp.Run(sctx) }()
	go func() {
		if ferr := fc.Run(sctx); ferr != nil {
			errCh <- ferr
		}
	}()
	go func() {
		if cerr := cr.Run(sctx); cerr == nil {
			// nil = cert rotated; signal reconnect with fresh TLS
			slog.Info("agent: cert rotated, reconnecting")
			cancel()
		}
	}()

	for {
		msg, err := stream.Recv()
		if err != nil {
			cancel()
			select {
			case goroutineErr := <-errCh:
				if goroutineErr != nil {
					return goroutineErr
				}
			default:
			}
			return fmt.Errorf("recv: %w", err)
		}

		switch p := msg.Payload.(type) {
		case *qfv1.ServerMessage_PolicyBundle:
			if berr := grpcclient.HandleBundle(stream, p.PolicyBundle, cfg.BundleKey, a.applyFn); berr != nil {
				cancel()
				return berr
			}

		case *qfv1.ServerMessage_ConfigUpdate:
			grpcclient.ApplyConfigUpdate(p.ConfigUpdate, hb, eb, cp, fc)

		case *qfv1.ServerMessage_DisconnectRequest:
			cancel()
			_ = grpcclient.SendSystemEvent(stream, grpcclient.EventCPDisconnected,
				qfv1.Severity_SEVERITY_INFO, p.DisconnectRequest.Reason, nil)
			waitMs := p.DisconnectRequest.ReconnectAfterMs
			if waitMs > 0 {
				select {
				case <-ctx.Done():
				case <-time.After(time.Duration(waitMs) * time.Millisecond):
				}
			}
			return nil

		case *qfv1.ServerMessage_CertRenewalResponse:
			cr.DeliverResponse(p.CertRenewalResponse)
		}
	}
}

func (a *Agent) applyFn(bundle *qfv1.PolicyBundle) (uint32, error) {
	ar, err := a.policy.Apply(bundle)
	if err != nil {
		return 0, err
	}
	return ar.DurationMs, nil
}
