package main

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/qf/qf/cp/internal/agentsrv"
	"github.com/qf/qf/cp/internal/api"
	"github.com/qf/qf/cp/internal/auth"
	"github.com/qf/qf/cp/internal/forwarder"
	"github.com/qf/qf/cp/internal/ingest"
	"github.com/qf/qf/cp/internal/metrics"
	"github.com/qf/qf/cp/internal/pki"
	"github.com/qf/qf/cp/internal/policy"
	"github.com/qf/qf/cp/internal/pubsub"
	"github.com/qf/qf/cp/internal/store"
	storegen "github.com/qf/qf/cp/internal/store/gen"
	qfv1 "github.com/qf/qf/proto/qf/v1"
	"github.com/qf/qf/version"
	"google.golang.org/grpc"
)

func main() {
	if err := run(); err != nil {
		slog.Error("startup failed", "err", err)
		os.Exit(1)
	}
}

func run() error {
	cfg := loadConfig()
	initLogger(cfg.logLevel)

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	// ── Database ──────────────────────────────────────────────────────────────
	pool, err := pgxpool.New(ctx, cfg.dbDSN)
	if err != nil {
		return fmt.Errorf("pgxpool: %w", err)
	}
	defer pool.Close()

	// Run goose migrations via database/sql.
	sqlDB := stdlib.OpenDBFromPool(pool)
	if err := store.Migrate(ctx, sqlDB); err != nil {
		return fmt.Errorf("migrate: %w", err)
	}
	sqlDB.Close()

	queries := storegen.New(pool)

	// ── Telemetry ingester + pub/sub hub ─────────────────────────────────────
	hub := pubsub.NewHub()
	var ing *ingest.Ingester
	if dsn := os.Getenv("QF_FORWARDER_DSN"); dsn != "" {
		fwd, err := forwarder.Open(dsn)
		if err != nil {
			return fmt.Errorf("forwarder: %w", err)
		}
		defer fwd.Close()
		slog.Info("log events forwarded to external sink", "dsn", dsn)
		ing = ingest.NewWithForwarder(queries, fwd, hub)
	} else {
		ing = ingest.New(queries, hub)
	}
	go ing.Start(ctx)

	pm := ingest.NewPartitionManager(pool)
	go pm.Start(ctx)

	// Periodically update DB pool metrics for Prometheus.
	go func() {
		t := time.NewTicker(15 * time.Second)
		defer t.Stop()
		for {
			select {
			case <-t.C:
				s := pool.Stat()
				metrics.DBPoolAcquired.Set(float64(s.AcquiredConns()))
				metrics.DBPoolIdle.Set(float64(s.IdleConns()))
			case <-ctx.Done():
				return
			}
		}
	}()

	// ── PKI ───────────────────────────────────────────────────────────────────
	masterKey, err := hex.DecodeString(cfg.masterKeyHex)
	if err != nil || len(masterKey) != 32 {
		return fmt.Errorf("QF_MASTER_KEY must be 32-byte hex (64 chars)")
	}
	if isLowEntropy(masterKey) {
		return fmt.Errorf("QF_MASTER_KEY has low entropy (all bytes identical); use 'openssl rand -hex 32' to generate")
	}

	ca, err := pki.LoadOrInit(cfg.pkiDir, masterKey)
	if err != nil {
		return fmt.Errorf("CA init: %w", err)
	}
	serverCert, err := ca.LoadOrGenerateServerCert(cfg.pkiDir, cfg.serverHosts)
	if err != nil {
		return fmt.Errorf("server cert: %w", err)
	}
	bundleSigner, err := pki.LoadOrInitBundleSigner(cfg.pkiDir)
	if err != nil {
		return fmt.Errorf("bundle signer: %w", err)
	}
	tokenStore := pki.NewTokenStore(pool)
	rc, err := pki.NewRevocationChecker(ctx, pool)
	if err != nil {
		return fmt.Errorf("revocation checker: %w", err)
	}
	rc.StartPeriodicReload(ctx, 5*time.Minute)

	// ── Policy compiler + gRPC stream server ─────────────────────────────────
	bundleBuilder := policy.NewBundleBuilder(queries, bundleSigner)
	registry := agentsrv.NewStreamRegistry()

	grpcSrv, err := agentsrv.NewMTLSServer(serverCert, ca, rc, queries, bundleBuilder, registry, version.Version, ing)
	if err != nil {
		return fmt.Errorf("mTLS gRPC server: %w", err)
	}

	// ── Enrollment gRPC server (plain, no mTLS) ───────────────────────────────
	enrollSrv := grpc.NewServer()
	enrollSvc := pki.NewEnrollmentServer(ca, bundleSigner, tokenStore, queries, cfg.cpEndpoint)
	qfv1.RegisterEnrollmentServiceServer(enrollSrv, enrollSvc)

	// ── Auth bootstrap ────────────────────────────────────────────────────────
	defaultTenant, err := store.EnsureDefaultTenant(ctx, queries)
	if err != nil {
		return fmt.Errorf("default tenant: %w", err)
	}
	if err := auth.EnsureAdminUser(ctx, queries, defaultTenant.ID); err != nil {
		return fmt.Errorf("admin bootstrap: %w", err)
	}

	jwtSecret := []byte(envOr("QF_JWT_SECRET", ""))
	if len(jwtSecret) == 0 {
		slog.Warn("QF_JWT_SECRET not set — using ephemeral secret; tokens invalidated on restart")
		jwtSecret = make([]byte, 32)
		if _, err := rand.Read(jwtSecret); err != nil {
			return fmt.Errorf("rand: %w", err)
		}
	} else if len(jwtSecret) < 32 {
		slog.Warn("QF_JWT_SECRET is shorter than 32 bytes; use at least 32 random bytes for security",
			"length", len(jwtSecret))
	}

	// ── OIDC (optional) ──────────────────────────────────────────────────────
	oidcCfg := auth.OIDCConfigFromEnv()
	oidcHandler, err := auth.NewOIDCHandler(ctx, queries, jwtSecret, defaultTenant.ID, oidcCfg)
	if err != nil {
		return fmt.Errorf("oidc init: %w", err)
	}
	if oidcCfg.Enabled() {
		slog.Info("OIDC enabled", "issuer", oidcCfg.Issuer)
	}

	// ── REST API ─────────────────────────────────────────────────────────────
	router := api.NewRouter(api.RouterConfig{
		Queries:     queries,
		Tokens:      tokenStore,
		JWTSecret:   jwtSecret,
		TenantID:    defaultTenant.ID,
		OIDCHandler: oidcHandler,
		OIDCEnabled: oidcCfg.Enabled(),
		Hub:         hub,
		Compiler:    policy.NewRulesetCompiler(queries),
	})
	httpSrv := &http.Server{
		Addr:    cfg.httpAddr,
		Handler: router,
	}

	// ── Start servers ─────────────────────────────────────────────────────────
	grpcLis, err := net.Listen("tcp", cfg.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", cfg.grpcAddr, err)
	}
	enrollLis, err := net.Listen("tcp", cfg.enrollAddr)
	if err != nil {
		return fmt.Errorf("listen enroll %s: %w", cfg.enrollAddr, err)
	}

	errCh := make(chan error, 3)

	go func() {
		slog.Info("gRPC mTLS listening", "addr", cfg.grpcAddr)
		errCh <- grpcSrv.Serve(grpcLis)
	}()
	go func() {
		slog.Info("enrollment gRPC listening", "addr", cfg.enrollAddr)
		errCh <- enrollSrv.Serve(enrollLis)
	}()
	go func() {
		slog.Info("REST API listening", "addr", cfg.httpAddr)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			errCh <- err
		}
	}()

	// ── Shutdown ──────────────────────────────────────────────────────────────
	select {
	case err := <-errCh:
		return fmt.Errorf("server error: %w", err)
	case <-ctx.Done():
		slog.Info("shutting down")
	}

	// Graceful: tell agents to reconnect, then stop servers.
	registry.DisconnectAll("server shutdown", 5_000)

	shutCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	httpSrv.Shutdown(shutCtx)
	grpcSrv.GracefulStop()
	enrollSrv.GracefulStop()

	slog.Info("shutdown complete")
	return nil
}

// cpConfig holds all runtime configuration for the CP.
type cpConfig struct {
	dbDSN        string
	pkiDir       string
	masterKeyHex string
	grpcAddr     string
	enrollAddr   string
	httpAddr     string
	cpEndpoint   string
	serverHosts  []string
	logLevel     string
}

func loadConfig() cpConfig {
	return cpConfig{
		dbDSN:        envOr("QF_DB_DSN", "postgres://qf:qf-pg-secret-2026@postgres-postgresql.qf.svc.cluster.local:5432/qf"),
		pkiDir:       envOr("QF_PKI_DIR", "/etc/qf/pki"),
		masterKeyHex: envOr("QF_MASTER_KEY", ""),
		grpcAddr:     envOr("QF_GRPC_ADDR", ":8443"),
		enrollAddr:   envOr("QF_ENROLL_ADDR", ":8444"),
		httpAddr:     envOr("QF_HTTP_ADDR", ":8080"),
		cpEndpoint:   envOr("QF_CP_ENDPOINT", "localhost:8444"),
		serverHosts:  []string{envOr("QF_CP_HOST", "localhost")},
		logLevel:     envOr("QF_LOG_LEVEL", "info"),
	}
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

// isLowEntropy returns true when all bytes in b are identical (e.g. all zeros).
// Such keys are trivially weak and must not be used as QF_MASTER_KEY.
func isLowEntropy(b []byte) bool {
	if len(b) == 0 {
		return true
	}
	first := b[0]
	for _, v := range b[1:] {
		if v != first {
			return false
		}
	}
	return true
}

func initLogger(level string) {
	var l slog.Level
	switch level {
	case "debug":
		l = slog.LevelDebug
	case "warn":
		l = slog.LevelWarn
	case "error":
		l = slog.LevelError
	default:
		l = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewJSONHandler(os.Stderr, &slog.HandlerOptions{Level: l})))
}
