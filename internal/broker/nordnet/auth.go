package nordnet

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

var (
	ErrAuthenticationFailed = errors.New("authentication failed")
	ErrSessionExpired       = errors.New("session expired")
	ErrInvalidCredentials   = errors.New("invalid credentials")
)

// baseURLs maps country codes to Nordnet base URLs.
// Note: classic.nordnet.* is deprecated, using www.nordnet.* instead
var baseURLs = map[string]string{
	"dk": "https://www.nordnet.dk",
	"se": "https://www.nordnet.se",
	"no": "https://www.nordnet.no",
	"fi": "https://www.nordnet.fi",
}

// Login authenticates with Nordnet using the 3-step flow:
// 1. GET start page to get initial cookies
// 2. POST anonymous login to get NOW cookie
// 3. POST basic login with credentials
func (c *Client) Login(username, password string) (*Session, error) {
	// Step 1: Get initial cookies from start page
	tuxCookie, err := c.getInitialCookies()
	if err != nil {
		return nil, fmt.Errorf("getting initial cookies: %w", err)
	}

	// Step 2: Anonymous login to get NOW cookie
	nowCookie, err := c.anonymousLogin(tuxCookie)
	if err != nil {
		return nil, fmt.Errorf("anonymous login: %w", err)
	}

	// Step 3: Basic login with credentials
	session, err := c.basicLogin(username, password, tuxCookie, nowCookie)
	if err != nil {
		return nil, fmt.Errorf("basic login: %w", err)
	}

	return session, nil
}

// getInitialCookies fetches the start page to get TUX-COOKIE.
func (c *Client) getInitialCookies() (string, error) {
	startURL := c.baseURL + "/mux/login/start.html?cmpi=start-loggain&state=signin"

	req, err := http.NewRequest("GET", startURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Accept", "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	// Extract TUX-COOKIE from response cookies
	var tuxCookie string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "TUX-COOKIE" {
			tuxCookie = cookie.Value
			break
		}
	}

	if tuxCookie == "" {
		return "", errors.New("TUX-COOKIE not found in response")
	}

	return tuxCookie, nil
}

// anonymousLogin performs the anonymous login step to get the NOW cookie.
func (c *Client) anonymousLogin(tuxCookie string) (string, error) {
	anonURL := c.baseURL + "/api/2/login/anonymous"

	req, err := http.NewRequest("POST", anonURL, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "TUX-COOKIE", Value: tuxCookie})

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("anonymous login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Extract NOW cookie from response
	var nowCookie string
	for _, cookie := range resp.Cookies() {
		if cookie.Name == "NOW" {
			nowCookie = cookie.Value
			break
		}
	}

	if nowCookie == "" {
		return "", errors.New("NOW cookie not found in response")
	}

	return nowCookie, nil
}

// basicLogin performs the final login step with username and password.
func (c *Client) basicLogin(username, password, tuxCookie, nowCookie string) (*Session, error) {
	loginURL := c.baseURL + "/api/2/authentication/basic/login"

	// Prepare form data
	form := url.Values{}
	form.Set("username", username)
	form.Set("password", password)

	req, err := http.NewRequest("POST", loginURL, strings.NewReader(form.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("User-Agent", c.userAgent)
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.AddCookie(&http.Cookie{Name: "TUX-COOKIE", Value: tuxCookie})
	req.AddCookie(&http.Cookie{Name: "NOW", Value: nowCookie})

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, ErrInvalidCredentials
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("login failed with status %d: %s", resp.StatusCode, string(body))
	}

	// Parse response to verify login success
	var loginResp LoginResponse
	if err := json.NewDecoder(resp.Body).Decode(&loginResp); err != nil {
		// Even if response parsing fails, check cookies
	}

	// Collect all session cookies
	session := &Session{
		Cookies:   resp.Cookies(),
		ExpiresAt: time.Now().Add(24 * time.Hour), // Sessions typically last 24h
	}

	// Find XSRF token from cookies
	for _, cookie := range session.Cookies {
		if cookie.Name == "xsrf" || cookie.Name == "XSRF-TOKEN" {
			session.XSRFToken = cookie.Value
			break
		}
	}

	// Also check response headers for XSRF token
	if session.XSRFToken == "" {
		session.XSRFToken = resp.Header.Get("X-XSRF-TOKEN")
	}

	return session, nil
}

// ValidateSession checks if a session is still valid by making a test request.
func (c *Client) ValidateSession(session *Session) bool {
	if session == nil || session.IsExpired() {
		return false
	}

	// Try to fetch accounts as a validation check
	_, err := c.GetAccounts(session)
	return err == nil
}
