// Package middleware provides HTTP middleware for the wealth tracker.
package middleware

import (
	"context"
	"net/http"
	"os"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// IsDemoMode returns true if the app is running in demo mode.
func IsDemoMode() bool {
	return os.Getenv("DEMO_MODE") == "true"
}

// ContextKey is a type for context keys to avoid collisions.
type ContextKey string

const (
	// UserContextKey is the context key for the authenticated user.
	UserContextKey ContextKey = "user"

	// SessionCookieName is the name of the session cookie.
	SessionCookieName = "session_id"
)

// AuthMiddleware handles authentication for protected routes.
type AuthMiddleware struct {
	sessionManager *auth.SessionManager
	userRepo       *repository.UserRepository
}

// NewAuthMiddleware creates a new AuthMiddleware.
func NewAuthMiddleware(sm *auth.SessionManager, userRepo *repository.UserRepository) *AuthMiddleware {
	return &AuthMiddleware{
		sessionManager: sm,
		userRepo:       userRepo,
	}
}

// RequireAuth is middleware that requires authentication.
// Redirects to login page if not authenticated.
func (m *AuthMiddleware) RequireAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// LoadUser is middleware that loads the current user from the session cookie.
// It does not require authentication - just loads the user if present.
func (m *AuthMiddleware) LoadUser(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		cookie, err := r.Cookie(SessionCookieName)
		if err != nil {
			// No session cookie, continue without user
			next.ServeHTTP(w, r)
			return
		}

		// Validate session
		userID, err := m.sessionManager.Validate(cookie.Value)
		if err != nil {
			// Invalid or expired session, clear the cookie
			clearSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		// Load user
		user, err := m.userRepo.GetByID(userID)
		if err != nil || user == nil {
			clearSessionCookie(w)
			next.ServeHTTP(w, r)
			return
		}

		// Add user to context
		ctx := context.WithValue(r.Context(), UserContextKey, user)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// RedirectIfAuthenticated redirects to dashboard if already logged in.
// Used for login/register pages.
func (m *AuthMiddleware) RedirectIfAuthenticated(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user != nil {
			// If user must change password, redirect there instead
			if user.MustChangePassword {
				http.Redirect(w, r, "/change-password", http.StatusSeeOther)
				return
			}
			http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequirePasswordChanged is middleware that redirects users who must change their password.
// Use this on protected routes to enforce password change before accessing the app.
func (m *AuthMiddleware) RequirePasswordChanged(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := GetUser(r)
		if user != nil && user.MustChangePassword {
			http.Redirect(w, r, "/change-password", http.StatusSeeOther)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// RequireAdmin is middleware that requires admin privileges.
// Returns 403 Forbidden if user is not an admin or if in demo mode.
func (m *AuthMiddleware) RequireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Block admin access in demo mode
		if IsDemoMode() {
			http.Error(w, "Admin panel is disabled in demo mode", http.StatusForbidden)
			return
		}

		user := GetUser(r)
		if user == nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		if !user.IsAdmin {
			http.Error(w, "Forbidden - Admin access required", http.StatusForbidden)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// GetUser retrieves the authenticated user from the request context.
// Returns nil if no user is authenticated.
func GetUser(r *http.Request) *models.User {
	user, ok := r.Context().Value(UserContextKey).(*models.User)
	if !ok {
		return nil
	}
	return user
}

// SetSessionCookie sets the session cookie.
func SetSessionCookie(w http.ResponseWriter, sessionID string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    sessionID,
		Path:     "/",
		MaxAge:   maxAge,
		HttpOnly: true,
		Secure:   false, // Set to true in production with HTTPS
		SameSite: http.SameSiteLaxMode,
	})
}

// clearSessionCookie clears the session cookie.
func clearSessionCookie(w http.ResponseWriter) {
	http.SetCookie(w, &http.Cookie{
		Name:     SessionCookieName,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
	})
}

// ClearSessionCookie is the exported version for use in handlers.
func ClearSessionCookie(w http.ResponseWriter) {
	clearSessionCookie(w)
}
