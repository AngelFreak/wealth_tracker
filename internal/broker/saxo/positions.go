package saxo

import (
	"log"
	"math"
)

// PositionWithDetails combines a position with its instrument details.
type PositionWithDetails struct {
	Position   Position
	Instrument *InstrumentDetails
}

// GetPositionsWithDetails fetches positions and enriches them with instrument details.
func (c *Client) GetPositionsWithDetails(session *Session, accountKey string) ([]PositionWithDetails, error) {
	// Fetch positions
	positions, err := c.GetPositions(session, accountKey)
	if err != nil {
		return nil, err
	}

	if len(positions) == 0 {
		return nil, nil
	}

	// Collect unique UICs and their asset types for instrument lookup
	uicMap := make(map[int64]string) // UIC -> AssetType
	for _, pos := range positions {
		uicMap[pos.PositionBase.Uic] = pos.PositionBase.AssetType
	}

	uics := make([]int64, 0, len(uicMap))
	assetTypes := make([]string, 0)
	assetTypeSeen := make(map[string]bool)
	for uic, assetType := range uicMap {
		uics = append(uics, uic)
		if !assetTypeSeen[assetType] {
			assetTypes = append(assetTypes, assetType)
			assetTypeSeen[assetType] = true
		}
	}

	// Fetch instrument details
	instruments, err := c.GetInstrumentDetails(session, uics, assetTypes)
	if err != nil {
		log.Printf("[Saxo] Warning: failed to fetch instrument details: %v", err)
		// Continue without instrument details
	}

	// Build UIC -> InstrumentDetails map
	instrumentMap := make(map[int64]*InstrumentDetails)
	for i := range instruments {
		instrumentMap[instruments[i].Uic] = &instruments[i]
	}

	// Combine positions with instruments
	result := make([]PositionWithDetails, len(positions))
	for i, pos := range positions {
		result[i] = PositionWithDetails{
			Position:   pos,
			Instrument: instrumentMap[pos.PositionBase.Uic],
		}
	}

	return result, nil
}

// PositionHelpers - convenience methods for Position

// Symbol returns the instrument symbol or a UIC-based identifier.
func (p *PositionWithDetails) Symbol() string {
	if p.Instrument != nil && p.Instrument.Symbol != "" {
		return p.Instrument.Symbol
	}
	// Fallback to UIC-based identifier
	return p.Position.NetPositionID
}

// Name returns the instrument description/name.
func (p *PositionWithDetails) Name() string {
	if p.Instrument != nil && p.Instrument.Description != "" {
		return p.Instrument.Description
	}
	return p.Position.PositionBase.AssetType
}

// Currency returns the position currency.
func (p *PositionWithDetails) Currency() string {
	if p.Instrument != nil && p.Instrument.CurrencyCode != "" {
		return p.Instrument.CurrencyCode
	}
	return p.Position.PositionView.ExposureCurrency
}

// Quantity returns the position amount (can be negative for short positions).
func (p *PositionWithDetails) Quantity() float64 {
	return p.Position.PositionBase.Amount
}

// AbsQuantity returns the absolute quantity.
func (p *PositionWithDetails) AbsQuantity() float64 {
	return math.Abs(p.Position.PositionBase.Amount)
}

// OpenPrice returns the entry price.
func (p *PositionWithDetails) OpenPrice() float64 {
	return p.Position.PositionBase.OpenPrice
}

// CurrentPrice returns the current market price.
func (p *PositionWithDetails) CurrentPrice() float64 {
	return p.Position.PositionView.CurrentPrice
}

// MarketValue returns the current market value in base currency.
// Tries multiple fields since some may be 0 when markets are closed.
func (p *PositionWithDetails) MarketValue() float64 {
	// Try MarketValueInBaseCurrency first (most accurate)
	if p.Position.PositionView.MarketValueInBaseCurrency != 0 {
		return p.Position.PositionView.MarketValueInBaseCurrency
	}
	// Try MarketValue
	if p.Position.PositionView.MarketValue != 0 {
		return p.Position.PositionView.MarketValue
	}
	// Try ExposureInBaseCurrency
	if p.Position.PositionView.ExposureInBaseCurrency != 0 {
		return p.Position.PositionView.ExposureInBaseCurrency
	}
	// Try Exposure
	if p.Position.PositionView.Exposure != 0 {
		return p.Position.PositionView.Exposure
	}
	// Fallback: calculate from quantity × current price
	if p.Position.PositionView.CurrentPrice != 0 {
		return math.Abs(p.Position.PositionBase.Amount) * p.Position.PositionView.CurrentPrice
	}
	return 0
}

// ProfitLoss returns the unrealized P&L in base currency.
func (p *PositionWithDetails) ProfitLoss() float64 {
	return p.Position.PositionView.ProfitLossOnTradeInBaseCurrency
}

// AssetType returns the asset type (Stock, Bond, FxSpot, etc.).
func (p *PositionWithDetails) AssetType() string {
	return p.Position.PositionBase.AssetType
}

// Uic returns the Universal Instrument Code.
func (p *PositionWithDetails) Uic() int64 {
	return p.Position.PositionBase.Uic
}

// IsOpen returns true if the position is open.
func (p *PositionWithDetails) IsOpen() bool {
	return p.Position.PositionBase.Status == "Open"
}

// AccountTotals calculates total values from positions and balance.
type AccountTotals struct {
	PositionsValue float64
	CashBalance    float64
	TotalValue     float64
	Currency       string
}

// CalculateAccountTotals calculates totals from positions and balance.
func CalculateAccountTotals(positions []PositionWithDetails, balance *BalanceResponse) AccountTotals {
	totals := AccountTotals{}

	// Sum position values
	for _, p := range positions {
		totals.PositionsValue += p.MarketValue()
	}

	// Add cash balance
	if balance != nil {
		totals.CashBalance = balance.CashBalance
		totals.Currency = balance.Currency
	}

	totals.TotalValue = totals.PositionsValue + totals.CashBalance

	return totals
}
