package pki

import (
	"context"
	"crypto/x509"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// HostEnroller is called after a host is enrolled to trigger policy cascade.
type HostEnroller interface {
	OnHostEnrolled(ctx context.Context, tenantID, hostID string) error
}

const defaultCertTTL = 365 * 24 * time.Hour

// EnrollmentServer implements qfv1.EnrollmentServer.
type EnrollmentServer struct {
	qfv1.UnimplementedEnrollmentServiceServer
	ca         *CA
	signer     *BundleSigner
	tokens     *TokenStore
	queries    *storegen.Queries
	cpEndpoint string
	certTTL    time.Duration
	cascade    HostEnroller // may be nil
}

// NewEnrollmentServer creates an EnrollmentServer.
// cpEndpoint is returned to the agent so it knows where to connect for the main stream.
// cascade may be nil (no initial bundle computation on enrollment).
func NewEnrollmentServer(ca *CA, signer *BundleSigner, tokens *TokenStore, queries *storegen.Queries, cpEndpoint string, cascade HostEnroller) *EnrollmentServer {
	return &EnrollmentServer{
		ca:         ca,
		signer:     signer,
		tokens:     tokens,
		queries:    queries,
		cpEndpoint: cpEndpoint,
		certTTL:    defaultCertTTL,
		cascade:    cascade,
	}
}

// Enroll validates the bootstrap token, signs the agent's CSR, and returns credentials.
func (s *EnrollmentServer) Enroll(ctx context.Context, req *qfv1.EnrollRequest) (*qfv1.EnrollResponse, error) {
	if req.BootstrapToken == "" {
		return nil, status.Error(codes.InvalidArgument, "bootstrap_token required")
	}
	if req.CsrPem == "" {
		return nil, status.Error(codes.InvalidArgument, "csr_pem required")
	}
	if req.Hostname == "" {
		return nil, status.Error(codes.InvalidArgument, "hostname required")
	}

	bt, err := s.tokens.ValidateAndConsume(ctx, req.BootstrapToken)
	if err != nil {
		if err == ErrTokenNotFound {
			return nil, status.Error(codes.Unauthenticated, "invalid or expired token")
		}
		if err == ErrTokenExhausted {
			return nil, status.Error(codes.ResourceExhausted, "token max uses reached")
		}
		return nil, status.Errorf(codes.Internal, "token validation: %v", err)
	}

	hostID, err := s.resolveHost(ctx, bt, req)
	if err != nil {
		return nil, err
	}

	csrDER, err := parsePEMBlock(req.CsrPem, "CERTIFICATE REQUEST")
	if err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "csr_pem: %v", err)
	}

	serial, err := randSerial()
	if err != nil {
		return nil, status.Errorf(codes.Internal, "generate serial: %v", err)
	}

	certPEM, err := s.ca.SignHostCSR(csrDER, hostID, bt.TenantID, serial, s.certTTL)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "sign CSR: %v", err)
	}

	cert, err := parseCertPEM(certPEM)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "parse signed cert: %v", err)
	}

	var tenantUUID, hostUUID pgtype.UUID
	if err := tenantUUID.Scan(bt.TenantID); err != nil {
		return nil, status.Errorf(codes.Internal, "tenant uuid: %v", err)
	}
	if err := hostUUID.Scan(hostID); err != nil {
		return nil, status.Errorf(codes.Internal, "host uuid: %v", err)
	}

	_, err = s.queries.InsertCertificate(ctx, storegen.InsertCertificateParams{
		TenantID:  tenantUUID,
		HostID:    hostUUID,
		Serial:    serial.String(),
		NotBefore: pgtype.Timestamptz{Time: cert.NotBefore, Valid: true},
		NotAfter:  pgtype.Timestamptz{Time: cert.NotAfter, Valid: true},
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "save certificate: %v", err)
	}

	_, err = s.queries.UpdateHostStatus(ctx, storegen.UpdateHostStatusParams{
		ID:       hostUUID,
		TenantID: tenantUUID,
		Status:   "active",
	})
	if err != nil {
		return nil, status.Errorf(codes.Internal, "update host status: %v", err)
	}

	if s.cascade != nil {
		if cerr := s.cascade.OnHostEnrolled(ctx, bt.TenantID, hostID); cerr != nil {
			slog.Warn("enrollment: initial bundle computation failed", "host", hostID, "err", cerr)
		}
	}

	go func() {
		actx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		_, _ = s.queries.InsertAuditLog(actx, storegen.InsertAuditLogParams{
			TenantID:   tenantUUID,
			ActorType:  "agent",
			ObjectType: "host",
			ObjectID:   hostUUID,
			Action:     "agent.enrolled",
		})
	}()

	bundlePubPEM, err := marshalPublicKeyPEM(s.signer.PublicKey)
	if err != nil {
		return nil, status.Errorf(codes.Internal, "marshal bundle pub key: %v", err)
	}

	return &qfv1.EnrollResponse{
		CertPem:            string(certPEM),
		CaPem:              string(s.ca.CertPEM),
		BundleSigningPubPem: string(bundlePubPEM),
		HostId:             hostID,
		TenantId:           bt.TenantID,
		CpEndpoint:         s.cpEndpoint,
		CertNotAfter:       timestamppb.New(cert.NotAfter),
	}, nil
}

// resolveHost returns the hostID for the enrollment:
// - single_host: validates hostname matches target host, returns target host ID
// - bulk: creates a new host record, returns its ID
func (s *EnrollmentServer) resolveHost(ctx context.Context, bt *BootstrapToken, req *qfv1.EnrollRequest) (string, error) {
	var tenantUUID pgtype.UUID
	if err := tenantUUID.Scan(bt.TenantID); err != nil {
		return "", status.Errorf(codes.Internal, "tenant uuid: %v", err)
	}

	switch bt.Type {
	case "single_host":
		if bt.TargetHostID == nil {
			return "", status.Error(codes.Internal, "single_host token missing target_host_id")
		}
		var hostUUID pgtype.UUID
		if err := hostUUID.Scan(*bt.TargetHostID); err != nil {
			return "", status.Errorf(codes.Internal, "host uuid: %v", err)
		}
		host, err := s.queries.GetHost(ctx, storegen.GetHostParams{
			ID:       hostUUID,
			TenantID: tenantUUID,
		})
		if err != nil {
			return "", status.Errorf(codes.NotFound, "target host not found: %v", err)
		}
		if host.Hostname != req.Hostname {
			return "", status.Errorf(codes.PermissionDenied,
				"hostname mismatch: token targets %q, got %q", host.Hostname, req.Hostname)
		}
		return *bt.TargetHostID, nil

	case "bulk":
		labelsJSON := []byte("{}")
		if len(bt.LabelTemplate) > 0 {
			b, _ := json.Marshal(bt.LabelTemplate)
			labelsJSON = b
		}
		host, err := s.queries.UpsertBulkHost(ctx, storegen.UpsertBulkHostParams{
			TenantID: tenantUUID,
			Hostname: req.Hostname,
			Labels:   labelsJSON,
			Status:   "enrolling",
		})
		if err != nil {
			return "", status.Errorf(codes.Internal, "create host: %v", err)
		}
		return pgUUIDToString(host.ID), nil

	default:
		return "", status.Errorf(codes.Internal, "unknown token type: %s", bt.Type)
	}
}

func parsePEMBlock(pemStr, expectedType string) ([]byte, error) {
	block, _ := pem.Decode([]byte(pemStr))
	if block == nil {
		return nil, fmt.Errorf("no PEM block found")
	}
	if block.Type != expectedType {
		return nil, fmt.Errorf("expected %q, got %q", expectedType, block.Type)
	}
	return block.Bytes, nil
}

func parseCertPEM(certPEM []byte) (*x509.Certificate, error) {
	block, _ := pem.Decode(certPEM)
	if block == nil {
		return nil, fmt.Errorf("no PEM block")
	}
	return x509.ParseCertificate(block.Bytes)
}

// pgUUIDToString converts pgtype.UUID bytes to standard UUID string format.
func pgUUIDToString(u pgtype.UUID) string {
	b := u.Bytes
	return fmt.Sprintf("%x-%x-%x-%x-%x", b[0:4], b[4:6], b[6:8], b[8:10], b[10:])
}

func marshalPublicKeyPEM(pub interface{}) ([]byte, error) {
	der, err := x509.MarshalPKIXPublicKey(pub)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "PUBLIC KEY", Bytes: der}), nil
}
