package grpcclient

import (
	"context"
	"log/slog"
	"math/rand/v2"
	"strings"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

// BackoffConfig configures exponential backoff for reconnects.
type BackoffConfig struct {
	InitialMs  int64   // first delay; default 1000
	MaxMs      int64   // cap; default 60000
	Multiplier float64 // per-attempt factor; default 2.0
	JitterFrac float64 // ±fraction of delay; default 0.2
}

// DefaultBackoff returns production-ready backoff config (1s→60s, ×2, ±20%).
func DefaultBackoff() BackoffConfig {
	return BackoffConfig{
		InitialMs:  1_000,
		MaxMs:      60_000,
		Multiplier: 2.0,
		JitterFrac: 0.2,
	}
}

// RunWithReconnect runs sessionFn in a loop, re-dialing on transient failures.
//
// dialFn is called before each attempt to obtain a fresh *Client.
// sessionFn receives the context and client; it should block until the session
// ends. On clean exit (ctx cancelled) RunWithReconnect returns nil.
//
// Stops without retry on:
//   - ctx cancellation
//   - TLS errors (cert revoked / CA mismatch / handshake failure)
//   - codes.Unauthenticated
func RunWithReconnect(
	ctx context.Context,
	cfg BackoffConfig,
	dialFn func(context.Context) (*Client, error),
	sessionFn func(context.Context, *Client) error,
) error {
	if cfg.InitialMs <= 0 {
		cfg.InitialMs = 1_000
	}
	if cfg.MaxMs <= 0 {
		cfg.MaxMs = 60_000
	}
	if cfg.Multiplier <= 1 {
		cfg.Multiplier = 2.0
	}
	if cfg.JitterFrac <= 0 {
		cfg.JitterFrac = 0.2
	}

	delayMs := float64(cfg.InitialMs)
	attempts := 0

	for {
		if ctx.Err() != nil {
			return nil
		}

		c, err := dialFn(ctx)
		if err != nil {
			if isFatal(err) {
				slog.Error("grpcclient: TLS error, stopping reconnect", "err", err)
				return err
			}
			slog.Warn("grpcclient: dial failed", "attempt", attempts, "err", err)
		} else {
			sessionErr := sessionFn(ctx, c)
			c.Close()

			if ctx.Err() != nil {
				return nil
			}
			if sessionErr == nil {
				// Clean exit from session (e.g. DisconnectRequest handled).
				delayMs = float64(cfg.InitialMs)
				attempts = 0
				continue
			}
			if isFatal(sessionErr) {
				slog.Error("grpcclient: fatal error, stopping reconnect", "err", sessionErr)
				return sessionErr
			}
			slog.Warn("grpcclient: session ended, reconnecting",
				"attempt", attempts, "err", sessionErr)
		}

		// Backoff with jitter.
		jitter := 1 + cfg.JitterFrac*(2*rand.Float64()-1)
		wait := time.Duration(delayMs*jitter) * time.Millisecond
		slog.Info("grpcclient: next reconnect", "in", wait)

		select {
		case <-ctx.Done():
			return nil
		case <-time.After(wait):
		}

		delayMs = min(delayMs*cfg.Multiplier, float64(cfg.MaxMs))
		attempts++
	}
}

// isFatal returns true for errors that should stop the reconnect loop.
// TLS handshake failures and auth rejections are permanent until cert rotation.
func isFatal(err error) bool {
	if status.Code(err) == codes.Unauthenticated {
		return true
	}
	msg := strings.ToLower(err.Error())
	return strings.Contains(msg, "tls:") ||
		strings.Contains(msg, "certificate") && strings.Contains(msg, "revoked")
}
