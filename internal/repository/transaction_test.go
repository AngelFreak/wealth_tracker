package repository

import (
	"path/filepath"
	"testing"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

func setupTransactionTestDB(t *testing.T) (*database.DB, int64, int64) {
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

	// Create a test account
	result, err = db.Exec(`
		INSERT INTO accounts (user_id, name, currency, is_liability, is_active)
		VALUES (?, ?, ?, ?, ?)
	`, userID, "Test Account", "DKK", 0, 1)
	if err != nil {
		t.Fatalf("failed to create test account: %v", err)
	}
	accountID, _ := result.LastInsertId()

	return db, userID, accountID
}

// Create tests

func TestTransactionRepository_Create_ValidTransaction_ReturnsID(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          1000.50,
		BalanceAfter:    1000.50,
		Description:     "Initial deposit",
		TransactionDate: time.Now(),
	}

	id, err := repo.Create(txn)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestTransactionRepository_Create_NegativeAmount_Succeeds(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          -500.00,
		BalanceAfter:    500.50,
		Description:     "Withdrawal",
		TransactionDate: time.Now(),
	}

	id, err := repo.Create(txn)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

func TestTransactionRepository_Create_WithoutDescription_Succeeds(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          100.00,
		BalanceAfter:    100.00,
		TransactionDate: time.Now(),
	}

	id, err := repo.Create(txn)
	if err != nil {
		t.Fatalf("Create() error = %v, want nil", err)
	}
	if id <= 0 {
		t.Error("Create() returned non-positive ID")
	}
}

// GetByID tests

func TestTransactionRepository_GetByID_Existing_ReturnsTransaction(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	created := &models.Transaction{
		AccountID:       accountID,
		Amount:          2500.00,
		BalanceAfter:    2500.00,
		Description:     "Salary",
		TransactionDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC),
	}
	id, _ := repo.Create(created)

	found, err := repo.GetByID(id)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found == nil {
		t.Fatal("GetByID() returned nil for existing transaction")
	}
	if found.Amount != created.Amount {
		t.Errorf("GetByID() Amount = %f, want %f", found.Amount, created.Amount)
	}
	if found.Description != created.Description {
		t.Errorf("GetByID() Description = %s, want %s", found.Description, created.Description)
	}
	if found.AccountID != accountID {
		t.Errorf("GetByID() AccountID = %d, want %d", found.AccountID, accountID)
	}
}

func TestTransactionRepository_GetByID_NonExistent_ReturnsNil(t *testing.T) {
	db, _, _ := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	found, err := repo.GetByID(99999)
	if err != nil {
		t.Fatalf("GetByID() error = %v, want nil", err)
	}
	if found != nil {
		t.Error("GetByID() should return nil for non-existent ID")
	}
}

// GetByAccountID tests

func TestTransactionRepository_GetByAccountID_ReturnsTransactions(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create multiple transactions
	for i := 0; i < 3; i++ {
		repo.Create(&models.Transaction{
			AccountID:       accountID,
			Amount:          float64(i+1) * 100,
			BalanceAfter:    float64(i+1) * 100,
			TransactionDate: time.Now(),
		})
	}

	found, err := repo.GetByAccountID(accountID, 10, 0)
	if err != nil {
		t.Fatalf("GetByAccountID() error = %v, want nil", err)
	}
	if len(found) != 3 {
		t.Errorf("GetByAccountID() returned %d transactions, want 3", len(found))
	}
}

func TestTransactionRepository_GetByAccountID_SortedByDateDesc(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create transactions with different dates
	dates := []time.Time{
		time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
		time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	}
	for _, d := range dates {
		repo.Create(&models.Transaction{
			AccountID:       accountID,
			Amount:          100,
			BalanceAfter:    100,
			TransactionDate: d,
		})
	}

	found, _ := repo.GetByAccountID(accountID, 10, 0)

	// Should be sorted by date descending (newest first)
	if found[0].TransactionDate.Month() != time.March {
		t.Errorf("First transaction should be from March (newest), got %v", found[0].TransactionDate.Month())
	}
	if found[2].TransactionDate.Month() != time.January {
		t.Errorf("Last transaction should be from January (oldest), got %v", found[2].TransactionDate.Month())
	}
}

func TestTransactionRepository_GetByAccountID_WithPagination(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create 5 transactions
	for i := 0; i < 5; i++ {
		repo.Create(&models.Transaction{
			AccountID:       accountID,
			Amount:          float64(i+1) * 100,
			BalanceAfter:    float64(i+1) * 100,
			TransactionDate: time.Now().AddDate(0, 0, -i),
		})
	}

	// Get first page (2 items)
	page1, _ := repo.GetByAccountID(accountID, 2, 0)
	if len(page1) != 2 {
		t.Errorf("Page 1 should have 2 items, got %d", len(page1))
	}

	// Get second page (2 items)
	page2, _ := repo.GetByAccountID(accountID, 2, 2)
	if len(page2) != 2 {
		t.Errorf("Page 2 should have 2 items, got %d", len(page2))
	}

	// Get third page (1 item)
	page3, _ := repo.GetByAccountID(accountID, 2, 4)
	if len(page3) != 1 {
		t.Errorf("Page 3 should have 1 item, got %d", len(page3))
	}
}

// GetByUserID tests

func TestTransactionRepository_GetByUserID_ReturnsAllUserTransactions(t *testing.T) {
	db, userID, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create second account for same user
	result, _ := db.Exec(`
		INSERT INTO accounts (user_id, name, currency, is_liability, is_active)
		VALUES (?, ?, ?, ?, ?)
	`, userID, "Second Account", "EUR", 0, 1)
	account2ID, _ := result.LastInsertId()

	// Create transactions in both accounts
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 100, BalanceAfter: 100, TransactionDate: time.Now()})
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 200, BalanceAfter: 300, TransactionDate: time.Now()})
	repo.Create(&models.Transaction{AccountID: account2ID, Amount: 500, BalanceAfter: 500, TransactionDate: time.Now()})

	found, err := repo.GetByUserID(userID, 10, 0)
	if err != nil {
		t.Fatalf("GetByUserID() error = %v, want nil", err)
	}
	if len(found) != 3 {
		t.Errorf("GetByUserID() returned %d transactions, want 3", len(found))
	}
}

// GetByDateRange tests

func TestTransactionRepository_GetByDateRange_ReturnsFilteredTransactions(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create transactions across different months
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 100, BalanceAfter: 100, TransactionDate: time.Date(2024, 1, 15, 0, 0, 0, 0, time.UTC)})
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 200, BalanceAfter: 300, TransactionDate: time.Date(2024, 2, 15, 0, 0, 0, 0, time.UTC)})
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 300, BalanceAfter: 600, TransactionDate: time.Date(2024, 3, 15, 0, 0, 0, 0, time.UTC)})

	// Query for February only
	start := time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC)
	end := time.Date(2024, 2, 28, 23, 59, 59, 0, time.UTC)

	found, err := repo.GetByDateRange(accountID, start, end)
	if err != nil {
		t.Fatalf("GetByDateRange() error = %v, want nil", err)
	}
	if len(found) != 1 {
		t.Errorf("GetByDateRange() returned %d transactions, want 1", len(found))
	}
	if found[0].Amount != 200 {
		t.Errorf("GetByDateRange() Amount = %f, want 200", found[0].Amount)
	}
}

// Update tests

func TestTransactionRepository_Update_ValidData_UpdatesTransaction(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          100.00,
		BalanceAfter:    100.00,
		Description:     "Original",
		TransactionDate: time.Now(),
	}
	id, _ := repo.Create(txn)

	// Update it
	txn.ID = id
	txn.Amount = 150.00
	txn.Description = "Updated"

	err := repo.Update(txn)
	if err != nil {
		t.Fatalf("Update() error = %v, want nil", err)
	}

	// Verify
	found, _ := repo.GetByID(id)
	if found.Amount != 150.00 {
		t.Errorf("Update() Amount = %f, want 150", found.Amount)
	}
	if found.Description != "Updated" {
		t.Errorf("Update() Description = %s, want Updated", found.Description)
	}
}

func TestTransactionRepository_Update_NonExistent_ReturnsError(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		ID:              99999,
		AccountID:       accountID,
		Amount:          100.00,
		BalanceAfter:    100.00,
		TransactionDate: time.Now(),
	}

	err := repo.Update(txn)
	if err == nil {
		t.Error("Update() should return error for non-existent transaction")
	}
}

// Delete tests

func TestTransactionRepository_Delete_ExistingTransaction_Succeeds(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	txn := &models.Transaction{
		AccountID:       accountID,
		Amount:          100.00,
		BalanceAfter:    100.00,
		TransactionDate: time.Now(),
	}
	id, _ := repo.Create(txn)

	err := repo.Delete(id)
	if err != nil {
		t.Fatalf("Delete() error = %v, want nil", err)
	}

	// Verify it's gone
	found, _ := repo.GetByID(id)
	if found != nil {
		t.Error("Transaction should be deleted")
	}
}

func TestTransactionRepository_Delete_NonExistent_ReturnsError(t *testing.T) {
	db, _, _ := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	err := repo.Delete(99999)
	if err == nil {
		t.Error("Delete() should return error for non-existent transaction")
	}
}

// Count tests

func TestTransactionRepository_CountByAccountID_ReturnsCorrectCount(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create some transactions
	for i := 0; i < 5; i++ {
		repo.Create(&models.Transaction{
			AccountID:       accountID,
			Amount:          100,
			BalanceAfter:    float64(i+1) * 100,
			TransactionDate: time.Now(),
		})
	}

	count, err := repo.CountByAccountID(accountID)
	if err != nil {
		t.Fatalf("CountByAccountID() error = %v, want nil", err)
	}
	if count != 5 {
		t.Errorf("CountByAccountID() = %d, want 5", count)
	}
}

// GetLatestBalance tests

func TestTransactionRepository_GetLatestBalance_ReturnsCorrectBalance(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create transactions with different dates
	repo.Create(&models.Transaction{
		AccountID:       accountID,
		Amount:          100,
		BalanceAfter:    100,
		TransactionDate: time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
	})
	repo.Create(&models.Transaction{
		AccountID:       accountID,
		Amount:          200,
		BalanceAfter:    300,
		TransactionDate: time.Date(2024, 2, 1, 0, 0, 0, 0, time.UTC),
	})
	repo.Create(&models.Transaction{
		AccountID:       accountID,
		Amount:          -50,
		BalanceAfter:    250,
		TransactionDate: time.Date(2024, 3, 1, 0, 0, 0, 0, time.UTC),
	})

	balance, err := repo.GetLatestBalance(accountID)
	if err != nil {
		t.Fatalf("GetLatestBalance() error = %v, want nil", err)
	}
	if balance != 250 {
		t.Errorf("GetLatestBalance() = %f, want 250", balance)
	}
}

func TestTransactionRepository_GetLatestBalance_NoTransactions_ReturnsZero(t *testing.T) {
	db, _, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	balance, err := repo.GetLatestBalance(accountID)
	if err != nil {
		t.Fatalf("GetLatestBalance() error = %v, want nil", err)
	}
	if balance != 0 {
		t.Errorf("GetLatestBalance() = %f, want 0", balance)
	}
}

// SumByUserID tests

func TestTransactionRepository_SumByUserID_ReturnsCorrectTotals(t *testing.T) {
	db, userID, accountID := setupTransactionTestDB(t)
	repo := NewTransactionRepository(db)

	// Create transactions
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 1000, BalanceAfter: 1000, TransactionDate: time.Now()})
	repo.Create(&models.Transaction{AccountID: accountID, Amount: 500, BalanceAfter: 1500, TransactionDate: time.Now()})
	repo.Create(&models.Transaction{AccountID: accountID, Amount: -200, BalanceAfter: 1300, TransactionDate: time.Now()})

	total, err := repo.SumBalancesByUserID(userID)
	if err != nil {
		t.Fatalf("SumBalancesByUserID() error = %v, want nil", err)
	}
	// Should return the latest balance for each account
	if total != 1300 {
		t.Errorf("SumBalancesByUserID() = %f, want 1300", total)
	}
}
