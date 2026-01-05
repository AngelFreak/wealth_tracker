// Package middleware provides HTTP middleware for the wealth tracker.
package middleware

import (
	"net/http"
)

// SecurityHeaders adds security-related HTTP headers to responses.
// These headers help protect against common web vulnerabilities.
func SecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Prevent clickjacking by disallowing embedding in iframes
		w.Header().Set("X-Frame-Options", "DENY")

		// Prevent MIME type sniffing
		w.Header().Set("X-Content-Type-Options", "nosniff")

		// Enable XSS filter in older browsers
		w.Header().Set("X-XSS-Protection", "1; mode=block")

		// Control referrer information sent with requests
		w.Header().Set("Referrer-Policy", "strict-origin-when-cross-origin")

		// Restrict permissions/features the browser can use
		w.Header().Set("Permissions-Policy", "geolocation=(), microphone=(), camera=()")

		// Content Security Policy - restrict sources of content
		// Note: 'unsafe-inline' is required for:
		//   - Alpine.js x-* directives that contain inline expressions
		//   - Tailwind CSS inline styles for dynamic classes
		//   - HTMX hx-* attributes with inline values
		// Alpine.js does NOT require 'unsafe-eval' as of v3.x
		// In production, consider using CSP nonces for stricter control
		csp := "default-src 'self'; " +
			"script-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://unpkg.com; " +
			"style-src 'self' 'unsafe-inline' https://cdn.jsdelivr.net https://fonts.googleapis.com; " +
			"font-src 'self' https://fonts.gstatic.com https://cdn.jsdelivr.net; " +
			"img-src 'self' data: blob:; " +
			"connect-src 'self'; " +
			"frame-ancestors 'none'; " +
			"form-action 'self'; " +
			"base-uri 'self'"
		w.Header().Set("Content-Security-Policy", csp)

		next.ServeHTTP(w, r)
	})
}
