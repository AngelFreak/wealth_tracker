package repository

import (
	"path/filepath"
	"testing"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

func setupCategoryTestDB(t *testing.T) (*database.DB, int64) {
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

	// Create a test user
	result, err := db.Exec(`
		INSERT INTO users (email, password_hash, name)
		VALUES (?, ?, ?)
	`, "test@example.com", "hashedpassword", "Test User")
	if err != nil {
		t.Fatalf("failed to create test user: %v", err)
	}
	userID, _ := result.LastInsertId()

	return db, userID
}

// Create tests

func TestCategoryRepository_Create_ValidCategory_ReturnsID(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	category := &models.Category{
		UserID:    userID,
		Name:      "Stocks",
		Color:     "#6366f1",
		Icon:      "chart-line",
		SortOrder: 1,
	}

	id, err := repo.Create(category)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestCategoryRepository_Create_DuplicateName_ReturnsError(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	category1 := &models.Category{
		UserID: userID,
		Name:   "Stocks",
		Color:  "#6366f1",
	}
	category2 := &models.Category{
		UserID: userID,
		Name:   "Stocks",
		Color:  "#22c55e",
	}

	_, err := repo.Create(category1)
	if err != nil {
		t.Fatalf("First Create() error = %v", err)
	}

	_, err = repo.Create(category2)
	if err == nil {
		t.Error("Create() should return error for duplicate category name per user")
	}
}

func TestCategoryRepository_Create_SameNameDifferentUsers_Succeeds(t *testing.T) {
	db, userID1 := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create second user
	result, _ := db.Exec(`
		INSERT INTO users (email, password_hash, name)
		VALUES (?, ?, ?)
	`, "test2@example.com", "hashedpassword", "Test User 2")
	userID2, _ := result.LastInsertId()

	category1 := &models.Category{
		UserID: userID1,
		Name:   "Stocks",
		Color:  "#6366f1",
	}
	category2 := &models.Category{
		UserID: userID2,
		Name:   "Stocks",
		Color:  "#22c55e",
	}

	_, err := repo.Create(category1)
	if err != nil {
		t.Fatalf("First Create() error = %v", err)
	}

	_, err = repo.Create(category2)
	if err != nil {
		t.Errorf("Create() should succeed for same name with different users, got error = %v", err)
	}
}

// GetByID tests

func TestCategoryRepository_GetByID_Existing_ReturnsCategory(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	created := &models.Category{
		UserID:    userID,
		Name:      "Crypto",
		Color:     "#f59e0b",
		Icon:      "bitcoin",
		SortOrder: 2,
	}
	id, _ := repo.Create(created)

	found, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil for existing category")
	}
	if found.Name != created.Name {
		t.Errorf("GetByID() Name = %s, want %s", found.Name, created.Name)
	}
	if found.Color != created.Color {
		t.Errorf("GetByID() Color = %s, want %s", found.Color, created.Color)
	}
	if found.Icon != created.Icon {
		t.Errorf("GetByID() Icon = %s, want %s", found.Icon, created.Icon)
	}
	if found.UserID != userID {
		t.Errorf("GetByID() UserID = %d, want %d", found.UserID, userID)
	}
}

func TestCategoryRepository_GetByID_NonExistent_ReturnsNil(t *testing.T) {
	db, _ := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	found, err := repo.GetByID(99999)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found != nil {
		t.Error("GetByID() should return nil for non-existent ID")
	}
}

// GetByUserID tests

func TestCategoryRepository_GetByUserID_ReturnsUserCategories(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create multiple categories
	categories := []*models.Category{
		{UserID: userID, Name: "Stocks", Color: "#6366f1", SortOrder: 1},
		{UserID: userID, Name: "Crypto", Color: "#f59e0b", SortOrder: 2},
		{UserID: userID, Name: "Cash", Color: "#22c55e", SortOrder: 0},
	}
	for _, c := range categories {
		repo.Create(c)
	}

	found, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}
	if len(found) != 3 {
		t.Errorf("GetByUserID() returned %d categories, want 3", len(found))
	}
}

func TestCategoryRepository_GetByUserID_SortedBySortOrder(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create categories with specific sort order
	repo.Create(&models.Category{UserID: userID, Name: "Third", Color: "#000", SortOrder: 3})
	repo.Create(&models.Category{UserID: userID, Name: "First", Color: "#000", SortOrder: 1})
	repo.Create(&models.Category{UserID: userID, Name: "Second", Color: "#000", SortOrder: 2})

	found, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}

	// Should be sorted by sort_order
	if found[0].Name != "First" {
		t.Errorf("First category should be 'First', got %s", found[0].Name)
	}
	if found[1].Name != "Second" {
		t.Errorf("Second category should be 'Second', got %s", found[1].Name)
	}
	if found[2].Name != "Third" {
		t.Errorf("Third category should be 'Third', got %s", found[2].Name)
	}
}

func TestCategoryRepository_GetByUserID_EmptyResult(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	found, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}
	if found == nil {
		t.Error("GetByUserID() should return empty slice, not nil")
	}
	if len(found) != 0 {
		t.Errorf("GetByUserID() returned %d categories, want 0", len(found))
	}
}

// Update tests

func TestCategoryRepository_Update_ValidData_UpdatesCategory(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create category
	category := &models.Category{
		UserID:    userID,
		Name:      "Stocks",
		Color:     "#6366f1",
		SortOrder: 1,
	}
	id, _ := repo.Create(category)

	// Update it
	category.ID = id
	category.Name = "Equities"
	category.Color = "#22c55e"
	category.SortOrder = 5

	err := repo.Update(category)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify
	found, _ := repo.GetByID(id)
	if found.Name != "Equities" {
		t.Errorf("Update() Name = %s, want Equities", found.Name)
	}
	if found.Color != "#22c55e" {
		t.Errorf("Update() Color = %s, want #22c55e", found.Color)
	}
	if found.SortOrder != 5 {
		t.Errorf("Update() SortOrder = %d, want 5", found.SortOrder)
	}
}

func TestCategoryRepository_Update_NonExistent_ReturnsError(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	category := &models.Category{
		ID:     99999,
		UserID: userID,
		Name:   "Fake",
		Color:  "#000",
	}

	err := repo.Update(category)
	if err == nil {
		t.Error("Update() should return error for non-existent category")
	}
}

// Delete tests

func TestCategoryRepository_Delete_ExistingCategory_Succeeds(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create category
	category := &models.Category{
		UserID: userID,
		Name:   "ToDelete",
		Color:  "#ff0000",
	}
	id, _ := repo.Create(category)

	// Delete it
	err := repo.Delete(id)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	found, _ := repo.GetByID(id)
	if found != nil {
		t.Error("Category should be deleted")
	}
}

func TestCategoryRepository_Delete_NonExistent_ReturnsError(t *testing.T) {
	db, _ := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	err := repo.Delete(99999)
	if err == nil {
		t.Error("Delete() should return error for non-existent category")
	}
}

// Count tests

func TestCategoryRepository_CountByUserID_ReturnsCorrectCount(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	// Create some categories
	repo.Create(&models.Category{UserID: userID, Name: "Cat1", Color: "#000"})
	repo.Create(&models.Category{UserID: userID, Name: "Cat2", Color: "#000"})

	count, err := repo.CountByUserID(userID)
	if err != nil {
		t.Fatalf("CountByUserID() error = %v, want nil", err)
	}
	if count != 2 {
		t.Errorf("CountByUserID() = %d, want 2", count)
	}
}

// NameExists tests

func TestCategoryRepository_NameExists_ExistingName_ReturnsTrue(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	repo.Create(&models.Category{UserID: userID, Name: "Stocks", Color: "#000"})

	exists, err := repo.NameExists(userID, "Stocks", 0)
	if err != nil {
		t.Fatalf("NameExists() error = %v, want nil", err)
	}
	if !exists {
		t.Error("NameExists() should return true for existing name")
	}
}

func TestCategoryRepository_NameExists_NonExistingName_ReturnsFalse(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	exists, err := repo.NameExists(userID, "NonExistent", 0)
	if err != nil {
		t.Fatalf("NameExists() error = %v, want nil", err)
	}
	if exists {
		t.Error("NameExists() should return false for non-existing name")
	}
}

func TestCategoryRepository_NameExists_ExcludingOwnID_ReturnsFalse(t *testing.T) {
	db, userID := setupCategoryTestDB(t)
	repo := NewCategoryRepository(db)

	category := &models.Category{UserID: userID, Name: "Stocks", Color: "#000"}
	id, _ := repo.Create(category)

	// When updating a category, we want to allow keeping the same name
	exists, err := repo.NameExists(userID, "Stocks", id)
	if err != nil {
		t.Fatalf("NameExists() error = %v, want nil", err)
	}
	if exists {
		t.Error("NameExists() should return false when excluding own ID")
	}
}
