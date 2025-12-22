// Package handlers provides HTTP handlers for the wealth tracker.
package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"wealth_tracker/internal/auth"
	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// AuthHandler handles authentication routes.
type AuthHandler struct {
	templates      map[string]*template.Template
	userRepo       *repository.UserRepository
	sessionManager *auth.SessionManager
}

// NewAuthHandler creates a new AuthHandler.
func NewAuthHandler(
	templates map[string]*template.Template,
	userRepo *repository.UserRepository,
	sessionManager *auth.SessionManager,
) *AuthHandler {
	return &AuthHandler{
		templates:      templates,
		userRepo:       userRepo,
		sessionManager: sessionManager,
	}
}

// LoginPage renders the login page.
func (h *AuthHandler) LoginPage(w http.ResponseWriter, r *http.Request) {
	h.render(w, "login.html", map[string]any{
		"Title": "Login",
	})
}

// Login handles the login form submission.
func (h *AuthHandler) Login(w http.ResponseWriter, r *http.Request) {
	if err := r.ParseForm(); err != nil {
		h.renderLoginError(w, "Invalid form data")
		return
	}

	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")

	// Validate input
	if email == "" || password == "" {
		h.renderLoginError(w, "Email and password are required")
		return
	}

	// Find user
	user, err := h.userRepo.GetByEmail(email)
	if err != nil {
		log.Printf("Login error finding user: %v", err)
		h.renderLoginError(w, "An error occurred. Please try again.")
		return
	}

	if user == nil {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Check password
	if !auth.CheckPassword(password, user.PasswordHash) {
		h.renderLoginError(w, "Invalid email or password")
		return
	}

	// Create session
	session, err := h.sessionManager.Create(user.ID)
	if err != nil {
		log.Printf("Login error creating session: %v", err)
		h.renderLoginError(w, "An error occurred. Please try again.")
		return
	}

	// Set session cookie
	middleware.SetSessionCookie(w, session.ID, 7*24*60*60) // 7 days

	// Check if user must change password
	if user.MustChangePassword {
		http.Redirect(w, r, "/change-password", http.StatusSeeOther)
		return
	}

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// RegisterPage renders the registration page.
func (h *AuthHandler) RegisterPage(w http.ResponseWriter, r *http.Request) {
	// Check if registration is allowed (no user with must_change_password=true)
	if blocked, msg := h.isRegistrationBlocked(); blocked {
		h.render(w, "register.html", map[string]any{
			"Title":              "Register",
			"RegistrationLocked": true,
			"LockMessage":        msg,
		})
		return
	}

	h.render(w, "register.html", map[string]any{
		"Title": "Register",
	})
}

// isRegistrationBlocked checks if registration should be blocked.
// Registration is blocked if an admin user hasn't changed their default password yet.
func (h *AuthHandler) isRegistrationBlocked() (bool, string) {
	users, err := h.userRepo.GetAll()
	if err != nil {
		return false, "" // Allow registration on error
	}

	for _, user := range users {
		if user.IsAdmin && user.MustChangePassword {
			return true, "Registration is disabled until the administrator completes initial setup."
		}
	}
	return false, ""
}

// Register handles the registration form submission.
func (h *AuthHandler) Register(w http.ResponseWriter, r *http.Request) {
	// Check if registration is allowed
	if blocked, _ := h.isRegistrationBlocked(); blocked {
		http.Redirect(w, r, "/register", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderRegisterError(w, "Invalid form data")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	email := strings.TrimSpace(r.FormValue("email"))
	password := r.FormValue("password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate input
	if name == "" {
		h.renderRegisterError(w, "Name is required")
		return
	}
	if email == "" {
		h.renderRegisterError(w, "Email is required")
		return
	}
	if password == "" {
		h.renderRegisterError(w, "Password is required")
		return
	}
	if len(password) < 8 {
		h.renderRegisterError(w, "Password must be at least 8 characters")
		return
	}
	if password != confirmPassword {
		h.renderRegisterError(w, "Passwords do not match")
		return
	}

	// Check if email already exists
	exists, err := h.userRepo.EmailExists(email)
	if err != nil {
		log.Printf("Register error checking email: %v", err)
		h.renderRegisterError(w, "An error occurred. Please try again.")
		return
	}
	if exists {
		h.renderRegisterError(w, "An account with this email already exists")
		return
	}

	// Hash password
	passwordHash, err := auth.HashPassword(password)
	if err != nil {
		log.Printf("Register error hashing password: %v", err)
		h.renderRegisterError(w, "An error occurred. Please try again.")
		return
	}

	// Create user
	user := &models.User{
		Email:        email,
		PasswordHash: passwordHash,
		Name:         name,
	}

	userID, err := h.userRepo.Create(user)
	if err != nil {
		log.Printf("Register error creating user: %v", err)
		h.renderRegisterError(w, "An error occurred. Please try again.")
		return
	}

	// Create session
	session, err := h.sessionManager.Create(userID)
	if err != nil {
		log.Printf("Register error creating session: %v", err)
		// User was created, redirect to login
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	// Set session cookie
	middleware.SetSessionCookie(w, session.ID, 7*24*60*60) // 7 days

	// Redirect to dashboard
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// Logout handles user logout.
func (h *AuthHandler) Logout(w http.ResponseWriter, r *http.Request) {
	// Get session cookie
	cookie, err := r.Cookie(middleware.SessionCookieName)
	if err == nil {
		// Delete session from database
		h.sessionManager.Delete(cookie.Value)
	}

	// Clear session cookie
	middleware.ClearSessionCookie(w)

	// Redirect to login
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

// ChangePasswordPage renders the change password page.
func (h *AuthHandler) ChangePasswordPage(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "change-password.html", map[string]any{
		"Title":    "Change Password",
		"User":     user,
		"Required": user.MustChangePassword,
	})
}

// ChangePassword handles the change password form submission.
func (h *AuthHandler) ChangePassword(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderChangePasswordError(w, user, "Invalid form data")
		return
	}

	currentPassword := r.FormValue("current_password")
	newPassword := r.FormValue("new_password")
	confirmPassword := r.FormValue("confirm_password")

	// Validate current password
	if !auth.CheckPassword(currentPassword, user.PasswordHash) {
		h.renderChangePasswordError(w, user, "Current password is incorrect")
		return
	}

	// Validate new password
	if newPassword == "" {
		h.renderChangePasswordError(w, user, "New password is required")
		return
	}
	if len(newPassword) < 8 {
		h.renderChangePasswordError(w, user, "New password must be at least 8 characters")
		return
	}
	if newPassword != confirmPassword {
		h.renderChangePasswordError(w, user, "New passwords do not match")
		return
	}
	if newPassword == currentPassword {
		h.renderChangePasswordError(w, user, "New password must be different from current password")
		return
	}

	// Hash new password
	passwordHash, err := auth.HashPassword(newPassword)
	if err != nil {
		log.Printf("ChangePassword error hashing: %v", err)
		h.renderChangePasswordError(w, user, "An error occurred. Please try again.")
		return
	}

	// Update password
	if err := h.userRepo.UpdatePassword(user.ID, passwordHash); err != nil {
		log.Printf("ChangePassword error updating: %v", err)
		h.renderChangePasswordError(w, user, "An error occurred. Please try again.")
		return
	}

	// Clear must_change_password flag
	if user.MustChangePassword {
		if err := h.userRepo.SetMustChangePassword(user.ID, false); err != nil {
			log.Printf("ChangePassword error clearing flag: %v", err)
		}
	}

	// Redirect to dashboard with success
	http.Redirect(w, r, "/dashboard", http.StatusSeeOther)
}

// renderChangePasswordError renders the change password page with an error.
func (h *AuthHandler) renderChangePasswordError(w http.ResponseWriter, user *models.User, errMsg string) {
	h.render(w, "change-password.html", map[string]any{
		"Title":    "Change Password",
		"User":     user,
		"Required": user.MustChangePassword,
		"Error":    errMsg,
	})
}

// render renders a template with the given data.
func (h *AuthHandler) render(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = make(map[string]any)
	}

	tmpl, ok := h.templates[name]
	if !ok {
		http.Error(w, "Template not found: "+name, http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.ExecuteTemplate(w, "base.html", data); err != nil {
		log.Printf("Error rendering template %s: %v", name, err)
		http.Error(w, "Error rendering page", http.StatusInternalServerError)
	}
}

// renderLoginError renders the login page with an error message.
func (h *AuthHandler) renderLoginError(w http.ResponseWriter, errMsg string) {
	h.render(w, "login.html", map[string]any{
		"Title": "Login",
		"Error": errMsg,
	})
}

// renderRegisterError renders the register page with an error message.
func (h *AuthHandler) renderRegisterError(w http.ResponseWriter, errMsg string) {
	h.render(w, "register.html", map[string]any{
		"Title": "Register",
		"Error": errMsg,
	})
}
