package api

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	authpkg "github.com/qf/qf/cp/internal/auth"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// auditBeforeVal is a writable slot injected into context by auditMiddleware.
// Handlers call SetAuditBefore to populate it before applying changes.
type auditBeforeVal struct{ data []byte }

type auditCtxKey struct{}

// SetAuditBefore records the current resource state for the audit log.
// Call from PUT/PATCH handlers before applying changes.
func SetAuditBefore(ctx context.Context, data []byte) {
	if v, ok := ctx.Value(auditCtxKey{}).(*auditBeforeVal); ok {
		v.data = data
	}
}

// auditMiddleware records POST/PUT/PATCH/DELETE mutations to audit_log.
// Actor identity is read from JWT/API-token Claims in context.
// "after" is captured from the response body; "before" is populated by handlers
// via SetAuditBefore before applying changes.
type auditMiddleware struct {
	q *storegen.Queries
}

func newAuditMiddleware(q *storegen.Queries) func(http.Handler) http.Handler {
	am := &auditMiddleware{q: q}
	return am.wrap
}

func (am *auditMiddleware) wrap(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet || r.Method == http.MethodHead || r.Method == http.MethodOptions {
			next.ServeHTTP(w, r)
			return
		}

		beforeVal := &auditBeforeVal{}
		r = r.WithContext(context.WithValue(r.Context(), auditCtxKey{}, beforeVal))

		ww := &captureWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		if ww.status < 200 || ww.status >= 300 {
			return
		}

		claims := authpkg.ClaimsFromCtx(r.Context())
		if claims == nil {
			return
		}

		var tenantUUID pgtype.UUID
		if err := tenantUUID.Scan(claims.TenantID); err != nil {
			return
		}

		actorType := "api_token"
		if claims.Email != "" {
			actorType = "user"
		}
		var actorUUID pgtype.UUID
		_ = actorUUID.Scan(claims.UserID)

		objectType, objectUUID := extractObjectFromPath(r.URL.Path)
		action := r.Method + " " + r.URL.Path

		var after []byte
		if ww.body.Len() > 0 && r.Method != http.MethodDelete {
			after = ww.body.Bytes()
		}

		before := beforeVal.data

		go func() {
			ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
			defer cancel()
			_, _ = am.q.InsertAuditLog(ctx, storegen.InsertAuditLogParams{
				TenantID:   tenantUUID,
				ActorType:  actorType,
				ActorID:    actorUUID,
				Action:     action,
				ObjectType: objectType,
				ObjectID:   objectUUID,
				Before:     before,
				After:      after,
			})
		}()
	})
}

// captureWriter records the status code and response body for audit logging.
type captureWriter struct {
	http.ResponseWriter
	status int
	body   bytes.Buffer
}

func (cw *captureWriter) WriteHeader(code int) {
	cw.status = code
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *captureWriter) Write(b []byte) (int, error) {
	cw.body.Write(b)
	return cw.ResponseWriter.Write(b)
}

// uuidSegment matches a lowercase UUID (with hyphens).
var uuidSegment = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$`)

// extractObjectFromPath returns the object type name and UUID from a URL path.
func extractObjectFromPath(path string) (objectType string, objectID pgtype.UUID) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "unknown", pgtype.UUID{}
	}

	typeMap := map[string]string{
		"hosts":          "host",
		"policies":       "policy",
		"objectgroups":   "objectgroup",
		"tokens":         "token",
		"default-policy": "defaultpolicy",
		"audit-log":      "audit_log",
	}
	objectType = typeMap[parts[0]]
	if objectType == "" {
		objectType = parts[0]
	}

	for i := len(parts) - 1; i >= 0; i-- {
		if uuidSegment.MatchString(parts[i]) {
			_ = objectID.Scan(parts[i])
			if i > 0 {
				if sub, ok := typeMap[parts[i-1]]; ok {
					objectType = sub
				}
			}
			return
		}
	}
	return
}
