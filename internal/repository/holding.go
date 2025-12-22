package repository

import (
	"database/sql"
	"errors"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// HoldingRepository handles holding database operations.
type HoldingRepository struct {
	db *database.DB
}

// NewHoldingRepository creates a new HoldingRepository.
func NewHoldingRepository(db *database.DB) *HoldingRepository {
	return &HoldingRepository{db: db}
}

// Create inserts a new holding and returns its ID.
func (r *HoldingRepository) Create(holding *models.Holding) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO holdings (account_id, external_id, symbol, name, quantity, avg_price, current_price, current_value, currency, instrument_type)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, holding.AccountID, holding.ExternalID, holding.Symbol, holding.Name, holding.Quantity,
		holding.AvgPrice, holding.CurrentPrice, holding.CurrentValue, holding.Currency, holding.InstrumentType)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Upsert inserts or updates a holding based on account_id and symbol.
func (r *HoldingRepository) Upsert(holding *models.Holding) error {
	_, err := r.db.Exec(`
		INSERT INTO holdings (account_id, external_id, symbol, name, quantity, avg_price, current_price, current_value, currency, instrument_type, last_updated)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(account_id, symbol) DO UPDATE SET
			external_id = excluded.external_id,
			name = excluded.name,
			quantity = excluded.quantity,
			avg_price = excluded.avg_price,
			current_price = excluded.current_price,
			current_value = excluded.current_value,
			currency = excluded.currency,
			instrument_type = excluded.instrument_type,
			last_updated = excluded.last_updated
	`, holding.AccountID, holding.ExternalID, holding.Symbol, holding.Name, holding.Quantity,
		holding.AvgPrice, holding.CurrentPrice, holding.CurrentValue, holding.Currency,
		holding.InstrumentType, time.Now())
	return err
}

// GetByID retrieves a holding by ID.
func (r *HoldingRepository) GetByID(id int64) (*models.Holding, error) {
	row := r.db.QueryRow(`
		SELECT id, account_id, external_id, symbol, name, quantity, avg_price, current_price, current_value, currency, instrument_type, last_updated, created_at
		FROM holdings
		WHERE id = ?
	`, id)

	return r.scanHolding(row)
}

// GetByAccountID retrieves all holdings for an account.
func (r *HoldingRepository) GetByAccountID(accountID int64) ([]*models.Holding, error) {
	rows, err := r.db.Query(`
		SELECT id, account_id, external_id, symbol, name, quantity, avg_price, current_price, current_value, currency, instrument_type, last_updated, created_at
		FROM holdings
		WHERE account_id = ?
		ORDER BY current_value DESC
	`, accountID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanHoldings(rows)
}

// GetByAccountIDWithValue retrieves holdings for an account with minimum value.
func (r *HoldingRepository) GetByAccountIDWithValue(accountID int64, minValue float64) ([]*models.Holding, error) {
	rows, err := r.db.Query(`
		SELECT id, account_id, external_id, symbol, name, quantity, avg_price, current_price, current_value, currency, instrument_type, last_updated, created_at
		FROM holdings
		WHERE account_id = ? AND current_value >= ?
		ORDER BY current_value DESC
	`, accountID, minValue)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanHoldings(rows)
}

// GetTotalValueByAccountID returns the total value of all holdings for an account.
func (r *HoldingRepository) GetTotalValueByAccountID(accountID int64) (float64, error) {
	var total sql.NullFloat64
	err := r.db.QueryRow(`
		SELECT SUM(current_value) FROM holdings WHERE account_id = ?
	`, accountID).Scan(&total)
	if err != nil {
		return 0, err
	}
	if total.Valid {
		return total.Float64, nil
	}
	return 0, nil
}

// Update updates an existing holding.
func (r *HoldingRepository) Update(holding *models.Holding) error {
	result, err := r.db.Exec(`
		UPDATE holdings
		SET external_id = ?, symbol = ?, name = ?, quantity = ?, avg_price = ?, current_price = ?, current_value = ?, currency = ?, instrument_type = ?, last_updated = ?
		WHERE id = ?
	`, holding.ExternalID, holding.Symbol, holding.Name, holding.Quantity,
		holding.AvgPrice, holding.CurrentPrice, holding.CurrentValue, holding.Currency,
		holding.InstrumentType, time.Now(), holding.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("holding not found")
	}
	return nil
}

// Delete removes a holding by ID.
func (r *HoldingRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM holdings WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("holding not found")
	}
	return nil
}

// DeleteByAccountID removes all holdings for an account.
func (r *HoldingRepository) DeleteByAccountID(accountID int64) error {
	_, err := r.db.Exec(`DELETE FROM holdings WHERE account_id = ?`, accountID)
	return err
}

// DeleteStaleHoldings removes holdings that haven't been updated since the given time.
func (r *HoldingRepository) DeleteStaleHoldings(accountID int64, since time.Time) error {
	_, err := r.db.Exec(`DELETE FROM holdings WHERE account_id = ? AND last_updated < ?`, accountID, since)
	return err
}

// CountByAccountID returns the number of holdings for an account.
func (r *HoldingRepository) CountByAccountID(accountID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM holdings WHERE account_id = ?`, accountID).Scan(&count)
	return count, err
}

// scanHolding scans a single row into a Holding.
func (r *HoldingRepository) scanHolding(row *sql.Row) (*models.Holding, error) {
	holding := &models.Holding{}
	var externalID, instrumentType sql.NullString
	var avgPrice, currentPrice sql.NullFloat64

	err := row.Scan(
		&holding.ID,
		&holding.AccountID,
		&externalID,
		&holding.Symbol,
		&holding.Name,
		&holding.Quantity,
		&avgPrice,
		&currentPrice,
		&holding.CurrentValue,
		&holding.Currency,
		&instrumentType,
		&holding.LastUpdated,
		&holding.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if externalID.Valid {
		holding.ExternalID = externalID.String
	}
	if avgPrice.Valid {
		holding.AvgPrice = avgPrice.Float64
	}
	if currentPrice.Valid {
		holding.CurrentPrice = currentPrice.Float64
	}
	if instrumentType.Valid {
		holding.InstrumentType = instrumentType.String
	}

	return holding, nil
}

// scanHoldings scans multiple rows into Holdings.
func (r *HoldingRepository) scanHoldings(rows *sql.Rows) ([]*models.Holding, error) {
	holdings := make([]*models.Holding, 0)

	for rows.Next() {
		holding := &models.Holding{}
		var externalID, instrumentType sql.NullString
		var avgPrice, currentPrice sql.NullFloat64

		err := rows.Scan(
			&holding.ID,
			&holding.AccountID,
			&externalID,
			&holding.Symbol,
			&holding.Name,
			&holding.Quantity,
			&avgPrice,
			&currentPrice,
			&holding.CurrentValue,
			&holding.Currency,
			&instrumentType,
			&holding.LastUpdated,
			&holding.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if externalID.Valid {
			holding.ExternalID = externalID.String
		}
		if avgPrice.Valid {
			holding.AvgPrice = avgPrice.Float64
		}
		if currentPrice.Valid {
			holding.CurrentPrice = currentPrice.Float64
		}
		if instrumentType.Valid {
			holding.InstrumentType = instrumentType.String
		}

		holdings = append(holdings, holding)
	}

	return holdings, rows.Err()
}
