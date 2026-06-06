package api

import (
	"bytes"
	"context"
	"net/http"
	"regexp"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// auditMiddleware records POST/PUT/PATCH/DELETE mutations to audit_log.
// Actor identity comes from X-Actor-ID + X-Actor-Type headers.
// "after" is captured from the response body; "before" is not queried
// at middleware level (requires explicit before-capture in handlers).
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

		ww := &captureWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(ww, r)

		if ww.status < 200 || ww.status >= 300 {
			return
		}

		tenantRaw := r.Header.Get("X-Tenant-ID")
		var tenantUUID pgtype.UUID
		if err := tenantUUID.Scan(tenantRaw); err != nil {
			return
		}

		actorType := r.Header.Get("X-Actor-Type")
		if actorType == "" {
			actorType = "api_token"
		}
		var actorUUID pgtype.UUID
		_ = actorUUID.Scan(r.Header.Get("X-Actor-ID"))

		objectType, objectUUID := extractObjectFromPath(r.URL.Path)
		action := r.Method + " " + r.URL.Path

		var after []byte
		if ww.body.Len() > 0 && r.Method != http.MethodDelete {
			after = ww.body.Bytes()
		}

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
				Before:     nil,
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
// Examples:
//
//	/hosts/uuid      → ("host", uuid)
//	/policies/uuid   → ("policy", uuid)
//	/tokens          → ("token", pgtype.UUID{})
func extractObjectFromPath(path string) (objectType string, objectID pgtype.UUID) {
	parts := strings.Split(strings.Trim(path, "/"), "/")
	if len(parts) == 0 {
		return "unknown", pgtype.UUID{}
	}

	// Map first path segment to object type.
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

	// Walk parts in reverse to find the last UUID segment.
	for i := len(parts) - 1; i >= 0; i-- {
		if uuidSegment.MatchString(parts[i]) {
			_ = objectID.Scan(parts[i])
			// Refine object type for nested resources.
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
