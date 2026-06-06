package api

import (
	"net"
	"net/http"
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

// Middleware wraps a handler with per-IP rate limiting (5 req/min).
func (l *loginRateLimiter) Middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _, err := net.SplitHostPort(r.RemoteAddr)
		if err != nil {
			ip = r.RemoteAddr
		}
		if !l.Allow(ip) {
			http.Error(w, "too many requests", http.StatusTooManyRequests)
			return
		}
		next.ServeHTTP(w, r)
	})
}
