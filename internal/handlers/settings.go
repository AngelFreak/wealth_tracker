package handlers

import (
	"html/template"
	"log"
	"net/http"
	"strings"

	"wealth_tracker/internal/middleware"
	"wealth_tracker/internal/repository"
)

// SettingsHandler handles settings routes.
type SettingsHandler struct {
	templates map[string]*template.Template
	userRepo  *repository.UserRepository
}

// NewSettingsHandler creates a new SettingsHandler.
func NewSettingsHandler(
	templates map[string]*template.Template,
	userRepo *repository.UserRepository,
) *SettingsHandler {
	return &SettingsHandler{
		templates: templates,
		userRepo:  userRepo,
	}
}

// Settings renders the settings page.
func (h *SettingsHandler) Settings(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}

	h.render(w, "settings.html", map[string]any{
		"Title":     "Settings",
		"User":      user,
		"ActiveNav": "settings",
	})
}

// Update handles updating user settings.
func (h *SettingsHandler) Update(w http.ResponseWriter, r *http.Request) {
	user := middleware.GetUser(r)
	if user == nil {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	if err := r.ParseForm(); err != nil {
		h.renderError(w, user, "Invalid form data")
		return
	}

	name := strings.TrimSpace(r.FormValue("name"))
	defaultCurrency := strings.TrimSpace(r.FormValue("default_currency"))
	numberFormat := strings.TrimSpace(r.FormValue("number_format"))
	theme := strings.TrimSpace(r.FormValue("theme"))

	// Validate name
	if name == "" {
		h.renderError(w, user, "Name is required")
		return
	}

	// Validate currency
	validCurrencies := map[string]bool{
		"DKK": true, "EUR": true, "USD": true, "GBP": true, "SEK": true, "NOK": true,
	}
	if defaultCurrency == "" || !validCurrencies[defaultCurrency] {
		defaultCurrency = "DKK"
	}

	// Validate number format
	validFormats := map[string]bool{
		"da": true, // Danish: 1.234,56
		"en": true, // English: 1,234.56
		"de": true, // German: 1.234,56 (same as Danish)
		"fr": true, // French: 1 234,56
	}
	if numberFormat == "" || !validFormats[numberFormat] {
		numberFormat = "da"
	}

	// Validate theme
	if theme != "light" && theme != "dark" {
		theme = "dark"
	}

	// Update user
	user.Name = name
	user.DefaultCurrency = defaultCurrency
	user.NumberFormat = numberFormat
	user.Theme = theme

	err := h.userRepo.Update(user)
	if err != nil {
		log.Printf("Error updating user settings: %v", err)
		h.renderError(w, user, "Failed to save settings")
		return
	}

	h.render(w, "settings.html", map[string]any{
		"Title":     "Settings",
		"User":      user,
		"ActiveNav": "settings",
		"Success":   "Settings saved successfully",
	})
}

// render renders a template with the given data.
func (h *SettingsHandler) render(w http.ResponseWriter, name string, data map[string]any) {
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

// renderError re-renders the settings page with an error message.
func (h *SettingsHandler) renderError(w http.ResponseWriter, user any, errMsg string) {
	h.render(w, "settings.html", map[string]any{
		"Title":     "Settings",
		"User":      user,
		"ActiveNav": "settings",
		"Error":     errMsg,
	})
}
