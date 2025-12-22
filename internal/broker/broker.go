package broker

import (
	"net/http"
	"time"
)

// Session represents a generic broker session.
type Session struct {
	Cookies   []*http.Cookie
	Token     string
	ExpiresAt time.Time
	Data      map[string]string
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// ExternalAccount represents a broker account.
type ExternalAccount struct {
	ID       string
	Name     string
	Type     string
	Currency string
	Value    float64
}

// Position represents a position/holding.
type Position struct {
	ExternalID      string
	Symbol          string // ISIN or ticker
	Name            string
	Quantity        float64
	AcquisitionPrice float64
	CurrentPrice    float64
	CurrentValue    float64
	Currency        string
	InstrumentType  string
}

// Broker defines the interface for broker integrations.
type Broker interface {
	// Login authenticates with the broker and returns a session.
	Login(username, password string) (*Session, error)

	// GetAccounts returns all accounts for the authenticated user.
	GetAccounts(session *Session) ([]ExternalAccount, error)

	// GetPositions returns all positions for a specific account.
	GetPositions(session *Session, accountID string) ([]Position, error)

	// ValidateSession checks if a session is still valid.
	ValidateSession(session *Session) bool

	// Country returns the country code for this broker.
	Country() string
}
