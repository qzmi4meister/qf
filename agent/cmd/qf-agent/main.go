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

	bundleKey, err := loadBundleSigningKey(cfg.PKIDir + "/bundle-signing.pub")
	if err != nil {
		slog.Error("bundle signing key load failed", "path", cfg.PKIDir+"/bundle-signing.pub", "err", err)
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

	grpcCfg := grpcclient.Config{
		CertFile:   cfg.PKIDir + "/agent.crt",
		KeyFile:    cfg.PKIDir + "/agent.key",
		CAFile:     cfg.PKIDir + "/ca.crt",
		CPEndpoint: cfg.CPEndpoint,
	}
	diskBuf := grpcclient.NewDiskBuffer("")

	if err := ag.RunFull(ctx, agent.RunFullConfig{
		GRPC:      grpcCfg,
		BundleKey: bundleKey,
		DiskBuf:   diskBuf,
	}); err != nil {
		slog.Error("agent exited with error", "err", err)
		os.Exit(1)
	}

	slog.Info("qf-agent stopped")
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
