package nordnet

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"sync"
	"time"
)

const (
	defaultUserAgent   = "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36"
	defaultTimeout     = 30 * time.Second
	rateLimitWait      = 10 * time.Second
	minRequestInterval = 1 * time.Second
)

// Client is an HTTP client for the Nordnet API.
type Client struct {
	httpClient  *http.Client
	baseURL     string
	country     string
	userAgent   string

	// Rate limiting
	mu          sync.Mutex
	lastRequest time.Time
}

// NewClient creates a new Nordnet API client for the specified country.
// Valid countries: dk, se, no, fi
func NewClient(country string) (*Client, error) {
	baseURL, ok := baseURLs[country]
	if !ok {
		return nil, fmt.Errorf("unsupported country: %s (valid: dk, se, no, fi)", country)
	}

	jar, err := cookiejar.New(nil)
	if err != nil {
		return nil, fmt.Errorf("creating cookie jar: %w", err)
	}

	return &Client{
		httpClient: &http.Client{
			Timeout: defaultTimeout,
			Jar:     jar,
		},
		baseURL:   baseURL,
		country:   country,
		userAgent: defaultUserAgent,
	}, nil
}

// Country returns the country code for this client.
func (c *Client) Country() string {
	return c.country
}

// doRequest executes an HTTP request with rate limiting and error handling.
func (c *Client) doRequest(req *http.Request, session *Session) (*http.Response, error) {
	// Rate limiting
	c.mu.Lock()
	elapsed := time.Since(c.lastRequest)
	if elapsed < minRequestInterval {
		time.Sleep(minRequestInterval - elapsed)
	}
	c.lastRequest = time.Now()
	c.mu.Unlock()

	// Add common headers
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "application/json")

	// Add session authentication
	if session != nil {
		// Always add cookies if present (both MitID and legacy use them)
		for _, cookie := range session.Cookies {
			req.AddCookie(cookie)
		}

		if session.IsMitIDSession() {
			// MitID authentication - use ntag header (cookies are also needed)
			req.Header.Set("ntag", session.NTag)
			req.Header.Set("client-id", "NEXT")
			req.Header.Set("x-locale", "da-DK")
			// Note: JWT is short-lived (30s), so we rely on cookies + ntag for API calls
		} else if session.XSRFToken != "" {
			// Legacy cookie-based authentication with XSRF token
			req.Header.Set("X-XSRF-TOKEN", session.XSRFToken)
		}
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}

	// Handle rate limiting (429 Too Many Requests)
	if resp.StatusCode == http.StatusTooManyRequests {
		resp.Body.Close()
		time.Sleep(rateLimitWait)
		return c.doRequest(req, session) // Retry once
	}

	return resp, nil
}

// GetAccounts fetches all accounts for the authenticated user.
func (c *Client) GetAccounts(session *Session) ([]Account, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	req, err := http.NewRequest("GET", c.baseURL+"/api/2/accounts", nil)
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
		return nil, fmt.Errorf("failed to get accounts: status %d, body: %s", resp.StatusCode, string(body))
	}

	var accounts []Account
	if err := json.NewDecoder(resp.Body).Decode(&accounts); err != nil {
		return nil, fmt.Errorf("decoding accounts: %w", err)
	}

	return accounts, nil
}

// GetAccountInfo fetches detailed information for a specific account.
func (c *Client) GetAccountInfo(session *Session, accountID string) (*AccountInfo, error) {
	if session == nil || session.IsExpired() {
		return nil, ErrSessionExpired
	}

	url := fmt.Sprintf("%s/api/2/accounts/%s/info", c.baseURL, accountID)
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
		return nil, fmt.Errorf("failed to get account info: status %d, body: %s", resp.StatusCode, string(body))
	}

	var info AccountInfo
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return nil, fmt.Errorf("decoding account info: %w", err)
	}

	return &info, nil
}
