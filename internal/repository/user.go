// Package repository provides data access layer for the wealth tracker.
package repository

import (
	"database/sql"
	"fmt"
	"time"

	"wealth_tracker/internal/database"
	"wealth_tracker/internal/models"
)

// UserRepository handles user data operations.
type UserRepository struct {
	db *database.DB
}

// NewUserRepository creates a new UserRepository.
func NewUserRepository(db *database.DB) *UserRepository {
	return &UserRepository{db: db}
}

// Create inserts a new user and returns the ID.
func (r *UserRepository) Create(user *models.User) (int64, error) {
	query := `
		INSERT INTO users (email, password_hash, name, default_currency, number_format, theme, is_admin, must_change_password, created_at, updated_at)
		VALUES (?, ?, ?, COALESCE(NULLIF(?, ''), 'DKK'), COALESCE(NULLIF(?, ''), 'da'), COALESCE(NULLIF(?, ''), 'dark'), ?, ?, ?, ?)
	`
	now := time.Now()

	result, err := r.db.Exec(query,
		user.Email,
		user.PasswordHash,
		user.Name,
		user.DefaultCurrency,
		user.NumberFormat,
		user.Theme,
		boolToInt(user.IsAdmin),
		boolToInt(user.MustChangePassword),
		now,
		now,
	)
	if err != nil {
		return 0, fmt.Errorf("creating user: %w", err)
	}

	id, err := result.LastInsertId()
	if err != nil {
		return 0, fmt.Errorf("getting last insert id: %w", err)
	}

	return id, nil
}

// GetByID retrieves a user by ID. Returns nil if not found.
func (r *UserRepository) GetByID(id int64) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, default_currency, COALESCE(number_format, 'da'), theme,
		       COALESCE(is_admin, 0), COALESCE(must_change_password, 0), created_at, updated_at
		FROM users
		WHERE id = ?
	`

	user := &models.User{}
	var isAdmin, mustChangePassword int
	err := r.db.QueryRow(query, id).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.DefaultCurrency,
		&user.NumberFormat,
		&user.Theme,
		&isAdmin,
		&mustChangePassword,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by id: %w", err)
	}

	user.IsAdmin = isAdmin == 1
	user.MustChangePassword = mustChangePassword == 1
	return user, nil
}

// GetByEmail retrieves a user by email. Returns nil if not found.
func (r *UserRepository) GetByEmail(email string) (*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, default_currency, COALESCE(number_format, 'da'), theme,
		       COALESCE(is_admin, 0), COALESCE(must_change_password, 0), created_at, updated_at
		FROM users
		WHERE email = ?
	`

	user := &models.User{}
	var isAdmin, mustChangePassword int
	err := r.db.QueryRow(query, email).Scan(
		&user.ID,
		&user.Email,
		&user.PasswordHash,
		&user.Name,
		&user.DefaultCurrency,
		&user.NumberFormat,
		&user.Theme,
		&isAdmin,
		&mustChangePassword,
		&user.CreatedAt,
		&user.UpdatedAt,
	)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("getting user by email: %w", err)
	}

	user.IsAdmin = isAdmin == 1
	user.MustChangePassword = mustChangePassword == 1
	return user, nil
}

// Update updates a user's profile information (not password).
func (r *UserRepository) Update(user *models.User) error {
	query := `
		UPDATE users
		SET name = ?, default_currency = ?, number_format = ?, theme = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.Exec(query,
		user.Name,
		user.DefaultCurrency,
		user.NumberFormat,
		user.Theme,
		time.Now(),
		user.ID,
	)
	if err != nil {
		return fmt.Errorf("updating user: %w", err)
	}

	return nil
}

// UpdatePassword updates a user's password hash.
func (r *UserRepository) UpdatePassword(userID int64, passwordHash string) error {
	query := `
		UPDATE users
		SET password_hash = ?, updated_at = ?
		WHERE id = ?
	`

	_, err := r.db.Exec(query, passwordHash, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("updating password: %w", err)
	}

	return nil
}

// Delete removes a user by ID.
func (r *UserRepository) Delete(id int64) error {
	query := `DELETE FROM users WHERE id = ?`

	_, err := r.db.Exec(query, id)
	if err != nil {
		return fmt.Errorf("deleting user: %w", err)
	}

	return nil
}

// EmailExists checks if an email is already registered.
func (r *UserRepository) EmailExists(email string) (bool, error) {
	query := `SELECT COUNT(*) FROM users WHERE email = ?`

	var count int
	err := r.db.QueryRow(query, email).Scan(&count)
	if err != nil {
		return false, fmt.Errorf("checking email exists: %w", err)
	}

	return count > 0, nil
}

// GetAll retrieves all users.
func (r *UserRepository) GetAll() ([]*models.User, error) {
	query := `
		SELECT id, email, password_hash, name, default_currency, COALESCE(number_format, 'da'), theme,
		       COALESCE(is_admin, 0), COALESCE(must_change_password, 0), created_at, updated_at
		FROM users
		ORDER BY id ASC
	`

	rows, err := r.db.Query(query)
	if err != nil {
		return nil, fmt.Errorf("getting all users: %w", err)
	}
	defer rows.Close()

	var users []*models.User
	for rows.Next() {
		user := &models.User{}
		var isAdmin, mustChangePassword int
		err := rows.Scan(
			&user.ID,
			&user.Email,
			&user.PasswordHash,
			&user.Name,
			&user.DefaultCurrency,
			&user.NumberFormat,
			&user.Theme,
			&isAdmin,
			&mustChangePassword,
			&user.CreatedAt,
			&user.UpdatedAt,
		)
		if err != nil {
			return nil, fmt.Errorf("scanning user: %w", err)
		}
		user.IsAdmin = isAdmin == 1
		user.MustChangePassword = mustChangePassword == 1
		users = append(users, user)
	}

	return users, nil
}

// CountAll returns the total number of users.
func (r *UserRepository) CountAll() (int, error) {
	query := `SELECT COUNT(*) FROM users`

	var count int
	err := r.db.QueryRow(query).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("counting users: %w", err)
	}

	return count, nil
}

// SetAdmin updates a user's admin status.
func (r *UserRepository) SetAdmin(userID int64, isAdmin bool) error {
	query := `UPDATE users SET is_admin = ?, updated_at = ? WHERE id = ?`

	_, err := r.db.Exec(query, boolToInt(isAdmin), time.Now(), userID)
	if err != nil {
		return fmt.Errorf("setting admin status: %w", err)
	}

	return nil
}

// SetMustChangePassword updates a user's must_change_password flag.
func (r *UserRepository) SetMustChangePassword(userID int64, must bool) error {
	query := `UPDATE users SET must_change_password = ?, updated_at = ? WHERE id = ?`

	_, err := r.db.Exec(query, boolToInt(must), time.Now(), userID)
	if err != nil {
		return fmt.Errorf("setting must_change_password: %w", err)
	}

	return nil
}

// UpdateEmailAndName updates a user's email and name.
func (r *UserRepository) UpdateEmailAndName(userID int64, email, name string) error {
	query := `UPDATE users SET email = ?, name = ?, updated_at = ? WHERE id = ?`

	_, err := r.db.Exec(query, email, name, time.Now(), userID)
	if err != nil {
		return fmt.Errorf("updating email and name: %w", err)
	}

	return nil
}
