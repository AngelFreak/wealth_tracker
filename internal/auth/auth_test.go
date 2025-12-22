package auth

import (
	"path/filepath"
	"testing"
	"time"

	"wealth_tracker/internal/database"
)

func setupTestDB(t *testing.T) *database.DB {
	t.Helper()
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := database.New(dbPath)
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}

	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}

	t.Cleanup(func() {
		db.Close()
	})

	return db
}

// createTestUser creates a test user and returns the user ID
func createTestUser(t *testing.T, db *database.DB) int64 {
	t.Helper()
	result, err := db.Exec(`
		INSERT INTO users (email, password_hash, name)
		VALUES (?, ?, ?)
	`, "test@example.com", "hashedpassword", "Test User")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	id, _ := result.LastInsertId()
	return id
}

// Password hashing tests

func TestHashPassword_ValidPassword_ReturnsHash(t *testing.T) {
	password := "securepassword123"

	hash, err := HashPassword(password)
	if err != nil {
		t.Fatalf("HashPassword() error = %v, want nil", err)
	}
	if hash == "" {
		t.Error("HashPassword() returned empty hash")
	}
	if hash == password {
		t.Error("HashPassword() returned plaintext password")
	}
}

func TestHashPassword_DifferentPasswords_DifferentHashes(t *testing.T) {
	hash1, _ := HashPassword("password1")
	hash2, _ := HashPassword("password2")

	if hash1 == hash2 {
		t.Error("HashPassword() should return different hashes for different passwords")
	}
}

func TestHashPassword_SamePassword_DifferentHashes(t *testing.T) {
	// Due to salting, same password should produce different hashes
	hash1, _ := HashPassword("samepassword")
	hash2, _ := HashPassword("samepassword")

	if hash1 == hash2 {
		t.Error("HashPassword() should return different hashes even for same password (due to salting)")
	}
}

func TestCheckPassword_CorrectPassword_ReturnsTrue(t *testing.T) {
	password := "correctpassword"
	hash, _ := HashPassword(password)

	if !CheckPassword(password, hash) {
		t.Error("CheckPassword() should return true for correct password")
	}
}

func TestCheckPassword_IncorrectPassword_ReturnsFalse(t *testing.T) {
	password := "correctpassword"
	hash, _ := HashPassword(password)

	if CheckPassword("wrongpassword", hash) {
		t.Error("CheckPassword() should return false for incorrect password")
	}
}

func TestCheckPassword_EmptyPassword_ReturnsFalse(t *testing.T) {
	hash, _ := HashPassword("somepassword")

	if CheckPassword("", hash) {
		t.Error("CheckPassword() should return false for empty password")
	}
}

func TestCheckPassword_EmptyHash_ReturnsFalse(t *testing.T) {
	if CheckPassword("password", "") {
		t.Error("CheckPassword() should return false for empty hash")
	}
}

// Session tests

func TestSessionManager_Create_ReturnsValidSession(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	session, err := sm.Create(userID)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if session == nil {
		t.Fatal("Create() returned nil session")
	}
	if session.ID == "" {
		t.Error("Create() returned empty session ID")
	}
	if session.UserID != userID {
		t.Errorf("Create() UserID = %d, want %d", session.UserID, userID)
	}
	if session.ExpiresAt.Before(time.Now()) {
		t.Error("Create() returned already expired session")
	}
}

func TestSessionManager_Get_ExistingSession_ReturnsSession(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	// Create session first
	created, _ := sm.Create(userID)

	// Get it back
	found, err := sm.Get(created.ID)
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("Get() returned nil for existing session")
	}
	if found.UserID != created.UserID {
		t.Errorf("Get() UserID = %d, want %d", found.UserID, created.UserID)
	}
}

func TestSessionManager_Get_NonExistent_ReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db)

	found, err := sm.Get("nonexistent-session-id")
	if err != nil {
		t.Fatalf("Get() error = %v, want nil", err)
	}
	if found != nil {
		t.Error("Get() should return nil for non-existent session")
	}
}

func TestSessionManager_Delete_RemovesSession(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	// Create session
	created, _ := sm.Create(userID)

	// Delete it
	err := sm.Delete(created.ID)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	found, _ := sm.Get(created.ID)
	if found != nil {
		t.Error("Get() after Delete() should return nil")
	}
}

func TestSessionManager_DeleteByUserID_RemovesAllUserSessions(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	// Create multiple sessions for same user
	s1, _ := sm.Create(userID)
	s2, _ := sm.Create(userID)

	// Delete all sessions for user
	err := sm.DeleteByUserID(userID)
	if err != nil {
		t.Fatalf("DeleteByUserID() error = %v, want nil", err)
	}

	// Verify all are gone
	found1, _ := sm.Get(s1.ID)
	found2, _ := sm.Get(s2.ID)
	if found1 != nil || found2 != nil {
		t.Error("DeleteByUserID() should remove all user sessions")
	}
}

func TestSessionManager_CleanExpired_RemovesExpiredSessions(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	// Insert an expired session directly
	_, err := db.Exec(`
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES ('expired-session', ?, ?, ?)
	`, userID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("inserting expired session: %v", err)
	}

	// Create a valid session
	validSession, _ := sm.Create(userID)

	// Clean expired
	count, err := sm.CleanExpired()
	if err != nil {
		t.Fatalf("CleanExpired() error = %v, want nil", err)
	}
	if count != 1 {
		t.Errorf("CleanExpired() removed %d sessions, want 1", count)
	}

	// Verify expired is gone but valid remains
	found, _ := sm.Get("expired-session")
	if found != nil {
		t.Error("expired session should be removed")
	}
	found, _ = sm.Get(validSession.ID)
	if found == nil {
		t.Error("valid session should remain")
	}
}

func TestSessionManager_Validate_ValidSession_ReturnsUserID(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	session, _ := sm.Create(userID)

	validatedUserID, err := sm.Validate(session.ID)
	if err != nil {
		t.Fatalf("Validate() error = %v, want nil", err)
	}
	if validatedUserID != userID {
		t.Errorf("Validate() userID = %d, want %d", validatedUserID, userID)
	}
}

func TestSessionManager_Validate_ExpiredSession_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	userID := createTestUser(t, db)
	sm := NewSessionManager(db)

	// Insert an expired session
	_, err := db.Exec(`
		INSERT INTO sessions (id, user_id, expires_at, created_at)
		VALUES ('expired-session', ?, ?, ?)
	`, userID, time.Now().Add(-time.Hour), time.Now().Add(-2*time.Hour))
	if err != nil {
		t.Fatalf("inserting expired session: %v", err)
	}

	_, err = sm.Validate("expired-session")
	if err == nil {
		t.Error("Validate() should return error for expired session")
	}
}

func TestSessionManager_Validate_NonExistent_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	sm := NewSessionManager(db)

	_, err := sm.Validate("nonexistent")
	if err == nil {
		t.Error("Validate() should return error for non-existent session")
	}
}
