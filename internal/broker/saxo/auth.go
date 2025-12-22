package saxo

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

const (
	// OAuth2 endpoints - Production (Danish Saxo Investor)
	authURLProduction  = "https://live.logonvalidation.net"
	authURLSimulation  = "https://sim.logonvalidation.net"
	authorizePath      = "/authorize"
	tokenPath          = "/token"

	// OAuth2 client configuration
	saxoRedirectURI = "http://localhost:33847/callback"

	// OAuth2 scopes (openid is typically sufficient for portfolio access)
	saxoScopes = "openid"

	// Timeouts
	oauthTimeout         = 5 * time.Minute
	oauthSessionTimeout  = 90 * time.Second
	httpClientTimeout    = 30 * time.Second
)

// getSaxoClientID returns the Saxo App Key from environment variable
func getSaxoClientID() string {
	if key := os.Getenv("SAXO_APP_KEY"); key != "" {
		return key
	}
	return "YOUR_SAXO_APP_KEY" // Placeholder - set SAXO_APP_KEY env var
}

var (
	// Active OAuth sessions by connection ID
	activeOAuthSessions      = make(map[int64]*OAuthSession)
	activeOAuthSessionsMutex sync.RWMutex

	// Cached sessions by connection ID
	cachedSessions      = make(map[int64]*Session)
	cachedSessionsMutex sync.RWMutex

	// Use production auth URL by default
	authBaseURL = authURLProduction
)

// SetSimulationMode switches to simulation environment.
func SetSimulationMode(simulation bool) {
	if simulation {
		authBaseURL = authURLSimulation
	} else {
		authBaseURL = authURLProduction
	}
}

// GetActiveOAuthSession returns the active OAuth session for a connection.
func GetActiveOAuthSession(connectionID int64) *OAuthSession {
	activeOAuthSessionsMutex.RLock()
	defer activeOAuthSessionsMutex.RUnlock()
	return activeOAuthSessions[connectionID]
}

// GetOAuthStatus returns the current OAuth status for a connection.
func GetOAuthStatus(connectionID int64) string {
	session := GetActiveOAuthSession(connectionID)
	if session == nil {
		return "none"
	}
	return session.Status
}

// GetOAuthURL returns the OAuth authorization URL for a connection.
func GetOAuthURL(connectionID int64) string {
	session := GetActiveOAuthSession(connectionID)
	if session == nil {
		return ""
	}
	return session.AuthURL
}

// GetCachedSession returns a cached session if still valid.
func GetCachedSession(connectionID int64) *Session {
	cachedSessionsMutex.RLock()
	defer cachedSessionsMutex.RUnlock()
	return cachedSessions[connectionID]
}

// CacheSession stores a session for future use.
func CacheSession(connectionID int64, session *Session) {
	cachedSessionsMutex.Lock()
	defer cachedSessionsMutex.Unlock()
	cachedSessions[connectionID] = session
}

// ClearCachedSession removes a cached session.
func ClearCachedSession(connectionID int64) {
	cachedSessionsMutex.Lock()
	defer cachedSessionsMutex.Unlock()
	delete(cachedSessions, connectionID)
}

// ClearActiveOAuthSession removes an active OAuth session, allowing a new auth to start.
func ClearActiveOAuthSession(connectionID int64) {
	activeOAuthSessionsMutex.Lock()
	defer activeOAuthSessionsMutex.Unlock()
	delete(activeOAuthSessions, connectionID)
}

// generatePKCE creates PKCE code verifier and challenge.
// Returns verifier (43+ chars) and challenge (base64url SHA256 of verifier).
func generatePKCE() (verifier, challenge string, err error) {
	// Generate 32 bytes of random data for verifier (results in 43 base64 chars)
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", fmt.Errorf("generating random bytes: %w", err)
	}

	// Use RawURLEncoding (no padding, URL-safe)
	verifier = base64.RawURLEncoding.EncodeToString(b)

	// Create SHA256 hash of verifier for challenge
	h := sha256.New()
	h.Write([]byte(verifier))
	challenge = base64.RawURLEncoding.EncodeToString(h.Sum(nil))

	return verifier, challenge, nil
}

// generateState creates a random state parameter for CSRF protection.
func generateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(b), nil
}

// openBrowser opens a URL in the default browser.
func openBrowser(urlStr string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", urlStr)
	case "darwin":
		cmd = exec.Command("open", urlStr)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", urlStr)
	default:
		return fmt.Errorf("unsupported platform: %s", runtime.GOOS)
	}
	return cmd.Start()
}

// AuthenticateWithOAuth performs OAuth2 authentication for Saxo.
// Opens a browser for user login and listens for the callback.
// If appSecret is provided, uses standard Authorization Code flow with client_secret.
// If appSecret is empty, uses PKCE flow (for native/public clients).
func AuthenticateWithOAuth(connectionID int64, appKey, appSecret, redirectURI string) (*Session, error) {
	// Check for existing active session
	activeOAuthSessionsMutex.Lock()
	if existing := activeOAuthSessions[connectionID]; existing != nil {
		if time.Since(existing.StartedAt) < oauthSessionTimeout {
			activeOAuthSessionsMutex.Unlock()
			return nil, ErrOAuthInProgress
		}
		// Clean up stale session
		delete(activeOAuthSessions, connectionID)
	}
	activeOAuthSessionsMutex.Unlock()

	// Use provided appKey or fall back to environment variable
	clientID := appKey
	if clientID == "" {
		clientID = getSaxoClientID()
	}

	// Use provided redirectURI or fall back to default
	callbackURI := redirectURI
	if callbackURI == "" {
		callbackURI = saxoRedirectURI
	}

	// Generate state for CSRF protection
	state, err := generateState()
	if err != nil {
		return nil, fmt.Errorf("generating state: %w", err)
	}

	// Determine if we use PKCE or client_secret flow
	usePKCE := appSecret == ""
	var verifier, challenge string

	if usePKCE {
		// Generate PKCE parameters for public clients
		verifier, challenge, err = generatePKCE()
		if err != nil {
			return nil, fmt.Errorf("generating PKCE: %w", err)
		}
		log.Printf("[Saxo OAuth] Using PKCE flow (no client secret)")
	} else {
		log.Printf("[Saxo OAuth] Using Authorization Code flow with client secret")
	}

	// Build authorization URL
	var authURL string
	if usePKCE {
		authURL = fmt.Sprintf("%s%s?"+
			"response_type=code&"+
			"client_id=%s&"+
			"redirect_uri=%s&"+
			"state=%s&"+
			"code_challenge=%s&"+
			"code_challenge_method=S256",
			authBaseURL, authorizePath,
			url.QueryEscape(clientID),
			url.QueryEscape(callbackURI),
			url.QueryEscape(state),
			url.QueryEscape(challenge),
		)
	} else {
		// Standard Authorization Code flow (no PKCE challenge)
		authURL = fmt.Sprintf("%s%s?"+
			"response_type=code&"+
			"client_id=%s&"+
			"redirect_uri=%s&"+
			"state=%s",
			authBaseURL, authorizePath,
			url.QueryEscape(clientID),
			url.QueryEscape(callbackURI),
			url.QueryEscape(state),
		)
	}

	// Track OAuth session
	oauthSession := &OAuthSession{
		ConnectionID: connectionID,
		State:        state,
		Verifier:     verifier, // Empty if not using PKCE
		AppKey:       clientID,
		AppSecret:    appSecret,
		RedirectURI:  callbackURI,
		StartedAt:    time.Now(),
		Status:       "pending",
		AuthURL:      authURL,
	}

	activeOAuthSessionsMutex.Lock()
	activeOAuthSessions[connectionID] = oauthSession
	activeOAuthSessionsMutex.Unlock()

	// Clean up on exit
	defer func() {
		activeOAuthSessionsMutex.Lock()
		delete(activeOAuthSessions, connectionID)
		activeOAuthSessionsMutex.Unlock()
	}()

	// Start local callback server
	codeChan := make(chan string, 1)
	errChan := make(chan error, 1)

	// Parse redirect URI to get port
	redirectURL, err := url.Parse(callbackURI)
	if err != nil {
		return nil, fmt.Errorf("parsing redirect URI: %w", err)
	}
	_, port, _ := net.SplitHostPort(redirectURL.Host)
	if port == "" {
		port = "33847"
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("localhost:%s", port))
	if err != nil {
		return nil, fmt.Errorf("starting callback server on port %s: %w", port, err)
	}
	defer listener.Close()

	// HTTP handler for OAuth callback
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		log.Printf("[Saxo OAuth] Callback received: %s", r.URL.String())
		// Verify state to prevent CSRF
		if r.URL.Query().Get("state") != state {
			log.Printf("[Saxo OAuth] State mismatch: expected %s, got %s", state, r.URL.Query().Get("state"))
			errChan <- ErrInvalidState
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`<!DOCTYPE html><html><head><title>Error</title></head><body>
				<h1>Authentication Failed</h1>
				<p>Security validation failed. Please try again.</p>
				<p>You can close this window.</p>
			</body></html>`))
			return
		}

		// Check for OAuth error
		if errParam := r.URL.Query().Get("error"); errParam != "" {
			errDesc := r.URL.Query().Get("error_description")
			log.Printf("[Saxo OAuth] Error from Saxo: %s - %s", errParam, errDesc)
			errChan <- fmt.Errorf("OAuth error: %s - %s", errParam, errDesc)
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.Write([]byte(fmt.Sprintf(`<!DOCTYPE html><html><head><title>Error</title></head><body>
				<h1>Authentication Failed</h1>
				<p>%s: %s</p>
				<p>You can close this window.</p>
			</body></html>`, errParam, errDesc)))
			return
		}

		// Get authorization code
		code := r.URL.Query().Get("code")
		if code == "" {
			log.Printf("[Saxo OAuth] No authorization code in callback")
			errChan <- ErrNoAuthCode
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			w.WriteHeader(http.StatusBadRequest)
			w.Write([]byte(`<!DOCTYPE html><html><head><title>Error</title></head><body>
				<h1>Authentication Failed</h1>
				<p>No authorization code received.</p>
				<p>You can close this window.</p>
			</body></html>`))
			return
		}

		codeChan <- code
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		w.Write([]byte(`<!DOCTYPE html><html><head><title>Success</title></head><body>
			<h1>Authentication Successful</h1>
			<p>You can close this window and return to the application.</p>
		</body></html>`))
	})

	server := &http.Server{Handler: handler}
	go func() {
		if err := server.Serve(listener); err != nil && err != http.ErrServerClosed {
			log.Printf("[Saxo OAuth] Callback server error: %v", err)
		}
	}()
	defer server.Shutdown(context.Background())

	// Update status and open browser
	oauthSession.Status = "waiting"
	log.Printf("[Saxo OAuth] Opening browser for authentication")
	log.Printf("[Saxo OAuth] Auth URL: %s", authURL)

	if err := openBrowser(authURL); err != nil {
		log.Printf("[Saxo OAuth] Failed to open browser: %v", err)
		log.Printf("[Saxo OAuth] Please open this URL manually: %s", authURL)
	}

	// Wait for callback with timeout
	ctx, cancel := context.WithTimeout(context.Background(), oauthTimeout)
	defer cancel()

	var code string
	select {
	case code = <-codeChan:
		log.Printf("[Saxo OAuth] Received authorization code")
	case err := <-errChan:
		oauthSession.Status = "failed"
		oauthSession.ErrorMsg = err.Error()
		return nil, err
	case <-ctx.Done():
		oauthSession.Status = "failed"
		oauthSession.ErrorMsg = "Timeout waiting for user to complete login"
		return nil, ErrOAuthTimeout
	}

	// Exchange code for tokens
	oauthSession.Status = "exchanging"
	session, err := exchangeCodeForTokens(code, oauthSession.Verifier, clientID, appSecret, callbackURI)
	if err != nil {
		oauthSession.Status = "failed"
		oauthSession.ErrorMsg = err.Error()
		return nil, fmt.Errorf("exchanging code for tokens: %w", err)
	}

	oauthSession.Status = "complete"
	log.Printf("[Saxo OAuth] Successfully authenticated, token expires at %v", session.ExpiresAt)

	// Cache the session
	CacheSession(connectionID, session)

	return session, nil
}

// exchangeCodeForTokens exchanges authorization code for tokens.
// If appSecret is provided, uses client_secret flow. Otherwise uses PKCE with code_verifier.
func exchangeCodeForTokens(code, verifier, appKey, appSecret, redirectURI string) (*Session, error) {
	// Use provided appKey or fall back to environment variable
	clientID := appKey
	if clientID == "" {
		clientID = getSaxoClientID()
	}

	// Use provided redirectURI or fall back to default
	callbackURI := redirectURI
	if callbackURI == "" {
		callbackURI = saxoRedirectURI
	}

	data := url.Values{}
	data.Set("grant_type", "authorization_code")
	data.Set("code", code)
	data.Set("redirect_uri", callbackURI)
	data.Set("client_id", clientID)

	// Use client_secret if available, otherwise use PKCE code_verifier
	if appSecret != "" {
		data.Set("client_secret", appSecret)
		log.Printf("[Saxo OAuth] Token request using client_secret flow")
	} else if verifier != "" {
		data.Set("code_verifier", verifier)
		log.Printf("[Saxo OAuth] Token request using PKCE flow")
	}

	log.Printf("[Saxo OAuth] Token request to: %s", authBaseURL+tokenPath)
	log.Printf("[Saxo OAuth] Token request params: grant_type=authorization_code, client_id=%s, redirect_uri=%s", clientID, callbackURI)
	// Don't log full body to avoid exposing secrets
	log.Printf("[Saxo OAuth] Request includes: code length=%d, secret=%v, verifier=%v", len(code), appSecret != "", verifier != "")

	req, err := http.NewRequest("POST", authBaseURL+tokenPath, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Set("Accept", "application/json")

	client := &http.Client{Timeout: httpClientTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("token request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	log.Printf("[Saxo OAuth] Token response status: %d, body: %s", resp.StatusCode, string(body))

	// Check for non-2xx status
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("token request failed with status %d: %s", resp.StatusCode, string(body))
	}

	var tokenResp OAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding token response: %w (body: %s)", err, string(body))
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("token error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	if tokenResp.AccessToken == "" {
		return nil, fmt.Errorf("no access token in response")
	}

	now := time.Now()
	session := &Session{
		AccessToken:      tokenResp.AccessToken,
		RefreshToken:     tokenResp.RefreshToken,
		TokenType:        tokenResp.TokenType,
		ExpiresAt:        now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second),
		RefreshExpiresAt: now.Add(time.Duration(tokenResp.RefreshTokenExpiresIn) * time.Second),
	}

	return session, nil
}

// RefreshAccessToken refreshes an expired access token using the refresh token.
func RefreshAccessToken(session *Session) (*Session, error) {
	if session == nil || session.RefreshToken == "" {
		return nil, ErrRefreshTokenExpired
	}

	if !session.CanRefresh() {
		return nil, ErrRefreshTokenExpired
	}

	log.Printf("[Saxo OAuth] Refreshing access token")

	data := url.Values{}
	data.Set("grant_type", "refresh_token")
	data.Set("refresh_token", session.RefreshToken)
	data.Set("client_id", getSaxoClientID())

	req, err := http.NewRequest("POST", authBaseURL+tokenPath, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	client := &http.Client{Timeout: httpClientTimeout}
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("refresh request failed: %w", err)
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("reading refresh response: %w", err)
	}

	var tokenResp OAuthTokenResponse
	if err := json.Unmarshal(body, &tokenResp); err != nil {
		return nil, fmt.Errorf("decoding refresh response: %w", err)
	}

	if tokenResp.Error != "" {
		return nil, fmt.Errorf("refresh error: %s - %s", tokenResp.Error, tokenResp.ErrorDescription)
	}

	// Update session with new tokens
	now := time.Now()
	session.AccessToken = tokenResp.AccessToken
	session.ExpiresAt = now.Add(time.Duration(tokenResp.ExpiresIn) * time.Second)

	// If a new refresh token was issued, update it
	if tokenResp.RefreshToken != "" {
		session.RefreshToken = tokenResp.RefreshToken
		session.RefreshExpiresAt = now.Add(time.Duration(tokenResp.RefreshTokenExpiresIn) * time.Second)
	}

	log.Printf("[Saxo OAuth] Token refreshed, expires at %v", session.ExpiresAt)

	return session, nil
}

// GetOrRefreshSession gets a valid session, refreshing if needed.
func GetOrRefreshSession(connectionID int64) (*Session, error) {
	session := GetCachedSession(connectionID)
	if session == nil {
		return nil, ErrSessionExpired
	}

	// If token is still valid (with buffer), return it
	if !session.NeedsRefresh() {
		return session, nil
	}

	// Try to refresh
	if session.CanRefresh() {
		refreshed, err := RefreshAccessToken(session)
		if err != nil {
			log.Printf("[Saxo OAuth] Refresh failed: %v", err)
			ClearCachedSession(connectionID)
			return nil, ErrRefreshTokenExpired
		}
		CacheSession(connectionID, refreshed)
		return refreshed, nil
	}

	// Refresh token expired
	ClearCachedSession(connectionID)
	return nil, ErrRefreshTokenExpired
}
