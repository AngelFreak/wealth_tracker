package repository

import (
	"database/sql"
	"errors"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// BrokerConnectionRepository handles broker connection database operations.
type BrokerConnectionRepository struct {
	db *database.DB
}

// NewBrokerConnectionRepository creates a new BrokerConnectionRepository.
func NewBrokerConnectionRepository(db *database.DB) *BrokerConnectionRepository {
	return &BrokerConnectionRepository{db: db}
}

// Create inserts a new broker connection and returns its ID.
// Note: CPR is stored for Signicat MitID-CPR verification (should be encrypted in production).
func (r *BrokerConnectionRepository) Create(conn *models.BrokerConnection) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO broker_connections (user_id, broker_type, username, cpr, country, app_key, app_secret, redirect_uri, is_active)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)
	`, conn.UserID, conn.BrokerType, conn.Username, conn.CPR, conn.Country, conn.AppKey, conn.AppSecret, conn.RedirectURI, boolToInt(conn.IsActive))
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves a broker connection by ID.
func (r *BrokerConnectionRepository) GetByID(id int64) (*models.BrokerConnection, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, broker_type, username, cpr, country, app_key, app_secret, redirect_uri,
		       is_active, last_sync_at, last_sync_status, last_sync_error, created_at, updated_at
		FROM broker_connections
		WHERE id = ?
	`, id)

	return r.scanConnection(row)
}

// GetByUserID retrieves all broker connections for a user.
func (r *BrokerConnectionRepository) GetByUserID(userID int64) ([]*models.BrokerConnection, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, broker_type, username, cpr, country, app_key, app_secret, redirect_uri,
		       is_active, last_sync_at, last_sync_status, last_sync_error, created_at, updated_at
		FROM broker_connections
		WHERE user_id = ?
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanConnections(rows)
}

// GetByUserAndBroker retrieves a connection for a specific user and broker type.
func (r *BrokerConnectionRepository) GetByUserAndBroker(userID int64, brokerType string) (*models.BrokerConnection, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, broker_type, username, cpr, country, app_key, app_secret, redirect_uri,
		       is_active, last_sync_at, last_sync_status, last_sync_error, created_at, updated_at
		FROM broker_connections
		WHERE user_id = ? AND broker_type = ?
	`, userID, brokerType)

	return r.scanConnection(row)
}

// GetActiveByUserID retrieves only active broker connections for a user.
func (r *BrokerConnectionRepository) GetActiveByUserID(userID int64) ([]*models.BrokerConnection, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, broker_type, username, cpr, country, app_key, app_secret, redirect_uri,
		       is_active, last_sync_at, last_sync_status, last_sync_error, created_at, updated_at
		FROM broker_connections
		WHERE user_id = ? AND is_active = 1
		ORDER BY created_at DESC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanConnections(rows)
}

// Update updates an existing broker connection.
func (r *BrokerConnectionRepository) Update(conn *models.BrokerConnection) error {
	result, err := r.db.Exec(`
		UPDATE broker_connections
		SET username = ?, cpr = ?, country = ?, app_key = ?, app_secret = ?, redirect_uri = ?, is_active = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, conn.Username, conn.CPR, conn.Country, conn.AppKey, conn.AppSecret, conn.RedirectURI, boolToInt(conn.IsActive), conn.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("broker connection not found")
	}
	return nil
}

// UpdateSyncStatus updates the sync status of a connection.
func (r *BrokerConnectionRepository) UpdateSyncStatus(id int64, status, errorMsg string) error {
	result, err := r.db.Exec(`
		UPDATE broker_connections
		SET last_sync_at = ?, last_sync_status = ?, last_sync_error = ?, updated_at = CURRENT_TIMESTAMP
		WHERE id = ?
	`, time.Now(), status, errorMsg, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("broker connection not found")
	}
	return nil
}

// Delete removes a broker connection by ID.
func (r *BrokerConnectionRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM broker_connections WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("broker connection not found")
	}
	return nil
}

// scanConnection scans a single row into a BrokerConnection.
func (r *BrokerConnectionRepository) scanConnection(row *sql.Row) (*models.BrokerConnection, error) {
	conn := &models.BrokerConnection{}
	var isActive int
	var lastSyncAt sql.NullTime
	var lastSyncStatus, lastSyncError, cpr, appKey, appSecret, redirectURI sql.NullString

	err := row.Scan(
		&conn.ID,
		&conn.UserID,
		&conn.BrokerType,
		&conn.Username,
		&cpr,
		&conn.Country,
		&appKey,
		&appSecret,
		&redirectURI,
		&isActive,
		&lastSyncAt,
		&lastSyncStatus,
		&lastSyncError,
		&conn.CreatedAt,
		&conn.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	conn.IsActive = isActive == 1
	if cpr.Valid {
		conn.CPR = cpr.String
	}
	if appKey.Valid {
		conn.AppKey = appKey.String
	}
	if appSecret.Valid {
		conn.AppSecret = appSecret.String
	}
	if redirectURI.Valid {
		conn.RedirectURI = redirectURI.String
	}
	if lastSyncAt.Valid {
		conn.LastSyncAt = &lastSyncAt.Time
	}
	if lastSyncStatus.Valid {
		conn.LastSyncStatus = lastSyncStatus.String
	}
	if lastSyncError.Valid {
		conn.LastSyncError = lastSyncError.String
	}

	return conn, nil
}

// scanConnections scans multiple rows into BrokerConnections.
func (r *BrokerConnectionRepository) scanConnections(rows *sql.Rows) ([]*models.BrokerConnection, error) {
	connections := make([]*models.BrokerConnection, 0)

	for rows.Next() {
		conn := &models.BrokerConnection{}
		var isActive int
		var lastSyncAt sql.NullTime
		var lastSyncStatus, lastSyncError, cpr, appKey, appSecret, redirectURI sql.NullString

		err := rows.Scan(
			&conn.ID,
			&conn.UserID,
			&conn.BrokerType,
			&conn.Username,
			&cpr,
			&conn.Country,
			&appKey,
			&appSecret,
			&redirectURI,
			&isActive,
			&lastSyncAt,
			&lastSyncStatus,
			&lastSyncError,
			&conn.CreatedAt,
			&conn.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}

		conn.IsActive = isActive == 1
		if cpr.Valid {
			conn.CPR = cpr.String
		}
		if appKey.Valid {
			conn.AppKey = appKey.String
		}
		if appSecret.Valid {
			conn.AppSecret = appSecret.String
		}
		if redirectURI.Valid {
			conn.RedirectURI = redirectURI.String
		}
		if lastSyncAt.Valid {
			conn.LastSyncAt = &lastSyncAt.Time
		}
		if lastSyncStatus.Valid {
			conn.LastSyncStatus = lastSyncStatus.String
		}
		if lastSyncError.Valid {
			conn.LastSyncError = lastSyncError.String
		}

		connections = append(connections, conn)
	}

	return connections, rows.Err()
}
