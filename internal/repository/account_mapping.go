package repository

import (
	"database/sql"
	"errors"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// AccountMappingRepository handles account mapping database operations.
type AccountMappingRepository struct {
	db *database.DB
}

// NewAccountMappingRepository creates a new AccountMappingRepository.
func NewAccountMappingRepository(db *database.DB) *AccountMappingRepository {
	return &AccountMappingRepository{db: db}
}

// Create inserts a new account mapping and returns its ID.
func (r *AccountMappingRepository) Create(mapping *models.AccountMapping) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO account_mappings (connection_id, local_account_id, external_account_id, external_account_name, auto_sync)
		VALUES (?, ?, ?, ?, ?)
	`, mapping.ConnectionID, mapping.LocalAccountID, mapping.ExternalAccountID, mapping.ExternalAccountName, boolToInt(mapping.AutoSync))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves an account mapping by ID.
func (r *AccountMappingRepository) GetByID(id int64) (*models.AccountMapping, error) {
	row := r.db.QueryRow(`
		SELECT id, connection_id, local_account_id, external_account_id, external_account_name, auto_sync, created_at
		FROM account_mappings
		WHERE id = ?
	`, id)

	return r.scanMapping(row)
}

// GetByConnectionID retrieves all mappings for a broker connection.
func (r *AccountMappingRepository) GetByConnectionID(connectionID int64) ([]*models.AccountMapping, error) {
	rows, err := r.db.Query(`
		SELECT id, connection_id, local_account_id, external_account_id, external_account_name, auto_sync, created_at
		FROM account_mappings
		WHERE connection_id = ?
		ORDER BY created_at ASC
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// GetByLocalAccountID retrieves the mapping for a local account.
func (r *AccountMappingRepository) GetByLocalAccountID(localAccountID int64) (*models.AccountMapping, error) {
	row := r.db.QueryRow(`
		SELECT id, connection_id, local_account_id, external_account_id, external_account_name, auto_sync, created_at
		FROM account_mappings
		WHERE local_account_id = ?
	`, localAccountID)

	return r.scanMapping(row)
}

// GetByExternalAccountID retrieves a mapping by external account ID within a connection.
func (r *AccountMappingRepository) GetByExternalAccountID(connectionID int64, externalAccountID string) (*models.AccountMapping, error) {
	row := r.db.QueryRow(`
		SELECT id, connection_id, local_account_id, external_account_id, external_account_name, auto_sync, created_at
		FROM account_mappings
		WHERE connection_id = ? AND external_account_id = ?
	`, connectionID, externalAccountID)

	return r.scanMapping(row)
}

// GetAutoSyncByConnectionID retrieves all auto-sync enabled mappings for a connection.
func (r *AccountMappingRepository) GetAutoSyncByConnectionID(connectionID int64) ([]*models.AccountMapping, error) {
	rows, err := r.db.Query(`
		SELECT id, connection_id, local_account_id, external_account_id, external_account_name, auto_sync, created_at
		FROM account_mappings
		WHERE connection_id = ? AND auto_sync = 1
		ORDER BY created_at ASC
	`, connectionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanMappings(rows)
}

// Update updates an existing account mapping.
func (r *AccountMappingRepository) Update(mapping *models.AccountMapping) error {
	result, err := r.db.Exec(`
		UPDATE account_mappings
		SET local_account_id = ?, external_account_id = ?, external_account_name = ?, auto_sync = ?
		WHERE id = ?
	`, mapping.LocalAccountID, mapping.ExternalAccountID, mapping.ExternalAccountName, boolToInt(mapping.AutoSync), mapping.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("account mapping not found")
	}
	return nil
}

// SetAutoSync updates the auto_sync flag for a mapping.
func (r *AccountMappingRepository) SetAutoSync(id int64, autoSync bool) error {
	result, err := r.db.Exec(`
		UPDATE account_mappings SET auto_sync = ? WHERE id = ?
	`, boolToInt(autoSync), id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("account mapping not found")
	}
	return nil
}

// Delete removes an account mapping by ID.
func (r *AccountMappingRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM account_mappings WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("account mapping not found")
	}
	return nil
}

// DeleteByConnectionID removes all mappings for a connection.
func (r *AccountMappingRepository) DeleteByConnectionID(connectionID int64) error {
	_, err := r.db.Exec(`DELETE FROM account_mappings WHERE connection_id = ?`, connectionID)
	return err
}

// scanMapping scans a single row into an AccountMapping.
func (r *AccountMappingRepository) scanMapping(row *sql.Row) (*models.AccountMapping, error) {
	mapping := &models.AccountMapping{}
	var autoSync int
	var externalAccountName sql.NullString

	err := row.Scan(
		&mapping.ID,
		&mapping.ConnectionID,
		&mapping.LocalAccountID,
		&mapping.ExternalAccountID,
		&externalAccountName,
		&autoSync,
		&mapping.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	mapping.AutoSync = autoSync == 1
	if externalAccountName.Valid {
		mapping.ExternalAccountName = externalAccountName.String
	}

	return mapping, nil
}

// scanMappings scans multiple rows into AccountMappings.
func (r *AccountMappingRepository) scanMappings(rows *sql.Rows) ([]*models.AccountMapping, error) {
	mappings := make([]*models.AccountMapping, 0)

	for rows.Next() {
		mapping := &models.AccountMapping{}
		var autoSync int
		var externalAccountName sql.NullString

		err := rows.Scan(
			&mapping.ID,
			&mapping.ConnectionID,
			&mapping.LocalAccountID,
			&mapping.ExternalAccountID,
			&externalAccountName,
			&autoSync,
			&mapping.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		mapping.AutoSync = autoSync == 1
		if externalAccountName.Valid {
			mapping.ExternalAccountName = externalAccountName.String
		}

		mappings = append(mappings, mapping)
	}

	return mappings, rows.Err()
}
