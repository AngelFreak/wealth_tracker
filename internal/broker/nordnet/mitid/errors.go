// Package mitid implements native Go MitID authentication for Nordnet.
package mitid

import "errors"

// MitID authentication errors.
var (
	// ErrTimeout is returned when the user doesn't approve within the time limit.
	ErrTimeout = errors.New("MitID authentication timed out - user did not approve in time")

	// ErrUserNotFound is returned when the MitID user ID doesn't exist.
	ErrUserNotFound = errors.New("MitID user not found")

	// ErrSessionNotFound is returned when the authentication session is invalid or expired.
	ErrSessionNotFound = errors.New("MitID authentication session not found")

	// ErrParallelSessions is returned when another MitID session is already active.
	ErrParallelSessions = errors.New("MitID parallel sessions detected - only one login session allowed at a time")

	// ErrSRPVerifyFailed is returned when SRP proof verification fails.
	ErrSRPVerifyFailed = errors.New("MitID SRP verification failed")

	// ErrAuthenticatorNotAvailable is returned when the requested auth method isn't available.
	ErrAuthenticatorNotAvailable = errors.New("MitID authenticator not available for this user")

	// ErrAuthenticatorCannotStart is returned when the authenticator fails to start.
	ErrAuthenticatorCannotStart = errors.New("MitID authenticator cannot be started")

	// ErrLoginRejected is returned when the user rejects the login in the app.
	ErrLoginRejected = errors.New("MitID login was rejected by user")

	// ErrIPBlocked is returned when the client IP is blocked by MitID.
	ErrIPBlocked = errors.New("MitID client IP is blocked - contact MitID support")

	// ErrInvalidPassword is returned when password verification fails.
	ErrInvalidPassword = errors.New("MitID password is invalid")

	// ErrInvalidToken is returned when TOTP token verification fails.
	ErrInvalidToken = errors.New("MitID token code is invalid")

	// ErrFinalizationFailed is returned when the final authorization code retrieval fails.
	ErrFinalizationFailed = errors.New("MitID finalization failed")
)
