package saxo

import (
	"net/http"
	"time"
)

// Session holds OAuth tokens and session state.
type Session struct {
	AccessToken      string
	RefreshToken     string
	TokenType        string         // "Bearer"
	ExpiresAt        time.Time      // access_token expiry
	RefreshExpiresAt time.Time      // refresh_token expiry
	ClientKey        string         // Retrieved from /clients/me
	Cookies          []*http.Cookie // Any cookies from auth flow
}

// IsExpired returns true if the access token has expired.
func (s *Session) IsExpired() bool {
	return time.Now().After(s.ExpiresAt)
}

// NeedsRefresh returns true if the token should be refreshed (5min buffer).
func (s *Session) NeedsRefresh() bool {
	return time.Now().Add(5 * time.Minute).After(s.ExpiresAt)
}

// CanRefresh returns true if the refresh token is still valid.
func (s *Session) CanRefresh() bool {
	return time.Now().Before(s.RefreshExpiresAt)
}

// ClientInfo from /port/v1/clients/me endpoint.
type ClientInfo struct {
	ClientKey        string `json:"ClientKey"`
	ClientID         string `json:"ClientId"`
	Name             string `json:"Name"`
	DefaultAccountID string `json:"DefaultAccountId,omitempty"`
}

// AccountsResponse wraps the account list from /port/v1/accounts.
type AccountsResponse struct {
	Data []Account `json:"Data"`
}

// Account represents a Saxo trading account.
type Account struct {
	AccountGroupKey  string   `json:"AccountGroupKey"`
	AccountID        string   `json:"AccountId"`
	AccountKey       string   `json:"AccountKey"`
	AccountType      string   `json:"AccountType"`
	Active           bool     `json:"Active"`
	ClientID         string   `json:"ClientId"`
	ClientKey        string   `json:"ClientKey"`
	Currency         string   `json:"Currency"`
	CurrencyDecimals int      `json:"CurrencyDecimals"`
	DisplayName      string   `json:"DisplayName,omitempty"`
	IsTrialAccount   bool     `json:"IsTrialAccount,omitempty"`
	LegalAssetTypes  []string `json:"LegalAssetTypes,omitempty"`
}

// PositionsResponse wraps the position list from /port/v1/positions.
type PositionsResponse struct {
	Count int        `json:"__count"`
	Data  []Position `json:"Data"`
}

// Position represents a trading position.
type Position struct {
	NetPositionID string       `json:"NetPositionId"`
	PositionID    string       `json:"PositionId"`
	PositionBase  PositionBase `json:"PositionBase"`
	PositionView  PositionView `json:"PositionView"`
}

// PositionBase contains core position data.
type PositionBase struct {
	AccountID            string   `json:"AccountId"`
	Amount               float64  `json:"Amount"`
	AssetType            string   `json:"AssetType"`
	CanBeClosed          bool     `json:"CanBeClosed"`
	ClientID             string   `json:"ClientId"`
	ExecutionTimeOpen    string   `json:"ExecutionTimeOpen,omitempty"`
	IsForceOpen          bool     `json:"IsForceOpen,omitempty"`
	IsMarketOpen         bool     `json:"IsMarketOpen"`
	LockedByBackOffice   bool     `json:"LockedByBackOffice,omitempty"`
	OpenPrice            float64  `json:"OpenPrice"`
	RelatedOpenOrders    []string `json:"RelatedOpenOrders,omitempty"`
	SourceOrderID        string   `json:"SourceOrderId,omitempty"`
	SpotDate             string   `json:"SpotDate,omitempty"`
	Status               string   `json:"Status"`
	Uic                  int64    `json:"Uic"`
	ValueDate            string   `json:"ValueDate,omitempty"`
	CorrelationKey       string   `json:"CorrelationKey,omitempty"`
	CloseConversionRateSettled bool `json:"CloseConversionRateSettled,omitempty"`
}

// PositionView contains calculated/display values.
type PositionView struct {
	Ask                             float64 `json:"Ask,omitempty"`
	Bid                             float64 `json:"Bid,omitempty"`
	CalculationReliability          string  `json:"CalculationReliability"`
	ConversionRateCurrent           float64 `json:"ConversionRateCurrent"`
	ConversionRateOpen              float64 `json:"ConversionRateOpen"`
	CurrentPrice                    float64 `json:"CurrentPrice"`
	CurrentPriceDelayMinutes        int     `json:"CurrentPriceDelayMinutes"`
	CurrentPriceType                string  `json:"CurrentPriceType"`
	Exposure                        float64 `json:"Exposure"`
	ExposureCurrency                string  `json:"ExposureCurrency"`
	ExposureInBaseCurrency          float64 `json:"ExposureInBaseCurrency"`
	InstrumentPriceDayPercentChange float64 `json:"InstrumentPriceDayPercentChange,omitempty"`
	MarketValue                     float64 `json:"MarketValue,omitempty"`
	MarketValueInBaseCurrency       float64 `json:"MarketValueInBaseCurrency,omitempty"`
	ProfitLossOnTrade               float64 `json:"ProfitLossOnTrade"`
	ProfitLossOnTradeInBaseCurrency float64 `json:"ProfitLossOnTradeInBaseCurrency"`
	TradeCostsTotal                 float64 `json:"TradeCostsTotal,omitempty"`
	TradeCostsTotalInBaseCurrency   float64 `json:"TradeCostsTotalInBaseCurrency,omitempty"`
}

// BalanceResponse from /port/v1/balances endpoint.
type BalanceResponse struct {
	CalculationReliability    string        `json:"CalculationReliability"`
	CashAvailableForTrading   float64       `json:"CashAvailableForTrading"`
	CashBalance               float64       `json:"CashBalance"`
	CashBlocked               float64       `json:"CashBlocked,omitempty"`
	ChangesScheduled          bool          `json:"ChangesScheduled,omitempty"`
	ClosedPositionsCount      int           `json:"ClosedPositionsCount,omitempty"`
	CollateralAvailable       float64       `json:"CollateralAvailable,omitempty"`
	CostToClosePositions      float64       `json:"CostToClosePositions,omitempty"`
	Currency                  string        `json:"Currency"`
	CurrencyDecimals          int           `json:"CurrencyDecimals"`
	InitialMargin             *InitialMargin `json:"InitialMargin,omitempty"`
	MarginAvailableForTrading float64       `json:"MarginAvailableForTrading,omitempty"`
	MarginUsedByCurrentPositions float64    `json:"MarginUsedByCurrentPositions,omitempty"`
	MarginUtilizationPct      float64       `json:"MarginUtilizationPct,omitempty"`
	NetPositionsCount         int           `json:"NetPositionsCount,omitempty"`
	NonMarginPositionsValue   float64       `json:"NonMarginPositionsValue,omitempty"`
	TotalValue                float64       `json:"TotalValue,omitempty"`
	UnrealizedMarginClosedProfitLoss float64 `json:"UnrealizedMarginClosedProfitLoss,omitempty"`
	UnrealizedMarginOpenProfitLoss   float64 `json:"UnrealizedMarginOpenProfitLoss,omitempty"`
	UnrealizedMarginProfitLoss       float64 `json:"UnrealizedMarginProfitLoss,omitempty"`
	UnrealizedPositionsValue         float64 `json:"UnrealizedPositionsValue,omitempty"`
}

// InitialMargin contains margin-related balance info.
type InitialMargin struct {
	CollateralAvailable          float64 `json:"CollateralAvailable,omitempty"`
	MarginAvailable              float64 `json:"MarginAvailable"`
	MarginCollateralNotAvailable float64 `json:"MarginCollateralNotAvailable,omitempty"`
	MarginUsedByCurrentPositions float64 `json:"MarginUsedByCurrentPositions"`
	MarginUtilizationPct         float64 `json:"MarginUtilizationPct,omitempty"`
	NetEquityForMargin           float64 `json:"NetEquityForMargin,omitempty"`
	OtherCollateralDeduction     float64 `json:"OtherCollateralDeduction,omitempty"`
}

// InstrumentDetailsResponse from /ref/v1/instruments/details endpoint.
// Can be either a single instrument or a list.
type InstrumentDetailsResponse struct {
	Data []InstrumentDetails `json:"Data,omitempty"`
}

// InstrumentDetails provides instrument metadata.
type InstrumentDetails struct {
	AssetType       string           `json:"AssetType"`
	CurrencyCode    string           `json:"CurrencyCode"`
	Description     string           `json:"Description"`
	Exchange        *ExchangeDetails `json:"Exchange,omitempty"`
	Identifier      int64            `json:"Identifier,omitempty"` // Same as Uic in search results
	IsTradable      bool             `json:"IsTradable,omitempty"`
	PrimaryListing  int64            `json:"PrimaryListing,omitempty"`
	Symbol          string           `json:"Symbol"`
	TradableAs      []string         `json:"TradableAs,omitempty"`
	Uic             int64            `json:"Uic"`
}

// ExchangeDetails provides exchange information.
type ExchangeDetails struct {
	CountryCode string `json:"CountryCode"`
	ExchangeID  string `json:"ExchangeId"`
	Name        string `json:"Name"`
}

// InstrumentSearchResponse from /ref/v1/instruments endpoint.
type InstrumentSearchResponse struct {
	Data []InstrumentSearchResult `json:"Data"`
}

// InstrumentSearchResult from instrument search.
type InstrumentSearchResult struct {
	AssetType    string   `json:"AssetType"`
	CurrencyCode string   `json:"CurrencyCode"`
	Description  string   `json:"Description"`
	ExchangeID   string   `json:"ExchangeId"`
	Identifier   int64    `json:"Identifier"` // This is the UIC
	Symbol       string   `json:"Symbol"`
	TradableAs   []string `json:"TradableAs,omitempty"`
}

// OAuthTokenResponse from token endpoint.
type OAuthTokenResponse struct {
	AccessToken           string `json:"access_token"`
	ExpiresIn             int    `json:"expires_in"`
	TokenType             string `json:"token_type"`
	RefreshToken          string `json:"refresh_token"`
	RefreshTokenExpiresIn int    `json:"refresh_token_expires_in"`
	Error                 string `json:"error,omitempty"`
	ErrorDescription      string `json:"error_description,omitempty"`
}

// OAuthSession tracks an in-progress OAuth authentication.
type OAuthSession struct {
	ConnectionID int64
	State        string
	Verifier     string    // PKCE code verifier (empty if using client_secret flow)
	AppKey       string    // Saxo App Key (client_id)
	AppSecret    string    // Saxo App Secret (client_secret) - empty for PKCE flow
	RedirectURI  string    // OAuth redirect URI (registered in developer portal)
	StartedAt    time.Time
	Status       string // "pending", "waiting", "exchanging", "complete", "failed"
	AuthURL      string // URL to open in browser
	ErrorMsg     string // Error message if failed
}
