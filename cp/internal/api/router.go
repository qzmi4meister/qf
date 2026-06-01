package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/auth"
	"github.com/qf/qf/cp/internal/pki"
	"github.com/qf/qf/cp/internal/pubsub"
	"github.com/qf/qf/cp/internal/policy"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// RouterConfig holds dependencies for NewRouter.
type RouterConfig struct {
	Queries     *storegen.Queries
	Tokens      *pki.TokenStore
	JWTSecret   []byte
	TenantID    pgtype.UUID
	OIDCHandler *auth.OIDCHandler   // nil = OIDC disabled
	OIDCEnabled bool
	Hub         *pubsub.Hub         // nil = SSE disabled
	Compiler    *policy.RulesetCompiler
}

// NewRouter builds the chi router with standard middleware and base routes.
func NewRouter(cfg RouterConfig) *chi.Mux {
	queries := cfg.Queries
	tokens := cfg.Tokens

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)
	r.Use(newAuditMiddleware(queries))

	r.Get("/healthz", handleHealthz)
	r.Get("/metrics", handleMetrics)

	// Auth endpoints — no JWT required
	authH := auth.NewHandler(queries, cfg.JWTSecret, cfg.TenantID)
	r.Post("/auth/login", authH.Login)
	r.Post("/auth/logout", authH.Logout)
	r.Post("/auth/refresh", authH.Refresh)
	r.Get("/auth/oidc/enabled", auth.OIDCEnabled(cfg.OIDCEnabled))
	if cfg.OIDCHandler != nil {
		r.Get("/auth/oidc/login", cfg.OIDCHandler.Login)
		r.Get("/auth/oidc/callback", cfg.OIDCHandler.Callback)
	}

	// Protected API — JWT or API token required
	jwtMW := auth.JWTMiddleware(cfg.JWTSecret)
	apiTokenMW := auth.APITokenMiddleware(queries, cfg.TenantID)
	authAny := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Try JWT cookie/header first, then API token Bearer.
			if c, _ := r.Cookie("qf_token"); c != nil {
				jwtMW(next).ServeHTTP(w, r)
				return
			}
			h := r.Header.Get("Authorization")
			if len(h) > 7 && h[:7] == "Bearer " {
				// Determine if it looks like a JWT (two dots) or opaque API token.
				tok := h[7:]
				dotCount := 0
				for _, ch := range tok {
					if ch == '.' {
						dotCount++
					}
				}
				if dotCount == 2 {
					jwtMW(next).ServeHTTP(w, r)
					return
				}
				apiTokenMW(next).ServeHTTP(w, r)
				return
			}
			http.Error(w, "unauthorized", http.StatusUnauthorized)
		})
	}

	r.Get("/auth/me", func(w http.ResponseWriter, r *http.Request) {
		jwtMW(http.HandlerFunc(authH.Me)).ServeHTTP(w, r)
	})

	r.Group(func(r chi.Router) {
		r.Use(authAny)

		r.Route("/hosts", func(r chi.Router) {
			registerHosts(r, queries)
			r.Route("/{id}", func(r chi.Router) {
				registerEvents(r, queries, cfg.Hub)
			})
		})
		r.Route("/policies", func(r chi.Router) { registerPolicies(r, queries, cfg.Compiler, cfg.TenantID) })
		r.Route("/objectgroups", func(r chi.Router) { registerObjectGroups(r, queries) })
		r.Route("/tokens", func(r chi.Router) { registerTokens(r, tokens) })
		r.Route("/default-policy", func(r chi.Router) { registerDefaultPolicy(r, queries) })
		r.Route("/audit-log", func(r chi.Router) { registerAuditLog(r, queries) })
		r.Route("/users", func(r chi.Router) { auth.RegisterUsers(r, queries, cfg.TenantID) })
		r.Route("/api-tokens", func(r chi.Router) { auth.RegisterAPITokens(r, queries, cfg.TenantID) })
	})

	return r
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
}

// structuredLogger logs each request via slog.
func structuredLogger(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		ww := middleware.NewWrapResponseWriter(w, r.ProtoMajor)
		next.ServeHTTP(ww, r)
		slog.Info("http",
			"method", r.Method,
			"path", r.URL.Path,
			"status", ww.Status(),
			"bytes", ww.BytesWritten(),
			"duration", time.Since(start),
			"request_id", middleware.GetReqID(r.Context()),
		)
	})
}
