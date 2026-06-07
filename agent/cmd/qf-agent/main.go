package main

import (
	"context"
	"crypto/ed25519"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/qf/qf/agent/internal/agent"
	"github.com/qf/qf/agent/internal/config"
	"github.com/qf/qf/agent/internal/grpcclient"
	"github.com/qf/qf/agent/internal/loader"
)

func main() {
	cfg, err := config.LoadDefault()
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(1)
	}

	setupLogging(cfg.LogLevel)

	if loader.KernelVersion() < loader.KernelVer(5, 15, 0) {
		slog.Error("kernel too old: qf-agent requires Linux 5.15 or newer",
			"kernel", loader.KernelVersion())
		os.Exit(1)
	}

	l, err := loader.Load(cfg.Interface)
	if err != nil {
		slog.Error("BPF load failed", "iface", cfg.Interface, "err", err)
		os.Exit(1)
	}
	defer l.Close()

	ag := agent.New(l, nil)

	ctx, cancel := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer cancel()

	slog.Info("qf-agent starting", "iface", cfg.Interface, "cp", cfg.CPEndpoint)

	diskBuf := grpcclient.NewDiskBuffer("")

	// Initial enrollment: if PKI is absent and we have a token, enroll before connecting.
	if _, err := os.Stat(cfg.PKIDir + "/bundle-signing.pub"); os.IsNotExist(err) {
		if cfg.EnrollToken == "" {
			slog.Error("PKI not found and no enroll token configured")
			os.Exit(1)
		}
		slog.Info("agent: no PKI found, starting initial enrollment", "addr", cfg.EnrollEndpoint)
		hostname, _ := os.Hostname()
		if _, enrollErr := grpcclient.Enroll(ctx, cfg.EnrollEndpoint, cfg.EnrollToken, hostname, cfg.PKIDir); enrollErr != nil {
			slog.Error("agent: initial enrollment failed", "err", enrollErr)
			os.Exit(1)
		}
		slog.Info("agent: initial enrollment succeeded")
	}

	for {
		bundleKey, err := loadBundleSigningKey(cfg.PKIDir + "/bundle-signing.pub")
		if err != nil {
			slog.Error("bundle signing key load failed", "path", cfg.PKIDir+"/bundle-signing.pub", "err", err)
			os.Exit(1)
		}

		grpcCfg := grpcclient.Config{
			CertFile:   cfg.PKIDir + "/agent.crt",
			KeyFile:    cfg.PKIDir + "/agent.key",
			CAFile:     cfg.PKIDir + "/ca.crt",
			CPEndpoint: cfg.CPEndpoint,
		}

		runErr := ag.RunFull(ctx, agent.RunFullConfig{
			GRPC:      grpcCfg,
			BundleKey: bundleKey,
			DiskBuf:   diskBuf,
		})

		if ctx.Err() != nil {
			break
		}
		if runErr == nil {
			break
		}

		if isCertExpiredError(runErr) && cfg.EnrollToken != "" {
			slog.Warn("agent: cert expired, attempting re-enrollment",
				"enroll_addr", cfg.EnrollEndpoint)
			hostname, _ := os.Hostname()
			_, enrollErr := grpcclient.Enroll(ctx, cfg.EnrollEndpoint,
				cfg.EnrollToken, hostname, cfg.PKIDir)
			if enrollErr != nil {
				slog.Error("agent: re-enrollment failed", "err", enrollErr)
				os.Exit(1)
			}
			slog.Info("agent: re-enrollment succeeded, reconnecting")
			continue
		}

		slog.Error("agent exited with error", "err", runErr)
		os.Exit(1)
	}

	slog.Info("qf-agent stopped")
}

// isCertExpiredError returns true when the error indicates a TLS failure caused
// by an expired client certificate being rejected by the server.
func isCertExpiredError(err error) bool {
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "expired") ||
		strings.Contains(msg, "not yet valid")
}

func loadBundleSigningKey(path string) (ed25519.PublicKey, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return nil, fmt.Errorf("no PEM block in %s", path)
	}
	pub, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return nil, err
	}
	ed, ok := pub.(ed25519.PublicKey)
	if !ok {
		return nil, fmt.Errorf("%s is not an Ed25519 public key", path)
	}
	return ed, nil
}

func setupLogging(level string) {
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
