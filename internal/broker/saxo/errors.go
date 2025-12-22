// Package saxo provides a client for the Saxo Bank OpenAPI.
package saxo

import "errors"

var (
	// ErrSessionExpired indicates the OAuth session has expired.
	ErrSessionExpired = errors.New("session expired")

	// ErrAuthenticationFailed indicates authentication failed.
	ErrAuthenticationFailed = errors.New("authentication failed")

	// ErrOAuthTimeout indicates the user did not complete OAuth login in time.
	ErrOAuthTimeout = errors.New("OAuth authentication timed out - user did not complete login")

	// ErrOAuthCancelled indicates the user cancelled OAuth authentication.
	ErrOAuthCancelled = errors.New("OAuth authentication was cancelled")

	// ErrOAuthInProgress indicates an OAuth flow is already in progress.
	ErrOAuthInProgress = errors.New("OAuth authentication already in progress")

	// ErrRefreshTokenExpired indicates the refresh token has expired.
	ErrRefreshTokenExpired = errors.New("refresh token expired - please re-authenticate")

	// ErrClientKeyNotFound indicates the ClientKey could not be retrieved.
	ErrClientKeyNotFound = errors.New("could not retrieve client key from Saxo")

	// ErrInvalidState indicates OAuth state mismatch (possible CSRF attack).
	ErrInvalidState = errors.New("OAuth state mismatch - possible security issue")

	// ErrNoAuthCode indicates no authorization code was received.
	ErrNoAuthCode = errors.New("no authorization code received from Saxo")
)
