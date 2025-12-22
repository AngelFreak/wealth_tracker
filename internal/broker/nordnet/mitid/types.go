package mitid

import (
	"net/http"
	"sync"
)

// AuthMethod represents the authentication method to use.
type AuthMethod string

const (
	// AuthMethodApp uses the MitID mobile app for authentication.
	AuthMethodApp AuthMethod = "APP"
	// AuthMethodToken uses a hardware TOTP token with password.
	AuthMethodToken AuthMethod = "TOKEN"
)

// CombinationID maps auth methods to MitID combination IDs.
var combinationIDs = map[AuthMethod]string{
	AuthMethodApp:   "S3",
	AuthMethodToken: "S1",
}

// AuthSessionResponse represents the initial authentication session data.
type AuthSessionResponse struct {
	BrokerSecurityContext string `json:"brokerSecurityContext"`
	ServiceProviderName   string `json:"serviceProviderName"`
	ReferenceTextHeader   string `json:"referenceTextHeader"`
	ReferenceTextBody     string `json:"referenceTextBody"`
}

// NextAuthenticator contains authenticator session details.
type NextAuthenticator struct {
	AuthenticatorType           string `json:"authenticatorType"`
	AuthenticatorSessionFlowKey string `json:"authenticatorSessionFlowKey"`
	EAFEHash                    string `json:"eafeHash"`
	AuthenticatorSessionID      string `json:"authenticatorSessionId"`
}

// CombinationItem represents an available auth combination item.
type CombinationItem struct {
	Name string `json:"name"`
}

// Combination represents an available authentication combination.
type Combination struct {
	ID               string            `json:"id"`
	CombinationItems []CombinationItem `json:"combinationItems"`
}

// NextResponse is the response from the /next endpoint.
type NextResponse struct {
	NextAuthenticator *NextAuthenticator `json:"nextAuthenticator"`
	Combinations      []Combination      `json:"combinations"`
	Errors            []AuthError        `json:"errors"`
	NextSessionID     string             `json:"nextSessionId"`
}

// AuthError represents an authentication error or continuation state from MitID.
type AuthError struct {
	ErrorCode            string                `json:"errorCode"`
	Message              string                `json:"message"`
	ContinueText         string                `json:"continueText"`
	UserMessage          *UserMessage          `json:"userMessage"`
	AuthenticatorContext *AuthenticatorContext `json:"authenticatorContext"`
	CorrelationID        string                `json:"correlationId"`
}

// AuthenticatorContext contains context for the current authenticator state.
type AuthenticatorContext struct {
	Name          *TextContent `json:"name"`
	Text          *TextContent `json:"text"`
	RenderingType string       `json:"renderingType"`
}

// IsActualError returns true if this represents an actual error vs a continuation state.
func (e *AuthError) IsActualError() bool {
	// If errorCode is set, it's an actual error
	if e.ErrorCode != "" {
		return true
	}
	// If there's no continueText or authenticatorContext, treat as error
	if e.ContinueText == "" && e.AuthenticatorContext == nil {
		return true
	}
	// Otherwise it's a continuation state (not an error)
	return false
}

// UserMessage contains a user-facing error message.
type UserMessage struct {
	Text *TextContent `json:"text"`
}

// TextContent contains the actual text content.
type TextContent struct {
	Text string `json:"text"`
}

// SRPValue wraps a hex string value.
type SRPValue struct {
	Value string `json:"value"`
}

// SRPInitRequest is the request to initialize SRP.
type SRPInitRequest struct {
	RandomA SRPValue `json:"randomA"`
}

// SRPInitResponse is the response from SRP init endpoints.
type SRPInitResponse struct {
	PBKDF2Salt SRPValue `json:"pbkdf2Salt,omitempty"`
	SRPSalt    SRPValue `json:"srpSalt"`
	RandomB    SRPValue `json:"randomB"`
}

// SRPProveRequest is the request to prove SRP authentication.
type SRPProveRequest struct {
	M1             SRPValue `json:"m1"`
	FlowValueProof SRPValue `json:"flowValueProof"`
}

// SRPProveRequestWithTime includes processing time.
type SRPProveRequestWithTime struct {
	M1                     SRPValue `json:"m1"`
	FlowValueProof         SRPValue `json:"flowValueProof"`
	FrontEndProcessingTime int      `json:"frontEndProcessingTime"`
}

// SRPProveResponse is the response from prove endpoints.
type SRPProveResponse struct {
	M2 SRPValue `json:"m2"`
}

// AppInitAuthResponse is the response from app init-auth endpoint.
type AppInitAuthResponse struct {
	PollURL                     string `json:"pollUrl"`
	Ticket                      string `json:"ticket"`
	ChannelBindingValueAppSwitch string `json:"channelBindingValueAppSwitch,omitempty"`
	ErrorCode                   string `json:"errorCode,omitempty"`
}

// PollRequest is the request body for polling.
type PollRequest struct {
	Ticket string `json:"ticket"`
}

// PollResponse represents a poll status response.
type PollResponse struct {
	Status              string       `json:"status"`
	ChannelBindingValue string       `json:"channelBindingValue,omitempty"`
	UpdateCount         int          `json:"updateCount,omitempty"`
	Confirmation        bool         `json:"confirmation,omitempty"`
	Payload             *PollPayload `json:"payload,omitempty"`
}

// PollPayload contains the response and signature from app auth.
type PollPayload struct {
	Response          string `json:"response"`
	ResponseSignature string `json:"responseSignature"`
}

// AppVerifyRequest is the request to verify app authentication.
type AppVerifyRequest struct {
	EncAuth                string `json:"encAuth"`
	FrontEndProcessingTime int    `json:"frontEndProcessingTime"`
}

// FinalizationResponse contains the authorization code.
type FinalizationResponse struct {
	AuthorizationCode string `json:"authorizationCode"`
}

// QRData represents the data encoded in MitID QR codes.
type QRData struct {
	Version     int    `json:"v"`  // Always 1
	Part        int    `json:"p"`  // 1 or 2 (QR code pair)
	Type        int    `json:"t"`  // Always 2 for TQR
	HalfData    string `json:"h"`  // Half of channelBindingValue
	UpdateCount int    `json:"uc"` // Update counter
}

// Client is the main MitID authentication client.
type Client struct {
	httpClient *http.Client
	qrDir      string
	qrMutex    sync.RWMutex
	qrStopChan chan struct{}
	qrRunning  bool

	// Base URL for MitID API (production or test)
	baseURL string

	// Session identifiers
	clientHash              string
	authenticationSessionID string

	// Session context for flow value proofs
	brokerSecurityContext string
	serviceProviderName   string
	referenceTextHeader   string
	referenceTextBody     string

	// Current authenticator state
	userID                           string
	currentAuthenticatorType         string
	currentAuthenticatorSessionFlowKey string
	currentAuthenticatorEAFEHash     string
	currentAuthenticatorSessionID    string

	// Finalization
	finalizationAuthSessionID string
}
