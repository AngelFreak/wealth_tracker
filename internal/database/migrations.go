package database

// SQL migrations for the wealth tracker database.
// All migrations use IF NOT EXISTS to be idempotent.

const migrationUsers = `
CREATE TABLE IF NOT EXISTS users (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    email TEXT UNIQUE NOT NULL,
    password_hash TEXT NOT NULL,
    name TEXT NOT NULL,
    default_currency TEXT DEFAULT 'DKK',
    theme TEXT DEFAULT 'dark',
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCategories = `
CREATE TABLE IF NOT EXISTS categories (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    color TEXT DEFAULT '#6366f1',
    icon TEXT,
    sort_order INTEGER DEFAULT 0,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationAccounts = `
CREATE TABLE IF NOT EXISTS accounts (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL,
    name TEXT NOT NULL,
    currency TEXT DEFAULT 'DKK',
    is_liability INTEGER DEFAULT 0,
    is_active INTEGER DEFAULT 1,
    notes TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationTransactions = `
CREATE TABLE IF NOT EXISTS transactions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    amount REAL NOT NULL,
    balance_after REAL NOT NULL,
    description TEXT,
    transaction_date DATE NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationGoals = `
CREATE TABLE IF NOT EXISTS goals (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    target_amount REAL NOT NULL,
    target_currency TEXT DEFAULT 'DKK',
    deadline DATE,
    reached_date DATE,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationCurrencyRates = `
CREATE TABLE IF NOT EXISTS currency_rates (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    from_currency TEXT NOT NULL,
    to_currency TEXT NOT NULL,
    rate REAL NOT NULL,
    fetched_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(from_currency, to_currency)
);
`

const migrationSessions = `
CREATE TABLE IF NOT EXISTS sessions (
    id TEXT PRIMARY KEY,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

const migrationIndexes = `
CREATE INDEX IF NOT EXISTS idx_accounts_user ON accounts(user_id);
CREATE INDEX IF NOT EXISTS idx_transactions_account ON transactions(account_id);
CREATE INDEX IF NOT EXISTS idx_transactions_date ON transactions(transaction_date);
CREATE INDEX IF NOT EXISTS idx_categories_user ON categories(user_id);
CREATE INDEX IF NOT EXISTS idx_goals_user ON goals(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_user ON sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_sessions_expires ON sessions(expires_at);
CREATE UNIQUE INDEX IF NOT EXISTS idx_categories_user_name ON categories(user_id, name);
CREATE UNIQUE INDEX IF NOT EXISTS idx_accounts_user_name ON accounts(user_id, name);
`

// migrationAddNumberFormat adds the number_format column to users table
const migrationAddNumberFormat = `
ALTER TABLE users ADD COLUMN number_format TEXT DEFAULT 'da';
`

// migrationAddGoalCategory adds the category_id column to goals table
const migrationAddGoalCategory = `
ALTER TABLE goals ADD COLUMN category_id INTEGER REFERENCES categories(id) ON DELETE SET NULL;
`

// migrationAddIsAdmin adds the is_admin column to users table
const migrationAddIsAdmin = `
ALTER TABLE users ADD COLUMN is_admin INTEGER DEFAULT 0;
`

// migrationAddMustChangePassword adds the must_change_password column to users table
const migrationAddMustChangePassword = `
ALTER TABLE users ADD COLUMN must_change_password INTEGER DEFAULT 0;
`

// migrationBrokerConnections stores broker connection settings.
// Note: password_encrypted and encryption_iv columns were removed in favor of MitID authentication.
const migrationBrokerConnections = `
CREATE TABLE IF NOT EXISTS broker_connections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    broker_type TEXT NOT NULL DEFAULT 'nordnet',
    username TEXT NOT NULL,
    country TEXT NOT NULL DEFAULT 'dk',
    is_active INTEGER DEFAULT 1,
    last_sync_at DATETIME,
    last_sync_status TEXT,
    last_sync_error TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, broker_type)
);
`

// migrationDropBrokerPasswordColumns removes deprecated password columns.
// This handles migration from password-based to MitID authentication.
const migrationDropBrokerPasswordColumns = `
-- SQLite 3.35+ supports DROP COLUMN
-- For older versions, this will fail silently and the columns remain (unused)
ALTER TABLE broker_connections DROP COLUMN password_encrypted;
`

const migrationDropBrokerPasswordIV = `
ALTER TABLE broker_connections DROP COLUMN encryption_iv;
`

// migrationBrokerSessions caches active broker sessions to avoid re-auth
const migrationBrokerSessions = `
CREATE TABLE IF NOT EXISTS broker_sessions (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id INTEGER NOT NULL REFERENCES broker_connections(id) ON DELETE CASCADE,
    session_data TEXT NOT NULL,
    expires_at DATETIME NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// migrationHoldings stores position/instrument data per account
const migrationHoldings = `
CREATE TABLE IF NOT EXISTS holdings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    external_id TEXT,
    symbol TEXT NOT NULL,
    name TEXT NOT NULL,
    quantity REAL NOT NULL,
    avg_price REAL,
    current_price REAL,
    current_value REAL NOT NULL,
    currency TEXT NOT NULL DEFAULT 'DKK',
    instrument_type TEXT,
    last_updated DATETIME DEFAULT CURRENT_TIMESTAMP,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(account_id, symbol)
);
`

// migrationAccountMappings links broker accounts to local accounts
const migrationAccountMappings = `
CREATE TABLE IF NOT EXISTS account_mappings (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id INTEGER NOT NULL REFERENCES broker_connections(id) ON DELETE CASCADE,
    local_account_id INTEGER NOT NULL REFERENCES accounts(id) ON DELETE CASCADE,
    external_account_id TEXT NOT NULL,
    external_account_name TEXT,
    auto_sync INTEGER DEFAULT 1,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(connection_id, external_account_id),
    UNIQUE(connection_id, local_account_id)
);
`

// migrationSyncHistory tracks sync operations for auditing
const migrationSyncHistory = `
CREATE TABLE IF NOT EXISTS sync_history (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    connection_id INTEGER NOT NULL REFERENCES broker_connections(id) ON DELETE CASCADE,
    sync_type TEXT NOT NULL,
    status TEXT NOT NULL,
    accounts_synced INTEGER DEFAULT 0,
    positions_synced INTEGER DEFAULT 0,
    error_message TEXT,
    started_at DATETIME NOT NULL,
    completed_at DATETIME,
    duration_ms INTEGER
);
`

// migrationBrokerIndexes adds indexes for broker-related tables
const migrationBrokerIndexes = `
CREATE INDEX IF NOT EXISTS idx_broker_connections_user ON broker_connections(user_id);
CREATE INDEX IF NOT EXISTS idx_broker_sessions_connection ON broker_sessions(connection_id);
CREATE INDEX IF NOT EXISTS idx_holdings_account ON holdings(account_id);
CREATE INDEX IF NOT EXISTS idx_account_mappings_connection ON account_mappings(connection_id);
CREATE INDEX IF NOT EXISTS idx_sync_history_connection ON sync_history(connection_id);
`

// migrationAddBrokerCPR adds the CPR column to broker_connections for Signicat verification.
// CPR is required for MitID-CPR authentication flow used by Nordnet.
const migrationAddBrokerCPR = `
ALTER TABLE broker_connections ADD COLUMN cpr TEXT;
`

// migrationAddSaxoOAuthFields adds columns for Saxo OAuth token storage.
// Saxo uses OAuth2 PKCE which requires storing refresh tokens for session persistence.
const migrationAddSaxoRefreshToken = `
ALTER TABLE broker_connections ADD COLUMN refresh_token_encrypted TEXT;
`

const migrationAddSaxoTokenExpiry = `
ALTER TABLE broker_connections ADD COLUMN token_expires_at DATETIME;
`

const migrationAddSaxoRefreshExpiry = `
ALTER TABLE broker_connections ADD COLUMN refresh_expires_at DATETIME;
`

const migrationAddSaxoAppKey = `
ALTER TABLE broker_connections ADD COLUMN app_key TEXT;
`

const migrationAddSaxoAppSecret = `
ALTER TABLE broker_connections ADD COLUMN app_secret TEXT;
`

const migrationAddSaxoRedirectURI = `
ALTER TABLE broker_connections ADD COLUMN redirect_uri TEXT;
`

// migrationAllocationTargets stores user-defined portfolio allocation targets.
// Used by the Portfolio Analyzer to compare actual vs target allocations.
const migrationAllocationTargets = `
CREATE TABLE IF NOT EXISTS allocation_targets (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    target_type TEXT NOT NULL,
    target_key TEXT NOT NULL,
    target_pct REAL NOT NULL,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
    UNIQUE(user_id, target_type, target_key)
);
`

// migrationAllocationTargetsIndex adds index for efficient user lookup
const migrationAllocationTargetsIndex = `
CREATE INDEX IF NOT EXISTS idx_allocation_targets_user ON allocation_targets(user_id);
`

// migrationAuditLog stores audit entries for tracking user and admin actions.
const migrationAuditLog = `
CREATE TABLE IF NOT EXISTS audit_log (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    user_id INTEGER NOT NULL,
    actor_id INTEGER NOT NULL,
    action TEXT NOT NULL,
    entity_type TEXT NOT NULL,
    entity_id INTEGER,
    old_values TEXT,
    new_values TEXT,
    ip_address TEXT,
    user_agent TEXT,
    created_at DATETIME DEFAULT CURRENT_TIMESTAMP
);
`

// migrationAuditLogIndexes adds indexes for efficient audit log queries
const migrationAuditLogIndexes = `
CREATE INDEX IF NOT EXISTS idx_audit_log_user ON audit_log(user_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_actor ON audit_log(actor_id);
CREATE INDEX IF NOT EXISTS idx_audit_log_action ON audit_log(action);
CREATE INDEX IF NOT EXISTS idx_audit_log_created ON audit_log(created_at);
`
