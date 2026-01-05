package database

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNew_CreatesConnection(t *testing.T) {
	// Setup: use temporary directory
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	// Test: create new database connection
	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v, want nil", err)
	}
	defer db.Close()

	// Verify: database file exists
	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		t.Error("database file was not created")
	}

	// Verify: can ping database
	if err := db.Ping(); err != nil {
		t.Errorf("Ping() error = %v, want nil", err)
	}
}

func TestNew_InvalidPath_ReturnsError(t *testing.T) {
	// Test with invalid path (directory that doesn't exist and can't be created)
	_, err := New("/nonexistent/path/that/cannot/be/created/test.db")
	if err == nil {
		t.Error("New() with invalid path should return error")
	}
}

func TestRunMigrations_CreatesAllTables(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	// Test: run migrations
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations() error = %v, want nil", err)
	}

	// Verify: all tables exist
	expectedTables := []string{
		"users",
		"categories",
		"accounts",
		"transactions",
		"goals",
		"currency_rates",
		"sessions",
	}

	for _, table := range expectedTables {
		var exists int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?`
		err := db.QueryRow(query, table).Scan(&exists)
		if err != nil {
			t.Errorf("checking table %s: %v", table, err)
			continue
		}
		if exists != 1 {
			t.Errorf("table %s does not exist", table)
		}
	}
}

func TestRunMigrations_CreatesIndexes(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Verify: indexes exist
	expectedIndexes := []string{
		"idx_accounts_user",
		"idx_transactions_account",
		"idx_transactions_date",
		"idx_categories_user",
		"idx_goals_user",
		"idx_sessions_user",
		"idx_sessions_expires",
	}

	for _, index := range expectedIndexes {
		var exists int
		query := `SELECT COUNT(*) FROM sqlite_master WHERE type='index' AND name=?`
		err := db.QueryRow(query, index).Scan(&exists)
		if err != nil {
			t.Errorf("checking index %s: %v", index, err)
			continue
		}
		if exists != 1 {
			t.Errorf("index %s does not exist", index)
		}
	}
}

func TestRunMigrations_Idempotent(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	// Test: run migrations multiple times
	for i := 0; i < 3; i++ {
		if err := db.RunMigrations(); err != nil {
			t.Fatalf("RunMigrations() iteration %d error = %v, want nil", i+1, err)
		}
	}

	// Verify: still works and has correct tables
	var tableCount int
	query := `SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name NOT LIKE 'sqlite_%'`
	if err := db.QueryRow(query).Scan(&tableCount); err != nil {
		t.Fatalf("counting tables: %v", err)
	}

	expectedCount := 14 // users, categories, accounts, transactions, goals, currency_rates, sessions + broker_connections, broker_sessions, holdings, account_mappings, sync_history + allocation_targets + audit_log
	if tableCount != expectedCount {
		t.Errorf("table count = %d, want %d", tableCount, expectedCount)
	}
}

func TestDB_Close(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}

	// Test: close database
	if err := db.Close(); err != nil {
		t.Errorf("Close() error = %v, want nil", err)
	}

	// Verify: operations fail after close
	if err := db.Ping(); err == nil {
		t.Error("Ping() after Close() should return error")
	}
}

func TestDB_Exec_InsertAndQuery(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Test: insert a user
	result, err := db.Exec(
		`INSERT INTO users (email, password_hash, name) VALUES (?, ?, ?)`,
		"test@example.com",
		"hashedpassword",
		"Test User",
	)
	if err != nil {
		t.Fatalf("Exec() insert error = %v", err)
	}

	// Verify: got last insert ID
	id, err := result.LastInsertId()
	if err != nil {
		t.Fatalf("LastInsertId() error = %v", err)
	}
	if id != 1 {
		t.Errorf("LastInsertId() = %d, want 1", id)
	}

	// Verify: can query the user
	var email, name string
	err = db.QueryRow(`SELECT email, name FROM users WHERE id = ?`, id).Scan(&email, &name)
	if err != nil {
		t.Fatalf("QueryRow() error = %v", err)
	}
	if email != "test@example.com" {
		t.Errorf("email = %q, want %q", email, "test@example.com")
	}
	if name != "Test User" {
		t.Errorf("name = %q, want %q", name, "Test User")
	}
}

func TestDB_ForeignKeyConstraints(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Test: try to insert account with non-existent user_id
	_, err = db.Exec(
		`INSERT INTO accounts (user_id, name) VALUES (?, ?)`,
		999, // Non-existent user
		"Test Account",
	)
	if err == nil {
		t.Error("inserting account with invalid user_id should fail")
	}
}

func TestDB_CascadeDelete(t *testing.T) {
	// Setup
	tmpDir := t.TempDir()
	dbPath := filepath.Join(tmpDir, "test.db")

	db, err := New(dbPath)
	if err != nil {
		t.Fatalf("New() error = %v", err)
	}
	defer db.Close()

	if err := db.RunMigrations(); err != nil {
		t.Fatalf("RunMigrations() error = %v", err)
	}

	// Insert a user
	result, err := db.Exec(
		`INSERT INTO users (email, password_hash, name) VALUES (?, ?, ?)`,
		"test@example.com",
		"hashedpassword",
		"Test User",
	)
	if err != nil {
		t.Fatalf("insert user error = %v", err)
	}
	userID, _ := result.LastInsertId()

	// Insert an account for the user
	result, err = db.Exec(
		`INSERT INTO accounts (user_id, name) VALUES (?, ?)`,
		userID,
		"Test Account",
	)
	if err != nil {
		t.Fatalf("insert account error = %v", err)
	}
	accountID, _ := result.LastInsertId()

	// Insert a transaction for the account
	_, err = db.Exec(
		`INSERT INTO transactions (account_id, amount, balance_after, transaction_date) VALUES (?, ?, ?, ?)`,
		accountID,
		1000.00,
		1000.00,
		"2024-01-15",
	)
	if err != nil {
		t.Fatalf("insert transaction error = %v", err)
	}

	// Test: delete user (should cascade delete account and transaction)
	_, err = db.Exec(`DELETE FROM users WHERE id = ?`, userID)
	if err != nil {
		t.Fatalf("delete user error = %v", err)
	}

	// Verify: account is deleted
	var accountCount int
	db.QueryRow(`SELECT COUNT(*) FROM accounts WHERE id = ?`, accountID).Scan(&accountCount)
	if accountCount != 0 {
		t.Error("account should be deleted after user delete")
	}

	// Verify: transaction is deleted
	var txCount int
	db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, accountID).Scan(&txCount)
	if txCount != 0 {
		t.Error("transaction should be deleted after account delete")
	}
}
