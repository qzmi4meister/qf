package auth

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// APITokenMiddleware validates Bearer tokens against the api_tokens table.
// On success it injects Claims into context (same key as JWTMiddleware).
func APITokenMiddleware(q *storegen.Queries, tenantID pgtype.UUID) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			h := r.Header.Get("Authorization")
			if !strings.HasPrefix(h, "Bearer ") {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			raw := h[7:]

			hash := sha256Hex(raw)
			tok, err := q.GetAPITokenByHash(r.Context(), hash)
			if err != nil {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if tok.TenantID != tenantID {
				http.Error(w, "unauthorized", http.StatusUnauthorized)
				return
			}
			if tok.ExpiresAt.Valid && tok.ExpiresAt.Time.Before(time.Now()) {
				http.Error(w, "token expired", http.StatusUnauthorized)
				return
			}

			go q.UpdateAPITokenLastUsed(context.Background(), tok.ID) //nolint:errcheck

			claims := &Claims{
				UserID:   uuidStr(tok.CreatedBy),
				TenantID: uuidStr(tenantID),
				Role:     tok.Role,
			}
			ctx := context.WithValue(r.Context(), claimsKey, claims)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

func sha256Hex(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])
}
