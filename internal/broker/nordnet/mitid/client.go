package mitid

import (
	"bytes"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"strings"
	"time"
)

const (
	// MitIDBaseURL is the base URL for MitID API (production).
	MitIDBaseURL = "https://www.mitid.dk"

	// MitIDTestBaseURL is the base URL for MitID API (pre-production/test).
	MitIDTestBaseURL = "https://pp.mitid.dk"

	// CoreClientBackend is the path for core client backend API.
	CoreClientBackend = "/mitid-core-client-backend"

	// CodeAppAuth is the path for code app authentication API.
	CodeAppAuth = "/mitid-code-app-auth"

	// Default poll timeout.
	pollTimeout = 2 * time.Minute
)

// NewClient creates a new MitID client for production.
// Parameters:
//   - clientHash: The client hash from aux parameters (hex-encoded checksum)
//   - authSessionID: The authentication session ID
//   - httpClient: HTTP client to use for requests
//   - qrDir: Directory to write QR code files
func NewClient(clientHash, authSessionID string, httpClient *http.Client, qrDir string) (*Client, error) {
	return NewClientWithBaseURL(clientHash, authSessionID, httpClient, qrDir, MitIDBaseURL)
}

// NewTestClient creates a new MitID client for the pre-production/test environment.
// Use this with test users created at https://pp.mitid.dk/test-tool/frontend/
func NewTestClient(clientHash, authSessionID string, httpClient *http.Client, qrDir string) (*Client, error) {
	return NewClientWithBaseURL(clientHash, authSessionID, httpClient, qrDir, MitIDTestBaseURL)
}

// NewClientWithBaseURL creates a new MitID client with a custom base URL.
// Parameters:
//   - clientHash: The client hash from aux parameters (hex-encoded checksum)
//   - authSessionID: The authentication session ID
//   - httpClient: HTTP client to use for requests
//   - qrDir: Directory to write QR code files
//   - baseURL: Base URL for MitID API (e.g., "https://www.mitid.dk" or "https://pp.mitid.dk")
func NewClientWithBaseURL(clientHash, authSessionID string, httpClient *http.Client, qrDir, baseURL string) (*Client, error) {
	c := &Client{
		httpClient:              httpClient,
		qrDir:                   qrDir,
		clientHash:              clientHash,
		authenticationSessionID: authSessionID,
		baseURL:                 baseURL,
	}

	// Fetch session details
	if err := c.fetchSessionDetails(); err != nil {
		return nil, err
	}

	return c, nil
}

// fetchSessionDetails retrieves the authentication session details.
func (c *Client) fetchSessionDetails() error {
	url := fmt.Sprintf("%s%s/v1/authentication-sessions/%s",
		c.baseURL, CoreClientBackend, c.authenticationSessionID)

	resp, err := c.httpClient.Get(url)
	if err != nil {
		return fmt.Errorf("fetching session: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("failed to get authentication session (status %d): %s", resp.StatusCode, string(body))
	}

	var session AuthSessionResponse
	if err := json.NewDecoder(resp.Body).Decode(&session); err != nil {
		return fmt.Errorf("decoding session response: %w", err)
	}

	c.brokerSecurityContext = session.BrokerSecurityContext
	c.serviceProviderName = session.ServiceProviderName
	c.referenceTextHeader = session.ReferenceTextHeader
	c.referenceTextBody = session.ReferenceTextBody

	return nil
}

// IdentifyUser identifies the user and gets available authenticators.
// Returns a map of authenticator names to their display names.
func (c *Client) IdentifyUser(userID string) (map[string]string, error) {
	c.userID = userID

	// PUT identity claim
	url := fmt.Sprintf("%s%s/v1/authentication-sessions/%s",
		c.baseURL, CoreClientBackend, c.authenticationSessionID)

	body := map[string]string{"identityClaim": userID}
	resp, err := c.doJSON(http.MethodPut, url, body)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)

		// Check for specific error codes
		var errResp struct {
			ErrorCode string `json:"errorCode"`
		}
		if json.Unmarshal(respBody, &errResp) == nil {
			switch errResp.ErrorCode {
			case "control.identity_not_found":
				return nil, ErrUserNotFound
			case "control.authentication_session_not_found":
				return nil, ErrSessionNotFound
			case "control.client_ip_blocked":
				return nil, ErrIPBlocked
			}
		}

		return nil, fmt.Errorf("identify user failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	// POST /next to get available authenticators
	nextURL := fmt.Sprintf("%s%s/v2/authentication-sessions/%s/next",
		c.baseURL, CoreClientBackend, c.authenticationSessionID)

	nextBody := map[string]string{"combinationId": ""}
	resp, err = c.doJSON(http.MethodPost, nextURL, nextBody)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("get authenticators failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var nextResp NextResponse
	if err := json.NewDecoder(resp.Body).Decode(&nextResp); err != nil {
		return nil, fmt.Errorf("decoding next response: %w", err)
	}

	// Check for errors
	if len(nextResp.Errors) > 0 && nextResp.Errors[0].ErrorCode == "control.authenticator_cannot_be_started" {
		errText := ""
		if nextResp.Errors[0].UserMessage != nil && nextResp.Errors[0].UserMessage.Text != nil {
			errText = nextResp.Errors[0].UserMessage.Text.Text
		}
		return nil, fmt.Errorf("%w: %s", ErrAuthenticatorCannotStart, errText)
	}

	// Store current authenticator state
	if nextResp.NextAuthenticator != nil {
		c.currentAuthenticatorType = nextResp.NextAuthenticator.AuthenticatorType
		c.currentAuthenticatorSessionFlowKey = nextResp.NextAuthenticator.AuthenticatorSessionFlowKey
		c.currentAuthenticatorEAFEHash = nextResp.NextAuthenticator.EAFEHash
		c.currentAuthenticatorSessionID = nextResp.NextAuthenticator.AuthenticatorSessionID
		log.Printf("[MitID Client] Stored authenticator state: type=%s, sessionID=%s, flowKey=%s",
			c.currentAuthenticatorType, c.currentAuthenticatorSessionID, c.currentAuthenticatorSessionFlowKey)
	} else {
		log.Printf("[MitID Client] WARNING: nextResp.NextAuthenticator is nil")
	}

	// Build available authenticators map
	available := make(map[string]string)
	for _, combo := range nextResp.Combinations {
		var methodName string
		switch combo.ID {
		case "S3":
			methodName = "APP"
		case "S1":
			methodName = "TOKEN"
		default:
			continue
		}
		if len(combo.CombinationItems) > 0 {
			available[methodName] = combo.CombinationItems[0].Name
		}
	}

	return available, nil
}

// selectAuthenticator switches to the specified authenticator type.
func (c *Client) selectAuthenticator(authType string) error {
	log.Printf("[MitID Client] selectAuthenticator: current=%s, want=%s", c.currentAuthenticatorType, authType)
	if c.currentAuthenticatorType == authType {
		log.Printf("[MitID Client] Already using authenticator %s, skipping switch", authType)
		return nil
	}

	var combinationID string
	switch authType {
	case "APP":
		combinationID = "S3"
	case "TOKEN":
		combinationID = "S1"
	default:
		return fmt.Errorf("unknown authenticator type: %s", authType)
	}

	url := fmt.Sprintf("%s%s/v2/authentication-sessions/%s/next",
		c.baseURL, CoreClientBackend, c.authenticationSessionID)

	body := map[string]string{"combinationId": combinationID}
	resp, err := c.doJSON(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("select authenticator failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var nextResp NextResponse
	if err := json.NewDecoder(resp.Body).Decode(&nextResp); err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	// Check for errors
	if len(nextResp.Errors) > 0 {
		errText := nextResp.Errors[0].Message
		if nextResp.Errors[0].UserMessage != nil && nextResp.Errors[0].UserMessage.Text != nil {
			errText = nextResp.Errors[0].UserMessage.Text.Text
		}
		return fmt.Errorf("%w: %s", ErrAuthenticatorCannotStart, errText)
	}

	// Update authenticator state
	if nextResp.NextAuthenticator != nil {
		c.currentAuthenticatorType = nextResp.NextAuthenticator.AuthenticatorType
		c.currentAuthenticatorSessionFlowKey = nextResp.NextAuthenticator.AuthenticatorSessionFlowKey
		c.currentAuthenticatorEAFEHash = nextResp.NextAuthenticator.EAFEHash
		c.currentAuthenticatorSessionID = nextResp.NextAuthenticator.AuthenticatorSessionID
		log.Printf("[MitID Client] After selectAuthenticator: type=%s, sessionID=%s",
			c.currentAuthenticatorType, c.currentAuthenticatorSessionID)
	} else {
		log.Printf("[MitID Client] WARNING: nextResp.NextAuthenticator is nil after selectAuthenticator")
	}

	if c.currentAuthenticatorType != authType {
		return fmt.Errorf("failed to select authenticator %s, got %s", authType, c.currentAuthenticatorType)
	}

	return nil
}

// createFlowValueProof creates the flow value proof string for HMAC.
func (c *Client) createFlowValueProof() []byte {
	// Hash broker security context
	hashedBSC := sha256.Sum256([]byte(c.brokerSecurityContext))
	hashedBSCHex := hex.EncodeToString(hashedBSC[:])

	// Base64 encode reference texts
	b64Header := base64.StdEncoding.EncodeToString([]byte(c.referenceTextHeader))
	b64Body := base64.StdEncoding.EncodeToString([]byte(c.referenceTextBody))
	b64SPName := base64.StdEncoding.EncodeToString([]byte(c.serviceProviderName))

	proof := fmt.Sprintf("%s,%s,%s,%s,%s,%s,%s,%s",
		c.currentAuthenticatorSessionID,
		c.currentAuthenticatorSessionFlowKey,
		c.clientHash,
		c.currentAuthenticatorEAFEHash,
		hashedBSCHex,
		b64Header,
		b64Body,
		b64SPName,
	)

	return []byte(proof)
}

// AuthenticateWithApp performs MitID app authentication.
func (c *Client) AuthenticateWithApp() error {
	if err := c.selectAuthenticator("APP"); err != nil {
		return err
	}

	// Initialize QR manager
	qrManager := NewQRManager(c.qrDir)
	if err := qrManager.EnsureDirectory(); err != nil {
		return fmt.Errorf("creating QR directory: %w", err)
	}
	qrManager.SetStatus("initializing")

	// Init app auth
	initURL := fmt.Sprintf("%s%s/v1/authenticator-sessions/web/%s/init-auth",
		c.baseURL, CodeAppAuth, c.currentAuthenticatorSessionID)

	log.Printf("[MitID Client] Calling init-auth URL: %s", initURL)
	log.Printf("[MitID Client] AuthenticatorSessionID: %s", c.currentAuthenticatorSessionID)
	resp, err := c.doJSON(http.MethodPost, initURL, map[string]interface{}{})
	if err != nil {
		log.Printf("[MitID Client] Init-auth request error: %v", err)
		return err
	}
	defer resp.Body.Close()

	// Read raw response body for debugging
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		log.Printf("[MitID Client] Error reading init-auth response body: %v", err)
		return fmt.Errorf("reading init response: %w", err)
	}
	log.Printf("[MitID Client] Init-auth raw response (status %d): %s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		var errResp AppInitAuthResponse
		if json.Unmarshal(respBody, &errResp) == nil {
			if errResp.ErrorCode == "auth.codeapp.authentication.parallel_sessions_detected" {
				return ErrParallelSessions
			}
		}

		return fmt.Errorf("init app auth failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var initResp AppInitAuthResponse
	if err := json.Unmarshal(respBody, &initResp); err != nil {
		log.Printf("[MitID Client] Error decoding init response: %v", err)
		return fmt.Errorf("decoding init response: %w", err)
	}

	log.Printf("[MitID Client] Init-auth parsed: pollURL=%s, ticket=%s, appSwitchValue=%s",
		initResp.PollURL, initResp.Ticket, initResp.ChannelBindingValueAppSwitch)

	// Start QR animator
	qrAnimator := NewQRAnimator(qrManager)

	// If we got channelBindingValueAppSwitch directly, use it to generate QR codes immediately
	// This is a newer MitID behavior where the binding value is provided upfront
	if initResp.ChannelBindingValueAppSwitch != "" {
		log.Printf("[MitID Client] Using channelBindingValueAppSwitch for immediate QR generation")
		if err := qrManager.GenerateQRCodePair(initResp.ChannelBindingValueAppSwitch, 1); err != nil {
			qrManager.SetStatus("failed")
			return fmt.Errorf("generating QR codes from appSwitch: %w", err)
		}
		qrManager.SetStatus("qr_ready")
		qrAnimator.Start()
	}

	// Poll for app response
	var pollResp PollResponse
	deadline := time.Now().Add(pollTimeout)

	log.Printf("[MitID Client] Starting poll loop, pollURL=%s", initResp.PollURL)

	pollCount := 0
	for time.Now().Before(deadline) {
		pollCount++
		pollResp, err = c.pollAppStatus(initResp.PollURL, initResp.Ticket)
		if err != nil {
			log.Printf("[MitID Client] Poll #%d error: %v", pollCount, err)
			qrAnimator.Stop()
			qrManager.SetStatus("failed")
			return err
		}

		log.Printf("[MitID Client] Poll #%d response: status=%s, elapsed=%v",
			pollCount, pollResp.Status, time.Since(deadline.Add(-pollTimeout)))

		switch pollResp.Status {
		case "timeout":
			// MitID says "try again" - this is normal, keep polling
			continue

		case "channel_validation_tqr":
			// Generate and display QR codes
			if err := qrManager.GenerateQRCodePair(pollResp.ChannelBindingValue, pollResp.UpdateCount); err != nil {
				qrAnimator.Stop()
				qrManager.SetStatus("failed")
				return fmt.Errorf("generating QR codes: %w", err)
			}
			qrManager.SetStatus("qr_ready")
			qrAnimator.Start()
			continue

		case "channel_verified":
			// QR code verified, waiting for user approval
			qrAnimator.Stop()
			qrManager.SetStatus("waiting_approval")
			continue

		case "OK":
			log.Printf("[MitID Client] Got OK status, confirmation=%v, payload=%v", pollResp.Confirmation, pollResp.Payload != nil)
			if pollResp.Confirmation {
				qrAnimator.Stop()
				qrManager.SetStatus("approved")
				break
			}
			continue

		default:
			qrAnimator.Stop()
			qrManager.SetStatus("failed")
			return fmt.Errorf("unexpected poll status: %s", pollResp.Status)
		}

		// If we got OK with confirmation, break the loop
		if pollResp.Status == "OK" && pollResp.Confirmation {
			break
		}
	}

	if pollResp.Status != "OK" || !pollResp.Confirmation {
		qrManager.SetStatus("failed")
		if time.Now().After(deadline) {
			return ErrTimeout
		}
		return ErrLoginRejected
	}

	// Extract response and signature
	if pollResp.Payload == nil {
		log.Printf("[MitID Client] ERROR: pollResp.Payload is nil, pollResp=%+v", pollResp)
		return fmt.Errorf("missing payload in poll response")
	}

	response := pollResp.Payload.Response
	responseSignature := pollResp.Payload.ResponseSignature
	log.Printf("[MitID Client] Got payload: response=%s, sig=%s", response, responseSignature)

	// Complete SRP exchange
	if err := c.completeSRPExchange(response, responseSignature); err != nil {
		log.Printf("[MitID Client] SRP exchange failed: %v", err)
		qrManager.SetStatus("failed")
		return err
	}

	qrManager.SetStatus("complete")
	return nil
}

// pollAppStatus polls the app authentication status.
func (c *Client) pollAppStatus(pollURL, ticket string) (PollResponse, error) {
	body := PollRequest{Ticket: ticket}
	resp, err := c.doJSON(http.MethodPost, pollURL, body)
	if err != nil {
		return PollResponse{}, err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return PollResponse{}, fmt.Errorf("poll failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var pollResp PollResponse
	if err := json.NewDecoder(resp.Body).Decode(&pollResp); err != nil {
		return PollResponse{}, fmt.Errorf("decoding poll response: %w", err)
	}

	return pollResp, nil
}

// completeSRPExchange completes the SRP protocol after app approval.
func (c *Client) completeSRPExchange(response, responseSignature string) error {
	// Stage 1: Generate client ephemeral
	srp := NewSRP()
	aHex, err := srp.Stage1()
	if err != nil {
		return fmt.Errorf("SRP stage 1: %w", err)
	}

	// Init SRP exchange
	initURL := fmt.Sprintf("%s%s/v1/authenticator-sessions/web/%s/init",
		c.baseURL, CodeAppAuth, c.currentAuthenticatorSessionID)

	initReq := SRPInitRequest{RandomA: SRPValue{Value: aHex}}
	resp, err := c.doJSON(http.MethodPost, initURL, initReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("SRP init failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var initResp SRPInitResponse
	if err := json.NewDecoder(resp.Body).Decode(&initResp); err != nil {
		return fmt.Errorf("decoding SRP init response: %w", err)
	}

	// Compute password from app response
	// password = SHA256(base64_decode(response) || session_flow_key)
	responseBytes, err := base64.StdEncoding.DecodeString(response)
	if err != nil {
		return fmt.Errorf("decoding response: %w", err)
	}

	passwordInput := append(responseBytes, []byte(c.currentAuthenticatorSessionFlowKey)...)
	passwordHash := sha256.Sum256(passwordInput)
	password := hex.EncodeToString(passwordHash[:])

	// Stage 3: Compute M1
	m1Hex, err := srp.Stage3(initResp.SRPSalt.Value, initResp.RandomB.Value, password, c.currentAuthenticatorSessionID)
	if err != nil {
		return fmt.Errorf("SRP stage 3: %w", err)
	}

	// Create flow value proof
	flowValueProof := c.createFlowValueProof()
	proofKey := CreateFlowValueProofKey("flowValues", srp.GetSessionKey())
	flowValueProofHex := HMACSHA256(proofKey, flowValueProof)

	// Prove
	proveURL := fmt.Sprintf("%s%s/v1/authenticator-sessions/web/%s/prove",
		c.baseURL, CodeAppAuth, c.currentAuthenticatorSessionID)

	proveReq := SRPProveRequest{
		M1:             SRPValue{Value: m1Hex},
		FlowValueProof: SRPValue{Value: flowValueProofHex},
	}

	resp, err = c.doJSON(http.MethodPost, proveURL, proveReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	proveRespBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Client] SRP prove response (status %d): %s", resp.StatusCode, string(proveRespBody))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("SRP prove failed (status %d): %s", resp.StatusCode, string(proveRespBody))
	}

	var proveResp SRPProveResponse
	if err := json.Unmarshal(proveRespBody, &proveResp); err != nil {
		return fmt.Errorf("decoding prove response: %w", err)
	}

	// Stage 5: Verify M2
	log.Printf("[MitID Client] Verifying M2: %s", proveResp.M2.Value)
	if !srp.Stage5(proveResp.M2.Value) {
		log.Printf("[MitID Client] M2 verification FAILED")
		return ErrSRPVerifyFailed
	}
	log.Printf("[MitID Client] M2 verification succeeded")

	// Encrypt response signature for verification
	// Decode base64 signature (add padding if needed for base64)
	sigBytes, err := base64.StdEncoding.DecodeString(responseSignature)
	if err != nil {
		// Try adding base64 padding
		padded := responseSignature
		for len(padded)%4 != 0 {
			padded += "="
		}
		sigBytes, err = base64.StdEncoding.DecodeString(padded)
		if err != nil {
			return fmt.Errorf("decoding response signature: %w", err)
		}
	}

	// AES-GCM doesn't need PKCS7 padding - encrypt raw signature bytes
	encAuth, err := srp.AuthEnc(sigBytes)
	if err != nil {
		return fmt.Errorf("encrypting auth: %w", err)
	}

	// Verify
	verifyURL := fmt.Sprintf("%s%s/v1/authenticator-sessions/web/%s/verify",
		c.baseURL, CodeAppAuth, c.currentAuthenticatorSessionID)

	verifyReq := AppVerifyRequest{
		EncAuth:                encAuth,
		FrontEndProcessingTime: 100, // Approximate
	}

	resp, err = c.doJSON(http.MethodPost, verifyURL, verifyReq)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	verifyRespBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Client] Verify response (status %d): %s", resp.StatusCode, string(verifyRespBody))

	if resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusOK {
		return fmt.Errorf("verify failed (status %d): %s", resp.StatusCode, string(verifyRespBody))
	}

	// Post next to finalize authenticator
	nextURL := fmt.Sprintf("%s%s/v2/authentication-sessions/%s/next",
		c.baseURL, CoreClientBackend, c.authenticationSessionID)

	nextBody := map[string]string{"combinationId": ""}
	resp, err = c.doJSON(http.MethodPost, nextURL, nextBody)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	respBody, _ := io.ReadAll(resp.Body)
	log.Printf("[MitID Client] Finalize authenticator response (status %d): %s", resp.StatusCode, string(respBody))

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("finalize authenticator failed (status %d): %s", resp.StatusCode, string(respBody))
	}

	var nextResp NextResponse
	if err := json.Unmarshal(respBody, &nextResp); err != nil {
		return fmt.Errorf("decoding next response: %w", err)
	}

	log.Printf("[MitID Client] Finalize parsed: nextSessionID=%s, errors=%d, nextAuth=%+v",
		nextResp.NextSessionID, len(nextResp.Errors), nextResp.NextAuthenticator)

	// Check for errors
	if len(nextResp.Errors) > 0 {
		errInfo := nextResp.Errors[0]

		// Check if this is a continuation state vs actual error
		if !errInfo.IsActualError() {
			// This is a continuation state - authentication flow is incomplete
			contextName := ""
			if errInfo.AuthenticatorContext != nil && errInfo.AuthenticatorContext.Name != nil {
				contextName = errInfo.AuthenticatorContext.Name.Text
			}
			log.Printf("[MitID Client] Authentication incomplete: continueText=%s, context=%s",
				errInfo.ContinueText, contextName)
			return fmt.Errorf("authentication incomplete: %s (try again)", contextName)
		}

		// This is an actual error
		errMsg := errInfo.Message
		if errMsg == "" && errInfo.UserMessage != nil && errInfo.UserMessage.Text != nil {
			errMsg = errInfo.UserMessage.Text.Text
		}
		if errMsg == "" {
			errMsg = errInfo.ErrorCode
		}
		log.Printf("[MitID Client] Authentication error: code=%s, message=%s, userMessage=%+v",
			errInfo.ErrorCode, errInfo.Message, errInfo.UserMessage)
		return fmt.Errorf("authentication error: %s (code: %s)", errMsg, errInfo.ErrorCode)
	}

	c.finalizationAuthSessionID = nextResp.NextSessionID
	log.Printf("[MitID Client] Got finalization session ID: %s", c.finalizationAuthSessionID)
	return nil
}

// FinalizeAndGetAuthCode finalizes authentication and returns the authorization code.
func (c *Client) FinalizeAndGetAuthCode() (string, error) {
	if c.finalizationAuthSessionID == "" {
		return "", fmt.Errorf("no finalization session ID - complete authentication first")
	}

	url := fmt.Sprintf("%s%s/v1/authentication-sessions/%s/finalization",
		c.baseURL, CoreClientBackend, c.finalizationAuthSessionID)

	req, err := http.NewRequest(http.MethodPut, url, nil)
	if err != nil {
		return "", err
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("%w: status %d - %s", ErrFinalizationFailed, resp.StatusCode, string(respBody))
	}

	var finalResp FinalizationResponse
	if err := json.NewDecoder(resp.Body).Decode(&finalResp); err != nil {
		return "", fmt.Errorf("decoding finalization response: %w", err)
	}

	return finalResp.AuthorizationCode, nil
}

// GetServiceProviderName returns the service provider name for display.
func (c *Client) GetServiceProviderName() string {
	return c.serviceProviderName
}

// GetReferenceText returns the reference text header and body.
func (c *Client) GetReferenceText() (string, string) {
	return c.referenceTextHeader, c.referenceTextBody
}

// doJSON performs a JSON HTTP request.
func (c *Client) doJSON(method, url string, body interface{}) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		jsonBody, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshaling body: %w", err)
		}
		bodyReader = bytes.NewReader(jsonBody)
	}

	req, err := http.NewRequest(method, url, bodyReader)
	if err != nil {
		return nil, fmt.Errorf("creating request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	return c.httpClient.Do(req)
}

// ExtractAuxParameters extracts clientHash and authSessionID from aux JSON.
func ExtractAuxParameters(aux map[string]interface{}) (clientHash, authSessionID string, err error) {
	// Extract client hash from coreClient.checksum (base64 -> hex)
	coreClient, ok := aux["coreClient"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("missing coreClient in aux")
	}

	checksumB64, ok := coreClient["checksum"].(string)
	if !ok {
		return "", "", fmt.Errorf("missing checksum in coreClient")
	}

	checksumBytes, err := base64.StdEncoding.DecodeString(checksumB64)
	if err != nil {
		return "", "", fmt.Errorf("decoding checksum: %w", err)
	}
	clientHash = hex.EncodeToString(checksumBytes)

	// Extract auth session ID from parameters.authenticationSessionId
	params, ok := aux["parameters"].(map[string]interface{})
	if !ok {
		return "", "", fmt.Errorf("missing parameters in aux")
	}

	authSessionID, ok = params["authenticationSessionId"].(string)
	if !ok {
		return "", "", fmt.Errorf("missing authenticationSessionId in parameters")
	}

	return clientHash, authSessionID, nil
}

// ParseAuxFromJSON parses aux parameters from a JSON string.
func ParseAuxFromJSON(auxJSON string) (map[string]interface{}, error) {
	// Handle escaped JSON if necessary
	auxJSON = strings.TrimPrefix(auxJSON, "\"")
	auxJSON = strings.TrimSuffix(auxJSON, "\"")
	auxJSON = strings.ReplaceAll(auxJSON, "\\\"", "\"")

	var aux map[string]interface{}
	if err := json.Unmarshal([]byte(auxJSON), &aux); err != nil {
		return nil, fmt.Errorf("parsing aux JSON: %w", err)
	}
	return aux, nil
}
