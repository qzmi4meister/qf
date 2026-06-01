package grpcclient

import (
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"

	qfv1 "github.com/qf/qf/proto/qf/v1"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

// EnrollResult holds the credentials returned by EnrollmentService.Enroll.
type EnrollResult struct {
	// CPEndpoint is the agent stream endpoint returned by CP (may differ from enrollAddr).
	CPEndpoint string
}

// Enroll dials the enrollment endpoint (plain gRPC, no mTLS), sends an EnrollRequest
// with the given bootstrap token and a freshly generated CSR, and atomically writes
// the issued credentials to pkiDir:
//
//   - agent.key           (PKCS8 PEM, 0600)
//   - agent.crt           (PEM, 0644)
//   - ca.crt              (PEM, 0644)
//   - bundle-signing.pub  (PEM, 0644) — written only when CP returns it
//
// pkiDir must already exist. Files are written atomically via a temp-file rename.
func Enroll(ctx context.Context, enrollAddr, token, hostname, pkiDir string) (*EnrollResult, error) {
	conn, err := grpc.NewClient(enrollAddr, grpc.WithTransportCredentials(insecure.NewCredentials()))
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
