// Package middleware provides HTTP middleware for the wealth tracker.
package middleware

import (
	"net/http"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// RateLimiter provides per-IP rate limiting.
type RateLimiter struct {
	visitors map[string]*visitor
	mu       sync.RWMutex
	rate     rate.Limit
	burst    int
	cleanup  time.Duration
}

type visitor struct {
	limiter  *rate.Limiter
	lastSeen time.Time
}

// NewRateLimiter creates a new rate limiter.
// r is requests per second, b is burst size.
func NewRateLimiter(r float64, b int) *RateLimiter {
	rl := &RateLimiter{
		visitors: make(map[string]*visitor),
		rate:     rate.Limit(r),
		burst:    b,
		cleanup:  3 * time.Minute,
	}

	// Start background cleanup
	go rl.cleanupLoop()

	return rl
}

// getVisitor returns the rate limiter for an IP, creating one if needed.
func (rl *RateLimiter) getVisitor(ip string) *rate.Limiter {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	v, exists := rl.visitors[ip]
	if !exists {
		limiter := rate.NewLimiter(rl.rate, rl.burst)
		rl.visitors[ip] = &visitor{limiter: limiter, lastSeen: time.Now()}
		return limiter
	}

	v.lastSeen = time.Now()
	return v.limiter
}

// cleanupLoop removes old visitors periodically.
func (rl *RateLimiter) cleanupLoop() {
	ticker := time.NewTicker(time.Minute)
	defer ticker.Stop()

	for range ticker.C {
		rl.mu.Lock()
		for ip, v := range rl.visitors {
			if time.Since(v.lastSeen) > rl.cleanup {
				delete(rl.visitors, ip)
			}
		}
		rl.mu.Unlock()
	}
}

// Limit is middleware that rate limits requests by IP.
func (rl *RateLimiter) Limit(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip := getIP(r)
		limiter := rl.getVisitor(ip)

		if !limiter.Allow() {
			http.Error(w, "Too many requests", http.StatusTooManyRequests)
			return
		}

		next.ServeHTTP(w, r)
	})
}

// LimitStrict is middleware for sensitive endpoints with stricter limits.
// Uses 1 request per 2 seconds with burst of 3.
func LimitStrict(next http.Handler) http.Handler {
	limiter := NewRateLimiter(0.5, 3)
	return limiter.Limit(next)
}

// LimitAuth is middleware for auth endpoints.
// Uses 1 request per second with burst of 5.
func LimitAuth(next http.Handler) http.Handler {
	limiter := NewRateLimiter(1, 5)
	return limiter.Limit(next)
}

// LimitAPI is middleware for API endpoints.
// Uses 10 requests per second with burst of 20.
func LimitAPI(next http.Handler) http.Handler {
	limiter := NewRateLimiter(10, 20)
	return limiter.Limit(next)
}

// getIP extracts the client IP from the request.
func getIP(r *http.Request) string {
	// Check X-Forwarded-For first (for reverse proxies)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		return xff
	}
	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}
	// Fall back to RemoteAddr
	return r.RemoteAddr
}
