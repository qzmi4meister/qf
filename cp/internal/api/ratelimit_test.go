package api

import (
	"testing"
)

func TestLoginRateLimiter_Allow(t *testing.T) {
	rl := newLoginRateLimiter()
	ip := "192.0.2.1"

	for i := 1; i <= loginMaxAttempts; i++ {
		if !rl.Allow(ip) {
			t.Fatalf("attempt %d should be allowed", i)
		}
	}
	if rl.Allow(ip) {
		t.Fatal("attempt after limit should be rejected")
	}
}

func TestLoginRateLimiter_DifferentIPs(t *testing.T) {
	rl := newLoginRateLimiter()

	for i := 0; i < loginMaxAttempts; i++ {
		rl.Allow("192.0.2.1")
	}
	// Different IP must be unaffected.
	if !rl.Allow("192.0.2.2") {
		t.Fatal("different IP should be allowed independently")
	}
}

func TestLoginRateLimiter_WindowReset(t *testing.T) {
	rl := newLoginRateLimiter()
	ip := "192.0.2.3"

	for i := 0; i < loginMaxAttempts; i++ {
		rl.Allow(ip)
	}
	// Manually expire the window.
	rl.mu.Lock()
	rl.windows[ip].start = rl.windows[ip].start.Add(-loginWindowDur)
	rl.mu.Unlock()

	if !rl.Allow(ip) {
		t.Fatal("should be allowed after window expiry")
	}
}
