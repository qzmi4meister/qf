// Package config loads qf-agent configuration from /etc/qf/agent.conf
// and environment variable overrides.
//
// File format: KEY=VALUE lines; blank lines and # comments ignored.
// Environment variables override file values.
package config

import (
	"bufio"
	"fmt"
	"log/slog"
	"os"
	"strings"
)

const DefaultConfigFile = "/etc/qf/agent.conf"

// Config holds qf-agent runtime configuration.
type Config struct {
	// CPEndpoint is the CP gRPC address (host:port).
	CPEndpoint string
	// Interface is the network interface to attach the BPF program to.
	Interface string
	// PKIDir is the directory containing agent.crt, agent.key, ca.crt, bundle_signing.pub.
	PKIDir string
	// LogLevel is the slog level (debug|info|warn|error).
	LogLevel string
}

// Load reads the config file (if it exists) and applies environment overrides.
func Load(path string) (*Config, error) {
	cfg := &Config{
		CPEndpoint: "localhost:8443",
		Interface:  "eth0",
		PKIDir:     "/etc/qf",
		LogLevel:   "info",
	}

	if err := loadFile(path, cfg); err != nil {
		return nil, fmt.Errorf("config: load file %s: %w", path, err)
	}
	applyEnv(cfg)
	return cfg, nil
}

// LoadDefault loads from DefaultConfigFile.
func LoadDefault() (*Config, error) {
	return Load(DefaultConfigFile)
}

func loadFile(path string, cfg *Config) error {
	f, err := os.Open(path)
	if os.IsNotExist(err) {
		return nil // file optional
	}
	if err != nil {
		return err
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, ok := strings.Cut(line, "=")
		if !ok {
			continue
		}
		applyKV(cfg, strings.TrimSpace(k), strings.TrimSpace(v))
	}
	return scanner.Err()
}

func applyEnv(cfg *Config) {
	envMap := map[string]string{
		"QF_CP_ENDPOINT": "",
		"QF_IFACE":       "",
		"QF_PKI_DIR":     "",
		"QF_LOG_LEVEL":   "",
	}
	for k := range envMap {
		if v := os.Getenv(k); v != "" {
			applyKV(cfg, k, v)
		}
	}
}

func applyKV(cfg *Config, k, v string) {
	switch k {
	case "QF_CP_ENDPOINT":
		cfg.CPEndpoint = v
	case "QF_IFACE":
		cfg.Interface = v
	case "QF_PKI_DIR":
		cfg.PKIDir = v
	case "QF_LOG_LEVEL":
		cfg.LogLevel = v
	default:
		slog.Debug("config: unknown key", "key", k)
	}
}
