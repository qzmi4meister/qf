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
	"log/slog"
	"os"
	"path/filepath"
	"time"

	qfv1 "github.com/qf/qf/proto/qf/v1"
)

const renewalResponseTimeout = 30 * time.Second

// CertRotator monitors cert lifetime and renews at 50% threshold.
// Renewal sends CertRenewalRequest on the stream; the main receive loop
// must call DeliverResponse when it sees a CertRenewalResponse message.
type CertRotator struct {
	certFile   string
	keyFile    string
	sendFn     func(*qfv1.AgentMessage) error
	responseCh chan *qfv1.CertRenewalResponse
}

// NewCertRotator creates a CertRotator.
// sendFn is stream.Send; certFile/keyFile are paths to the agent's current cert and key.
func NewCertRotator(certFile, keyFile string, sendFn func(*qfv1.AgentMessage) error) *CertRotator {
	return &CertRotator{
		certFile:   certFile,
		keyFile:    keyFile,
		sendFn:     sendFn,
		responseCh: make(chan *qfv1.CertRenewalResponse, 1),
	}
}

// DeliverResponse is called by the main receive loop when CP sends CertRenewalResponse.
func (r *CertRotator) DeliverResponse(resp *qfv1.CertRenewalResponse) {
	select {
	case r.responseCh <- resp:
	default: // no pending request, discard
	}
}

// Run blocks until renewal is needed, performs it, then returns.
// Returning nil signals the caller to reconnect with fresh TLS credentials.
// Returning non-nil means ctx was cancelled or a fatal error occurred.
func (r *CertRotator) Run(ctx context.Context) error {
	cert, err := r.loadCert()
	if err != nil {
		return fmt.Errorf("certrotate: load cert: %w", err)
	}

	threshold := renewalThreshold(cert.NotBefore, cert.NotAfter)
	wait := time.Until(threshold)
	if wait < 0 {
		wait = 0
	}
	slog.Info("certrotate: renewal scheduled", "at", threshold.UTC().Format(time.RFC3339), "in", wait.Round(time.Second))

	select {
	case <-ctx.Done():
		return nil
	case <-time.After(wait):
	}

	slog.Info("certrotate: starting renewal")
	return r.renew(ctx, cert)
}

func (r *CertRotator) renew(ctx context.Context, current *x509.Certificate) error {
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		return fmt.Errorf("certrotate: generate key: %w", err)
	}

	csrPEM, err := buildCSR(key, current.Subject)
	if err != nil {
		return fmt.Errorf("certrotate: build CSR: %w", err)
	}

	if err := r.sendFn(&qfv1.AgentMessage{
		Payload: &qfv1.AgentMessage_CertRenewalRequest{
			CertRenewalRequest: &qfv1.CertRenewalRequest{CsrPem: string(csrPEM)},
		},
	}); err != nil {
		return fmt.Errorf("certrotate: send request: %w", err)
	}

	// Wait for response from main receive loop.
	ctx2, cancel := context.WithTimeout(ctx, renewalResponseTimeout)
	defer cancel()
	select {
	case <-ctx2.Done():
		return fmt.Errorf("certrotate: timeout waiting for response")
	case resp := <-r.responseCh:
		if !resp.Success {
			return fmt.Errorf("certrotate: CP rejected renewal: %s", resp.ErrorMessage)
		}
		if err := r.saveCreds(resp.CertPem, key); err != nil {
			return fmt.Errorf("certrotate: save creds: %w", err)
		}
		slog.Info("certrotate: renewed cert saved",
			"not_after", resp.NotAfter.AsTime().UTC().Format(time.RFC3339))
		// Return nil → caller reconnects with new cert.
		return nil
	}
}

func (r *CertRotator) loadCert() (*x509.Certificate, error) {
	pemData, err := os.ReadFile(r.certFile)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(pemData)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", r.certFile)
	}
	return x509.ParseCertificate(block.Bytes)
}

func renewalThreshold(notBefore, notAfter time.Time) time.Time {
	lifetime := notAfter.Sub(notBefore)
	return notBefore.Add(lifetime / 2)
}

func buildCSR(key *ecdsa.PrivateKey, subject pkix.Name) ([]byte, error) {
	tmpl := &x509.CertificateRequest{Subject: subject}
	csrDER, err := x509.CreateCertificateRequest(rand.Reader, tmpl, key)
	if err != nil {
		return nil, err
	}
	return pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE REQUEST", Bytes: csrDER}), nil
}

func (r *CertRotator) saveCreds(certPEM string, key *ecdsa.PrivateKey) error {
	keyDER, err := x509.MarshalPKCS8PrivateKey(key)
	if err != nil {
		return fmt.Errorf("marshal key: %w", err)
	}
	keyPEM := pem.EncodeToMemory(&pem.Block{Type: "PRIVATE KEY", Bytes: keyDER})

	if err := atomicWrite(r.certFile, []byte(certPEM), 0600); err != nil {
		return fmt.Errorf("write cert: %w", err)
	}
	if err := atomicWrite(r.keyFile, keyPEM, 0600); err != nil {
		return fmt.Errorf("write key: %w", err)
	}
	return nil
}

func atomicWrite(path string, data []byte, mode os.FileMode) error {
	dir := filepath.Dir(path)
	tmp, err := os.CreateTemp(dir, ".qf-cert-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Chmod(mode); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return err
	}
	return nil
}
