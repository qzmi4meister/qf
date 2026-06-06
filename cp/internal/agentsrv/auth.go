package agentsrv

import (
	"context"
	"fmt"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/peer"
	"google.golang.org/grpc/status"
)

type ctxKeyPeerIdentity struct{}

// PeerIdentity holds the host and tenant IDs extracted from the mTLS client cert.
type PeerIdentity struct {
	HostID   string
	TenantID string
}

// PeerIdentityFromContext retrieves the PeerIdentity injected by the stream interceptor.
func PeerIdentityFromContext(ctx context.Context) (*PeerIdentity, error) {
	id, ok := ctx.Value(ctxKeyPeerIdentity{}).(*PeerIdentity)
	if !ok || id == nil {
		return nil, fmt.Errorf("agentsrv: no peer identity in context")
	}
	return id, nil
}

// extractPeerIdentity reads CN and OU from the mTLS peer certificate.
// CN = host_id, OU[0] = tenant_id (set by CA.SignHostCSR).
func extractPeerIdentity(ctx context.Context) (*PeerIdentity, error) {
	p, ok := peer.FromContext(ctx)
	if !ok {
		return nil, fmt.Errorf("agentsrv: no peer info in context")
	}
	tlsInfo, ok := p.AuthInfo.(credentials.TLSInfo)
	if !ok {
		return nil, fmt.Errorf("agentsrv: peer has no TLS info")
	}
	certs := tlsInfo.State.PeerCertificates
	if len(certs) == 0 {
		return nil, fmt.Errorf("agentsrv: no peer certificates")
	}
	cert := certs[0]
	hostID := cert.Subject.CommonName
	if hostID == "" {
		return nil, fmt.Errorf("agentsrv: cert CN (host_id) is empty")
	}
	ous := cert.Subject.OrganizationalUnit
	if len(ous) == 0 || ous[0] == "" {
		return nil, fmt.Errorf("agentsrv: cert OU (tenant_id) is empty")
	}
	return &PeerIdentity{HostID: hostID, TenantID: ous[0]}, nil
}

// StreamAuthInterceptor extracts PeerIdentity from the mTLS cert and injects it into ctx.
func StreamAuthInterceptor(srv interface{}, ss grpc.ServerStream, _ *grpc.StreamServerInfo, handler grpc.StreamHandler) error {
	id, err := extractPeerIdentity(ss.Context())
	if err != nil {
		return status.Errorf(codes.Unauthenticated, "%v", err)
	}
	wrapped := &wrappedStream{ss, context.WithValue(ss.Context(), ctxKeyPeerIdentity{}, id)}
	return handler(srv, wrapped)
}

// wrappedStream replaces the context of a grpc.ServerStream.
type wrappedStream struct {
	grpc.ServerStream
	ctx context.Context
}

func (w *wrappedStream) Context() context.Context { return w.ctx }
