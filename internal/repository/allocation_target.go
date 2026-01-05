package repository

import (
	"database/sql"
	"errors"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// AllocationTargetRepository handles allocation target database operations.
type AllocationTargetRepository struct {
	db *database.DB
}

// NewAllocationTargetRepository creates a new AllocationTargetRepository.
func NewAllocationTargetRepository(db *database.DB) *AllocationTargetRepository {
	return &AllocationTargetRepository{db: db}
}

// Create inserts a new allocation target and returns its ID.
func (r *AllocationTargetRepository) Create(target *models.AllocationTarget) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO allocation_targets (user_id, target_type, target_key, target_pct)
		VALUES (?, ?, ?, ?)
	`, target.UserID, target.TargetType, target.TargetKey, target.TargetPct)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves an allocation target by ID.
func (r *AllocationTargetRepository) GetByID(id int64) (*models.AllocationTarget, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, target_type, target_key, target_pct, created_at, updated_at
		FROM allocation_targets
		WHERE id = ?
	`, id)

	target := &models.AllocationTarget{}
	err := row.Scan(
		&target.ID,
		&target.UserID,
		&target.TargetType,
		&target.TargetKey,
		&target.TargetPct,
		&target.CreatedAt,
		&target.UpdatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return target, nil
}

// GetByUserID retrieves all allocation targets for a user.
func (r *AllocationTargetRepository) GetByUserID(userID int64) ([]*models.AllocationTarget, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, target_type, target_key, target_pct, created_at, updated_at
		FROM allocation_targets
		WHERE user_id = ?
		ORDER BY target_type, target_key
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]*models.AllocationTarget, 0)
	for rows.Next() {
		target := &models.AllocationTarget{}
		err := rows.Scan(
			&target.ID,
			&target.UserID,
			&target.TargetType,
			&target.TargetKey,
			&target.TargetPct,
			&target.CreatedAt,
			&target.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// GetByUserIDAndType retrieves allocation targets for a user filtered by type.
func (r *AllocationTargetRepository) GetByUserIDAndType(userID int64, targetType string) ([]*models.AllocationTarget, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, target_type, target_key, target_pct, created_at, updated_at
		FROM allocation_targets
		WHERE user_id = ? AND target_type = ?
		ORDER BY target_key
	`, userID, targetType)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	targets := make([]*models.AllocationTarget, 0)
	for rows.Next() {
		target := &models.AllocationTarget{}
		err := rows.Scan(
			&target.ID,
			&target.UserID,
			&target.TargetType,
			&target.TargetKey,
			&target.TargetPct,
			&target.CreatedAt,
			&target.UpdatedAt,
		)
		if err != nil {
			return nil, err
		}
		targets = append(targets, target)
	}
	return targets, rows.Err()
}

// Upsert creates or updates an allocation target based on user_id, target_type, target_key.
func (r *AllocationTargetRepository) Upsert(target *models.AllocationTarget) (int64, error) {
	_, err := r.db.Exec(`
		INSERT INTO allocation_targets (user_id, target_type, target_key, target_pct, updated_at)
		VALUES (?, ?, ?, ?, ?)
		ON CONFLICT(user_id, target_type, target_key)
		DO UPDATE SET target_pct = excluded.target_pct, updated_at = excluded.updated_at
	`, target.UserID, target.TargetType, target.TargetKey, target.TargetPct, time.Now())
	if err != nil {
		return 0, err
	}

	// LastInsertId() returns 0 on UPDATE in SQLite, so query the ID explicitly
	var id int64
	err = r.db.QueryRow(`
		SELECT id FROM allocation_targets
		WHERE user_id = ? AND target_type = ? AND target_key = ?
	`, target.UserID, target.TargetType, target.TargetKey).Scan(&id)
	return id, err
}

// Update updates an existing allocation target.
func (r *AllocationTargetRepository) Update(target *models.AllocationTarget) error {
	result, err := r.db.Exec(`
		UPDATE allocation_targets
		SET target_pct = ?, updated_at = ?
		WHERE id = ?
	`, target.TargetPct, time.Now(), target.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("allocation target not found")
	}
	return nil
}

// Delete removes an allocation target by ID.
func (r *AllocationTargetRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM allocation_targets WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("allocation target not found")
	}
	return nil
}

// DeleteByUserIDAndType deletes all targets for a user of a specific type.
func (r *AllocationTargetRepository) DeleteByUserIDAndType(userID int64, targetType string) error {
	_, err := r.db.Exec(`
		DELETE FROM allocation_targets
		WHERE user_id = ? AND target_type = ?
	`, userID, targetType)
	return err
}

// GetTotalPercentByType returns the sum of target percentages for a given type.
// Used to validate that targets sum to 100%.
func (r *AllocationTargetRepository) GetTotalPercentByType(userID int64, targetType string) (float64, error) {
	var total float64
	err := r.db.QueryRow(`
		SELECT COALESCE(SUM(target_pct), 0)
		FROM allocation_targets
		WHERE user_id = ? AND target_type = ?
	`, userID, targetType).Scan(&total)
	return total, err
}
