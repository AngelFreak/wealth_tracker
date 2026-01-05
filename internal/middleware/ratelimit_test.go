package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGetIP_XForwardedFor_SingleIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")

	ip := getIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q", ip, "192.168.1.1")
	}
}

func TestGetIP_XForwardedFor_MultipleIPs(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	// Attacker might try: "fake-ip, attacker-proxy, real-client"
	// We should only use the first IP (original client)
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1, 172.16.0.1")

	ip := getIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q (first IP only)", ip, "192.168.1.1")
	}
}

func TestGetIP_XForwardedFor_WithSpaces(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "  192.168.1.1  ,  10.0.0.1  ")

	ip := getIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q (trimmed)", ip, "192.168.1.1")
	}
}

func TestGetIP_XRealIP(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.1")

	ip := getIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q", ip, "192.168.1.1")
	}
}

func TestGetIP_XRealIP_WithSpaces(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "  192.168.1.1  ")

	ip := getIP(req)
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q (trimmed)", ip, "192.168.1.1")
	}
}

func TestGetIP_FallbackToRemoteAddr(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:12345"

	ip := getIP(req)
	if ip != "127.0.0.1:12345" {
		t.Errorf("getIP() = %q, want %q", ip, "127.0.0.1:12345")
	}
}

func TestGetIP_XForwardedForPriority(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1")
	req.Header.Set("X-Real-IP", "10.0.0.1")
	req.RemoteAddr = "127.0.0.1:12345"

	ip := getIP(req)
	// X-Forwarded-For should take priority
	if ip != "192.168.1.1" {
		t.Errorf("getIP() = %q, want %q (X-Forwarded-For has priority)", ip, "192.168.1.1")
	}
}

func TestRateLimiter_AllowsBurst(t *testing.T) {
	limiter := NewRateLimiter(1, 3) // 1 req/sec, burst of 3

	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First 3 requests should succeed (burst)
	for i := 0; i < 3; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Errorf("Request %d: got status %d, want %d", i+1, rec.Code, http.StatusOK)
		}
	}
}

func TestRateLimiter_BlocksAfterBurst(t *testing.T) {
	limiter := NewRateLimiter(0.1, 2) // Very slow rate, burst of 2

	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// Use up the burst
	for i := 0; i < 2; i++ {
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "192.168.1.1:12345"
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
	}

	// Next request should be rate limited
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusTooManyRequests {
		t.Errorf("Got status %d, want %d (rate limited)", rec.Code, http.StatusTooManyRequests)
	}
}

func TestRateLimiter_DifferentIPs(t *testing.T) {
	limiter := NewRateLimiter(0.1, 1) // Very slow rate, burst of 1

	handler := limiter.Limit(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	// First IP uses its burst
	req1 := httptest.NewRequest("GET", "/", nil)
	req1.RemoteAddr = "192.168.1.1:12345"
	rec1 := httptest.NewRecorder()
	handler.ServeHTTP(rec1, req1)

	if rec1.Code != http.StatusOK {
		t.Errorf("First IP first request: got %d, want %d", rec1.Code, http.StatusOK)
	}

	// Second IP should still be allowed (separate limiter)
	req2 := httptest.NewRequest("GET", "/", nil)
	req2.RemoteAddr = "192.168.1.2:12345"
	rec2 := httptest.NewRecorder()
	handler.ServeHTTP(rec2, req2)

	if rec2.Code != http.StatusOK {
		t.Errorf("Second IP first request: got %d, want %d", rec2.Code, http.StatusOK)
	}
}

func TestLimitAuth_Creates(t *testing.T) {
	handler := LimitAuth(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/login", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("LimitAuth: got status %d, want %d", rec.Code, http.StatusOK)
	}
}

func TestLimitAPI_Creates(t *testing.T) {
	handler := LimitAPI(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/api/test", nil)
	req.RemoteAddr = "192.168.1.1:12345"
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("LimitAPI: got status %d, want %d", rec.Code, http.StatusOK)
	}
}
