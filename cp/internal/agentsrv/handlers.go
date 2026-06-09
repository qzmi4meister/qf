package agentsrv

import (
	"context"
	"crypto/x509"
	"encoding/pem"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/pki"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

const staleThreshold = 3 * time.Minute

// handleHeartbeat updates last_heartbeat_at, current_generation, and agent/kernel version.
// Stale detection: if host was stale before this heartbeat, it implicitly recovers
// because last_heartbeat_at is refreshed; a background job (MAIN-01) will re-check.
func (s *AgentServer) handleHeartbeat(ctx context.Context, id *PeerIdentity, hb *qfv1.Heartbeat) error {
	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		return status.Errorf(codes.Internal, "parse identity: %v", err)
	}

	if err := s.queries.UpdateHostHeartbeat(ctx, storegen.UpdateHostHeartbeatParams{
		ID:                hostUUID,
		TenantID:          tenantUUID,
		CurrentGeneration: int32(hb.CurrentGeneration),
	}); err != nil {
		// Heartbeat is best-effort; a transient DB error must not tear down the stream.
		slog.Warn("agentsrv: heartbeat DB update failed", "host", id.HostID, "err", err)
		return nil
	}

	if hb.Ts != nil {
		lag := time.Since(hb.Ts.AsTime())
		if lag > staleThreshold {
			slog.Warn("agentsrv: stale heartbeat received",
				"host", id.HostID, "lag", lag.Round(time.Second))
		}
	}
	return nil
}

// handleBundleAck logs the agent's pre-apply acknowledgment.
func (s *AgentServer) handleBundleAck(_ context.Context, id *PeerIdentity, ack *qfv1.BundleAck) error {
	if !ack.SignatureVerified {
		slog.Warn("agentsrv: agent reported bad bundle signature",
			"host", id.HostID, "generation", ack.Generation, "reason", ack.ErrorMessage)
	}
	return nil
}

// handleBundleApplied updates host.current_generation on success; logs failure.
func (s *AgentServer) handleBundleApplied(ctx context.Context, id *PeerIdentity, applied *qfv1.BundleApplied) error {
	if !applied.Success {
		slog.Error("agentsrv: agent failed to apply bundle",
			"host", id.HostID,
			"generation", applied.Generation,
			"err", applied.ErrorMessage,
			"duration_ms", applied.DurationMs)
		return nil
	}

	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		return status.Errorf(codes.Internal, "parse identity: %v", err)
	}

	if err := s.queries.UpdateHostGeneration(ctx, storegen.UpdateHostGenerationParams{
		ID:                hostUUID,
		TenantID:          tenantUUID,
		CurrentGeneration: int32(applied.Generation),
	}); err != nil {
		slog.Error("agentsrv: failed to update host generation",
			"host", id.HostID, "generation", applied.Generation, "err", err)
		return status.Errorf(codes.Internal, "update generation: %v", err)
	}

	slog.Info("agentsrv: bundle applied",
		"host", id.HostID,
		"generation", applied.Generation,
		"duration_ms", applied.DurationMs)
	return nil
}

const certRenewalTTL = 365 * 24 * time.Hour

// handleCertRenewal validates the current cert, signs the CSR, saves the new cert,
// marks the old one as rotated, and sends CertRenewalResponse on the stream.
func (s *AgentServer) handleCertRenewal(ctx context.Context, stream qfv1.AgentService_StreamServer, id *PeerIdentity, req *qfv1.CertRenewalRequest) error {
	sendResp := func(resp *qfv1.CertRenewalResponse) error {
		return stream.Send(&qfv1.ServerMessage{
			Payload: &qfv1.ServerMessage_CertRenewalResponse{CertRenewalResponse: resp},
		})
	}
	fail := func(msg string) error {
		slog.Warn("agentsrv: cert renewal failed", "host", id.HostID, "reason", msg)
		return sendResp(&qfv1.CertRenewalResponse{Success: false, ErrorMessage: msg})
	}

	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		return fail("parse identity")
	}

	// Verify current cert is active and not revoked.
	oldCert, err := s.queries.GetActiveCertificateForHost(ctx, hostUUID)
	if err != nil {
		return fail("no active cert found")
	}
	if s.rc != nil && s.rc.IsRevoked(oldCert.Serial) {
		return fail("current cert is revoked")
	}

	// Parse CSR.
	block, _ := pem.Decode([]byte(req.CsrPem))
	if block == nil || block.Type != "CERTIFICATE REQUEST" {
		return fail("invalid CSR PEM")
	}

	// Sign new cert.
	serial, err := pki.RandSerial()
	if err != nil {
		return fail("serial generation failed")
	}
	newCertDER, err := s.ca.SignHostCSR(block.Bytes, id.HostID, id.TenantID, serial, certRenewalTTL)
	if err != nil {
		return fail("sign CSR: " + err.Error())
	}
	newCert, err := x509.ParseCertificate(newCertDER)
	if err != nil {
		return fail("parse signed cert")
	}

	// Persist new cert.
	if _, err := s.queries.InsertCertificate(ctx, storegen.InsertCertificateParams{
		TenantID:  tenantUUID,
		HostID:    hostUUID,
		Serial:    serial.String(),
		NotBefore: pgtype.Timestamptz{Time: newCert.NotBefore, Valid: true},
		NotAfter:  pgtype.Timestamptz{Time: newCert.NotAfter, Valid: true},
	}); err != nil {
		return fail("save cert: " + err.Error())
	}

	// Mark old cert as rotated (non-fatal if it fails).
	if err := s.queries.UpdateCertificateStatus(ctx, storegen.UpdateCertificateStatusParams{
		Serial: oldCert.Serial,
		Status: "rotated",
	}); err != nil {
		slog.Warn("agentsrv: mark old cert rotated failed", "host", id.HostID, "err", err)
	}

	certPEM := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: newCertDER})
	slog.Info("agentsrv: cert rotated", "host", id.HostID, "serial", serial.String(),
		"not_after", newCert.NotAfter.UTC().Format(time.RFC3339))

	return sendResp(&qfv1.CertRenewalResponse{
		Success:  true,
		CertPem:  string(certPEM),
		NotAfter: timestamppb.New(newCert.NotAfter),
	})
}

// handleLogEvents forwards a log event batch to the ingester (best-effort).
func (s *AgentServer) handleLogEvents(id *PeerIdentity, msg *qfv1.LogEvents) {
	if s.ingester == nil {
		return
	}
	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		slog.Warn("agentsrv: LogEvents parse identity failed", "host", id.HostID, "err", err)
		return
	}
	s.ingester.IngestLogEvents(tenantUUID, hostUUID, msg)
}

// handleFlowEvents forwards a flow event batch to the ingester (best-effort).
func (s *AgentServer) handleFlowEvents(id *PeerIdentity, msg *qfv1.FlowEvents) {
	if s.ingester == nil {
		return
	}
	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		slog.Warn("agentsrv: FlowEvents parse identity failed", "host", id.HostID, "err", err)
		return
	}
	s.ingester.IngestFlowEvents(tenantUUID, hostUUID, msg)
}

// handleCounterUpdate forwards counter snapshots to the ingester (best-effort).
func (s *AgentServer) handleCounterUpdate(id *PeerIdentity, msg *qfv1.CounterUpdate) {
	if s.ingester == nil {
		return
	}
	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		slog.Warn("agentsrv: CounterUpdate parse identity failed", "host", id.HostID, "err", err)
		return
	}
	s.ingester.IngestCounterUpdate(tenantUUID, hostUUID, msg)
}

// handleSystemEvent forwards a system event to the ingester (best-effort).
func (s *AgentServer) handleSystemEvent(id *PeerIdentity, msg *qfv1.SystemEvent) {
	if s.ingester == nil {
		return
	}
	hostUUID, tenantUUID, err := parseUUIDs(id.HostID, id.TenantID)
	if err != nil {
		slog.Warn("agentsrv: SystemEvent parse identity failed", "host", id.HostID, "err", err)
		return
	}
	s.ingester.IngestSystemEvent(tenantUUID, hostUUID, msg)
}

func parseUUIDs(hostID, tenantID string) (pgtype.UUID, pgtype.UUID, error) {
	var h, t pgtype.UUID
	if err := h.Scan(hostID); err != nil {
		return h, t, err
	}
	if err := t.Scan(tenantID); err != nil {
		return h, t, err
	}
	return h, t, nil
}
