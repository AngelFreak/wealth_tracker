package services

import (
	"testing"

	"wealth_tracker/internal/models"
	"wealth_tracker/internal/repository"
)

// setupSyncTest builds a loan service backed by real account/transaction
// repos and returns a user + a category to attach loans to.
func setupSyncTest(t *testing.T) (*LoanService, *repository.LoanRepository, *repository.AccountRepository, *repository.TransactionRepository, int64, int64) {
	t.Helper()
	db := setupServiceLoanDB(t)
	loanRepo := repository.NewLoanRepository(db)
	accountRepo := repository.NewAccountRepository(db)
	txnRepo := repository.NewTransactionRepository(db)
	svc := NewLoanService(loanRepo, accountRepo, txnRepo)
	userID := insertServiceUser(t, db)

	res, err := db.Exec(`INSERT INTO categories (user_id, name, color) VALUES (?, ?, ?)`, userID, "Ejendom", "#8b5cf6")
	if err != nil {
		t.Fatalf("insert category: %v", err)
	}
	catID, _ := res.LastInsertId()
	return svc, loanRepo, accountRepo, txnRepo, userID, catID
}

func balanceOf(t *testing.T, txnRepo *repository.TransactionRepository, accountID *int64) float64 {
	t.Helper()
	if accountID == nil {
		t.Fatal("expected an account id, got nil")
	}
	bal, err := txnRepo.GetLatestBalance(*accountID)
	if err != nil {
		t.Fatalf("GetLatestBalance: %v", err)
	}
	return bal
}

// TestSyncManagedAccounts_SplitApartment mirrors the real case: 1,195,000
// property, 546,000 loan, 50/50, self has paid 170,800. Expect a property
// asset account = 597,500 + receivable, and a liability account = 187,600.
func TestSyncManagedAccounts_SplitApartment(t *testing.T) {
	svc, loanRepo, accountRepo, txnRepo, userID, catID := setupSyncTest(t)

	loanID := makeLoan(t, loanRepo, userID, "Lejlighed", models.LoanTypeSplit, 546000, 1195000, true)
	// attach category
	loan, _ := loanRepo.GetByID(loanID)
	loan.CategoryID = &catID
	if err := loanRepo.Update(loan); err != nil {
		t.Fatalf("update loan category: %v", err)
	}
	self := addPart(t, loanRepo, loanID, "Teis", 50, true)
	addPart(t, loanRepo, loanID, "Signe", 50, false)
	addPay(t, loanRepo, loanID, self, 168000)
	addPay(t, loanRepo, loanID, self, 2800)

	if err := svc.SyncManagedAccounts(loanID); err != nil {
		t.Fatalf("SyncManagedAccounts: %v", err)
	}

	loan, _ = loanRepo.GetByID(loanID)
	if loan.AssetAccountID == nil || loan.LiabilityAccountID == nil {
		t.Fatalf("expected both managed accounts to be created, got asset=%v liab=%v",
			loan.AssetAccountID, loan.LiabilityAccountID)
	}

	// total paid 170,800; remaining 375,200; self fair share 85,400;
	// self overpaid by 85,400 (receivable). Property share 597,500.
	// Asset account = 597,500 + 85,400 = 682,900. Liability = 187,600.
	if got := balanceOf(t, txnRepo, loan.AssetAccountID); got != 682900 {
		t.Errorf("asset account balance = %v, want 682900", got)
	}
	if got := balanceOf(t, txnRepo, loan.LiabilityAccountID); got != 187600 {
		t.Errorf("liability account balance = %v, want 187600", got)
	}

	// The asset account is a non-liability under Ejendom; the liability
	// account is a liability, both managed.
	asset, _ := accountRepo.GetByID(*loan.AssetAccountID)
	liab, _ := accountRepo.GetByID(*loan.LiabilityAccountID)
	if asset.IsLiability || !asset.IsManaged() || asset.CategoryID == nil || *asset.CategoryID != catID {
		t.Errorf("asset account wrong: %+v", asset)
	}
	if !liab.IsLiability || !liab.IsManaged() {
		t.Errorf("liability account wrong: %+v", liab)
	}
}

// TestSyncManagedAccounts_NoDoubleCount: a categorised loan is excluded
// from LoanNetWorth (its equity is in the managed accounts instead).
func TestSyncManagedAccounts_NoDoubleCount(t *testing.T) {
	svc, loanRepo, _, _, userID, catID := setupSyncTest(t)

	loanID := makeLoan(t, loanRepo, userID, "Lejlighed", models.LoanTypeSplit, 546000, 1195000, true)
	loan, _ := loanRepo.GetByID(loanID)
	loan.CategoryID = &catID
	loanRepo.Update(loan)
	self := addPart(t, loanRepo, loanID, "Teis", 50, true)
	addPart(t, loanRepo, loanID, "Signe", 50, false)
	addPay(t, loanRepo, loanID, self, 170800)
	if err := svc.SyncManagedAccounts(loanID); err != nil {
		t.Fatalf("sync: %v", err)
	}

	// Because the loan has a category, LoanNetWorth must contribute zero
	// (the equity now lives in the managed accounts).
	a, l, r, err := svc.LoanNetWorth(userID)
	if err != nil {
		t.Fatalf("LoanNetWorth: %v", err)
	}
	if a != 0 || l != 0 || r != 0 {
		t.Errorf("categorised loan should not contribute to LoanNetWorth, got a=%v l=%v r=%v", a, l, r)
	}
}

// TestSyncManagedAccounts_PaymentUpdatesBalances: recording a further
// payment shrinks the liability account and the receivable.
func TestSyncManagedAccounts_PaymentUpdatesBalances(t *testing.T) {
	svc, loanRepo, _, txnRepo, userID, catID := setupSyncTest(t)

	loanID := makeLoan(t, loanRepo, userID, "Lejlighed", models.LoanTypeSplit, 546000, 1195000, true)
	loan, _ := loanRepo.GetByID(loanID)
	loan.CategoryID = &catID
	loanRepo.Update(loan)
	self := addPart(t, loanRepo, loanID, "Teis", 50, true)
	addPart(t, loanRepo, loanID, "Signe", 50, false)
	addPay(t, loanRepo, loanID, self, 170800)
	svc.SyncManagedAccounts(loanID)

	loan, _ = loanRepo.GetByID(loanID)
	liab0 := balanceOf(t, txnRepo, loan.LiabilityAccountID)

	// Signe pays 100,000. Remaining drops by 100k -> self liability share
	// drops by 50k. Sync again.
	parts, _ := loanRepo.GetParticipants(loanID)
	var signeID int64
	for _, p := range parts {
		if p.Name == "Signe" {
			signeID = p.ID
		}
	}
	addPay(t, loanRepo, loanID, signeID, 100000)
	if err := svc.SyncManagedAccounts(loanID); err != nil {
		t.Fatalf("sync 2: %v", err)
	}

	loan, _ = loanRepo.GetByID(loanID)
	liab1 := balanceOf(t, txnRepo, loan.LiabilityAccountID)
	if liab1 >= liab0 {
		t.Errorf("liability should shrink after a payment: before=%v after=%v", liab0, liab1)
	}
	if liab1 != 137600 { // 50% of (375200-100000)=137600
		t.Errorf("liability after Signe's 100k = %v, want 137600", liab1)
	}
}

// TestSyncManagedAccounts_RemoveCategoryTearsDown: clearing the category
// removes the managed accounts.
func TestSyncManagedAccounts_RemoveCategoryTearsDown(t *testing.T) {
	svc, loanRepo, accountRepo, _, userID, catID := setupSyncTest(t)

	loanID := makeLoan(t, loanRepo, userID, "Lejlighed", models.LoanTypeSplit, 546000, 1195000, true)
	loan, _ := loanRepo.GetByID(loanID)
	loan.CategoryID = &catID
	loanRepo.Update(loan)
	self := addPart(t, loanRepo, loanID, "Teis", 50, true)
	addPart(t, loanRepo, loanID, "Signe", 50, false)
	addPay(t, loanRepo, loanID, self, 170800)
	svc.SyncManagedAccounts(loanID)

	loan, _ = loanRepo.GetByID(loanID)
	assetID, liabID := loan.AssetAccountID, loan.LiabilityAccountID
	if assetID == nil || liabID == nil {
		t.Fatal("expected managed accounts before teardown")
	}

	// Remove the category and re-sync.
	loan.CategoryID = nil
	loanRepo.Update(loan)
	if err := svc.SyncManagedAccounts(loanID); err != nil {
		t.Fatalf("sync teardown: %v", err)
	}

	loan, _ = loanRepo.GetByID(loanID)
	if loan.AssetAccountID != nil || loan.LiabilityAccountID != nil {
		t.Errorf("links should be cleared after removing category")
	}
	if acc, _ := accountRepo.GetByID(*assetID); acc != nil {
		t.Errorf("asset account should be deleted")
	}
	if acc, _ := accountRepo.GetByID(*liabID); acc != nil {
		t.Errorf("liability account should be deleted")
	}
}
