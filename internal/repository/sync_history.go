package repository

import (
	"database/sql"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// SyncHistoryRepository handles sync history database operations.
type SyncHistoryRepository struct {
	db *database.DB
}

// NewSyncHistoryRepository creates a new SyncHistoryRepository.
func NewSyncHistoryRepository(db *database.DB) *SyncHistoryRepository {
	return &SyncHistoryRepository{db: db}
}

// Start creates a new sync history entry with status "started" and returns its ID.
func (r *SyncHistoryRepository) Start(connectionID int64, syncType string) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO sync_history (connection_id, sync_type, status, started_at)
		VALUES (?, ?, 'started', ?)
	`, connectionID, syncType, time.Now())
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// Complete marks a sync as successful.
func (r *SyncHistoryRepository) Complete(id int64, accountsSynced, positionsSynced int) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE sync_history
		SET status = 'success', accounts_synced = ?, positions_synced = ?, completed_at = ?,
		    duration_ms = (julianday(?) - julianday(started_at)) * 86400000
		WHERE id = ?
	`, accountsSynced, positionsSynced, now, now, id)
	return err
}

// Fail marks a sync as failed with an error message.
func (r *SyncHistoryRepository) Fail(id int64, errorMsg string) error {
	now := time.Now()
	_, err := r.db.Exec(`
		UPDATE sync_history
		SET status = 'error', error_message = ?, completed_at = ?,
		    duration_ms = (julianday(?) - julianday(started_at)) * 86400000
		WHERE id = ?
	`, errorMsg, now, now, id)
	return err
}

// GetByID retrieves a sync history entry by ID.
func (r *SyncHistoryRepository) GetByID(id int64) (*models.SyncHistory, error) {
	row := r.db.QueryRow(`
		SELECT id, connection_id, sync_type, status, accounts_synced, positions_synced, error_message, started_at, completed_at, duration_ms
		FROM sync_history
		WHERE id = ?
	`, id)

	return r.scanHistory(row)
}

// GetByConnectionID retrieves all sync history for a connection, most recent first.
func (r *SyncHistoryRepository) GetByConnectionID(connectionID int64, limit int) ([]*models.SyncHistory, error) {
	rows, err := r.db.Query(`
		SELECT id, connection_id, sync_type, status, accounts_synced, positions_synced, error_message, started_at, completed_at, duration_ms
		FROM sync_history
		WHERE connection_id = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, connectionID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanHistories(rows)
}

// GetLatestByConnectionID retrieves the most recent sync history for a connection.
func (r *SyncHistoryRepository) GetLatestByConnectionID(connectionID int64) (*models.SyncHistory, error) {
	row := r.db.QueryRow(`
		SELECT id, connection_id, sync_type, status, accounts_synced, positions_synced, error_message, started_at, completed_at, duration_ms
		FROM sync_history
		WHERE connection_id = ?
		ORDER BY started_at DESC
		LIMIT 1
	`, connectionID)

	return r.scanHistory(row)
}

// GetRecentByStatus retrieves recent sync history entries by status.
func (r *SyncHistoryRepository) GetRecentByStatus(status string, limit int) ([]*models.SyncHistory, error) {
	rows, err := r.db.Query(`
		SELECT id, connection_id, sync_type, status, accounts_synced, positions_synced, error_message, started_at, completed_at, duration_ms
		FROM sync_history
		WHERE status = ?
		ORDER BY started_at DESC
		LIMIT ?
	`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return r.scanHistories(rows)
}

// DeleteOlderThan removes sync history entries older than the given time.
func (r *SyncHistoryRepository) DeleteOlderThan(before time.Time) (int64, error) {
	result, err := r.db.Exec(`DELETE FROM sync_history WHERE started_at < ?`, before)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// DeleteByConnectionID removes all sync history for a connection.
func (r *SyncHistoryRepository) DeleteByConnectionID(connectionID int64) error {
	_, err := r.db.Exec(`DELETE FROM sync_history WHERE connection_id = ?`, connectionID)
	return err
}

// CountByConnectionID returns the number of sync history entries for a connection.
func (r *SyncHistoryRepository) CountByConnectionID(connectionID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`SELECT COUNT(*) FROM sync_history WHERE connection_id = ?`, connectionID).Scan(&count)
	return count, err
}

// scanHistory scans a single row into a SyncHistory.
func (r *SyncHistoryRepository) scanHistory(row *sql.Row) (*models.SyncHistory, error) {
	history := &models.SyncHistory{}
	var errorMsg sql.NullString
	var completedAt sql.NullTime
	var durationMs sql.NullInt64

	err := row.Scan(
		&history.ID,
		&history.ConnectionID,
		&history.SyncType,
		&history.Status,
		&history.AccountsSynced,
		&history.PositionsSynced,
		&errorMsg,
		&history.StartedAt,
		&completedAt,
		&durationMs,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if errorMsg.Valid {
		history.ErrorMessage = errorMsg.String
	}
	if completedAt.Valid {
		history.CompletedAt = &completedAt.Time
	}
	if durationMs.Valid {
		history.DurationMs = durationMs.Int64
	}

	return history, nil
}

// scanHistories scans multiple rows into SyncHistories.
func (r *SyncHistoryRepository) scanHistories(rows *sql.Rows) ([]*models.SyncHistory, error) {
	histories := make([]*models.SyncHistory, 0)

	for rows.Next() {
		history := &models.SyncHistory{}
		var errorMsg sql.NullString
		var completedAt sql.NullTime
		var durationMs sql.NullInt64

		err := rows.Scan(
			&history.ID,
			&history.ConnectionID,
			&history.SyncType,
			&history.Status,
			&history.AccountsSynced,
			&history.PositionsSynced,
			&errorMsg,
			&history.StartedAt,
			&completedAt,
			&durationMs,
		)
		if err != nil {
			return nil, err
		}

		if errorMsg.Valid {
			history.ErrorMessage = errorMsg.String
		}
		if completedAt.Valid {
			history.CompletedAt = &completedAt.Time
		}
		if durationMs.Valid {
			history.DurationMs = durationMs.Int64
		}

		histories = append(histories, history)
	}

	return histories, rows.Err()
}
