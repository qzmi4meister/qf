package agentsrv

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"

	"github.com/qf/qf/cp/internal/pki"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// BundleProvider returns the current compiled bundle for a host.
// Implemented by the bundle compiler (COMP-04); stubbed until then.
type BundleProvider interface {
	GetBundle(ctx context.Context, tenantID, hostID string) (*qfv1.PolicyBundle, error)
}

// AgentServer implements qfv1.AgentServiceServer.
type AgentServer struct {
	qfv1.UnimplementedAgentServiceServer
	queries  *storegen.Queries
	bundles  BundleProvider
	registry *StreamRegistry
	ca       *pki.CA
	rc       *pki.RevocationChecker
	version  string
}

// NewAgentServer creates an AgentServer.
func NewAgentServer(queries *storegen.Queries, bundles BundleProvider, registry *StreamRegistry, ca *pki.CA, rc *pki.RevocationChecker, version string) *AgentServer {
	return &AgentServer{queries: queries, bundles: bundles, registry: registry, ca: ca, rc: rc, version: version}
}

// NewMTLSServer builds a gRPC server with mutual TLS and registers AgentServer.
// Returns the server and the StreamRegistry (implements policy.Dispatcher).
func NewMTLSServer(serverCert tls.Certificate, ca *pki.CA, rc *pki.RevocationChecker, queries *storegen.Queries, bundles BundleProvider, registry *StreamRegistry, ver string) (*grpc.Server, error) {
	pool := x509.NewCertPool()
	if !pool.AppendCertsFromPEM(ca.CertPEM) {
		return nil, fmt.Errorf("agentsrv: failed to load CA cert into pool")
	}
	tlsCfg := &tls.Config{
		Certificates: []tls.Certificate{serverCert},
		ClientCAs:    pool,
		ClientAuth:   tls.RequireAndVerifyClientCert,
		MinVersion:   tls.VersionTLS13,
	}
	tlsCfg = rc.WrapTLSConfig(tlsCfg)

	srv := grpc.NewServer(
		grpc.Creds(credentials.NewTLS(tlsCfg)),
		grpc.StreamInterceptor(StreamAuthInterceptor),
	)
	qfv1.RegisterAgentServiceServer(srv, NewAgentServer(queries, bundles, registry, ca, rc, ver))
	return srv, nil
}
