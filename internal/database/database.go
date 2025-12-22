// Package database provides SQLite database connection and operations.
package database

import (
	"database/sql"
	"fmt"
	"os"
	"path/filepath"

	_ "modernc.org/sqlite"
)

// DB wraps the sql.DB connection with additional functionality.
type DB struct {
	*sql.DB
}

// New creates a new database connection at the specified path.
// It creates the parent directory if it doesn't exist.
func New(dbPath string) (*DB, error) {
	// Ensure parent directory exists
	dir := filepath.Dir(dbPath)
	if err := os.MkdirAll(dir, 0755); err != nil {
		return nil, fmt.Errorf("creating database directory: %w", err)
	}

	// Open database connection
	sqlDB, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// Configure connection pool - SQLite doesn't support concurrent writes
	sqlDB.SetMaxOpenConns(1)
	sqlDB.SetMaxIdleConns(1)

	// Enable foreign keys and WAL mode via PRAGMA
	pragmas := []string{
		"PRAGMA foreign_keys = ON",
		"PRAGMA journal_mode = WAL",
	}
	for _, pragma := range pragmas {
		if _, err := sqlDB.Exec(pragma); err != nil {
			sqlDB.Close()
			return nil, fmt.Errorf("setting pragma: %w", err)
		}
	}

	// Verify connection
	if err := sqlDB.Ping(); err != nil {
		sqlDB.Close()
		return nil, fmt.Errorf("connecting to database: %w", err)
	}

	return &DB{DB: sqlDB}, nil
}

// RunMigrations executes all database migrations.
// Migrations are idempotent and can be run multiple times safely.
func (db *DB) RunMigrations() error {
	migrations := []string{
		migrationUsers,
		migrationCategories,
		migrationAccounts,
		migrationTransactions,
		migrationGoals,
		migrationCurrencyRates,
		migrationSessions,
		migrationIndexes,
		// Broker integration tables
		migrationBrokerConnections,
		migrationBrokerSessions,
		migrationHoldings,
		migrationAccountMappings,
		migrationSyncHistory,
		migrationBrokerIndexes,
	}

	for i, migration := range migrations {
		if _, err := db.Exec(migration); err != nil {
			return fmt.Errorf("migration %d failed: %w", i+1, err)
		}
	}

	// Run ALTER TABLE migrations separately (they fail silently if column exists)
	alterMigrations := []string{
		migrationAddNumberFormat,
		migrationAddGoalCategory,
		migrationAddIsAdmin,
		migrationAddMustChangePassword,
		migrationAddBrokerCPR,
		// Saxo OAuth token storage
		migrationAddSaxoRefreshToken,
		migrationAddSaxoTokenExpiry,
		migrationAddSaxoRefreshExpiry,
		migrationAddSaxoAppKey,
		migrationAddSaxoAppSecret,
		migrationAddSaxoRedirectURI,
	}
	for _, migration := range alterMigrations {
		// Ignore "duplicate column" errors for idempotency
		db.Exec(migration)
	}

	// Run DROP COLUMN migrations for deprecated password columns
	// These may fail on older SQLite versions (< 3.35) - that's okay, columns just stay unused
	dropMigrations := []string{
		migrationDropBrokerPasswordColumns,
		migrationDropBrokerPasswordIV,
	}
	for _, migration := range dropMigrations {
		// Ignore errors - column may not exist or SQLite version doesn't support DROP COLUMN
		db.Exec(migration)
	}

	return nil
}
