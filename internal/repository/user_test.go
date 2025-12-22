package repository

import (
	"path/filepath"
	"testing"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
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

func TestUserRepository_Create_ValidUser_ReturnsID(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}

	id, err := repo.Create(user)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Errorf("Create() id = %d, want > 0", id)
	}
}

func TestUserRepository_Create_DuplicateEmail_ReturnsError(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}

	_, err := repo.Create(user)
	if err != nil {
		t.Fatalf("first Create() error = %v", err)
	}

	// Try to create another user with same email
	user2 := &models.User{
		Email:        "test@example.com",
		PasswordHash: "differenthash",
		Name:         "Another User",
	}

	_, err = repo.Create(user2)
	if err == nil {
		t.Error("Create() with duplicate email should return error")
	}
}

func TestUserRepository_GetByID_ExistingUser_ReturnsUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	// Create a user first
	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}
	id, _ := repo.Create(user)

	// Get by ID
	found, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil, want user")
	}
	if found.Email != user.Email {
		t.Errorf("GetByID() email = %q, want %q", found.Email, user.Email)
	}
	if found.Name != user.Name {
		t.Errorf("GetByID() name = %q, want %q", found.Name, user.Name)
	}
}

func TestUserRepository_GetByID_NonExistent_ReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	found, err := repo.GetByID(999)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found != nil {
		t.Errorf("GetByID() for non-existent id should return nil, got %v", found)
	}
}

func TestUserRepository_GetByEmail_ExistingUser_ReturnsUser(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	// Create a user first
	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}
	repo.Create(user)

	// Get by email
	found, err := repo.GetByEmail("test@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("GetByEmail() returned nil, want user")
	}
	if found.Email != user.Email {
		t.Errorf("GetByEmail() email = %q, want %q", found.Email, user.Email)
	}
}

func TestUserRepository_GetByEmail_NonExistent_ReturnsNil(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	found, err := repo.GetByEmail("nonexistent@example.com")
	if err != nil {
		t.Fatalf("GetByEmail() error = %v, want nil", err)
	}
	if found != nil {
		t.Errorf("GetByEmail() for non-existent email should return nil, got %v", found)
	}
}

func TestUserRepository_Update_ValidChanges_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	// Create a user first
	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}
	id, _ := repo.Create(user)

	// Update the user
	user.ID = id
	user.Name = "Updated Name"
	user.DefaultCurrency = "USD"
	user.Theme = "light"

	err := repo.Update(user)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify changes
	found, _ := repo.GetByID(id)
	if found.Name != "Updated Name" {
		t.Errorf("Update() name = %q, want %q", found.Name, "Updated Name")
	}
	if found.DefaultCurrency != "USD" {
		t.Errorf("Update() currency = %q, want %q", found.DefaultCurrency, "USD")
	}
	if found.Theme != "light" {
		t.Errorf("Update() theme = %q, want %q", found.Theme, "light")
	}
}

func TestUserRepository_UpdatePassword_ValidHash_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	// Create a user first
	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "oldhash",
		Name:         "Test User",
	}
	id, _ := repo.Create(user)

	// Update password
	err := repo.UpdatePassword(id, "newhash")
	if err != nil {
		t.Fatalf("UpdatePassword() error = %v, want nil", err)
	}

	// Verify change
	found, _ := repo.GetByID(id)
	if found.PasswordHash != "newhash" {
		t.Errorf("UpdatePassword() hash = %q, want %q", found.PasswordHash, "newhash")
	}
}

func TestUserRepository_Delete_ExistingUser_Succeeds(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	// Create a user first
	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}
	id, _ := repo.Create(user)

	// Delete
	err := repo.Delete(id)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify deleted
	found, _ := repo.GetByID(id)
	if found != nil {
		t.Error("GetByID() after Delete() should return nil")
	}
}

func TestUserRepository_Create_SetsDefaults(t *testing.T) {
	db := setupTestDB(t)
	repo := NewUserRepository(db)

	user := &models.User{
		Email:        "test@example.com",
		PasswordHash: "hashedpassword123",
		Name:         "Test User",
	}
	id, _ := repo.Create(user)

	found, _ := repo.GetByID(id)
	if found.DefaultCurrency != "DKK" {
		t.Errorf("Create() default currency = %q, want %q", found.DefaultCurrency, "DKK")
	}
	if found.Theme != "dark" {
		t.Errorf("Create() default theme = %q, want %q", found.Theme, "dark")
	}
}
