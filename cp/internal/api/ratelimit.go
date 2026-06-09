package api

import (
	"net"
	"net/http"
	"strings"
	"sync"
	"time"
)

const (
	loginMaxAttempts = 5
	loginWindowDur   = time.Minute
)

// loginRateLimiter tracks failed login attempts per IP using a fixed sliding window.
type loginRateLimiter struct {
	mu      sync.Mutex
	windows map[string]*loginWindow
}

type loginWindow struct {
	count int
	start time.Time
}

func newLoginRateLimiter() *loginRateLimiter {
	return &loginRateLimiter{windows: make(map[string]*loginWindow)}
}

// Allow returns true if the IP is within the rate limit.
func (l *loginRateLimiter) Allow(ip string) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	now := time.Now()
	w, ok := l.windows[ip]
	if !ok || now.Sub(w.start) >= loginWindowDur {
		// Lazy GC: prune all expired windows on new window creation.
		for k, v := range l.windows {
			if now.Sub(v.start) >= loginWindowDur {
				delete(l.windows, k)
			}
		}
		l.windows[ip] = &loginWindow{count: 1, start: now}
		return true
	}
	if w.count >= loginMaxAttempts {
		return false
	}
	w.count++
	return true
}

// clientIP extracts the real client IP, preferring X-Real-IP / X-Forwarded-For
// set by a trusted reverse proxy (Traefik/nginx) over r.RemoteAddr.
func clientIP(r *http.Request) string {
	if v := r.Header.Get("X-Real-IP"); v != "" {
		if ip := net.ParseIP(strings.TrimSpace(v)); ip != nil {
			return ip.String()
		}
	}
	if v := r.Header.Get("X-Forwarded-For"); v != "" {
		// X-Forwarded-For may be a comma-separated list; leftmost is the client.
		first := strings.TrimSpace(strings.SplitN(v, ",", 2)[0])
		if ip := net.ParseIP(first); ip != nil {
			return ip.String()
		}
	}
	ip, _, err := net.SplitHostPort(r.RemoteAddr)
	if err != nil {
		return r.RemoteAddr
	}
	return ip
}

// Middleware wraps a handler with per-IP rate limiting (5 req/min).
func (l *loginRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !l.Allow(clientIP(r)) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
