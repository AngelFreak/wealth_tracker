package nordnet

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
)

// GetPositions fetches all positions/holdings for a specific account.
func (c *Client) GetPositions(session *Session, accountID string) ([]Position, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	url := fmt.Sprintf("%s/api/2/accounts/%s/positions", c.baseURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, session)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionExpired
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading positions response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get positions: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Nordnet] Raw positions response for account %s: %s", accountID, string(body))

	var positions []Position
	if err := json.Unmarshal(body, &positions); err != nil {
		return nil, fmt.Errorf("decoding positions: %w", err)
	}

	// Log parsed positions
	for i, p := range positions {
		log.Printf("[Nordnet] Position %d: InstrumentID=%d, ISIN=%s, Name=%s, Symbol=%s, Qty=%.2f, MarketValueAcc=%.2f",
			i, p.InstrumentID(), p.ISIN(), p.Name(), p.Symbol(), p.Quantity(), p.MarketValueAccValue())
	}

	return positions, nil
}

// GetTrades fetches recent trades for a specific account (up to 7 days).
func (c *Client) GetTrades(session *Session, accountID string) ([]Trade, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	url := fmt.Sprintf("%s/api/2/accounts/%s/trades", c.baseURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, session)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionExpired
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("failed to get trades: status %d, body: %s", resp.StatusCode, string(body))
	}

	var trades []Trade
	if err := json.NewDecoder(resp.Body).Decode(&trades); err != nil {
		return nil, fmt.Errorf("decoding trades: %w", err)
	}

	return trades, nil
}

// GetLedgers fetches currency ledgers for a specific account.
func (c *Client) GetLedgers(session *Session, accountID string) ([]Ledger, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	url := fmt.Sprintf("%s/api/2/accounts/%s/ledgers", c.baseURL, accountID)
	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}

	resp, err := c.doRequest(req, session)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized {
		return nil, ErrSessionExpired
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading ledger response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get ledgers: status %d, body: %s", resp.StatusCode, string(body))
	}

	// Parse the wrapped ledger response
	var ledgerResp LedgerResponse
	if err := json.Unmarshal(body, &ledgerResp); err != nil {
		return nil, fmt.Errorf("decoding ledgers: %w", err)
	}

	log.Printf("[Nordnet] Ledger total: %.2f %s, ledger count: %d",
		ledgerResp.Total.Value, ledgerResp.Total.Currency, len(ledgerResp.Ledgers))

	return ledgerResp.Ledgers, nil
}

// CalculateTotalValue calculates the total value of all positions in a given currency.
// If positions have different currencies, it returns the sum in the account's default currency.
func CalculateTotalValue(positions []Position) float64 {
	var total float64
	for _, p := range positions {
		total += p.MarketValueAccValue() // Use account currency value
	}
	return total
}
