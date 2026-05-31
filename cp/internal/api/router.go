package api

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/go-chi/chi/v5/middleware"
	"github.com/qf/qf/cp/internal/pki"
	storegen "github.com/qf/qf/cp/internal/store/gen"
)

// NewRouter builds the chi router with standard middleware and base routes.
func NewRouter(queries *storegen.Queries, tokens *pki.TokenStore) *chi.Mux {
	r := chi.NewRouter()

	r.Use(middleware.RequestID)
	r.Use(structuredLogger)
	r.Use(middleware.Recoverer)

	r.Get("/healthz", handleHealthz)
	r.Route("/hosts", func(r chi.Router) { registerHosts(r, queries) })
	r.Route("/policies", func(r chi.Router) { registerPolicies(r, queries) })
	r.Route("/objectgroups", func(r chi.Router) { registerObjectGroups(r, queries) })
	r.Route("/tokens", func(r chi.Router) { registerTokens(r, tokens) })
	r.Route("/default-policy", func(r chi.Router) { registerDefaultPolicy(r, queries) })

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
