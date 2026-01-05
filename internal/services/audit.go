// Package services provides business logic services.
package services

import (
	"encoding/json"
	"fmt"
	"log"
	"time"

	"wealth_tracker/internal/database"
)

// AuditAction represents the type of audited action.
type AuditAction string

const (
	// User actions
	AuditUserCreated  AuditAction = "user.created"
	AuditUserUpdated  AuditAction = "user.updated"
	AuditUserDeleted  AuditAction = "user.deleted"
	AuditUserLogin    AuditAction = "user.login"
	AuditUserLogout   AuditAction = "user.logout"
	AuditPasswordChanged AuditAction = "user.password_changed"

	// Admin actions
	AuditAdminUserCreated    AuditAction = "admin.user_created"
	AuditAdminUserUpdated    AuditAction = "admin.user_updated"
	AuditAdminUserDeleted    AuditAction = "admin.user_deleted"
	AuditAdminPasswordReset  AuditAction = "admin.password_reset"
	AuditAdminSettingsChanged AuditAction = "admin.settings_changed"

	// Account actions
	AuditAccountCreated AuditAction = "account.created"
	AuditAccountUpdated AuditAction = "account.updated"
	AuditAccountDeleted AuditAction = "account.deleted"

	// Transaction actions
	AuditTransactionCreated AuditAction = "transaction.created"
	AuditTransactionUpdated AuditAction = "transaction.updated"
	AuditTransactionDeleted AuditAction = "transaction.deleted"

	// Broker actions
	AuditBrokerConnected    AuditAction = "broker.connected"
	AuditBrokerDisconnected AuditAction = "broker.disconnected"
	AuditBrokerSynced       AuditAction = "broker.synced"

	// Portfolio actions
	AuditTargetCreated AuditAction = "portfolio.target_created"
	AuditTargetUpdated AuditAction = "portfolio.target_updated"
	AuditTargetDeleted AuditAction = "portfolio.target_deleted"
)

// AuditEntry represents an audit log entry.
type AuditEntry struct {
	ID         int64       `json:"id"`
	UserID     int64       `json:"user_id"`
	ActorID    int64       `json:"actor_id"` // Who performed the action (may differ from UserID for admin actions)
	Action     AuditAction `json:"action"`
	EntityType string      `json:"entity_type"`
	EntityID   int64       `json:"entity_id"`
	OldValues  string      `json:"old_values,omitempty"` // JSON
	NewValues  string      `json:"new_values,omitempty"` // JSON
	IPAddress  string      `json:"ip_address"`
	UserAgent  string      `json:"user_agent"`
	CreatedAt  time.Time   `json:"created_at"`
}

// AuditService handles audit logging.
type AuditService struct {
	db *database.DB
}

// NewAuditService creates a new AuditService.
func NewAuditService(db *database.DB) *AuditService {
	return &AuditService{db: db}
}

// Log records an audit entry.
func (s *AuditService) Log(entry *AuditEntry) error {
	_, err := s.db.Exec(`
		INSERT INTO audit_log (user_id, actor_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent, created_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, entry.UserID, entry.ActorID, entry.Action, entry.EntityType, entry.EntityID,
		entry.OldValues, entry.NewValues, entry.IPAddress, entry.UserAgent, time.Now())

	if err != nil {
		log.Printf("Failed to write audit log: %v", err)
		return err
	}
	return nil
}

// LogAction is a convenience method for logging an action with automatic JSON serialization.
func (s *AuditService) LogAction(userID, actorID int64, action AuditAction, entityType string, entityID int64, oldVal, newVal any, ip, userAgent string) {
	entry := &AuditEntry{
		UserID:     userID,
		ActorID:    actorID,
		Action:     action,
		EntityType: entityType,
		EntityID:   entityID,
		IPAddress:  ip,
		UserAgent:  userAgent,
	}

	if oldVal != nil {
		if data, err := json.Marshal(oldVal); err == nil {
			entry.OldValues = string(data)
		}
	}

	if newVal != nil {
		if data, err := json.Marshal(newVal); err == nil {
			entry.NewValues = string(data)
		}
	}

	if err := s.Log(entry); err != nil {
		log.Printf("Audit log failed for action %s: %v", action, err)
	}
}

// GetByUserID retrieves audit entries for a user.
func (s *AuditService) GetByUserID(userID int64, limit, offset int) ([]*AuditEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, actor_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent, created_at
		FROM audit_log
		WHERE user_id = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID,
			&e.OldValues, &e.NewValues, &e.IPAddress, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetByAction retrieves audit entries by action type.
func (s *AuditService) GetByAction(action AuditAction, limit, offset int) ([]*AuditEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, actor_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent, created_at
		FROM audit_log
		WHERE action = ?
		ORDER BY created_at DESC
		LIMIT ? OFFSET ?
	`, action, limit, offset)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID,
			&e.OldValues, &e.NewValues, &e.IPAddress, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// GetRecent retrieves the most recent audit entries.
func (s *AuditService) GetRecent(limit int) ([]*AuditEntry, error) {
	rows, err := s.db.Query(`
		SELECT id, user_id, actor_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent, created_at
		FROM audit_log
		ORDER BY created_at DESC
		LIMIT ?
	`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []*AuditEntry
	for rows.Next() {
		e := &AuditEntry{}
		if err := rows.Scan(&e.ID, &e.UserID, &e.ActorID, &e.Action, &e.EntityType, &e.EntityID,
			&e.OldValues, &e.NewValues, &e.IPAddress, &e.UserAgent, &e.CreatedAt); err != nil {
			return nil, err
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// DeleteOlderThan removes audit entries older than the given duration.
func (s *AuditService) DeleteOlderThan(d time.Duration) (int64, error) {
	cutoff := time.Now().Add(-d)
	result, err := s.db.Exec(`DELETE FROM audit_log WHERE created_at < ?`, cutoff)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// FormatEntry returns a human-readable description of an audit entry.
func FormatEntry(e *AuditEntry) string {
	return fmt.Sprintf("[%s] User %d: %s %s (ID: %d)",
		e.CreatedAt.Format("2006-01-02 15:04:05"),
		e.ActorID, e.Action, e.EntityType, e.EntityID)
}
