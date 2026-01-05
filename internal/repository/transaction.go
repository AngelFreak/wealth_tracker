package repository

import (
	"database/sql"
	"errors"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// TransactionRepository handles transaction database operations.
type TransactionRepository struct {
	db *database.DB
}

// NewTransactionRepository creates a new TransactionRepository.
func NewTransactionRepository(db *database.DB) *TransactionRepository {
	return &TransactionRepository{db: db}
}

// Create inserts a new transaction and returns its ID.
func (r *TransactionRepository) Create(txn *models.Transaction) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO transactions (account_id, amount, balance_after, description, transaction_date)
		VALUES (?, ?, ?, ?, ?)
	`, txn.AccountID, txn.Amount, txn.BalanceAfter, txn.Description, txn.TransactionDate.Format("2006-01-02"))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves a transaction by ID.
func (r *TransactionRepository) GetByID(id int64) (*models.Transaction, error) {
	row := r.db.QueryRow(`
		SELECT id, account_id, amount, balance_after, description, transaction_date, created_at
		FROM transactions
		WHERE id = ?
	`, id)

	txn := &models.Transaction{}
	var description sql.NullString
	var transactionDate string

	err := row.Scan(
		&txn.ID,
		&txn.AccountID,
		&txn.Amount,
		&txn.BalanceAfter,
		&description,
		&transactionDate,
		&txn.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if description.Valid {
		txn.Description = description.String
	}
	txn.TransactionDate = parseDate(transactionDate)

	return txn, nil
}

// parseDate handles various date formats returned by SQLite.
func parseDate(s string) time.Time {
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05Z",
		"2006-01-02 15:04:05",
		time.RFC3339,
	}
	for _, f := range formats {
		if t, err := time.Parse(f, s); err == nil {
			return t
		}
	}
	return time.Time{}
}

// GetByAccountID retrieves transactions for an account with pagination.
func (r *TransactionRepository) GetByAccountID(accountID int64, limit, offset int) ([]*models.Transaction, error) {
	return r.queryTransactions(`
		SELECT id, account_id, amount, balance_after, description, transaction_date, created_at
		FROM transactions
		WHERE account_id = ?
		ORDER BY transaction_date DESC, id DESC
		LIMIT ? OFFSET ?
	`, accountID, limit, offset)
}

// GetByUserID retrieves all transactions for a user across all accounts.
func (r *TransactionRepository) GetByUserID(userID int64, limit, offset int) ([]*models.Transaction, error) {
	return r.queryTransactions(`
		SELECT t.id, t.account_id, t.amount, t.balance_after, t.description, t.transaction_date, t.created_at
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE a.user_id = ?
		ORDER BY t.transaction_date DESC, t.id DESC
		LIMIT ? OFFSET ?
	`, userID, limit, offset)
}

// GetByDateRange retrieves transactions for an account within a date range.
func (r *TransactionRepository) GetByDateRange(accountID int64, start, end time.Time) ([]*models.Transaction, error) {
	return r.queryTransactions(`
		SELECT id, account_id, amount, balance_after, description, transaction_date, created_at
		FROM transactions
		WHERE account_id = ? AND transaction_date >= ? AND transaction_date <= ?
		ORDER BY transaction_date DESC, id DESC
	`, accountID, start.Format("2006-01-02"), end.Format("2006-01-02"))
}

// queryTransactions is a helper to query multiple transactions.
func (r *TransactionRepository) queryTransactions(query string, args ...any) ([]*models.Transaction, error) {
	rows, err := r.db.Query(query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := make([]*models.Transaction, 0)
	for rows.Next() {
		txn := &models.Transaction{}
		var description sql.NullString
		var transactionDate string

		err := rows.Scan(
			&txn.ID,
			&txn.AccountID,
			&txn.Amount,
			&txn.BalanceAfter,
			&description,
			&transactionDate,
			&txn.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if description.Valid {
			txn.Description = description.String
		}
		txn.TransactionDate = parseDate(transactionDate)

		transactions = append(transactions, txn)
	}
	return transactions, rows.Err()
}

// Update updates an existing transaction.
func (r *TransactionRepository) Update(txn *models.Transaction) error {
	result, err := r.db.Exec(`
		UPDATE transactions
		SET amount = ?, balance_after = ?, description = ?, transaction_date = ?
		WHERE id = ?
	`, txn.Amount, txn.BalanceAfter, txn.Description, txn.TransactionDate.Format("2006-01-02"), txn.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("transaction not found")
	}
	return nil
}

// Delete removes a transaction by ID.
func (r *TransactionRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM transactions WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("transaction not found")
	}
	return nil
}

// CountByAccountID returns the number of transactions for an account.
func (r *TransactionRepository) CountByAccountID(accountID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM transactions WHERE account_id = ?
	`, accountID).Scan(&count)
	return count, err
}

// GetLatestBalance returns the balance after the most recent transaction for an account.
func (r *TransactionRepository) GetLatestBalance(accountID int64) (float64, error) {
	var balance sql.NullFloat64
	err := r.db.QueryRow(`
		SELECT balance_after
		FROM transactions
		WHERE account_id = ?
		ORDER BY transaction_date DESC, id DESC
		LIMIT 1
	`, accountID).Scan(&balance)

	if err == sql.ErrNoRows || !balance.Valid {
		return 0, nil
	}
	if err != nil {
		return 0, err
	}
	return balance.Float64, nil
}

// GetRecentByUserID retrieves the most recent transactions for a user.
func (r *TransactionRepository) GetRecentByUserID(userID int64, limit int) ([]*models.Transaction, error) {
	rows, err := r.db.Query(`
		SELECT t.id, t.account_id, t.amount, t.balance_after, t.description, t.transaction_date, t.created_at
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE a.user_id = ?
		ORDER BY t.transaction_date DESC, t.id DESC
		LIMIT ?
	`, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	transactions := make([]*models.Transaction, 0)
	for rows.Next() {
		txn := &models.Transaction{}
		var description sql.NullString
		var transactionDate string

		err := rows.Scan(
			&txn.ID,
			&txn.AccountID,
			&txn.Amount,
			&txn.BalanceAfter,
			&description,
			&transactionDate,
			&txn.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if description.Valid {
			txn.Description = description.String
		}
		txn.TransactionDate = parseDate(transactionDate)

		transactions = append(transactions, txn)
	}
	return transactions, rows.Err()
}

// GetSumSince returns the sum of transaction amounts since a given date.
func (r *TransactionRepository) GetSumSince(accountID int64, since time.Time) (float64, error) {
	var sum sql.NullFloat64
	err := r.db.QueryRow(`
		SELECT COALESCE(SUM(amount), 0)
		FROM transactions
		WHERE account_id = ? AND transaction_date >= ?
	`, accountID, since.Format("2006-01-02")).Scan(&sum)

	if err != nil {
		return 0, err
	}
	if !sum.Valid {
		return 0, nil
	}
	return sum.Float64, nil
}

// NetWorthPoint represents net worth at a specific date.
type NetWorthPoint struct {
	Date     time.Time
	NetWorth float64
}

// GetNetWorthHistory returns the net worth history for a user.
// It calculates net worth at each transaction date by tracking account balances.
func (r *TransactionRepository) GetNetWorthHistory(userID int64) ([]NetWorthPoint, error) {
	// Get all transactions with account liability info, ordered by date
	rows, err := r.db.Query(`
		SELECT t.transaction_date, t.balance_after, a.id, a.is_liability
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE a.user_id = ? AND a.is_active = 1
		ORDER BY t.transaction_date ASC, t.id ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	// Track latest balance for each account
	accountBalances := make(map[int64]float64)
	accountIsLiability := make(map[int64]bool)

	// Track net worth at each date
	dateNetWorth := make(map[string]float64)
	var dates []string

	for rows.Next() {
		var dateStr string
		var balance float64
		var accountID int64
		var isLiability int

		if err := rows.Scan(&dateStr, &balance, &accountID, &isLiability); err != nil {
			return nil, err
		}

		// Update account balance
		accountBalances[accountID] = balance
		accountIsLiability[accountID] = isLiability == 1

		// Calculate current net worth
		netWorth := 0.0
		for accID, bal := range accountBalances {
			if accountIsLiability[accID] {
				// Use absolute value for liabilities
				if bal < 0 {
					netWorth -= -bal
				} else {
					netWorth -= bal
				}
			} else {
				netWorth += bal
			}
		}

		// Store or update net worth for this date
		if _, exists := dateNetWorth[dateStr]; !exists {
			dates = append(dates, dateStr)
		}
		dateNetWorth[dateStr] = netWorth
	}

	if err := rows.Err(); err != nil {
		return nil, err
	}

	// Build result
	result := make([]NetWorthPoint, len(dates))
	for i, dateStr := range dates {
		date := parseDate(dateStr)
		result[i] = NetWorthPoint{
			Date:     date,
			NetWorth: dateNetWorth[dateStr],
		}
	}

	return result, nil
}

// GetByAccountIDPaginated retrieves transactions with full pagination info.
func (r *TransactionRepository) GetByAccountIDPaginated(accountID int64, p Pagination) (*PaginatedResult[*models.Transaction], error) {
	// Get total count
	var total int64
	err := r.db.QueryRow(`SELECT COUNT(*) FROM transactions WHERE account_id = ?`, accountID).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Get items
	items, err := r.GetByAccountID(accountID, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}

	result := NewPaginatedResult(items, total, p)
	return &result, nil
}

// GetByUserIDPaginated retrieves all user transactions with full pagination info.
func (r *TransactionRepository) GetByUserIDPaginated(userID int64, p Pagination) (*PaginatedResult[*models.Transaction], error) {
	// Get total count
	var total int64
	err := r.db.QueryRow(`
		SELECT COUNT(*)
		FROM transactions t
		JOIN accounts a ON t.account_id = a.id
		WHERE a.user_id = ?
	`, userID).Scan(&total)
	if err != nil {
		return nil, err
	}

	// Get items
	items, err := r.GetByUserID(userID, p.Limit, p.Offset)
	if err != nil {
		return nil, err
	}

	result := NewPaginatedResult(items, total, p)
	return &result, nil
}

// SumBalancesByUserID returns the total of latest balances across all user accounts.
func (r *TransactionRepository) SumBalancesByUserID(userID int64) (float64, error) {
	// Get the latest balance for each account and sum them
	rows, err := r.db.Query(`
		SELECT COALESCE(
			(SELECT balance_after
			 FROM transactions
			 WHERE account_id = a.id
			 ORDER BY transaction_date DESC, id DESC
			 LIMIT 1),
			0
		) as latest_balance
		FROM accounts a
		WHERE a.user_id = ? AND a.is_active = 1
	`, userID)
	if err != nil {
		return 0, err
	}
	defer rows.Close()

	var total float64
	for rows.Next() {
		var balance float64
		if err := rows.Scan(&balance); err != nil {
			return 0, err
		}
		total += balance
	}
	return total, rows.Err()
}
