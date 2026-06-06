package pki

import (
	"context"
	"crypto/tls"
	"crypto/x509"
	"fmt"
	"sync"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// RevocationChecker maintains an in-memory set of revoked certificate serials.
// Loaded from the certificates table at startup and refreshed on demand.
type RevocationChecker struct {
	mu      sync.RWMutex
	revoked map[string]struct{} // serial → struct{}
	pool    *pgxpool.Pool
}

// NewRevocationChecker creates a checker and loads the current revocation list.
func NewRevocationChecker(ctx context.Context, pool *pgxpool.Pool) (*RevocationChecker, error) {
	rc := &RevocationChecker{
		revoked: make(map[string]struct{}),
		pool:    pool,
	}
	if err := rc.Reload(ctx); err != nil {
		return nil, err
	}
	return rc, nil
}

// Reload refreshes the in-memory list from PostgreSQL.
// Fetches serials of certs that are revoked and not yet expired.
func (rc *RevocationChecker) Reload(ctx context.Context) error {
	rows, err := rc.pool.Query(ctx, `
		SELECT serial FROM certificates
		WHERE  status = 'revoked' AND not_after > NOW()`)
	if err != nil {
		return fmt.Errorf("pki: reload revocation list: %w", err)
	}
	defer rows.Close()

	fresh := make(map[string]struct{})
	for rows.Next() {
		var serial string
		if err := rows.Scan(&serial); err != nil {
			return err
		}
		fresh[serial] = struct{}{}
	}
	if err := rows.Err(); err != nil {
		return err
	}

	rc.mu.Lock()
	rc.revoked = fresh
	rc.mu.Unlock()
	return nil
}

// Revoke adds a serial to the in-memory blocklist immediately (without a DB round-trip).
// The caller is responsible for updating the DB row status separately.
func (rc *RevocationChecker) Revoke(serial string) {
	rc.mu.Lock()
	rc.revoked[serial] = struct{}{}
	rc.mu.Unlock()
}

// IsRevoked reports whether serial is in the blocklist.
func (rc *RevocationChecker) IsRevoked(serial string) bool {
	rc.mu.RLock()
	_, ok := rc.revoked[serial]
	rc.mu.RUnlock()
	return ok
}

// StartPeriodicReload spawns a goroutine that reloads every interval until ctx is done.
func (rc *RevocationChecker) StartPeriodicReload(ctx context.Context, interval time.Duration) {
	go func() {
		t := time.NewTicker(interval)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				_ = rc.Reload(ctx)
			}
		}
	}()
}

// VerifyPeerCertificate returns a tls.Config.VerifyPeerCertificate function
// that rejects certificates whose serial is in the blocklist.
func (rc *RevocationChecker) VerifyPeerCertificate(rawCerts [][]byte, _ [][]*x509.Certificate) error {
	for _, raw := range rawCerts {
		cert, err := x509.ParseCertificate(raw)
		if err != nil {
			return fmt.Errorf("pki: parse peer cert: %w", err)
		}
		serial := cert.SerialNumber.String()
		if rc.IsRevoked(serial) {
			return fmt.Errorf("pki: certificate %s is revoked", serial)
		}
	}
	return nil
}

// WrapTLSConfig returns a copy of cfg with VerifyPeerCertificate set to the revocation check.
func (rc *RevocationChecker) WrapTLSConfig(cfg *tls.Config) *tls.Config {
	out := cfg.Clone()
	out.VerifyPeerCertificate = rc.VerifyPeerCertificate
	return out
}
