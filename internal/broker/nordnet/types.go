// Package nordnet provides a client for the Nordnet broker API.
package nordnet

import (
	"encoding/json"
	"net/http"
	"time"
)

// FlexibleFloat handles JSON fields that can be either a number or an object.
// Nordnet API sometimes returns objects like {"value": 123.45} instead of plain numbers.
type FlexibleFloat float64

// UnmarshalJSON implements custom unmarshaling for FlexibleFloat.
func (f *FlexibleFloat) UnmarshalJSON(data []byte) error {
	// Try to unmarshal as a plain number first
	var num float64
	if err := json.Unmarshal(data, &num); err == nil {
		*f = FlexibleFloat(num)
		return nil
	}

	// Try to unmarshal as an object with a "value" field
	var obj struct {
		Value float64 `json:"value"`
	}
	if err := json.Unmarshal(data, &obj); err == nil {
		*f = FlexibleFloat(obj.Value)
		return nil
	}

	// If both fail, default to 0
	*f = 0
	return nil
}

// Session represents an authenticated Nordnet session.
type Session struct {
	// Legacy auth (username/password)
	Cookies   []*http.Cookie
	XSRFToken string

	// MitID auth (JWT-based)
	JWT    string // Bearer token for API calls
	NTag   string // Session tag header
	Domain string // Nordnet domain (e.g., www.nordnet.dk)

	ExpiresAt time.Time
}

// IsExpired returns true if the session has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// IsMitIDSession returns true if this is a MitID-authenticated session.
func (s *Session) IsMitIDSession() bool {
	return s.JWT != ""
}

// Account represents a Nordnet account.
type Account struct {
	AccID       json.Number `json:"accid"`
	AccNO       json.Number `json:"accno"`
	Type        string      `json:"type"`
	Default     bool        `json:"default"`
	Alias       string      `json:"alias"`
	IsBlocked   bool        `json:"is_blocked"`
	TotalValue  float64     `json:"total_value,omitempty"`
	Currency    string      `json:"currency,omitempty"`
}

// AccountInfo contains detailed account information.
type AccountInfo struct {
	AccID            string  `json:"accid"`
	AccNO            string  `json:"accno"`
	Type             string  `json:"type"`
	Alias            string  `json:"alias"`
	AccountValue     float64 `json:"account_value"`
	OwnCapital       float64 `json:"own_capital"`
	TradingPower     float64 `json:"trading_power"`
	AccountCurrency  string  `json:"account_currency"`
}

// Instrument represents instrument details from the Nordnet API.
type Instrument struct {
	InstrumentID        int64  `json:"instrument_id"`
	IsinCode            string `json:"isin_code"`
	Symbol              string `json:"symbol"`
	Name                string `json:"name"`
	Currency            string `json:"currency"`
	InstrumentType      string `json:"instrument_type"`
	InstrumentGroupType string `json:"instrument_group_type"`
}

// Amount represents a value with currency from the Nordnet API.
type Amount struct {
	Value    float64 `json:"value"`
	Currency string  `json:"currency"`
}

// Position represents a position/holding in an account.
// The Nordnet API returns positions with nested instrument and amount objects.
type Position struct {
	AccNo          int64      `json:"accno"`
	Instrument     Instrument `json:"instrument"`
	Qty            float64    `json:"qty"`
	PawnPercent    float64    `json:"pawn_percent"`
	MarketValueAcc Amount     `json:"market_value_acc"`
	MarketValue    Amount     `json:"market_value"`
	AcqPriceAcc    Amount     `json:"acq_price_acc"`
	AcqPrice       Amount     `json:"acq_price"`
	MorningPrice   Amount     `json:"morning_price"`
}

// Helper methods to access nested fields for backwards compatibility
func (p *Position) InstrumentID() int64     { return p.Instrument.InstrumentID }
func (p *Position) ISIN() string            { return p.Instrument.IsinCode }
func (p *Position) Name() string            { return p.Instrument.Name }
func (p *Position) Symbol() string          { return p.Instrument.Symbol }
func (p *Position) Currency() string        { return p.Instrument.Currency }
func (p *Position) InstrumentType() string  { return p.Instrument.InstrumentType }
func (p *Position) Quantity() float64       { return p.Qty }
func (p *Position) MarketValueAccValue() float64 { return p.MarketValueAcc.Value }
func (p *Position) AcqPriceValue() float64  { return p.AcqPrice.Value }
func (p *Position) MarketPriceValue() float64 { return p.MarketValue.Value / p.Qty } // Derive price from value

// Trade represents a completed trade.
type Trade struct {
	TradeID         string    `json:"trade_id"`
	InstrumentID    int64     `json:"instrument_id"`
	ISIN            string    `json:"isin"`
	Name            string    `json:"instrument_name"`
	Side            string    `json:"side"` // "BUY" or "SELL"
	Price           float64   `json:"price"`
	Volume          float64   `json:"volume"`
	Amount          float64   `json:"amount"`
	Commission      float64   `json:"commission"`
	Currency        string    `json:"currency"`
	TradedAt        time.Time `json:"traded_at"`
}

// LedgerResponse represents the full ledger API response.
type LedgerResponse struct {
	Total   CurrencyValue `json:"total"`
	Ledgers []Ledger      `json:"ledgers"`
}

// CurrencyValue represents a value with currency from the Nordnet API.
type CurrencyValue struct {
	Currency string  `json:"currency"`
	Value    float64 `json:"value"`
}

// Ledger represents a currency ledger entry.
type Ledger struct {
	Currency      string        `json:"currency"`
	AccountSum    CurrencyValue `json:"account_sum"`
	AccountSumAcc CurrencyValue `json:"account_sum_acc"`
}

// LoginResponse represents the response from the login endpoint.
type LoginResponse struct {
	SessionKey string `json:"session_key"`
	Country    string `json:"country"`
	UserID     int64  `json:"user_id"`
}

// ErrorResponse represents an API error response.
type ErrorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}
