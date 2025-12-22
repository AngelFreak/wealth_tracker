// Package auth provides authentication and session management.
package auth

import (
	"crypto/rand"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"time"

	"golang.org/x/crypto/bcrypt"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

const (
	// DefaultSessionDuration is the default session lifetime.
	DefaultSessionDuration = 7 * 24 * time.Hour // 7 days

	// BcryptCost is the bcrypt hashing cost.
	BcryptCost = 12
)

var (
	// ErrInvalidCredentials is returned when login credentials are invalid.
	ErrInvalidCredentials = errors.New("invalid email or password")

	// ErrSessionExpired is returned when a session has expired.
	ErrSessionExpired = errors.New("session expired")

	// ErrSessionNotFound is returned when a session doesn't exist.
	ErrSessionNotFound = errors.New("session not found")
)

// HashPassword hashes a password using bcrypt.
func HashPassword(password string) (string, error) {
	bytes, err := bcrypt.GenerateFromPassword([]byte(password), BcryptCost)
	if err != nil {
		return "", fmt.Errorf("hashing password: %w", err)
	}
	return string(bytes), nil
}

// CheckPassword compares a password with a hash.
func CheckPassword(password, hash string) bool {
	if password == "" || hash == "" {
		return false
	}
	err := bcrypt.CompareHashAndPassword([]byte(hash), []byte(password))
	return err == nil
}

// SessionManager handles session operations.
type SessionManager struct {
	db       *database.DB
	duration time.Duration
}

// NewSessionManager creates a new SessionManager.
func NewSessionManager(db *database.DB) *SessionManager {
	return &SessionManager{
		db:       db,
		duration: DefaultSessionDuration,
	}
}

// WithDuration sets a custom session duration.
func (sm *SessionManager) WithDuration(d time.Duration) *SessionManager {
	sm.duration = d
	return sm
}

// Create creates a new session for a user.
func (sm *SessionManager) Create(userID int64) (*models.Session, error) {
	// Generate random session ID
	id, err := generateSessionID()
	if err != nil {
		return nil, err
	}

	session := &models.Session{
		ID:        id,
		UserID:    userID,
		ExpiresAt: time.Now().Add(sm.duration),
		CreatedAt: time.Now(),
	}

	query := `
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES (?, ?, ?, ?)
	`
	_, err = sm.db.Exec(query, session.ID, session.UserID, session.ExpiresAt, session.CreatedAt)
	if err != nil {
		return nil, fmt.Errorf("creating session: %w", err)
	}

	return session, nil
}

// Get retrieves a session by ID. Returns nil if not found.
func (sm *SessionManager) Get(id string) (*models.Session, error) {
	query := `
		SELECT id, user_id, expires_at, created_at
		FROM sessions
		WHERE id = ?
	`

	session := &models.Session{}
	err := sm.db.QueryRow(query, id).Scan(
		&session.ID,
		&session.UserID,
		&session.ExpiresAt,
		&session.CreatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting session: %w", err)
	}

	return session, nil
}

// Validate checks if a session is valid and returns the user ID.
func (sm *SessionManager) Validate(id string) (int64, error) {
	session, err := sm.Get(id)
	if err != nil {
		return 0, err
	}
	if session == nil {
		return 0, ErrSessionNotFound
	}
	if session.IsExpired() {
		// Clean up the expired session
		sm.Delete(id)
		return 0, ErrSessionExpired
	}
	return session.UserID, nil
}

// Delete removes a session by ID.
func (sm *SessionManager) Delete(id string) error {
	query := `DELETE FROM sessions WHERE id = ?`
	_, err := sm.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("deleting session: %w", err)
	}
	return nil
}

// DeleteByUserID removes all sessions for a user.
func (sm *SessionManager) DeleteByUserID(userID int64) error {
	query := `DELETE FROM sessions WHERE user_id = ?`
	_, err := sm.db.Exec(query, userID)
	if err != nil {
		return fmt.Errorf("deleting user sessions: %w", err)
	}
	return nil
}

// CleanExpired removes all expired sessions and returns the count.
func (sm *SessionManager) CleanExpired() (int64, error) {
	query := `DELETE FROM sessions WHERE expires_at < ?`
	result, err := sm.db.Exec(query, time.Now())
	if err != nil {
		return 0, fmt.Errorf("cleaning expired sessions: %w", err)
	}

	count, err := result.RowsAffected()
	if err != nil {
		return 0, fmt.Errorf("getting affected rows: %w", err)
	}

	return count, nil
}

// generateSessionID creates a cryptographically secure session ID.
func generateSessionID() (string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", fmt.Errorf("generating session id: %w", err)
	}
	return hex.EncodeToString(bytes), nil
}
