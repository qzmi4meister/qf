package pki

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

var (
	ErrTokenNotFound = errors.New("pki: token not found or expired")
	ErrTokenExhausted = errors.New("pki: token max uses reached")
)

// BootstrapToken is a row from bootstrap_tokens.
type BootstrapToken struct {
	ID             string
	TenantID       string
	Type           string // "single_host" | "bulk"
	TargetHostID   *string
	LabelTemplate  map[string]string
	MaxUses        int
	UsesCount      int
	ExpiresAt      time.Time
}

// TokenStore manages bootstrap tokens.
type TokenStore struct {
	pool *pgxpool.Pool
}

// NewTokenStore creates a TokenStore backed by pool.
func NewTokenStore(pool *pgxpool.Pool) *TokenStore {
	return &TokenStore{pool: pool}
}

// CreateSingleHostToken generates a single-host bootstrap token for targetHostID.
// Returns the plaintext token (caller must deliver it once; not stored).
func (ts *TokenStore) CreateSingleHostToken(ctx context.Context, tenantID, targetHostID string, ttl time.Duration, maxUses int) (string, error) {
	plain, hash, err := generateToken()
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(ttl)
	_, err = ts.pool.Exec(ctx, `
		INSERT INTO bootstrap_tokens
			(tenant_id, type, token_hash, target_host_id, max_uses, expires_at)
		VALUES ($1, 'single_host', $2, $3, $4, $5)`,
		tenantID, hash, targetHostID, maxUses, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("pki: create single_host token: %w", err)
	}
	return plain, nil
}

// CreateBulkToken generates a bulk bootstrap token with an optional label template.
// labelTemplate is applied to enrolled hosts (e.g. {"env":"prod"}).
func (ts *TokenStore) CreateBulkToken(ctx context.Context, tenantID string, labelTemplate map[string]string, ttl time.Duration, maxUses int) (string, error) {
	plain, hash, err := generateToken()
	if err != nil {
		return "", err
	}
	tmplJSON, err := json.Marshal(labelTemplate)
	if err != nil {
		return "", err
	}
	expiresAt := time.Now().Add(ttl)
	_, err = ts.pool.Exec(ctx, `
		INSERT INTO bootstrap_tokens
			(tenant_id, type, token_hash, label_template, max_uses, expires_at)
		VALUES ($1, 'bulk', $2, $3, $4, $5)`,
		tenantID, hash, tmplJSON, maxUses, expiresAt,
	)
	if err != nil {
		return "", fmt.Errorf("pki: create bulk token: %w", err)
	}
	return plain, nil
}

// ValidateAndConsume validates the plaintext token, increments uses_count atomically,
// and returns the token record. Returns ErrTokenNotFound or ErrTokenExhausted on failure.
func (ts *TokenStore) ValidateAndConsume(ctx context.Context, plain string) (*BootstrapToken, error) {
	hash := hashToken(plain)

	row := ts.pool.QueryRow(ctx, `
		UPDATE bootstrap_tokens
		SET    uses_count = uses_count + 1
		WHERE  token_hash = $1
		  AND  expires_at > NOW()
		  AND  (max_uses = 0 OR uses_count < max_uses)
		RETURNING id, tenant_id, type, target_host_id, label_template, max_uses, uses_count, expires_at`,
		hash,
	)

	var bt BootstrapToken
	var labelJSON []byte
	err := row.Scan(
		&bt.ID, &bt.TenantID, &bt.Type,
		&bt.TargetHostID, &labelJSON,
		&bt.MaxUses, &bt.UsesCount, &bt.ExpiresAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		// Distinguish exhausted from not-found: check if token exists at all.
		var exists bool
		_ = ts.pool.QueryRow(ctx,
			`SELECT EXISTS(SELECT 1 FROM bootstrap_tokens WHERE token_hash=$1)`, hash,
		).Scan(&exists)
		if exists {
			return nil, ErrTokenExhausted
		}
		return nil, ErrTokenNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("pki: validate token: %w", err)
	}

	if len(labelJSON) > 0 {
		_ = json.Unmarshal(labelJSON, &bt.LabelTemplate)
	}
	return &bt, nil
}

// DeleteToken removes a token by ID.
func (ts *TokenStore) DeleteToken(ctx context.Context, id string) error {
	_, err := ts.pool.Exec(ctx, `DELETE FROM bootstrap_tokens WHERE id = $1`, id)
	return err
}

// ListTokens returns all non-expired tokens for a tenant.
func (ts *TokenStore) ListTokens(ctx context.Context, tenantID string) ([]*BootstrapToken, error) {
	rows, err := ts.pool.Query(ctx, `
		SELECT id, tenant_id, type, target_host_id, label_template,
		       max_uses, uses_count, expires_at
		FROM   bootstrap_tokens
		WHERE  tenant_id = $1 AND expires_at > NOW()
		ORDER  BY created_at DESC`,
		tenantID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var tokens []*BootstrapToken
	for rows.Next() {
		var bt BootstrapToken
		var labelJSON []byte
		if err := rows.Scan(
			&bt.ID, &bt.TenantID, &bt.Type,
			&bt.TargetHostID, &labelJSON,
			&bt.MaxUses, &bt.UsesCount, &bt.ExpiresAt,
		); err != nil {
			return nil, err
		}
		if len(labelJSON) > 0 {
			_ = json.Unmarshal(labelJSON, &bt.LabelTemplate)
		}
		tokens = append(tokens, &bt)
	}
	return tokens, rows.Err()
}

// generateToken returns (plaintext, sha256-hex-hash).
func generateToken() (string, string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	plain := base64.RawURLEncoding.EncodeToString(b)
	return plain, hashToken(plain), nil
}

func hashToken(plain string) string {
	sum := sha256.Sum256([]byte(plain))
	return hex.EncodeToString(sum[:])
}
