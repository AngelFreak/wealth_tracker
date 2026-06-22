package services

import (
	"fmt"
	"time"

	"wealth_tracker/internal/models"
)

// SyncManagedAccounts keeps a loan's two auto-managed accounts in step
// with its current figures. It is called after any change that affects a
// loan's equity (create, edit, payment add/delete, participant change).
//
// When the loan has a category:
//   - the property ASSET account holds the user's share of the property
//     value plus what co-owners owe them (the receivable);
//   - the loan LIABILITY account holds the user's share of the remaining
//     principal.
//
// For a "lent" loan there is no property and no liability: the single
// asset account holds the user's share of what is still owed back.
//
// When the loan has NO category, any previously-managed accounts are
// removed and the loan reverts to contributing through LoanNetWorth.
//
// Account balances are set by writing a balance-update transaction, the
// same mechanism manual balance edits use, so history and the dashboard
// stay consistent. Managed accounts are flagged (ManagedByLoanID) and
// are read-only in the Accounts UI.
func (s *LoanService) SyncManagedAccounts(loanID int64) error {
	if s.accountRepo == nil || s.transactionRepo == nil {
		return nil // math-only context, nothing to sync
	}

	loan, err := s.loanRepo.GetByID(loanID)
	if err != nil {
		return err
	}
	if loan == nil {
		return nil
	}

	// No category, or loan closed: tear down any managed accounts so the
	// figures don't linger in the Accounts view.
	if loan.CategoryID == nil || !loan.IsActive {
		return s.teardownManagedAccounts(loan)
	}

	summary, err := s.Summarize(loanID)
	if err != nil {
		return err
	}

	// Asset account: property share + receivable (or, for a lent loan,
	// the user's share of what's still owed back).
	assetBalance := summary.SelfAsset + summary.SelfReceivable
	assetName := loan.Name
	if loan.LoanType != models.LoanTypeLent {
		assetName = loan.Name + " (ejendom)"
	}
	if err := s.upsertManagedAccount(loan, &loan.AssetAccountID, assetName, assetBalance, false); err != nil {
		return err
	}

	// Liability account: the user's share of the remaining principal.
	// A lent loan has no liability side.
	if loan.LoanType == models.LoanTypeLent {
		if err := s.removeManagedAccount(&loan.LiabilityAccountID); err != nil {
			return err
		}
	} else {
		liabName := loan.Name + " (lån)"
		if err := s.upsertManagedAccount(loan, &loan.LiabilityAccountID, liabName, summary.SelfLoanShare, true); err != nil {
			return err
		}
	}

	// Persist any account IDs that were just created back onto the loan.
	return s.loanRepo.Update(loan)
}

// upsertManagedAccount creates the managed account if *accountID is nil
// (writing the new id back through the pointer), then sets its balance,
// category, name, and liability flag to match the loan.
func (s *LoanService) upsertManagedAccount(loan *models.Loan, accountID **int64, name string, balance float64, isLiability bool) error {
	balance = roundMoney(balance)

	if *accountID == nil {
		acc := &models.Account{
			UserID:          loan.UserID,
			CategoryID:      loan.CategoryID,
			Name:            name,
			Currency:        loan.Currency,
			IsLiability:     isLiability,
			IsActive:        true,
			ManagedByLoanID: &loan.ID,
		}
		id, err := s.accountRepo.Create(acc)
		if err != nil {
			return fmt.Errorf("creating managed account: %w", err)
		}
		*accountID = &id
	} else {
		acc, err := s.accountRepo.GetByID(**accountID)
		if err != nil {
			return err
		}
		if acc != nil {
			acc.Name = name
			acc.CategoryID = loan.CategoryID
			acc.Currency = loan.Currency
			acc.IsLiability = isLiability
			acc.IsActive = true
			acc.ManagedByLoanID = &loan.ID
			if err := s.accountRepo.Update(acc); err != nil {
				return err
			}
		}
	}

	return s.setAccountBalance(**accountID, balance)
}

// setAccountBalance writes a balance-update transaction so the account's
// latest balance equals target, but only when it actually changed.
func (s *LoanService) setAccountBalance(accountID int64, target float64) error {
	current, err := s.transactionRepo.GetLatestBalance(accountID)
	if err != nil {
		return err
	}
	if roundMoney(current) == roundMoney(target) {
		return nil
	}
	_, err = s.transactionRepo.Create(&models.Transaction{
		AccountID:       accountID,
		Amount:          roundMoney(target - current),
		BalanceAfter:    roundMoney(target),
		Description:     "Loan balance update",
		TransactionDate: time.Now(),
	})
	return err
}

// removeManagedAccount deletes the managed account referenced by
// *accountID (if any) and clears the pointer.
func (s *LoanService) removeManagedAccount(accountID **int64) error {
	if *accountID == nil {
		return nil
	}
	if err := s.accountRepo.Delete(**accountID); err != nil {
		// A missing account is fine — it may have been removed already.
		return nil
	}
	*accountID = nil
	return nil
}

// teardownManagedAccounts removes both managed accounts for a loan and
// clears the links, used when the loan loses its category or is closed.
func (s *LoanService) teardownManagedAccounts(loan *models.Loan) error {
	changed := loan.AssetAccountID != nil || loan.LiabilityAccountID != nil
	if err := s.removeManagedAccount(&loan.AssetAccountID); err != nil {
		return err
	}
	if err := s.removeManagedAccount(&loan.LiabilityAccountID); err != nil {
		return err
	}
	if changed {
		return s.loanRepo.Update(loan)
	}
	return nil
}
