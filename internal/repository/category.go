package repository

import (
	"database/sql"
	"errors"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// CategoryRepository handles category database operations.
type CategoryRepository struct {
	db *database.DB
}

// NewCategoryRepository creates a new CategoryRepository.
func NewCategoryRepository(db *database.DB) *CategoryRepository {
	return &CategoryRepository{db: db}
}

// Create inserts a new category and returns its ID.
func (r *CategoryRepository) Create(category *models.Category) (int64, error) {
	result, err := r.db.Exec(`
		INSERT INTO categories (user_id, name, color, icon, sort_order)
		VALUES (?, ?, ?, ?, ?)
	`, category.UserID, category.Name, category.Color, category.Icon, category.SortOrder)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// GetByID retrieves a category by ID.
func (r *CategoryRepository) GetByID(id int64) (*models.Category, error) {
	row := r.db.QueryRow(`
		SELECT id, user_id, name, color, icon, sort_order, created_at
		FROM categories
		WHERE id = ?
	`, id)

	category := &models.Category{}
	err := row.Scan(
		&category.ID,
		&category.UserID,
		&category.Name,
		&category.Color,
		&category.Icon,
		&category.SortOrder,
		&category.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return category, nil
}

// GetByUserID retrieves all categories for a user, sorted by sort_order.
func (r *CategoryRepository) GetByUserID(userID int64) ([]*models.Category, error) {
	rows, err := r.db.Query(`
		SELECT id, user_id, name, color, icon, sort_order, created_at
		FROM categories
		WHERE user_id = ?
		ORDER BY sort_order ASC, name ASC
	`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	categories := make([]*models.Category, 0)
	for rows.Next() {
		category := &models.Category{}
		err := rows.Scan(
			&category.ID,
			&category.UserID,
			&category.Name,
			&category.Color,
			&category.Icon,
			&category.SortOrder,
			&category.CreatedAt,
		)
		if err != nil {
			return nil, err
		}
		categories = append(categories, category)
	}
	return categories, rows.Err()
}

// Update updates an existing category.
func (r *CategoryRepository) Update(category *models.Category) error {
	result, err := r.db.Exec(`
		UPDATE categories
		SET name = ?, color = ?, icon = ?, sort_order = ?
		WHERE id = ?
	`, category.Name, category.Color, category.Icon, category.SortOrder, category.ID)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("category not found")
	}
	return nil
}

// Delete removes a category by ID.
func (r *CategoryRepository) Delete(id int64) error {
	result, err := r.db.Exec(`DELETE FROM categories WHERE id = ?`, id)
	if err != nil {
		return err
	}

	rowsAffected, err := result.RowsAffected()
	if err != nil {
		return err
	}
	if rowsAffected == 0 {
		return errors.New("category not found")
	}
	return nil
}

// CountByUserID returns the number of categories for a user.
func (r *CategoryRepository) CountByUserID(userID int64) (int, error) {
	var count int
	err := r.db.QueryRow(`
		SELECT COUNT(*) FROM categories WHERE user_id = ?
	`, userID).Scan(&count)
	return count, err
}

// NameExists checks if a category name already exists for a user.
// Pass excludeID > 0 to exclude a specific category (useful for updates).
func (r *CategoryRepository) NameExists(userID int64, name string, excludeID int64) (bool, error) {
	var count int
	query := `SELECT COUNT(*) FROM categories WHERE user_id = ? AND name = ?`
	args := []any{userID, name}

	if excludeID > 0 {
		query += ` AND id != ?`
		args = append(args, excludeID)
	}

	err := r.db.QueryRow(query, args...).Scan(&count)
	return count > 0, err
}
