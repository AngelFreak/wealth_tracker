package repository

import (
	"path/filepath"
	"testing"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

func setupAccountTestDB(t *testing.T) (*database.DB, int64, int64) {
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

	// Create a test category
	result, err = db.Exec(`
		INSERT INTO categories (user_id, name, color)
		VALUES (?, ?, ?)
	`, userID, "Stocks", "#6366f1")
	if err != nil {
		t.Fatalf("failed to create test category: %v", err)
	}
	categoryID, _ := result.LastInsertId()

	return db, userID, categoryID
}

// Create tests

func TestAccountRepository_Create_ValidAccount_ReturnsID(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	account := &models.Account{
		UserID:      userID,
		CategoryID:  &categoryID,
		Name:        "Nordnet",
		Currency:    "DKK",
		IsLiability: false,
		IsActive:    true,
		Notes:       "Main stock account",
	}

	id, err := repo.Create(account)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestAccountRepository_Create_WithoutCategory_Succeeds(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	account := &models.Account{
		UserID:   userID,
		Name:     "Cash",
		Currency: "DKK",
	}

	id, err := repo.Create(account)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestAccountRepository_Create_DuplicateName_ReturnsError(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	account1 := &models.Account{
		UserID:     userID,
		CategoryID: &categoryID,
		Name:       "Nordnet",
		Currency:   "DKK",
	}
	account2 := &models.Account{
		UserID:     userID,
		CategoryID: &categoryID,
		Name:       "Nordnet",
		Currency:   "USD",
	}

	_, err := repo.Create(account1)
	if err != nil {
		t.Fatalf("First Create() error = %v", err)
	}

	_, err = repo.Create(account2)
	if err == nil {
		t.Error("Create() should return error for duplicate account name per user")
	}
}

func TestAccountRepository_Create_LiabilityAccount_Succeeds(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	account := &models.Account{
		UserID:      userID,
		Name:        "Mortgage",
		Currency:    "DKK",
		IsLiability: true,
		IsActive:    true,
	}

	id, err := repo.Create(account)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}

	// Verify liability flag is stored correctly
	found, _ := repo.GetByID(id)
	if !found.IsLiability {
		t.Error("Create() should preserve IsLiability flag")
	}
}

// GetByID tests

func TestAccountRepository_GetByID_Existing_ReturnsAccount(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	created := &models.Account{
		UserID:      userID,
		CategoryID:  &categoryID,
		Name:        "SaxoInvester",
		Currency:    "EUR",
		IsLiability: false,
		IsActive:    true,
		Notes:       "European stocks",
	}
	id, _ := repo.Create(created)

	found, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil for existing account")
	}
	if found.Name != created.Name {
		t.Errorf("GetByID() Name = %s, want %s", found.Name, created.Name)
	}
	if found.Currency != created.Currency {
		t.Errorf("GetByID() Currency = %s, want %s", found.Currency, created.Currency)
	}
	if found.Notes != created.Notes {
		t.Errorf("GetByID() Notes = %s, want %s", found.Notes, created.Notes)
	}
	if found.CategoryID == nil || *found.CategoryID != categoryID {
		t.Errorf("GetByID() CategoryID mismatch")
	}
}

func TestAccountRepository_GetByID_NonExistent_ReturnsNil(t *testing.T) {
	db, _, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	found, err := repo.GetByID(99999)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found != nil {
		t.Error("GetByID() should return nil for non-existent ID")
	}
}

// GetByUserID tests

func TestAccountRepository_GetByUserID_ReturnsUserAccounts(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create multiple accounts
	accounts := []*models.Account{
		{UserID: userID, CategoryID: &categoryID, Name: "Nordnet", Currency: "DKK"},
		{UserID: userID, CategoryID: &categoryID, Name: "SaxoBank", Currency: "DKK"},
		{UserID: userID, Name: "Cash", Currency: "DKK"},
	}
	for _, a := range accounts {
		repo.Create(a)
	}

	found, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}
	if len(found) != 3 {
		t.Errorf("GetByUserID() returned %d accounts, want 3", len(found))
	}
}

func TestAccountRepository_GetByUserID_SortedByName(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create accounts out of order
	repo.Create(&models.Account{UserID: userID, Name: "Zebra Bank", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, Name: "Alpha Invest", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, Name: "Middle Fund", Currency: "DKK"})

	found, err := repo.GetByUserID(userID)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}

	// Should be sorted alphabetically
	if found[0].Name != "Alpha Invest" {
		t.Errorf("First account should be 'Alpha Invest', got %s", found[0].Name)
	}
	if found[1].Name != "Middle Fund" {
		t.Errorf("Second account should be 'Middle Fund', got %s", found[1].Name)
	}
	if found[2].Name != "Zebra Bank" {
		t.Errorf("Third account should be 'Zebra Bank', got %s", found[2].Name)
	}
}

func TestAccountRepository_GetByUserID_ActiveOnly(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create active and inactive accounts
	repo.Create(&models.Account{UserID: userID, Name: "Active1", Currency: "DKK", IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Inactive", Currency: "DKK", IsActive: false})
	repo.Create(&models.Account{UserID: userID, Name: "Active2", Currency: "DKK", IsActive: true})

	found, err := repo.GetByUserIDActiveOnly(userID)
	if err != nil {
		t.Fatalf("GetByUserIDActiveOnly() error = %v, want nil", err)
	}
	if len(found) != 2 {
		t.Errorf("GetByUserIDActiveOnly() returned %d accounts, want 2", len(found))
	}
}

// GetByCategoryID tests

func TestAccountRepository_GetByCategoryID_ReturnsLinkedAccounts(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create accounts with and without category
	repo.Create(&models.Account{UserID: userID, CategoryID: &categoryID, Name: "Stock1", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, CategoryID: &categoryID, Name: "Stock2", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, Name: "NoCat", Currency: "DKK"})

	found, err := repo.GetByCategoryID(categoryID)
	if err != nil {
		t.Fatalf("GetByCategoryID() error = %v, want nil", err)
	}
	if len(found) != 2 {
		t.Errorf("GetByCategoryID() returned %d accounts, want 2", len(found))
	}
}

// Update tests

func TestAccountRepository_Update_ValidData_UpdatesAccount(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create account
	account := &models.Account{
		UserID:     userID,
		CategoryID: &categoryID,
		Name:       "Original",
		Currency:   "DKK",
		IsActive:   true,
		Notes:      "Original notes",
	}
	id, _ := repo.Create(account)

	// Update it
	account.ID = id
	account.Name = "Updated"
	account.Currency = "EUR"
	account.Notes = "Updated notes"
	account.IsActive = false

	err := repo.Update(account)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify
	found, _ := repo.GetByID(id)
	if found.Name != "Updated" {
		t.Errorf("Update() Name = %s, want Updated", found.Name)
	}
	if found.Currency != "EUR" {
		t.Errorf("Update() Currency = %s, want EUR", found.Currency)
	}
	if found.Notes != "Updated notes" {
		t.Errorf("Update() Notes = %s, want 'Updated notes'", found.Notes)
	}
	if found.IsActive {
		t.Error("Update() should set IsActive to false")
	}
}

func TestAccountRepository_Update_ChangeCategory_Succeeds(t *testing.T) {
	db, userID, categoryID1 := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create second category
	result, _ := db.Exec(`
		INSERT INTO categories (user_id, name, color)
		VALUES (?, ?, ?)
	`, userID, "Crypto", "#f59e0b")
	categoryID2, _ := result.LastInsertId()

	// Create account with first category
	account := &models.Account{
		UserID:     userID,
		CategoryID: &categoryID1,
		Name:       "Account",
		Currency:   "DKK",
	}
	id, _ := repo.Create(account)

	// Update to second category
	account.ID = id
	account.CategoryID = &categoryID2

	err := repo.Update(account)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify
	found, _ := repo.GetByID(id)
	if found.CategoryID == nil || *found.CategoryID != categoryID2 {
		t.Error("Update() should change category")
	}
}

func TestAccountRepository_Update_RemoveCategory_Succeeds(t *testing.T) {
	db, userID, categoryID := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create account with category
	account := &models.Account{
		UserID:     userID,
		CategoryID: &categoryID,
		Name:       "Account",
		Currency:   "DKK",
	}
	id, _ := repo.Create(account)

	// Remove category
	account.ID = id
	account.CategoryID = nil

	err := repo.Update(account)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify
	found, _ := repo.GetByID(id)
	if found.CategoryID != nil {
		t.Error("Update() should remove category")
	}
}

func TestAccountRepository_Update_NonExistent_ReturnsError(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	account := &models.Account{
		ID:       99999,
		UserID:   userID,
		Name:     "Fake",
		Currency: "DKK",
	}

	err := repo.Update(account)
	if err == nil {
		t.Error("Update() should return error for non-existent account")
	}
}

// Delete tests

func TestAccountRepository_Delete_ExistingAccount_Succeeds(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create account
	account := &models.Account{
		UserID:   userID,
		Name:     "ToDelete",
		Currency: "DKK",
	}
	id, _ := repo.Create(account)

	// Delete it
	err := repo.Delete(id)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	found, _ := repo.GetByID(id)
	if found != nil {
		t.Error("Account should be deleted")
	}
}

func TestAccountRepository_Delete_NonExistent_ReturnsError(t *testing.T) {
	db, _, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	err := repo.Delete(99999)
	if err == nil {
		t.Error("Delete() should return error for non-existent account")
	}
}

// Count tests

func TestAccountRepository_CountByUserID_ReturnsCorrectCount(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create some accounts
	repo.Create(&models.Account{UserID: userID, Name: "Acc1", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, Name: "Acc2", Currency: "DKK"})
	repo.Create(&models.Account{UserID: userID, Name: "Acc3", Currency: "DKK"})

	count, err := repo.CountByUserID(userID)
	if err != nil {
		t.Fatalf("CountByUserID() error = %v, want nil", err)
	}
	if count != 3 {
		t.Errorf("CountByUserID() = %d, want 3", count)
	}
}

func TestAccountRepository_CountActiveAssets_ReturnsCorrectCount(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create mix of accounts
	repo.Create(&models.Account{UserID: userID, Name: "Asset1", Currency: "DKK", IsLiability: false, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Asset2", Currency: "DKK", IsLiability: false, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Liability", Currency: "DKK", IsLiability: true, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Inactive", Currency: "DKK", IsLiability: false, IsActive: false})

	count, err := repo.CountActiveAssets(userID)
	if err != nil {
		t.Fatalf("CountActiveAssets() error = %v, want nil", err)
	}
	if count != 2 {
		t.Errorf("CountActiveAssets() = %d, want 2", count)
	}
}

func TestAccountRepository_CountActiveLiabilities_ReturnsCorrectCount(t *testing.T) {
	db, userID, _ := setupAccountTestDB(t)
	repo := NewAccountRepository(db)

	// Create mix of accounts
	repo.Create(&models.Account{UserID: userID, Name: "Asset", Currency: "DKK", IsLiability: false, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Loan1", Currency: "DKK", IsLiability: true, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "Loan2", Currency: "DKK", IsLiability: true, IsActive: true})
	repo.Create(&models.Account{UserID: userID, Name: "PaidOff", Currency: "DKK", IsLiability: true, IsActive: false})

	count, err := repo.CountActiveLiabilities(userID)
	if err != nil {
		t.Fatalf("CountActiveLiabilities() error = %v, want nil", err)
	}
	if count != 2 {
		t.Errorf("CountActiveLiabilities() = %d, want 2", count)
	}
}
