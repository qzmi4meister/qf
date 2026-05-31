package agentsrv

import (
	"errors"
	"io"
	"log/slog"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// defaultConfig is sent in Welcome and applied on the agent until overridden.
var defaultConfig = &qfv1.ConfigUpdate{
	HeartbeatIntervalMs:       30_000,
	CounterReportIntervalMs:   60_000,
	EventBatchSize:            100,
	EventBatchMaxAgeMs:        5_000,
	DefaultLogRateLimitPerSec: 100,
}

// Stream handles the bidirectional agent<->CP stream.
func (s *AgentServer) Stream(stream qfv1.AgentService_StreamServer) error {
	id, err := PeerIdentityFromContext(stream.Context())
	if err != nil {
		return status.Error(codes.Unauthenticated, "missing peer identity")
	}

	// ── Hello ────────────────────────────────────────────────────────────────
	first, err := stream.Recv()
	if err != nil {
		return streamErr(err)
	}
	hello := first.GetHello()
	if hello == nil {
		return status.Error(codes.InvalidArgument, "first message must be Hello")
	}

	// Register stream for push dispatch and disconnect; deregister on exit.
	// disconnectCh is nil when registry is absent — nil channel blocks in select (never fires).
	var disconnectCh chan disconnectSignal
	if s.registry != nil {
		disconnectCh = s.registry.register(id.HostID, stream)
		defer s.registry.deregister(id.HostID)
	}

	ctx := stream.Context()

	// Look up server-side generation for this host.
	var hostUUID pgtype.UUID
	if err := hostUUID.Scan(id.HostID); err != nil {
		return status.Errorf(codes.Internal, "invalid host_id in cert: %v", err)
	}
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(id.TenantID); err != nil {
		return status.Errorf(codes.Internal, "invalid tenant_id in cert: %v", err)
	}

	host, err := s.queries.GetHost(ctx, storegen.GetHostParams{
		ID:       hostUUID,
		TenantID: tenantUUID,
	})
	if err != nil {
		return status.Errorf(codes.NotFound, "host not found: %v", err)
	}

	serverGen := int64(host.CurrentGeneration)

	// ── Welcome ──────────────────────────────────────────────────────────────
	if err := stream.Send(&qfv1.ServerMessage{
		Payload: &qfv1.ServerMessage_Welcome{
			Welcome: &qfv1.Welcome{
				Accepted:          true,
				ServerGeneration:  serverGen,
				ServerVersion:     s.version,
				Config:            defaultConfig,
			},
		},
	}); err != nil {
		return err
	}

	// ── Push bundle if agent is behind ───────────────────────────────────────
	if hello.CurrentGeneration < serverGen && s.bundles != nil {
		bundle, err := s.bundles.GetBundle(ctx, id.TenantID, id.HostID)
		if err != nil {
			slog.Warn("agentsrv: failed to get bundle for catch-up",
				"host", id.HostID, "err", err)
		} else if bundle != nil {
			if err := stream.Send(&qfv1.ServerMessage{
				Payload: &qfv1.ServerMessage_PolicyBundle{
					PolicyBundle: bundle,
				},
			}); err != nil {
				return err
			}
		}
	}

	// ── Main receive loop ────────────────────────────────────────────────────
	type recvResult struct {
		msg *qfv1.AgentMessage
		err error
	}
	recvCh := make(chan recvResult, 1)
	go func() {
		for {
			msg, err := stream.Recv()
			recvCh <- recvResult{msg, err}
			if err != nil {
				return
			}
		}
	}()

	for {
		select {
		case r := <-recvCh:
			if r.err != nil {
				return streamErr(r.err)
			}
			if err := s.handleMessage(stream, id, r.msg); err != nil {
				return err
			}
		case sig := <-disconnectCh:
			_ = stream.Send(&qfv1.ServerMessage{
				Payload: &qfv1.ServerMessage_DisconnectRequest{
					DisconnectRequest: &qfv1.DisconnectRequest{
						Reason:           sig.reason,
						ReconnectAfterMs: sig.reconnectAfterMs,
					},
				},
			})
			slog.Info("agentsrv: disconnecting host", "host", id.HostID, "reason", sig.reason)
			return nil
		}
	}
}

// handleMessage dispatches incoming AgentMessages.
// GRPC-03..06 fill in the cases; unknown messages are silently dropped.
func (s *AgentServer) handleMessage(stream qfv1.AgentService_StreamServer, id *PeerIdentity, msg *qfv1.AgentMessage) error {
	switch p := msg.Payload.(type) {
	case *qfv1.AgentMessage_Heartbeat:
		return s.handleHeartbeat(id, p.Heartbeat)
	case *qfv1.AgentMessage_BundleAck:
		return s.handleBundleAck(stream.Context(), id, p.BundleAck)
	case *qfv1.AgentMessage_BundleApplied:
		return s.handleBundleApplied(stream.Context(), id, p.BundleApplied)
	case *qfv1.AgentMessage_CertRenewalRequest:
		return s.handleCertRenewal(stream.Context(), stream, id, p.CertRenewalRequest)
	default:
		// Telemetry, cert renewal, etc. handled in later tasks.
		return nil
	}
}

// streamErr converts gRPC/io stream errors to appropriate gRPC status errors.
func streamErr(err error) error {
	if errors.Is(err, io.EOF) {
		return nil
	}
	if status.Code(err) == codes.Canceled {
		return nil
	}
	return err
}
