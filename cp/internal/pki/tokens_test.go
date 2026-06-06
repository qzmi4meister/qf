package pki

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// connectTestDB opens a pgxpool using QF_TEST_DSN.
// Skips the test if the env var is not set.
func connectTestDB(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dsn := os.Getenv("QF_TEST_DSN")
	if dsn == "" {
		t.Skip("QF_TEST_DSN not set; set it to a PostgreSQL DSN with the qf schema")
	}
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// insertTestToken inserts a raw bootstrap_token row for testing.
func insertTestToken(t *testing.T, pool *pgxpool.Pool, tenantID, tokenHash string, maxUses int, ttl time.Duration) {
	t.Helper()
	_, err := pool.Exec(context.Background(), `
		INSERT INTO bootstrap_tokens
			(tenant_id, type, token_hash, label_template, max_uses, expires_at)
		VALUES ((SELECT id FROM tenants LIMIT 1), 'bulk', $1, '{}', $2, $3)`,
		tokenHash, maxUses, time.Now().Add(ttl),
	)
	if err != nil {
		t.Fatalf("insertTestToken: %v", err)
	}
}

// deleteTestToken cleans up a token by its hash.
func deleteTestToken(t *testing.T, pool *pgxpool.Pool, tokenHash string) {
	t.Helper()
	pool.Exec(context.Background(), `DELETE FROM bootstrap_tokens WHERE token_hash = $1`, tokenHash) //nolint:errcheck
}

// TestTokenStore_UnlimitedToken verifies that max_uses=0 (unlimited) allows
// repeated consumption. Previously broken by: AND uses_count < max_uses
// (0 < 0 = false → token always rejected).
func TestTokenStore_UnlimitedToken(t *testing.T) {
	pool := connectTestDB(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	plain, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	insertTestToken(t, pool, "", hash, 0, time.Hour) // max_uses=0 = unlimited
	t.Cleanup(func() { deleteTestToken(t, pool, hash) })

	// Consume three times — all should succeed.
	for i := 0; i < 3; i++ {
		bt, err := ts.ValidateAndConsume(ctx, plain)
		if err != nil {
			t.Fatalf("attempt %d: ValidateAndConsume failed: %v", i+1, err)
		}
		if bt == nil {
			t.Fatalf("attempt %d: got nil BootstrapToken", i+1)
		}
	}
}

// TestTokenStore_SingleUse_IsRaceFree verifies that concurrent ValidateAndConsume
// calls on a single-use token (max_uses=1) result in exactly one success.
// The UPDATE ... WHERE (max_uses=0 OR uses_count < max_uses) is atomic at the
// DB level; only one concurrent caller gets the row.
func TestTokenStore_SingleUse_IsRaceFree(t *testing.T) {
	pool := connectTestDB(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	plain, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	insertTestToken(t, pool, "", hash, 1, time.Hour) // max_uses=1 = single-use
	t.Cleanup(func() { deleteTestToken(t, pool, hash) })

	const concurrency = 20
	successes := 0
	var mu sync.Mutex
	var wg sync.WaitGroup

	for i := 0; i < concurrency; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, err := ts.ValidateAndConsume(ctx, plain)
			if err == nil {
				mu.Lock()
				successes++
				mu.Unlock()
			}
		}()
	}
	wg.Wait()

	if successes != 1 {
		t.Errorf("single-use token: want exactly 1 successful consumer, got %d", successes)
	}
}

// TestTokenStore_ExpiredToken verifies that an expired token is rejected.
func TestTokenStore_ExpiredToken(t *testing.T) {
	pool := connectTestDB(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	plain, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	// Insert with TTL in the past.
	_, insertErr := pool.Exec(ctx, `
		INSERT INTO bootstrap_tokens
			(tenant_id, type, token_hash, label_template, max_uses, expires_at)
		VALUES ((SELECT id FROM tenants LIMIT 1), 'bulk', $1, '{}', 10, $2)`,
		hash, time.Now().Add(-time.Second),
	)
	if insertErr != nil {
		t.Fatalf("insert expired token: %v", insertErr)
	}
	t.Cleanup(func() { deleteTestToken(t, pool, hash) })

	_, err = ts.ValidateAndConsume(ctx, plain)
	if err != ErrTokenNotFound {
		t.Errorf("want ErrTokenNotFound for expired token, got %v", err)
	}
}

// TestTokenStore_ExhaustedToken verifies that a token at max_uses is rejected
// with ErrTokenExhausted.
func TestTokenStore_ExhaustedToken(t *testing.T) {
	pool := connectTestDB(t)
	ts := NewTokenStore(pool)
	ctx := context.Background()

	plain, hash, err := generateToken()
	if err != nil {
		t.Fatalf("generateToken: %v", err)
	}
	// Insert with max_uses=1, uses_count=1 (already consumed).
	_, insertErr := pool.Exec(ctx, `
		INSERT INTO bootstrap_tokens
			(tenant_id, type, token_hash, label_template, max_uses, uses_count, expires_at)
		VALUES ((SELECT id FROM tenants LIMIT 1), 'bulk', $1, '{}', 1, 1, $2)`,
		hash, time.Now().Add(time.Hour),
	)
	if insertErr != nil {
		t.Fatalf("insert exhausted token: %v", insertErr)
	}
	t.Cleanup(func() { deleteTestToken(t, pool, hash) })

	_, err = ts.ValidateAndConsume(ctx, plain)
	if err != ErrTokenExhausted {
		t.Errorf("want ErrTokenExhausted, got %v (insert ok)", fmt.Sprint(err))
	}
}
