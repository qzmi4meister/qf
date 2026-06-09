package grpcclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials"
)

// EnrollResult holds the credentials returned by EnrollmentService.Enroll.
type EnrollResult struct {
	// CPEndpoint is the agent stream endpoint returned by CP (may differ from enrollAddr).
	CPEndpoint string
}

// Enroll dials the enrollment endpoint over TLS, sends an EnrollRequest
// with the given bootstrap token and a freshly generated CSR, and atomically writes
// the issued credentials to pkiDir:
//
//   - agent.key           (PKCS8 PEM, 0600)
//   - agent.crt           (PEM, 0644)
//   - ca.crt              (PEM, 0644)
//   - bundle-signing.pub  (PEM, 0644) — written only when CP returns it
//
// caFile is the path to the CA cert PEM for verifying the enrollment server.
// When empty, the system trust store is used. If the enrollment server uses a
// self-signed CA not in the system store, point caFile at the CP CA cert.
// pkiDir must already exist. Files are written atomically via a temp-file rename.
func Enroll(ctx context.Context, enrollAddr, token, hostname, pkiDir, caFile string) (*EnrollResult, error) {
	tlsCfg := &tls.Config{MinVersion: tls.VersionTLS13}

	if caFile != "" {
		caPEM, err := os.ReadFile(caFile)
		if err != nil {
			return nil, fmt.Errorf("enroll: read CA file %s: %w", caFile, err)
		}
		pool := x509.NewCertPool()
		if !pool.AppendCertsFromPEM(caPEM) {
			return nil, fmt.Errorf("enroll: no valid certs in %s", caFile)
		}
		tlsCfg.RootCAs = pool
	} else {
		slog.Warn("enroll: QF_ENROLL_CA not set, using system trust store; " +
			"set QF_ENROLL_CA to the CP CA cert path for full verification")
	}

	conn, err := grpc.NewClient(enrollAddr, grpc.WithTransportCredentials(credentials.NewTLS(tlsCfg)))
	if err != nil {
		return nil, fmt.Errorf("enroll: dial %s: %w", enrollAddr, err)
	}
	defer conn.Close()

	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return nil, fmt.Errorf("enroll: generate key: %w", err)
	}

	csrPEM, err := buildCSR(key, pkix.Name{CommonName: hostname})
	if err != nil {
		return nil, fmt.Errorf("enroll: build CSR: %w", err)
	}

	svc := qfv1.NewEnrollmentServiceClient(conn)
	resp, err := svc.Enroll(ctx, &qfv1.EnrollRequest{
		BootstrapToken: token,
		CsrPem:         string(csrPEM),
		Hostname:       hostname,
	})
	if err != nil {
		return nil, fmt.Errorf("enroll: RPC: %w", err)
	}

	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return nil, fmt.Errorf("enroll: marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	writes := []struct {
		name string
		data []byte
		mode os.FileMode
	}{
		{"agent.key", keyPEM, 0600},
		{"agent.crt", []byte(resp.CertPem), 0644},
		{"ca.crt", []byte(resp.CaPem), 0644},
	}
	for _, w := range writes {
		if err := atomicWrite(filepath.Join(pkiDir, w.name), w.data, w.mode); err != nil {
			return nil, fmt.Errorf("enroll: save %s: %w", w.name, err)
		}
	}
	if resp.BundleSigningPubPem != "" {
		if err := atomicWrite(filepath.Join(pkiDir, "bundle-signing.pub"),
			[]byte(resp.BundleSigningPubPem), 0644); err != nil {
			return nil, fmt.Errorf("enroll: save bundle-signing.pub: %w", err)
		}
	}

	return &EnrollResult{CPEndpoint: resp.CpEndpoint}, nil
}
