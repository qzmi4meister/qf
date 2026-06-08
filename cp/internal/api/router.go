package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/qf/qf/cp/internal/auth"
	"github.com/qf/qf/cp/internal/embeddedui"
	"github.com/qf/qf/cp/internal/pki"
	"github.com/qf/qf/cp/internal/policy"
	"github.com/qf/qf/cp/internal/pubsub"
	"github.com/qf/qf/cp/internal/ws"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	"github.com/qf/qf/docs"
	"github.com/qf/qf/version"
)

// Disconnector can request an active agent stream to disconnect and reconnect.
type Disconnector interface {
	Disconnect(hostID, reason string, reconnectAfterMs uint32)
}

// RouterConfig holds dependencies for NewRouter.
type RouterConfig struct {
	Queries      *storegen.Queries
	Tokens       *pki.TokenStore
	JWTSecret    []byte
	TenantID     pgtype.UUID
	OIDCHandler  *auth.OIDCHandler         // nil = OIDC disabled
	OIDCEnabled  bool
	Hub          *pubsub.Hub               // nil = SSE disabled
	WSHub        *ws.Hub                   // nil = WS notifications disabled
	Compiler     *policy.RulesetCompiler
	Cascade      *policy.CascadeRecompiler // nil = push disabled
	Disconnector Disconnector              // nil = no live reconnect
}

// NewRouter builds the chi router with standard middleware and base routes.
func NewRouter(cfg RouterConfig) *chi.Mux {
	queries := cfg.Queries
	tokens := cfg.Tokens

	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)
	r.Use(newAuditMiddleware(queries, cfg.WSHub))

	r.Get("/healthz", handleHealthz)
	r.Get("/version", handleVersion)
	r.Get("/metrics", handleMetrics)
	r.Get("/openapi.yaml", handleOpenAPI)
	r.Get("/docs", handleSwaggerUI)

	// Embedded UI — served at /app/* with HSTS header.
	uiHandler := embeddedui.FileServer()
	r.Get("/", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/app/", http.StatusFound)
	})
	r.Group(func(r chi.Router) {
		r.Use(hstsMiddleware)
		r.Handle("/app", http.RedirectHandler("/app/", http.StatusFound))
		r.Handle("/app/*", http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			r.URL.Path = strings.TrimPrefix(r.URL.Path, "/app")
			uiHandler.ServeHTTP(w, r)
		}))
	})

	// Auth endpoints — no JWT required
	authH := auth.NewHandler(queries, cfg.JWTSecret, cfg.TenantID)
	loginRL := newLoginRateLimiter()
	r.With(loginRL.Middleware).Post("/auth/login", authH.Login)
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
	r.Post("/auth/change-password", func(w http.ResponseWriter, r *http.Request) {
		jwtMW(http.HandlerFunc(authH.ChangePassword)).ServeHTTP(w, r)
	})

	// WebSocket — authenticated, serves invalidation notifications to UI.
	if cfg.WSHub != nil {
		r.Group(func(r chi.Router) {
			r.Use(authAny)
			r.Get("/ws", cfg.WSHub.ServeHTTP)
		})
	}

	r.Group(func(r chi.Router) {
		r.Use(authAny)

		r.Route("/hosts", func(r chi.Router) {
			registerHosts(r, queries, cfg.Cascade, cfg.Compiler, cfg.Disconnector)
			registerEvents(r, queries, cfg.Hub)
		})
		r.Route("/policies", func(r chi.Router) { registerPolicies(r, queries, cfg.Compiler, cfg.Cascade, cfg.TenantID) })
		r.Route("/objectgroups", func(r chi.Router) { registerObjectGroups(r, queries, cfg.Cascade) })
		r.Route("/tokens", func(r chi.Router) { registerTokens(r, tokens) })
		r.Route("/default-policy", func(r chi.Router) { registerDefaultPolicy(r, queries, cfg.Cascade) })
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

func handleVersion(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(map[string]string{"version": version.Version})
}

func handleOpenAPI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/yaml")
	w.Header().Set("Cache-Control", "public, max-age=300")
	w.Write(docs.OpenAPISpec) //nolint:errcheck
}

const swaggerUIHTML = `<!DOCTYPE html>
<html>
<head>
  <title>qf API</title>
  <meta charset="utf-8"/>
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <link rel="stylesheet" href="https://unpkg.com/swagger-ui-dist/swagger-ui.css">
</head>
<body>
<div id="swagger-ui"></div>
<script src="https://unpkg.com/swagger-ui-dist/swagger-ui-bundle.js"></script>
<script>
window.onload = function() {
  SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIBundle.SwaggerUIStandalonePreset],
    layout: "BaseLayout",
    tryItOutEnabled: true,
  });
};
</script>
</body>
</html>`

func handleSwaggerUI(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write([]byte(swaggerUIHTML)) //nolint:errcheck
}

// hstsMiddleware sets Strict-Transport-Security for HTTPS enforcement.
func hstsMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Strict-Transport-Security", "max-age=31536000; includeSubDomains")
		next.ServeHTTP(w, r)
	})
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
