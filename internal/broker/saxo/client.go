package saxo

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// API base URLs
	apiURLProduction = "https://gateway.saxobank.com/openapi"
	apiURLSimulation = "https://gateway.saxobank.com/sim/openapi"

	// Rate limiting
	requestDelay = 200 * time.Millisecond
)

var (
	// Use production API by default
	apiBaseURL = apiURLProduction

	// Track last request time for rate limiting
	lastRequestTime time.Time
)

// Client provides methods for accessing the Saxo OpenAPI.
type Client struct {
	httpClient *http.Client
}

// NewClient creates a new Saxo API client.
func NewClient() *Client {
	return &Client{
		httpClient: &http.Client{
			Timeout: httpClientTimeout,
		},
	}
}

// SetSimulationAPI switches to the simulation API environment.
func SetSimulationAPI(simulation bool) {
	if simulation {
		apiBaseURL = apiURLSimulation
	} else {
		apiBaseURL = apiURLProduction
	}
}

// doRequest performs an authenticated API request.
func (c *Client) doRequest(req *http.Request, session *Session) (*http.Response, error) {
	// Rate limiting
	if elapsed := time.Since(lastRequestTime); elapsed < requestDelay {
		time.Sleep(requestDelay - elapsed)
	}
	lastRequestTime = time.Now()

	// Set required headers
	req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", session.AccessToken))
	req.Header.Set("Accept", "application/json")
	if req.Method == "POST" || req.Method == "PUT" || req.Method == "PATCH" {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	return resp, nil
}

// GetClientInfo retrieves the current user's client information.
// This is needed to get the ClientKey for other API calls.
func (c *Client) GetClientInfo(session *Session) (*ClientInfo, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	url := fmt.Sprintf("%s/port/v1/clients/me", apiBaseURL)
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
		return nil, fmt.Errorf("reading client info response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get client info: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Saxo] Client info response: %s", string(body))

	var clientInfo ClientInfo
	if err := json.Unmarshal(body, &clientInfo); err != nil {
		return nil, fmt.Errorf("decoding client info: %w", err)
	}

	// Store ClientKey in session for future use
	session.ClientKey = clientInfo.ClientKey

	return &clientInfo, nil
}

// GetAccounts retrieves all accounts for the authenticated user.
func (c *Client) GetAccounts(session *Session) ([]Account, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Ensure we have the ClientKey
	if session.ClientKey == "" {
		clientInfo, err := c.GetClientInfo(session)
		if err != nil {
			return nil, fmt.Errorf("getting client key: %w", err)
		}
		session.ClientKey = clientInfo.ClientKey
	}

	url := fmt.Sprintf("%s/port/v1/accounts?ClientKey=%s", apiBaseURL, session.ClientKey)
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
		return nil, fmt.Errorf("reading accounts response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get accounts: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Saxo] Accounts response: %s", string(body))

	var accountsResp AccountsResponse
	if err := json.Unmarshal(body, &accountsResp); err != nil {
		return nil, fmt.Errorf("decoding accounts: %w", err)
	}

	return accountsResp.Data, nil
}

// GetPositions retrieves all positions for a specific account.
func (c *Client) GetPositions(session *Session, accountKey string) ([]Position, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Ensure we have the ClientKey
	if session.ClientKey == "" {
		clientInfo, err := c.GetClientInfo(session)
		if err != nil {
			return nil, fmt.Errorf("getting client key: %w", err)
		}
		session.ClientKey = clientInfo.ClientKey
	}

	url := fmt.Sprintf("%s/port/v1/positions?ClientKey=%s&AccountKey=%s", apiBaseURL, session.ClientKey, accountKey)
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

	log.Printf("[Saxo] Positions response for account %s: %s", accountKey, string(body))

	var positionsResp PositionsResponse
	if err := json.Unmarshal(body, &positionsResp); err != nil {
		return nil, fmt.Errorf("decoding positions: %w", err)
	}

	return positionsResp.Data, nil
}

// GetBalance retrieves the balance for a specific account.
func (c *Client) GetBalance(session *Session, accountKey string) (*BalanceResponse, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Balance endpoint requires both AccountKey and ClientKey
	url := fmt.Sprintf("%s/port/v1/balances?AccountKey=%s&ClientKey=%s", apiBaseURL, accountKey, session.ClientKey)
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
		return nil, fmt.Errorf("reading balance response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get balance: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Saxo] Balance response for account %s: %s", accountKey, string(body))

	var balance BalanceResponse
	if err := json.Unmarshal(body, &balance); err != nil {
		return nil, fmt.Errorf("decoding balance: %w", err)
	}

	return &balance, nil
}

// GetInstrumentDetails retrieves details for one or more instruments by UIC.
func (c *Client) GetInstrumentDetails(session *Session, uics []int64, assetTypes []string) ([]InstrumentDetails, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	if len(uics) == 0 {
		return nil, nil
	}

	// Build comma-separated UIC list
	uicStrs := make([]string, len(uics))
	for i, uic := range uics {
		uicStrs[i] = fmt.Sprintf("%d", uic)
	}

	// Build comma-separated asset type list (if provided)
	assetTypeParam := ""
	if len(assetTypes) > 0 {
		assetTypeParam = fmt.Sprintf("&AssetTypes=%s", strings.Join(assetTypes, ","))
	}

	url := fmt.Sprintf("%s/ref/v1/instruments/details?Uics=%s%s", apiBaseURL, strings.Join(uicStrs, ","), assetTypeParam)
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
		return nil, fmt.Errorf("reading instrument details response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to get instrument details: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Saxo] Instrument details response: %s", string(body))

	var detailsResp InstrumentDetailsResponse
	if err := json.Unmarshal(body, &detailsResp); err != nil {
		return nil, fmt.Errorf("decoding instrument details: %w", err)
	}

	return detailsResp.Data, nil
}

// SearchInstruments searches for instruments by keyword (e.g., ISIN, symbol, name).
func (c *Client) SearchInstruments(session *Session, keywords string, assetTypes []string) ([]InstrumentSearchResult, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	// Build asset type filter if provided
	assetTypeParam := ""
	if len(assetTypes) > 0 {
		assetTypeParam = fmt.Sprintf("&AssetTypes=%s", strings.Join(assetTypes, ","))
	}

	url := fmt.Sprintf("%s/ref/v1/instruments?Keywords=%s%s", apiBaseURL, keywords, assetTypeParam)
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
		return nil, fmt.Errorf("reading instrument search response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("failed to search instruments: status %d, body: %s", resp.StatusCode, string(body))
	}

	log.Printf("[Saxo] Instrument search response for '%s': %s", keywords, string(body))

	var searchResp InstrumentSearchResponse
	if err := json.Unmarshal(body, &searchResp); err != nil {
		return nil, fmt.Errorf("decoding instrument search: %w", err)
	}

	return searchResp.Data, nil
}
