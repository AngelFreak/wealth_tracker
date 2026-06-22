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
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	CategoryID  *int64 `json:"category_id,omitempty"`
	Name        string `json:"name"`
	Currency    string `json:"currency"`
	IsLiability bool   `json:"is_liability"`
	IsActive    bool   `json:"is_active"`
	Notes       string `json:"notes,omitempty"`
	// ManagedByLoanID, when set, means this account's balance is derived
	// from a loan and must not be edited by hand. The Accounts UI hides
	// its balance/edit/delete controls and shows a "from loan" badge.
	ManagedByLoanID *int64    `json:"managed_by_loan_id,omitempty"`
	Balance         float64   `json:"balance"` // Calculated from transactions
	CreatedAt       time.Time `json:"created_at"`
}

// IsManaged reports whether this account is auto-managed by a loan.
func (a *Account) IsManaged() bool { return a.ManagedByLoanID != nil }

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
	BrokerType     string     `json:"broker_type"`  // "nordnet", "saxo", etc.
	Username       string     `json:"username"`     // MitID user identifier (Nordnet) or empty (Saxo)
	CPR            string     `json:"-"`            // CPR number for Signicat verification (never expose in JSON)
	Country        string     `json:"country"`      // "dk", "se", "no", "fi"
	AppKey         string     `json:"-"`            // Saxo App Key (client_id) - never expose in JSON
	AppSecret      string     `json:"-"`            // Saxo App Secret (client_secret) - for non-PKCE flow, never expose
	RedirectURI    string     `json:"redirect_uri"` // Saxo OAuth redirect URI (registered in developer portal)
	IsActive       bool       `json:"is_active"`
	LastSyncAt     *time.Time `json:"last_sync_at,omitempty"`
	LastSyncStatus string     `json:"last_sync_status,omitempty"` // "success", "error", "auth_failed"
	LastSyncError  string     `json:"last_sync_error,omitempty"`
	// Saxo OAuth2 token storage (encrypted)
	RefreshTokenEncrypted string     `json:"-"`                            // Encrypted refresh token (never expose)
	TokenExpiresAt        *time.Time `json:"token_expires_at,omitempty"`   // Access token expiry
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

// AllocationTarget represents a user-defined portfolio allocation target.
// Used by the Portfolio Analyzer to compare actual vs desired allocations.
type AllocationTarget struct {
	ID         int64     `json:"id"`
	UserID     int64     `json:"user_id"`
	TargetType string    `json:"target_type"` // "category", "asset_type", "currency"
	TargetKey  string    `json:"target_key"`  // Category ID, asset type name, or currency code
	TargetPct  float64   `json:"target_pct"`  // Target percentage (0-100)
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

// Target type constants for AllocationTarget
const (
	TargetTypeCategory  = "category"
	TargetTypeAssetType = "asset_type"
	TargetTypeCurrency  = "currency"
)

// Loan type constants.
const (
	LoanTypeOwed  = "owed"  // The user owes this loan (mortgage, car loan).
	LoanTypeLent  = "lent"  // Money the user lent to others (owed to the user).
	LoanTypeSplit = "split" // A co-owned loan shared between participants.
)

// Loan payment type constants. Payments reduce the outstanding principal;
// a withdrawal (a negative amount) increases it (borrowing more). The
// type is informational, used for display/grouping.
const (
	PaymentTypeDownPayment = "down_payment" // Initial lump-sum at the start.
	PaymentTypeRegular     = "regular"      // Ordinary monthly payment.
	PaymentTypeExtra       = "extra"        // Extra savings put toward the loan.
	PaymentTypeWithdrawal  = "withdrawal"   // Borrowed more / drawn from the account.
)

// Loan payment source constants.
const (
	PaymentSourceManual = "manual" // Entered by hand.
	PaymentSourceImport = "import" // Imported from a bank CSV.
)

// Loan represents a loan the user owes, has lent out, or co-owns (split).
//
// For split loans (a co-owned apartment, say) the loan has multiple
// participants each with an ownership percentage. The outstanding
// principal is derived from the payments — never stored — so it cannot
// drift from the payment history.
type Loan struct {
	ID            int64      `json:"id"`
	UserID        int64      `json:"user_id"`
	Name          string     `json:"name"`
	LoanType      string     `json:"loan_type"`      // owed | lent | split
	Principal     float64    `json:"principal"`      // Original loan amount.
	PropertyValue float64    `json:"property_value"` // Value of the asset the loan financed (0 if none).
	Currency      string     `json:"currency"`
	InterestRate  float64    `json:"interest_rate,omitempty"` // Annual %, informational.
	StartDate     *time.Time `json:"start_date,omitempty"`
	IsActive      bool       `json:"is_active"`
	Notes         string     `json:"notes,omitempty"`
	// CategoryID, when set, surfaces the loan in the Accounts view under
	// that category as two auto-managed accounts: a property asset
	// (AssetAccountID) and a loan liability (LiabilityAccountID).
	CategoryID         *int64    `json:"category_id,omitempty"`
	AssetAccountID     *int64    `json:"asset_account_id,omitempty"`
	LiabilityAccountID *int64    `json:"liability_account_id,omitempty"`
	CreatedAt          time.Time `json:"created_at"`
}

// LoanParticipant is one party to a loan. For a simple loan there is a
// single participant at 100%. Exactly one participant per loan has
// IsSelf set — that participant represents the user and is the one whose
// equity flows into the user's net worth.
type LoanParticipant struct {
	ID           int64     `json:"id"`
	LoanID       int64     `json:"loan_id"`
	Name         string    `json:"name"`
	OwnershipPct float64   `json:"ownership_pct"` // 0–100; sums to 100 across a loan.
	IsSelf       bool      `json:"is_self"`
	CreatedAt    time.Time `json:"created_at"`
}

// LoanPayment is a single movement on a loan by a participant. A
// positive amount is a payment (reduces the outstanding principal); a
// negative amount is a withdrawal / extra borrowing (increases it).
type LoanPayment struct {
	ID            int64     `json:"id"`
	LoanID        int64     `json:"loan_id"`
	ParticipantID int64     `json:"participant_id"`
	Amount        float64   `json:"amount"`
	PaymentType   string    `json:"payment_type"` // down_payment | regular | extra | withdrawal
	PaymentDate   time.Time `json:"payment_date"`
	Description   string    `json:"description,omitempty"`
	ImportHash    string    `json:"-"`      // dedup key for imported rows (empty for manual)
	Source        string    `json:"source"` // manual | import
	CreatedAt     time.Time `json:"created_at"`
}

// IsWithdrawal reports whether this entry increases the loan (money
// borrowed / drawn) rather than paying it down.
func (p *LoanPayment) IsWithdrawal() bool { return p.Amount < 0 }

// LoanImportRule attributes an imported posting to a participant when the
// posting's description contains MatchText (case-insensitive).
type LoanImportRule struct {
	ID            int64     `json:"id"`
	LoanID        int64     `json:"loan_id"`
	MatchText     string    `json:"match_text"`
	ParticipantID int64     `json:"participant_id"`
	CreatedAt     time.Time `json:"created_at"`
}

// ParticipantSummary holds the computed settlement figures for one
// participant of a loan.
type ParticipantSummary struct {
	Participant *LoanParticipant `json:"participant"`
	Contributed float64          `json:"contributed"` // Total this participant has paid.
	FairShare   float64          `json:"fair_share"`  // ownership_pct × total paid down.
	// Balance is contributed − fair_share. Positive means this participant
	// has overpaid relative to their share and is owed money by the others;
	// negative means they have underpaid and owe the others.
	Balance float64 `json:"balance"`
}

// LoanSummary is the fully-computed view of a loan: outstanding
// principal, per-participant settlement figures, and the self
// participant's contribution to the user's net worth.
type LoanSummary struct {
	Loan         *Loan                `json:"loan"`
	TotalPaid    float64              `json:"total_paid"` // Sum of all payments (principal paid down).
	Remaining    float64              `json:"remaining"`  // Principal − TotalPaid (never below 0).
	Participants []ParticipantSummary `json:"participants"`

	// Net-worth contribution of the self participant for this loan:
	//   SelfAsset      = self.ownership × property_value
	//   SelfLoanShare  = self.ownership × remaining principal (a liability)
	//   SelfReceivable = self.balance (positive = others owe self,
	//                    negative = self owes others)
	SelfAsset      float64 `json:"self_asset"`
	SelfLoanShare  float64 `json:"self_loan_share"`
	SelfReceivable float64 `json:"self_receivable"`
}

// SelfNetWorth returns this loan's net contribution to the user's net
// worth: their share of the asset, minus their share of the remaining
// loan, plus what they're owed (or minus what they owe) by co-owners.
func (s *LoanSummary) SelfNetWorth() float64 {
	return s.SelfAsset - s.SelfLoanShare + s.SelfReceivable
}

// ProgressPct returns how much of the loan has been paid down, 0–100.
func (s *LoanSummary) ProgressPct() float64 {
	if s.Loan == nil || s.Loan.Principal <= 0 {
		return 0
	}
	pct := (s.TotalPaid / s.Loan.Principal) * 100
	if pct > 100 {
		pct = 100
	}
	if pct < 0 {
		pct = 0
	}
	return pct
}

// IsOwed reports whether this participant is owed money by the others
// (they have overpaid their fair share).
func (p ParticipantSummary) IsOwed() bool { return p.Balance > 0.009 }

// Owes reports whether this participant owes the others (they have
// underpaid their fair share).
func (p ParticipantSummary) Owes() bool { return p.Balance < -0.009 }

// AbsBalance returns the magnitude of the participant's balance, for
// display alongside an "owes" / "is owed" label.
func (p ParticipantSummary) AbsBalance() float64 {
	if p.Balance < 0 {
		return -p.Balance
	}
	return p.Balance
}
