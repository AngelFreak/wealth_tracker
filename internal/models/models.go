// Package models contains the domain models for the wealth tracker.
package models

import "time"

// User represents a registered user.
type User struct {
	ID                 int64     `json:"id"`
	Email              string    `json:"email"`
	PasswordHash       string    `json:"-"` // Never expose in JSON
	Name               string    `json:"name"`
	DefaultCurrency    string    `json:"default_currency"`
	NumberFormat       string    `json:"number_format"` // "da" (Danish: 1.234,56), "en" (English: 1,234.56), "de" (German: 1.234,56), "fr" (French: 1 234,56)
	Theme              string    `json:"theme"`
	IsAdmin            bool      `json:"is_admin"`
	MustChangePassword bool      `json:"must_change_password"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`
}

// Category represents an asset category (e.g., Aktier, Krypto, Pension).
type Category struct {
	ID        int64     `json:"id"`
	UserID    int64     `json:"user_id"`
	Name      string    `json:"name"`
	Color     string    `json:"color"`
	Icon      string    `json:"icon,omitempty"`
	SortOrder int       `json:"sort_order"`
	CreatedAt time.Time `json:"created_at"`
}

// Account represents a financial account (e.g., Nordnet, SaxoInvester).
type Account struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	CategoryID  *int64    `json:"category_id,omitempty"`
	Name        string    `json:"name"`
	Currency    string    `json:"currency"`
	IsLiability bool      `json:"is_liability"`
	IsActive    bool      `json:"is_active"`
	Notes       string    `json:"notes,omitempty"`
	Balance     float64   `json:"balance"` // Calculated from transactions
	CreatedAt   time.Time `json:"created_at"`
}

// Transaction represents a financial transaction.
type Transaction struct {
	ID              int64     `json:"id"`
	AccountID       int64     `json:"account_id"`
	Amount          float64   `json:"amount"`
	BalanceAfter    float64   `json:"balance_after"`
	Description     string    `json:"description,omitempty"`
	TransactionDate time.Time `json:"transaction_date"`
	CreatedAt       time.Time `json:"created_at"`
}

// Goal represents a wealth milestone goal.
type Goal struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	CategoryID     *int64     `json:"category_id,omitempty"` // NULL = global (net worth), set = category-specific
	Name           string     `json:"name"`
	TargetAmount   float64    `json:"target_amount"`
	TargetCurrency string     `json:"target_currency"`
	Deadline       *time.Time `json:"deadline,omitempty"`
	ReachedDate    *time.Time `json:"reached_date,omitempty"`
	Progress       float64    `json:"progress"` // Calculated field (0-100)
	CreatedAt      time.Time  `json:"created_at"`
}

// CurrencyRate represents an exchange rate between two currencies.
type CurrencyRate struct {
	ID           int64     `json:"id"`
	FromCurrency string    `json:"from_currency"`
	ToCurrency   string    `json:"to_currency"`
	Rate         float64   `json:"rate"`
	FetchedAt    time.Time `json:"fetched_at"`
}

// Session represents a user session for authentication.
type Session struct {
	ID        string    `json:"id"`
	UserID    int64     `json:"user_id"`
	ExpiresAt time.Time `json:"expires_at"`
	CreatedAt time.Time `json:"created_at"`
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// BrokerConnection represents a connection to an external broker API.
// Authentication is handled via MitID (Nordnet) or OAuth2 (Saxo).
type BrokerConnection struct {
	ID             int64      `json:"id"`
	UserID         int64      `json:"user_id"`
	BrokerType     string     `json:"broker_type"` // "nordnet", "saxo", etc.
	Username       string     `json:"username"`    // MitID user identifier (Nordnet) or empty (Saxo)
	CPR            string     `json:"-"`           // CPR number for Signicat verification (never expose in JSON)
	Country        string     `json:"country"`     // "dk", "se", "no", "fi"
	AppKey         string     `json:"-"`           // Saxo App Key (client_id) - never expose in JSON
	AppSecret      string     `json:"-"`           // Saxo App Secret (client_secret) - for non-PKCE flow, never expose
	RedirectURI    string     `json:"redirect_uri"` // Saxo OAuth redirect URI (registered in developer portal)
	IsActive       bool       `json:"is_active"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus string     `json:"last_sync_status,omitempty"` // "success", "error", "auth_failed"
	LastSyncError  string     `json:"last_sync_error,omitempty"`
	// Saxo OAuth2 token storage (encrypted)
	RefreshTokenEncrypted string     `json:"-"`                           // Encrypted refresh token (never expose)
	TokenExpiresAt        *time.Time `json:"token_expires_at,omitempty"`  // Access token expiry
	RefreshExpiresAt      *time.Time `json:"refresh_expires_at,omitempty"` // Refresh token expiry
	CreatedAt             time.Time  `json:"created_at"`
	UpdatedAt             time.Time  `json:"updated_at"`
}

// BrokerSession caches an active broker API session.
type BrokerSession struct {
	ID           int64     `json:"id"`
	ConnectionID int64     `json:"connection_id"`
	SessionData  string    `json:"session_data"` // JSON-encoded cookies/tokens
	ExpiresAt    time.Time `json:"expires_at"`
	CreatedAt    time.Time `json:"created_at"`
}

// IsExpired returns true if the broker session has expired.
func (bs *BrokerSession) IsExpired() bool {
	return time.Now().After(bs.ExpiresAt)
}

// Holding represents a position/instrument in an account.
type Holding struct {
	ID             int64     `json:"id"`
	AccountID      int64     `json:"account_id"`
	ExternalID     string    `json:"external_id,omitempty"` // Broker's instrument ID
	Symbol         string    `json:"symbol"`                // ISIN or ticker
	Name           string    `json:"name"`
	Quantity       float64   `json:"quantity"`
	AvgPrice       float64   `json:"avg_price,omitempty"`     // Average acquisition price
	CurrentPrice   float64   `json:"current_price,omitempty"` // Latest price
	CurrentValue   float64   `json:"current_value"`           // Quantity * CurrentPrice
	Currency       string    `json:"currency"`
	InstrumentType string    `json:"instrument_type,omitempty"` // "stock", "etf", "fund", "bond", "cash"
	LastUpdated    time.Time `json:"last_updated"`
	CreatedAt      time.Time `json:"created_at"`
}

// ProfitLoss returns the unrealized P/L for this holding.
func (h *Holding) ProfitLoss() float64 {
	if h.AvgPrice == 0 {
		return 0
	}
	return h.CurrentValue - (h.Quantity * h.AvgPrice)
}

// ProfitLossPercent returns the unrealized P/L percentage.
func (h *Holding) ProfitLossPercent() float64 {
	cost := h.Quantity * h.AvgPrice
	if cost == 0 {
		return 0
	}
	return ((h.CurrentValue - cost) / cost) * 100
}

// AccountMapping links a broker account to a local account.
type AccountMapping struct {
	ID                  int64     `json:"id"`
	ConnectionID        int64     `json:"connection_id"`
	LocalAccountID      int64     `json:"local_account_id"`
	ExternalAccountID   string    `json:"external_account_id"`   // Broker's account ID
	ExternalAccountName string    `json:"external_account_name"` // Display name from broker
	AutoSync            bool      `json:"auto_sync"`
	CreatedAt           time.Time `json:"created_at"`
}

// SyncHistory tracks broker sync operations for auditing.
type SyncHistory struct {
	ID              int64      `json:"id"`
	ConnectionID    int64      `json:"connection_id"`
	SyncType        string     `json:"sync_type"` // "positions", "transactions", "full"
	Status          string     `json:"status"`    // "started", "success", "error"
	AccountsSynced  int        `json:"accounts_synced"`
	PositionsSynced int        `json:"positions_synced"`
	ErrorMessage    string     `json:"error_message,omitempty"`
	StartedAt       time.Time  `json:"started_at"`
	CompletedAt     *time.Time `json:"completed_at,omitempty"`
	DurationMs      int64      `json:"duration_ms,omitempty"`
}
