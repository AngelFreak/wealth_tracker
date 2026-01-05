// Package errors provides typed errors for the wealth tracker.
package errors

import (
	"errors"
	"fmt"
)

// Sentinel errors for common error cases.
var (
	// ErrNotFound indicates a resource was not found.
	ErrNotFound = errors.New("resource not found")

	// ErrUnauthorized indicates the user is not authenticated.
	ErrUnauthorized = errors.New("unauthorized")

	// ErrForbidden indicates the user lacks permission.
	ErrForbidden = errors.New("forbidden")

	// ErrValidation indicates a validation error.
	ErrValidation = errors.New("validation error")

	// ErrConflict indicates a resource conflict (e.g., duplicate).
	ErrConflict = errors.New("resource conflict")

	// ErrInternal indicates an internal server error.
	ErrInternal = errors.New("internal error")

	// ErrRateLimit indicates too many requests.
	ErrRateLimit = errors.New("rate limit exceeded")

	// ErrDemoMode indicates operation not allowed in demo mode.
	ErrDemoMode = errors.New("operation not allowed in demo mode")
)

// AppError is a structured application error.
type AppError struct {
	// Type is the error type (sentinel error).
	Type error
	// Message is the user-facing error message.
	Message string
	// Details contains additional error details.
	Details map[string]any
	// Cause is the underlying error.
	Cause error
}

// Error implements the error interface.
func (e *AppError) Error() string {
	if e.Cause != nil {
		return fmt.Sprintf("%s: %v", e.Message, e.Cause)
	}
	return e.Message
}

// Unwrap returns the underlying error type.
func (e *AppError) Unwrap() error {
	return e.Type
}

// Is checks if this error matches the target.
func (e *AppError) Is(target error) bool {
	return errors.Is(e.Type, target)
}

// New creates a new AppError.
func New(errType error, message string) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
	}
}

// Wrap wraps an error with additional context.
func Wrap(errType error, message string, cause error) *AppError {
	return &AppError{
		Type:    errType,
		Message: message,
		Cause:   cause,
	}
}

// WithDetails adds details to an AppError.
func (e *AppError) WithDetails(details map[string]any) *AppError {
	e.Details = details
	return e
}

// NotFound creates a not found error.
func NotFound(resource string) *AppError {
	return &AppError{
		Type:    ErrNotFound,
		Message: fmt.Sprintf("%s not found", resource),
	}
}

// NotFoundf creates a not found error with formatting.
func NotFoundf(format string, args ...any) *AppError {
	return &AppError{
		Type:    ErrNotFound,
		Message: fmt.Sprintf(format, args...),
	}
}

// Unauthorized creates an unauthorized error.
func Unauthorized(message string) *AppError {
	if message == "" {
		message = "authentication required"
	}
	return &AppError{
		Type:    ErrUnauthorized,
		Message: message,
	}
}

// Forbidden creates a forbidden error.
func Forbidden(message string) *AppError {
	if message == "" {
		message = "access denied"
	}
	return &AppError{
		Type:    ErrForbidden,
		Message: message,
	}
}

// Validation creates a validation error.
func Validation(message string) *AppError {
	return &AppError{
		Type:    ErrValidation,
		Message: message,
	}
}

// ValidationField creates a validation error for a specific field.
func ValidationField(field, message string) *AppError {
	return &AppError{
		Type:    ErrValidation,
		Message: message,
		Details: map[string]any{"field": field},
	}
}

// Conflict creates a conflict error.
func Conflict(message string) *AppError {
	return &AppError{
		Type:    ErrConflict,
		Message: message,
	}
}

// Internal creates an internal error.
func Internal(message string, cause error) *AppError {
	return &AppError{
		Type:    ErrInternal,
		Message: message,
		Cause:   cause,
	}
}

// IsNotFound checks if an error is a not found error.
func IsNotFound(err error) bool {
	return errors.Is(err, ErrNotFound)
}

// IsUnauthorized checks if an error is an unauthorized error.
func IsUnauthorized(err error) bool {
	return errors.Is(err, ErrUnauthorized)
}

// IsForbidden checks if an error is a forbidden error.
func IsForbidden(err error) bool {
	return errors.Is(err, ErrForbidden)
}

// IsValidation checks if an error is a validation error.
func IsValidation(err error) bool {
	return errors.Is(err, ErrValidation)
}

// IsConflict checks if an error is a conflict error.
func IsConflict(err error) bool {
	return errors.Is(err, ErrConflict)
}

// IsInternal checks if an error is an internal error.
func IsInternal(err error) bool {
	return errors.Is(err, ErrInternal)
}

// HTTPStatus returns the appropriate HTTP status code for an error.
func HTTPStatus(err error) int {
	switch {
	case errors.Is(err, ErrNotFound):
		return 404
	case errors.Is(err, ErrUnauthorized):
		return 401
	case errors.Is(err, ErrForbidden):
		return 403
	case errors.Is(err, ErrValidation):
		return 400
	case errors.Is(err, ErrConflict):
		return 409
	case errors.Is(err, ErrRateLimit):
		return 429
	case errors.Is(err, ErrDemoMode):
		return 403
	default:
		return 500
	}
}
