// Package middleware provides HTTP middleware for the wealth tracker.
package middleware

import (
	"encoding/json"
	"net/http"
	"regexp"
	"strings"
)

// Validator provides request validation utilities.
type Validator struct{}

// NewValidator creates a new Validator.
func NewValidator() *Validator {
	return &Validator{}
}

// ValidationError represents a validation error.
type ValidationError struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// ValidationErrors is a collection of validation errors.
type ValidationErrors struct {
	Errors []ValidationError `json:"errors"`
}

// Error implements the error interface.
func (v ValidationErrors) Error() string {
	if len(v.Errors) == 0 {
		return "validation failed"
	}
	var msgs []string
	for _, e := range v.Errors {
		msgs = append(msgs, e.Field+": "+e.Message)
	}
	return strings.Join(msgs, "; ")
}

// HasErrors returns true if there are validation errors.
func (v ValidationErrors) HasErrors() bool {
	return len(v.Errors) > 0
}

// Add adds a validation error.
func (v *ValidationErrors) Add(field, message string) {
	v.Errors = append(v.Errors, ValidationError{Field: field, Message: message})
}

// WriteJSON writes the validation errors as JSON response.
func (v ValidationErrors) WriteJSON(w http.ResponseWriter) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusBadRequest)
	json.NewEncoder(w).Encode(v)
}

// Common validation patterns.
var (
	emailRegex    = regexp.MustCompile(`^[a-zA-Z0-9._%+-]+@[a-zA-Z0-9.-]+\.[a-zA-Z]{2,}$`)
	currencyRegex = regexp.MustCompile(`^[A-Z]{3}$`)
	colorRegex    = regexp.MustCompile(`^#[0-9a-fA-F]{6}$`)
)

// ValidateEmail validates an email address.
func ValidateEmail(email string) bool {
	return emailRegex.MatchString(email)
}

// ValidateCurrency validates a currency code (3 uppercase letters).
func ValidateCurrency(code string) bool {
	return currencyRegex.MatchString(code)
}

// ValidateColor validates a hex color code.
func ValidateColor(color string) bool {
	return colorRegex.MatchString(color)
}

// ValidateRequired checks if a string is non-empty.
func ValidateRequired(value string) bool {
	return strings.TrimSpace(value) != ""
}

// ValidateLength checks if a string is within length bounds.
func ValidateLength(value string, min, max int) bool {
	l := len(value)
	return l >= min && l <= max
}

// ValidatePositive checks if a float is positive.
func ValidatePositive(value float64) bool {
	return value > 0
}

// ValidateNonNegative checks if a float is non-negative.
func ValidateNonNegative(value float64) bool {
	return value >= 0
}

// ValidateRange checks if a float is within a range.
func ValidateRange(value, min, max float64) bool {
	return value >= min && value <= max
}

// ValidatePercentage checks if a value is a valid percentage (0-100).
func ValidatePercentage(value float64) bool {
	return value >= 0 && value <= 100
}

// SanitizeString trims whitespace and removes control characters.
func SanitizeString(s string) string {
	// Trim whitespace
	s = strings.TrimSpace(s)
	// Remove null bytes and other control characters
	s = strings.Map(func(r rune) rune {
		if r < 32 && r != '\n' && r != '\r' && r != '\t' {
			return -1
		}
		return r
	}, s)
	return s
}

// SanitizeHTML escapes HTML special characters.
func SanitizeHTML(s string) string {
	replacer := strings.NewReplacer(
		"&", "&amp;",
		"<", "&lt;",
		">", "&gt;",
		"\"", "&quot;",
		"'", "&#39;",
	)
	return replacer.Replace(s)
}
