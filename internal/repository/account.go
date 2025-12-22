package repository

import (
	"database/sql"
	"errors"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// AccountRepository handles account database operations.
type AccountRepository struct {
	db *database.DB
}

// NewAccountRepository creates a new AccountRepository.
func NewAccountRepository(db *database.DB) *AccountRepository {
	return &AccountRepository{db: db}
}

// Create inserts a new account and returns its ID.
func (r *AccountRepository) Create(account *models.Account) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO accounts (user_id, category_id, name, currency, is_liability, is_active, notes)
		VALUES (?, ?, ?, ?, ?, ?, ?)
	`, account.UserID, account.CategoryID, account.Name, account.Currency,
		boolToInt(account.IsLiability), boolToInt(account.IsActive), account.Notes)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves an account by ID.
func (r *AccountRepository) GetByID(id int64) (*models.Account, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, category_id, name, currency, is_liability, is_active, notes, created_at
		FROM accounts
		WHERE id = ?
	`, id)

	account := &models.Account{}
	var categoryID sql.NullInt64
	var isLiability, isActive int
	var notes sql.NullString

	err := row.Scan(
		&account.ID,
		&account.UserID,
		&categoryID,
		&account.Name,
		&account.Currency,
		&isLiability,
		&isActive,
		&notes,
		&account.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if categoryID.Valid {
		account.CategoryID = &categoryID.Int64
	}
	account.IsLiability = isLiability == 1
	account.IsActive = isActive == 1
	if notes.Valid {
		account.Notes = notes.String
	}

	return account, nil
}

// GetByUserID retrieves all accounts for a user, sorted by name.
func (r *AccountRepository) GetByUserID(userID int64) ([]*models.Account, error) {
	return r.queryAccounts(`
		SELECT id, user_id, category_id, name, currency, is_liability, is_active, notes, created_at
		FROM accounts
		WHERE user_id = ?
		ORDER BY name ASC
	`, userID)
}

// GetByUserIDActiveOnly retrieves only active accounts for a user.
func (r *AccountRepository) GetByUserIDActiveOnly(userID int64) ([]*models.Account, error) {
	return r.queryAccounts(`
		SELECT id, user_id, category_id, name, currency, is_liability, is_active, notes, created_at
		FROM accounts
		WHERE user_id = ? AND is_active = 1
		ORDER BY name ASC
	`, userID)
}

// GetByCategoryID retrieves all accounts for a specific category.
func (r *AccountRepository) GetByCategoryID(categoryID int64) ([]*models.Account, error) {
	return r.queryAccounts(`
		SELECT id, user_id, category_id, name, currency, is_liability, is_active, notes, created_at
		FROM accounts
		WHERE category_id = ?
		ORDER BY name ASC
	`, categoryID)
}

// queryAccounts is a helper to query multiple accounts.
func (r *AccountRepository) queryAccounts(query string, args ...any) ([]*models.Account, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	accounts := make([]*models.Account, 0)
	for rows.Next() {
		account := &models.Account{}
		var categoryID sql.NullInt64
		var isLiability, isActive int
		var notes sql.NullString

		err := rows.Scan(
			&account.ID,
			&account.UserID,
			&categoryID,
			&account.Name,
			&account.Currency,
			&isLiability,
			&isActive,
			&notes,
			&account.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if categoryID.Valid {
			account.CategoryID = &categoryID.Int64
		}
		account.IsLiability = isLiability == 1
		account.IsActive = isActive == 1
		if notes.Valid {
			account.Notes = notes.String
		}

		accounts = append(accounts, account)
	}
	return accounts, rows.Err()
}

// Update updates an existing account.
func (r *AccountRepository) Update(account *models.Account) error {
	result, err := r.db.Exec(`
		UPDATE accounts
		SET category_id = ?, name = ?, currency = ?, is_liability = ?, is_active = ?, notes = ?
		WHERE id = ?
	`, account.CategoryID, account.Name, account.Currency,
		boolToInt(account.IsLiability), boolToInt(account.IsActive), account.Notes, account.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("account not found")
	}
	return nil
}

// Delete removes an account by ID.
func (r *AccountRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM accounts WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("account not found")
	}
	return nil
}

// CountByUserID returns the number of accounts for a user.
func (r *AccountRepository) CountByUserID(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM accounts WHERE user_id = ?
	`, userID).Scan(&count)
	return count, err
}

// CountActiveAssets returns the count of active non-liability accounts.
func (r *AccountRepository) CountActiveAssets(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM accounts
		WHERE user_id = ? AND is_liability = 0 AND is_active = 1
	`, userID).Scan(&count)
	return count, err
}

// CountActiveLiabilities returns the count of active liability accounts.
func (r *AccountRepository) CountActiveLiabilities(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM accounts
		WHERE user_id = ? AND is_liability = 1 AND is_active = 1
	`, userID).Scan(&count)
	return count, err
}

// boolToInt converts a boolean to SQLite integer (0 or 1).
func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}
