package repository

import (
	"database/sql"
	"errors"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// GoalRepository handles goal database operations.
type GoalRepository struct {
	db *database.DB
}

// NewGoalRepository creates a new GoalRepository.
func NewGoalRepository(db *database.DB) *GoalRepository {
	return &GoalRepository{db: db}
}

// Create inserts a new goal and returns its ID.
func (r *GoalRepository) Create(goal *models.Goal) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO goals (user_id, category_id, name, target_amount, target_currency, deadline)
		VALUES (?, ?, ?, ?, ?, ?)
	`, goal.UserID, goal.CategoryID, goal.Name, goal.TargetAmount, goal.TargetCurrency, goal.Deadline)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves a goal by ID.
func (r *GoalRepository) GetByID(id int64) (*models.Goal, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, category_id, name, target_amount, target_currency, deadline, reached_date, created_at
		FROM goals
		WHERE id = ?
	`, id)

	goal := &models.Goal{}
	var categoryID sql.NullInt64
	var deadline, reachedDate sql.NullTime

	err := row.Scan(
		&goal.ID,
		&goal.UserID,
		&categoryID,
		&goal.Name,
		&goal.TargetAmount,
		&goal.TargetCurrency,
		&deadline,
		&reachedDate,
		&goal.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	if categoryID.Valid {
		goal.CategoryID = &categoryID.Int64
	}
	if deadline.Valid {
		goal.Deadline = &deadline.Time
	}
	if reachedDate.Valid {
		goal.ReachedDate = &reachedDate.Time
	}

	return goal, nil
}

// GetByUserID retrieves all goals for a user, sorted by deadline then name.
func (r *GoalRepository) GetByUserID(userID int64) ([]*models.Goal, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, category_id, name, target_amount, target_currency, deadline, reached_date, created_at
		FROM goals
		WHERE user_id = ?
		ORDER BY COALESCE(deadline, '9999-12-31') ASC, name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	goals := make([]*models.Goal, 0)
	for rows.Next() {
		goal := &models.Goal{}
		var categoryID sql.NullInt64
		var deadline, reachedDate sql.NullTime

		err := rows.Scan(
			&goal.ID,
			&goal.UserID,
			&categoryID,
			&goal.Name,
			&goal.TargetAmount,
			&goal.TargetCurrency,
			&deadline,
			&reachedDate,
			&goal.CreatedAt,
		)
		if err != nil {
			return nil, err
		}

		if categoryID.Valid {
			goal.CategoryID = &categoryID.Int64
		}
		if deadline.Valid {
			goal.Deadline = &deadline.Time
		}
		if reachedDate.Valid {
			goal.ReachedDate = &reachedDate.Time
		}

		goals = append(goals, goal)
	}
	return goals, rows.Err()
}

// Update updates an existing goal.
func (r *GoalRepository) Update(goal *models.Goal) error {
	result, err := r.db.Exec(`
		UPDATE goals
		SET category_id = ?, name = ?, target_amount = ?, target_currency = ?, deadline = ?, reached_date = ?
		WHERE id = ?
	`, goal.CategoryID, goal.Name, goal.TargetAmount, goal.TargetCurrency, goal.Deadline, goal.ReachedDate, goal.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("goal not found")
	}
	return nil
}

// Delete removes a goal by ID.
func (r *GoalRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM goals WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("goal not found")
	}
	return nil
}

// CountByUserID returns the number of goals for a user.
func (r *GoalRepository) CountByUserID(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM goals WHERE user_id = ?
	`, userID).Scan(&count)
	return count, err
}

// CountReachedByUserID returns the number of reached goals for a user.
func (r *GoalRepository) CountReachedByUserID(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM goals WHERE user_id = ? AND reached_date IS NOT NULL
	`, userID).Scan(&count)
	return count, err
}

// MarkAsReached marks a goal as reached with the current timestamp.
func (r *GoalRepository) MarkAsReached(id int64) error {
	_, err := r.db.Exec(`
		UPDATE goals SET reached_date = CURRENT_TIMESTAMP WHERE id = ? AND reached_date IS NULL
	`, id)
	return err
}
