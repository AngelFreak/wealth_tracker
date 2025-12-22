package nordnet

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"wealth_tracker/internal/broker/nordnet/mitid"
)

// Country-specific Nordnet domains
var nordnetDomains = map[string]string{
	"dk": "www.nordnet.dk",
	"se": "www.nordnet.se",
	"no": "www.nordnet.no",
	"fi": "www.nordnet.fi",
}

// Country-specific Signicat client IDs
var signicatClients = map[string]string{
	"dk": "prod.nordnet.dk.8x",
	"se": "prod.nordnet.se.8x",
	"no": "prod.nordnet.no.8x",
	"fi": "prod.nordnet.fi.8x",
}

// ActiveMitIDSessionsNative tracks currently active native MitID auth sessions.
var (
	activeMitIDSessionsNative = make(map[int64]*MitIDSession)
	mitidSessionsNativeMutex  sync.RWMutex
)

// cachedNordnetSessions stores authenticated Nordnet sessions for reuse.
// Key is connectionID, value is the authenticated session.
var (
	cachedNordnetSessions      = make(map[int64]*Session)
	cachedNordnetSessionsMutex sync.RWMutex
)

// GetCachedSession returns a cached session if it exists and is still valid.
// Returns nil if no valid cached session exists.
func GetCachedSession(connectionID int64) *Session {
	cachedNordnetSessionsMutex.RLock()
	defer cachedNordnetSessionsMutex.RUnlock()

	session := cachedNordnetSessions[connectionID]
	if session == nil {
		return nil
	}

	// Check if session is still valid (with 5 minute buffer)
	if time.Now().Add(5 * time.Minute).After(session.ExpiresAt) {
		log.Printf("[Session Cache] Cached session for connection %d has expired or is about to expire", connectionID)
		return nil
	}

	log.Printf("[Session Cache] Using cached session for connection %d (expires in %v)", connectionID, time.Until(session.ExpiresAt))
	return session
}

// CacheSession stores an authenticated session for future reuse.
func CacheSession(connectionID int64, session *Session) {
	if session == nil {
		return
	}

	cachedNordnetSessionsMutex.Lock()
	defer cachedNordnetSessionsMutex.Unlock()

	cachedNordnetSessions[connectionID] = session
	log.Printf("[Session Cache] Cached session for connection %d (expires at %v)", connectionID, session.ExpiresAt)
}

// InvalidateCachedSession removes a cached session (e.g., after auth failure).
func InvalidateCachedSession(connectionID int64) {
	cachedNordnetSessionsMutex.Lock()
	defer cachedNordnetSessionsMutex.Unlock()

	delete(cachedNordnetSessions, connectionID)
	log.Printf("[Session Cache] Invalidated cached session for connection %d", connectionID)
}

// GetActiveMitIDSessionNative returns the active MitID session for a connection (native version).
func GetActiveMitIDSessionNative(connectionID int64) *MitIDSession {
	mitidSessionsNativeMutex.RLock()
	defer mitidSessionsNativeMutex.RUnlock()
	return activeMitIDSessionsNative[connectionID]
}

// GetQRCodePathNative returns the path to the current QR code image (native version).
func GetQRCodePathNative(connectionID int64) (string, error) {
	session := GetActiveMitIDSessionNative(connectionID)
	if session == nil {
		return "", fmt.Errorf("no active MitID session")
	}

	qrManager := mitid.NewQRManager(session.QRDir)
	frame, err := qrManager.GetCurrentFrame()
	if err != nil {
		return "", fmt.Errorf("QR code not ready yet: %w", err)
	}

	return qrManager.GetQRCodePath(frame), nil
}

// GetMitIDStatusNative returns the current status of MitID authentication (native version).
func GetMitIDStatusNative(connectionID int64) string {
	session := GetActiveMitIDSessionNative(connectionID)
	if session == nil {
		return "none"
	}

	qrManager := mitid.NewQRManager(session.QRDir)
	return qrManager.GetStatus()
}

// AuthenticateWithMitIDNative performs MitID authentication using native Go implementation.
// This is a drop-in replacement for AuthenticateWithMitID.
//
// Parameters:
//   - connectionID: ID of the broker connection (for session tracking)
//   - country: Nordnet country code (dk, se, no, fi)
//   - userID: MitID user identifier
//   - cpr: Danish CPR number for Signicat verification (10 digits)
//   - method: Authentication method ("APP" for MitID app)
//   - scriptDir: Unused, kept for API compatibility
//
// Returns a Session on success or an error if authentication fails.
func AuthenticateWithMitIDNative(connectionID int64, country, userID, cpr, method, scriptDir string) (*Session, error) {
	if method == "" {
		method = "APP"
	}

	// Only APP method supported in native implementation for now
	if method != "APP" {
		return nil, fmt.Errorf("native implementation only supports APP method, got %s", method)
	}

	// Check for a cached valid session first - avoids re-authentication
	if cachedSession := GetCachedSession(connectionID); cachedSession != nil {
		return cachedSession, nil
	}

	// Check if there's already an active authentication in progress
	// This prevents double authentication when multiple requests come in
	mitidSessionsNativeMutex.Lock()
	existingSession := activeMitIDSessionsNative[connectionID]
	if existingSession != nil {
		// Only block if it's very recent (within last 90 seconds) - allows retry after timeout
		if time.Since(existingSession.StartedAt) < 90*time.Second {
			mitidSessionsNativeMutex.Unlock()
			log.Printf("[MitID Native] Blocking concurrent auth attempt for connection %d - auth already in progress (started %v ago)",
				connectionID, time.Since(existingSession.StartedAt))
			return nil, fmt.Errorf("authentication already in progress for this connection - please wait for the current request to complete")
		}
		// Stale session (older than 90s), clean it up and allow new auth
		log.Printf("[MitID Native] Cleaning up stale session for connection %d (was started %v ago)", connectionID, time.Since(existingSession.StartedAt))
		delete(activeMitIDSessionsNative, connectionID)
	}
	mitidSessionsNativeMutex.Unlock()

	// Get domain and client ID
	domain := nordnetDomains[country]
	if domain == "" {
		domain = nordnetDomains["dk"]
	}
	clientID := signicatClients[country]
	if clientID == "" {
		clientID = signicatClients["dk"]
	}

	// Create QR output directory
	qrDir := fmt.Sprintf("%s/mitid_qr_%d", os.TempDir(), connectionID)
	os.RemoveAll(qrDir)
	if err := os.MkdirAll(qrDir, 0755); err != nil {
		return nil, fmt.Errorf("creating QR directory: %w", err)
	}

	// Track this session
	mitidSessionsNativeMutex.Lock()
	activeMitIDSessionsNative[connectionID] = &MitIDSession{
		ConnectionID: connectionID,
		QRDir:        qrDir,
		StartedAt:    time.Now(),
	}
	mitidSessionsNativeMutex.Unlock()

	// Clean up session when done
	defer func() {
		mitidSessionsNativeMutex.Lock()
		delete(activeMitIDSessionsNative, connectionID)
		mitidSessionsNativeMutex.Unlock()

		// Clean up QR files in background
		go func() {
			time.Sleep(5 * time.Second)
			os.RemoveAll(qrDir)
		}()
	}()

	// Create HTTP client with cookie jar
	jar, _ := cookiejar.New(nil)
	httpClient := &http.Client{
		Jar:     jar,
		Timeout: 2 * time.Minute,
	}

	// Set initial status
	qrManager := mitid.NewQRManager(qrDir)
	qrManager.SetStatus("initializing")

	log.Printf("[MitID Native] Starting authentication for connection %d, user %s", connectionID, userID)

	// Step 1: Initiate Signicat OIDC flow
	loginURL := fmt.Sprintf(
		"https://id.signicat.com/oidc/authorize?"+
			"client_id=%s&"+
			"response_type=code&"+
			"redirect_uri=https://%s/login&"+
			"scope=openid%%20signicat.national_id&"+
			"acr_values=urn:signicat:oidc:method:mitid-cpr&"+
			"state=NEXT_OIDC_STATE_%d",
		clientID, domain, time.Now().UnixNano())

	log.Printf("[MitID Native] Step 1: Initiating Signicat OIDC flow")
	resp, err := httpClient.Get(loginURL)
	if err != nil {
		log.Printf("[MitID Native] Step 1 FAILED: %v", err)
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("initiating login: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		qrManager.SetStatus("failed")
		body, _ := io.ReadAll(resp.Body)
		log.Printf("[MitID Native] Step 1 FAILED: status %d", resp.StatusCode)
		return nil, fmt.Errorf("login initiation failed (status %d): %s", resp.StatusCode, string(body))
	}
	log.Printf("[MitID Native] Step 1: Got response, parsing HTML")

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("reading response: %w", err)
	}

	// Parse HTML to get next URL
	indexURL := extractDataAttribute(string(body), "data-index-url")
	if indexURL == "" {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 1 FAILED: could not find data-index-url")
		return nil, fmt.Errorf("could not find data-index-url in response")
	}
	log.Printf("[MitID Native] Step 1: Found indexURL: %s", indexURL)

	// Fetch index page
	log.Printf("[MitID Native] Step 2: Fetching index page")
	resp, err = httpClient.Get(indexURL)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("fetching index: %w", err)
	}
	defer resp.Body.Close()

	body, _ = io.ReadAll(resp.Body)

	// Extract paths from HTML
	baseURL := extractDataAttribute(string(body), "data-base-url")
	initAuthPath := extractDataAttribute(string(body), "data-init-auth-path")
	authCodePath := extractDataAttribute(string(body), "data-auth-code-path")
	finalizeAuthPath := extractDataAttribute(string(body), "data-finalize-auth-path")

	if baseURL == "" || initAuthPath == "" {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 2 FAILED: baseURL=%s, initAuthPath=%s", baseURL, initAuthPath)
		return nil, fmt.Errorf("could not find required paths in HTML")
	}
	log.Printf("[MitID Native] Step 2: Found paths - base=%s, initAuth=%s, authCode=%s, finalize=%s",
		baseURL, initAuthPath, authCodePath, finalizeAuthPath)

	// Step 3: Initialize MitID authentication
	log.Printf("[MitID Native] Step 3: Initializing MitID auth")
	resp, err = httpClient.Post(baseURL+initAuthPath, "application/json", nil)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("init MitID auth: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		qrManager.SetStatus("failed")
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("init MitID auth failed (status %d): %s", resp.StatusCode, string(body))
	}

	var initResp struct {
		Aux string `json:"aux"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("decoding init response: %w", err)
	}

	// Decode aux from base64
	auxBytes, err := base64.StdEncoding.DecodeString(initResp.Aux)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("decoding aux: %w", err)
	}

	var auxData map[string]interface{}
	if err := json.Unmarshal(auxBytes, &auxData); err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("parsing aux: %w", err)
	}

	// Extract MitID parameters
	clientHash, authSessionID, err := mitid.ExtractAuxParameters(auxData)
	if err != nil {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 3 FAILED: extracting aux: %v", err)
		return nil, fmt.Errorf("extracting aux parameters: %w", err)
	}
	log.Printf("[MitID Native] Step 3: Got authSessionID=%s", authSessionID)

	// Step 4: Perform MitID authentication
	log.Printf("[MitID Native] Step 4: Creating MitID client")
	mitidClient, err := mitid.NewClient(clientHash, authSessionID, httpClient, qrDir)
	if err != nil {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 4 FAILED: creating client: %v", err)
		return nil, fmt.Errorf("creating MitID client: %w", err)
	}
	log.Printf("[MitID Native] Step 4: MitID client created, service: %s", mitidClient.GetServiceProviderName())

	// Identify user and get available authenticators
	log.Printf("[MitID Native] Step 5: Identifying user %s", userID)
	availableAuth, err := mitidClient.IdentifyUser(userID)
	if err != nil {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 5 FAILED: identifying user: %v", err)
		return nil, fmt.Errorf("%w: %v", ErrMitIDFailed, err)
	}
	log.Printf("[MitID Native] Step 5: User identified, available auth: %v", availableAuth)

	// Check if APP is available
	if _, ok := availableAuth["APP"]; !ok {
		qrManager.SetStatus("failed")
		log.Printf("[MitID Native] Step 5 FAILED: APP not available")
		return nil, fmt.Errorf("%w: APP authentication not available", mitid.ErrAuthenticatorNotAvailable)
	}

	// Authenticate with app
	log.Printf("[MitID Native] Step 6: Starting APP authentication")
	if err := mitidClient.AuthenticateWithApp(); err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("%w: %v", ErrMitIDFailed, err)
	}

	// Get authorization code
	authCode, err := mitidClient.FinalizeAndGetAuthCode()
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("%w: %v", ErrMitIDFailed, err)
	}

	// Step 4: Submit authorization code to Nordnet (multipart form)
	log.Printf("[MitID Native] Step 4: Submitting auth code to %s", baseURL+authCodePath)
	var formBody strings.Builder
	boundary := fmt.Sprintf("----------------------------%d", time.Now().UnixNano())

	formBody.WriteString("--" + boundary + "\r\n")
	formBody.WriteString("Content-Disposition: form-data; name=\"authCode\"\r\n\r\n")
	formBody.WriteString(authCode + "\r\n")
	formBody.WriteString("--" + boundary + "--\r\n")

	req, _ := http.NewRequest(http.MethodPost, baseURL+authCodePath, strings.NewReader(formBody.String()))
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err = httpClient.Do(req)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("submitting auth code: %w", err)
	}
	authCodeRespBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()
	log.Printf("[MitID Native] Step 4: Auth code response (status %d): %s", resp.StatusCode, string(authCodeRespBody))

	// Finalize auth - this may return a CPR form page
	log.Printf("[MitID Native] Step 5: Finalizing auth at %s", baseURL+finalizeAuthPath)
	resp, err = httpClient.Get(baseURL + finalizeAuthPath)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("finalizing auth: %w", err)
	}
	finalizeRespBody, _ := io.ReadAll(resp.Body)
	resp.Body.Close()

	// Check if we got a CPR form page
	finalURL := resp.Request.URL.String()
	log.Printf("[MitID Native] Step 5: Final URL after redirect: %s", finalURL)
	log.Printf("[MitID Native] Step 5: Finalize response length: %d bytes", len(finalizeRespBody))
	log.Printf("[MitID Native] Step 5: Contains 'cpr-form': %v, URL contains '/cpr': %v",
		strings.Contains(string(finalizeRespBody), "cpr-form"),
		strings.Contains(finalURL, "/cpr"))

	var signicatCode string
	if strings.Contains(string(finalizeRespBody), "cpr-form") || strings.Contains(finalURL, "/cpr") {
		log.Printf("[MitID Native] Step 5a: CPR verification required")

		// CPR is required - verify it
		if cpr == "" {
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("CPR number is required for Signicat verification but not provided")
		}

		// Extract CPR paths and CSRF token from the page
		cprVerifyPath := extractDataAttribute(string(finalizeRespBody), "data-verify-path")
		cprFinalizePath := extractDataAttribute(string(finalizeRespBody), "data-finalize-cpr-path")
		cprBaseURL := extractDataAttribute(string(finalizeRespBody), "data-base-url")
		csrfToken := extractDataAttribute(string(finalizeRespBody), "data-csrf")

		if cprVerifyPath == "" || cprFinalizePath == "" {
			log.Printf("[MitID Native] Could not extract CPR paths from page")
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("could not extract CPR verification paths from response")
		}

		log.Printf("[MitID Native] Step 5a: Submitting CPR (length %d) to %s%s, csrf=%s", len(cpr), cprBaseURL, cprVerifyPath, csrfToken)

		// Try form-urlencoded format first (more common for web forms)
		cprFormData := url.Values{}
		cprFormData.Set("cpr", cpr)

		log.Printf("[MitID Native] Step 5a: Sending CPR as form data: cpr=%s", cpr)
		req, _ := http.NewRequest(http.MethodPost, cprBaseURL+cprVerifyPath, strings.NewReader(cprFormData.Encode()))
		req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Set("Accept", "application/json")
		if csrfToken != "" {
			req.Header.Set("Csrf-Token", csrfToken)
			req.Header.Set("X-CSRF-Token", csrfToken)
		}
		req.Header.Set("Referer", cprBaseURL+"cpr")
		req.Header.Set("Origin", "https://signicat-sign.mitid.dk")

		resp, err = httpClient.Do(req)
		if err != nil {
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("submitting CPR: %w", err)
		}
		cprVerifyRespBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[MitID Native] Step 5a: CPR verify response (status %d): %s", resp.StatusCode, string(cprVerifyRespBody))

		if resp.StatusCode != http.StatusOK {
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("CPR verification failed (status %d): %s", resp.StatusCode, string(cprVerifyRespBody))
		}

		// Parse CPR verification response
		var cprVerifyResult struct {
			Success           bool   `json:"success"`
			RemainingAttempts int    `json:"remainingAttempts"`
			ProvidedCpr       string `json:"providedCpr"`
		}
		if err := json.Unmarshal(cprVerifyRespBody, &cprVerifyResult); err != nil {
			log.Printf("[MitID Native] Step 5a: Failed to parse CPR verify response: %v", err)
		}

		if !cprVerifyResult.Success {
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("CPR verification failed: CPR mismatch (remaining attempts: %d)", cprVerifyResult.RemainingAttempts)
		}

		log.Printf("[MitID Native] Step 5a: CPR verified successfully")

		// Finalize CPR - this should redirect to Nordnet with code
		log.Printf("[MitID Native] Step 5b: Finalizing CPR at %s%s", cprBaseURL, cprFinalizePath)
		resp, err = httpClient.Get(cprBaseURL + cprFinalizePath)
		if err != nil {
			qrManager.SetStatus("failed")
			return nil, fmt.Errorf("finalizing CPR: %w", err)
		}
		cprFinalizeRespBody, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		log.Printf("[MitID Native] Step 5b: CPR finalize response (status %d): %s", resp.StatusCode, string(cprFinalizeRespBody))

		finalURL = resp.Request.URL.String()
		log.Printf("[MitID Native] Step 5b: Final URL after CPR finalize: %s", finalURL)
	}

	// Extract code from redirect URL
	parsedURL, _ := url.Parse(finalURL)
	signicatCode = parsedURL.Query().Get("code")

	if signicatCode == "" {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("could not extract Signicat code from redirect URL: %s", finalURL)
	}
	log.Printf("[MitID Native] Step 5: Got Signicat code: %s...", signicatCode[:20])

	// Step 5: Exchange code for Nordnet session
	sessionPayload := map[string]interface{}{
		"authenticationProvider": "SIGNICAT",
		"countryCode":            strings.ToUpper(country),
		"signicat": map[string]string{
			"authorizationCode": signicatCode,
			"redirectUri":       fmt.Sprintf("https://%s/login", domain),
		},
	}

	sessionBody, _ := json.Marshal(sessionPayload)
	req, _ = http.NewRequest(http.MethodPost,
		fmt.Sprintf("https://%s/nnxapi/authentication/v2/sessions", domain),
		strings.NewReader(string(sessionBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("client-id", "NEXT")

	resp, err = httpClient.Do(req)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("creating Nordnet session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		qrManager.SetStatus("failed")
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("Nordnet session creation failed (status %d): %s", resp.StatusCode, string(body))
	}

	// Read session response - may contain useful data
	sessionRespBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Native] Step 5: Session response (status %d): %s", resp.StatusCode, string(sessionRespBody))

	// Check for ntag in the session response headers
	sessionNtag := resp.Header.Get("ntag")
	log.Printf("[MitID Native] Step 5: Session ntag from header: %s", sessionNtag)

	// Also check for set-cookie headers
	for _, cookie := range resp.Cookies() {
		log.Printf("[MitID Native] Step 5: Cookie: %s=%s", cookie.Name, cookie.Value[:min(20, len(cookie.Value))]+"...")
	}

	// Step 6: Login to get ntag
	loginBody, _ := json.Marshal(map[string]interface{}{})
	req, _ = http.NewRequest(http.MethodPost,
		fmt.Sprintf("https://%s/api/2/authentication/nnx-session/login", domain),
		strings.NewReader(string(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("client-id", "NEXT")
	req.Header.Set("x-locale", "da-DK")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	// Use ntag from session if available
	if sessionNtag != "" {
		req.Header.Set("ntag", sessionNtag)
	}

	resp, err = httpClient.Do(req)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("login request: %w", err)
	}
	defer resp.Body.Close()

	loginRespBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Native] Step 6: Login response (status %d): %s", resp.StatusCode, string(loginRespBody))

	ntag := resp.Header.Get("ntag")
	log.Printf("[MitID Native] Step 6: Got ntag from login response: %s", ntag)

	// If login didn't return ntag, try to use the one from session creation
	if ntag == "" && sessionNtag != "" {
		log.Printf("[MitID Native] Step 6: Using ntag from session creation instead")
		ntag = sessionNtag
	}

	// Step 7: Get JWT bearer token
	req, _ = http.NewRequest(http.MethodPost,
		fmt.Sprintf("https://%s/nnxapi/authorization/v1/tokens", domain),
		strings.NewReader(string(loginBody)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("client-id", "NEXT")
	req.Header.Set("x-locale", "da-DK")
	req.Header.Set("User-Agent", "Mozilla/5.0 (Windows NT 10.0; Win64; x64) AppleWebKit/537.36 (KHTML, like Gecko) Chrome/120.0.0.0 Safari/537.36")
	req.Header.Set("ntag", ntag)

	resp, err = httpClient.Do(req)
	if err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("token request: %w", err)
	}
	defer resp.Body.Close()

	tokenBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Native] Step 7: Token response (status %d): %s", resp.StatusCode, string(tokenBody))

	var tokenResp struct {
		JWT string `json:"jwt"`
	}
	if err := json.Unmarshal(tokenBody, &tokenResp); err != nil {
		qrManager.SetStatus("failed")
		return nil, fmt.Errorf("decoding token response: %w", err)
	}

	if tokenResp.JWT == "" {
		log.Printf("[MitID Native] Step 7: WARNING - JWT is empty!")
	} else {
		log.Printf("[MitID Native] Step 7: Got JWT (length %d)", len(tokenResp.JWT))
	}

	qrManager.SetStatus("complete")

	// Get cookies from cookie jar for the Nordnet domain
	nordnetURL, _ := url.Parse(fmt.Sprintf("https://%s", domain))
	cookies := jar.Cookies(nordnetURL)
	log.Printf("[MitID Native] Got %d cookies from session", len(cookies))
	for _, c := range cookies {
		log.Printf("[MitID Native] Cookie: %s (len=%d)", c.Name, len(c.Value))
	}

	session := &Session{
		JWT:       tokenResp.JWT,
		NTag:      ntag,
		Domain:    domain,
		Cookies:   cookies,
		ExpiresAt: time.Now().Add(24 * time.Hour),
	}

	// Cache the session for future requests
	CacheSession(connectionID, session)

	return session, nil
}

// extractDataAttribute extracts a data attribute value from HTML.
func extractDataAttribute(html, attrName string) string {
	// Simple regex to find data attributes
	pattern := fmt.Sprintf(`%s="([^"]*)"`, regexp.QuoteMeta(attrName))
	re := regexp.MustCompile(pattern)
	matches := re.FindStringSubmatch(html)
	if len(matches) > 1 {
		return matches[1]
	}
	return ""
}

// LoginWithMitIDNative is a convenience method on Client to authenticate using native MitID.
func (c *Client) LoginWithMitIDNative(connectionID int64, userID, cpr, method, scriptDir string) (*Session, error) {
	return AuthenticateWithMitIDNative(connectionID, c.country, userID, cpr, method, scriptDir)
}
