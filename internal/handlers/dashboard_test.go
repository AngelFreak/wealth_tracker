package handlers

import (
	"math"
	"path/filepath"
	"testing"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

func TestDashboard_CalculateMonthlyChange_DebtPaydownRaisesNetWorth(t *testing.T) {
	db, err := database.New(filepath.Join(t.TempDir(), "test.db"))
	if err != nil {
		t.Fatalf("failed to create database: %v", err)
	}
	if err := db.RunMigrations(); err != nil {
		t.Fatalf("failed to run migrations: %v", err)
	}
	t.Cleanup(func() { db.Close() })

	accountRepo := repository.NewAccountRepository(db)
	txnRepo := repository.NewTransactionRepository(db)
	h := NewDashboardHandler(nil, accountRepo, txnRepo, nil, nil, nil)
	user := insertHandlerUser(t, db, "owner@example.com")

	now := time.Now()
	lastMonth := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location()).AddDate(0, 0, -5)
	thisMonth := time.Date(now.Year(), now.Month(), 1, 12, 0, 0, 0, now.Location())

	addAccount := func(name string, liability bool) int64 {
		id, err := accountRepo.Create(&models.Account{UserID: user.ID, Name: name, Currency: "DKK", IsLiability: liability, IsActive: true})
		if err != nil {
			t.Fatalf("create account %s: %v", name, err)
		}
		return id
	}
	addTxn := func(accountID int64, amount, balance float64, date time.Time) {
		if _, err := txnRepo.Create(&models.Transaction{AccountID: accountID, Amount: amount, BalanceAfter: balance, Description: "test", TransactionDate: date}); err != nil {
			t.Fatalf("create transaction: %v", err)
		}
	}

	// Cash moves 25k into paying down the mortgage: net worth is unchanged.
	cash := addAccount("Cash", false)
	addTxn(cash, 90000, 90000, lastMonth)
	addTxn(cash, -25000, 65000, thisMonth)

	// Debt stored as a positive balance, paid down by 25k.
	mortgage := addAccount("Mortgage", true)
	addTxn(mortgage, 200000, 200000, lastMonth)
	addTxn(mortgage, -25000, 175000, thisMonth)

	if change, _ := h.calculateMonthlyChange(user.ID, 0); math.Abs(change) > 0.001 {
		t.Fatalf("moving cash into debt paydown: got change %.2f, want 0", change)
	}

	// Debt stored as a negative balance, paid down by 10k: net worth +10k.
	card := addAccount("Credit card", true)
	addTxn(card, -30000, -30000, lastMonth)
	addTxn(card, 10000, -20000, thisMonth)

	if change, _ := h.calculateMonthlyChange(user.ID, 0); math.Abs(change-10000) > 0.001 {
		t.Fatalf("paying down negatively stored debt: got change %.2f, want 10000", change)
	}

	// New borrowing lowers net worth.
	addTxn(mortgage, 5000, 180000, thisMonth)

	if change, _ := h.calculateMonthlyChange(user.ID, 0); math.Abs(change-5000) > 0.001 {
		t.Fatalf("after 5k new borrowing: got change %.2f, want 5000", change)
	}
}
