package pki

import (
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/tls"
	"crypto/x509"
	"crypto/x509/pkix"
	"math/big"
	"sync"
	"testing"
	"time"
)

// TestRevocationChecker_RevokeAndCheck verifies that Revoke immediately adds
// a serial to the in-memory blocklist and IsRevoked detects it.
func TestRevocationChecker_RevokeAndCheck(t *testing.T) {
	rc := &RevocationChecker{revoked: make(map[string]struct{})}

	if rc.IsRevoked("abc123") {
		t.Fatal("IsRevoked should return false for unknown serial")
	}
	rc.Revoke("abc123")
	if !rc.IsRevoked("abc123") {
		t.Fatal("IsRevoked should return true after Revoke")
	}
	if rc.IsRevoked("xyz999") {
		t.Fatal("IsRevoked should return false for unrelated serial")
	}
}

// TestRevocationChecker_ConcurrentAccess runs concurrent Revoke and IsRevoked
// calls to surface data races when run with -race.
func TestRevocationChecker_ConcurrentAccess(t *testing.T) {
	rc := &RevocationChecker{revoked: make(map[string]struct{})}

	var wg sync.WaitGroup
	serials := []string{"s1", "s2", "s3", "s4", "s5"}

	for _, s := range serials {
		s := s
		wg.Add(2)
		go func() {
			defer wg.Done()
			rc.Revoke(s)
		}()
		go func() {
			defer wg.Done()
			_ = rc.IsRevoked(s)
		}()
	}
	wg.Wait()

	for _, s := range serials {
		if !rc.IsRevoked(s) {
			t.Errorf("serial %s should be revoked", s)
		}
	}
}

// TestRevocationChecker_VerifyPeerCertificate verifies that a revoked cert is
// rejected by the TLS verification hook and a non-revoked cert is accepted.
func TestRevocationChecker_VerifyPeerCertificate(t *testing.T) {
	// Self-signed cert for test purposes.
	key, err := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	if err != nil {
		t.Fatalf("generate key: %v", err)
	}
	serial := big.NewInt(12345)
	tmpl := &x509.Certificate{
		SerialNumber: serial,
		Subject:      pkix.Name{CommonName: "test"},
		NotBefore:    time.Now().Add(-time.Minute),
		NotAfter:     time.Now().Add(time.Hour),
	}
	certDER, err := x509.CreateCertificate(rand.Reader, tmpl, tmpl, &key.PublicKey, key)
	if err != nil {
		t.Fatalf("create cert: %v", err)
	}

	rc := &RevocationChecker{revoked: make(map[string]struct{})}

	// Non-revoked cert: must pass.
	if err := rc.VerifyPeerCertificate([][]byte{certDER}, nil); err != nil {
		t.Errorf("non-revoked cert should pass: %v", err)
	}

	// Revoke and check rejection.
	rc.Revoke(serial.String())
	if err := rc.VerifyPeerCertificate([][]byte{certDER}, nil); err == nil {
		t.Error("revoked cert should be rejected")
	}
}

// TestRevocationChecker_WrapTLSConfig verifies that WrapTLSConfig sets the
// VerifyPeerCertificate hook in the returned config.
func TestRevocationChecker_WrapTLSConfig(t *testing.T) {
	rc := &RevocationChecker{revoked: make(map[string]struct{})}

	base := &tls.Config{MinVersion: tls.VersionTLS13}
	wrapped := rc.WrapTLSConfig(base)

	if wrapped.VerifyPeerCertificate == nil {
		t.Error("WrapTLSConfig must set VerifyPeerCertificate")
	}
	// Original must be unchanged.
	if base.VerifyPeerCertificate != nil {
		t.Error("WrapTLSConfig must not mutate the input config")
	}
	if wrapped.MinVersion != tls.VersionTLS13 {
		t.Error("WrapTLSConfig must copy other TLS settings")
	}
}
