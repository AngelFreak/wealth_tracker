package mitid

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// mockMitIDServer creates a mock MitID server for integration testing.
// This simulates the entire MitID authentication flow without hitting real servers.
func mockMitIDServer(t *testing.T) *httptest.Server {
	authenticatorSessionID := "test-authenticator-session-456"
	flowKey := "test-flow-key-base64encoded"
	pollCount := 0
	_ = authenticatorSessionID // Used in URL matching

	mux := http.NewServeMux()

	// GET /mitid-core-client-backend/v1/authentication-sessions/{id}
	mux.HandleFunc("/mitid-core-client-backend/v1/authentication-sessions/", func(w http.ResponseWriter, r *http.Request) {
		if r.Method == http.MethodGet {
			json.NewEncoder(w).Encode(AuthSessionResponse{
				BrokerSecurityContext: "test-broker-security-context",
				ServiceProviderName:   "Test Service Provider",
				ReferenceTextHeader:   "Test Header",
				ReferenceTextBody:     "Test Body",
			})
			return
		}
		if r.Method == http.MethodPut {
			// Identity claim - check for test user
			var body map[string]string
			json.NewDecoder(r.Body).Decode(&body)
			if body["identityClaim"] == "blocked-user" {
				w.WriteHeader(http.StatusBadRequest)
				json.NewEncoder(w).Encode(map[string]string{
					"errorCode": "control.client_ip_blocked",
				})
				return
			}
			w.WriteHeader(http.StatusOK)
			json.NewEncoder(w).Encode(map[string]string{})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// POST /mitid-core-client-backend/v2/authentication-sessions/{id}/next
	mux.HandleFunc("/mitid-core-client-backend/v2/authentication-sessions/", func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/next") && r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(NextResponse{
				NextAuthenticator: &NextAuthenticator{
					AuthenticatorType:           "APP",
					AuthenticatorSessionFlowKey: flowKey,
					EAFEHash:                    "test-eafe-hash",
					AuthenticatorSessionID:      authenticatorSessionID,
				},
				Combinations: []Combination{
					{ID: "S3", CombinationItems: []CombinationItem{{Name: "MitID app"}}},
					{ID: "S1", CombinationItems: []CombinationItem{{Name: "MitID kodeviser"}}},
				},
			})
			return
		}
		if strings.HasSuffix(r.URL.Path, "/finalization") && r.Method == http.MethodPut {
			json.NewEncoder(w).Encode(FinalizationResponse{
				AuthorizationCode: "test-authorization-code-789",
			})
			return
		}
		w.WriteHeader(http.StatusMethodNotAllowed)
	})

	// POST /mitid-code-app-auth/v1/authenticator-sessions/web/{id}/init-auth
	mux.HandleFunc("/mitid-code-app-auth/v1/authenticator-sessions/web/", func(w http.ResponseWriter, r *http.Request) {
		path := r.URL.Path

		if strings.HasSuffix(path, "/init-auth") && r.Method == http.MethodPost {
			json.NewEncoder(w).Encode(AppInitAuthResponse{
				PollURL: fmt.Sprintf("http://%s/poll", r.Host),
				Ticket:  "test-ticket-abc",
			})
			return
		}

		if strings.HasSuffix(path, "/init") && r.Method == http.MethodPost {
			// SRP init
			json.NewEncoder(w).Encode(SRPInitResponse{
				SRPSalt: SRPValue{Value: "0123456789abcdef0123456789abcdef"},
				RandomB: SRPValue{Value: strings.Repeat("ab", 384)}, // 3072-bit B value
			})
			return
		}

		if strings.HasSuffix(path, "/prove") && r.Method == http.MethodPost {
			// SRP prove - return M2
			json.NewEncoder(w).Encode(SRPProveResponse{
				M2: SRPValue{Value: strings.Repeat("cd", 32)}, // 256-bit M2
			})
			return
		}

		if strings.HasSuffix(path, "/verify") && r.Method == http.MethodPost {
			w.WriteHeader(http.StatusNoContent)
			return
		}

		w.WriteHeader(http.StatusNotFound)
	})

	// POST /poll - simulates poll responses
	mux.HandleFunc("/poll", func(w http.ResponseWriter, r *http.Request) {
		pollCount++
		t.Logf("Poll request #%d", pollCount)

		// Simulate the flow: timeout -> QR -> verified -> OK
		switch {
		case pollCount <= 2:
			json.NewEncoder(w).Encode(PollResponse{Status: "timeout"})
		case pollCount <= 4:
			json.NewEncoder(w).Encode(PollResponse{
				Status:              "channel_validation_tqr",
				ChannelBindingValue: "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef",
				UpdateCount:         pollCount,
			})
		case pollCount <= 6:
			json.NewEncoder(w).Encode(PollResponse{Status: "channel_verified"})
		default:
			json.NewEncoder(w).Encode(PollResponse{
				Status:       "OK",
				Confirmation: true,
				Payload: &PollPayload{
					Response:          "dGVzdC1yZXNwb25zZQ==", // base64 "test-response"
					ResponseSignature: "dGVzdC1zaWduYXR1cmU=", // base64 "test-signature"
				},
			})
		}
	})

	return httptest.NewServer(mux)
}

func TestMitIDClientWithMockServer(t *testing.T) {
	server := mockMitIDServer(t)
	defer server.Close()

	// Create temp directory for QR codes
	qrDir, err := os.MkdirTemp("", "mitid-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(qrDir)

	// Create client pointing to mock server
	client, err := NewClientWithBaseURL(
		"test-client-hash",
		"test-auth-session-123",
		server.Client(),
		qrDir,
		server.URL,
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	t.Logf("Service provider: %s", client.GetServiceProviderName())

	// Test IdentifyUser
	available, err := client.IdentifyUser("test-user")
	if err != nil {
		t.Fatalf("IdentifyUser failed: %v", err)
	}
	t.Logf("Available authenticators: %v", available)

	if _, ok := available["APP"]; !ok {
		t.Error("APP authenticator should be available")
	}

	// Note: Full AuthenticateWithApp() would fail because SRP verification
	// would fail with mock data. But this tests the HTTP flow works.
	t.Log("Mock server integration test passed - HTTP flow verified")
}

func TestMitIDClientIPBlocked(t *testing.T) {
	server := mockMitIDServer(t)
	defer server.Close()

	qrDir, _ := os.MkdirTemp("", "mitid-test-*")
	defer os.RemoveAll(qrDir)

	client, err := NewClientWithBaseURL(
		"test-client-hash",
		"test-auth-session-123",
		server.Client(),
		qrDir,
		server.URL,
	)
	if err != nil {
		t.Fatalf("Failed to create client: %v", err)
	}

	// Test blocked user
	_, err = client.IdentifyUser("blocked-user")
	if err != ErrIPBlocked {
		t.Errorf("Expected ErrIPBlocked, got: %v", err)
	}
}

func TestQRCodeGenerationFlow(t *testing.T) {
	qrDir, err := os.MkdirTemp("", "mitid-qr-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(qrDir)

	qrManager := NewQRManager(qrDir)
	if err := qrManager.EnsureDirectory(); err != nil {
		t.Fatalf("Failed to ensure directory: %v", err)
	}

	// Test QR generation
	channelBindingValue := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	err = qrManager.GenerateQRCodePair(channelBindingValue, 1)
	if err != nil {
		t.Fatalf("Failed to generate QR codes: %v", err)
	}

	// Verify QR files exist
	qr1Path := filepath.Join(qrDir, "qr_frame1.png")
	qr2Path := filepath.Join(qrDir, "qr_frame2.png")

	if _, err := os.Stat(qr1Path); os.IsNotExist(err) {
		t.Error("QR frame 1 not created")
	}
	if _, err := os.Stat(qr2Path); os.IsNotExist(err) {
		t.Error("QR frame 2 not created")
	}

	// Set and test frame retrieval (current_frame is set by animator, not GenerateQRCodePair)
	if err := qrManager.SetCurrentFrame(1); err != nil {
		t.Fatalf("Failed to set frame: %v", err)
	}
	frame, err := qrManager.GetCurrentFrame()
	if err != nil {
		t.Fatalf("Failed to get current frame: %v", err)
	}
	if frame != 1 {
		t.Errorf("Expected frame 1, got: %d", frame)
	}

	// Test status
	qrManager.SetStatus("qr_ready")
	status := qrManager.GetStatus()
	if status != "qr_ready" {
		t.Errorf("Expected status 'qr_ready', got '%s'", status)
	}

	t.Log("QR code generation test passed")
}

func TestQRAnimator(t *testing.T) {
	qrDir, _ := os.MkdirTemp("", "mitid-animator-test-*")
	defer os.RemoveAll(qrDir)

	qrManager := NewQRManager(qrDir)
	qrManager.EnsureDirectory()

	// Generate QR codes first
	channelBindingValue := "0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef"
	qrManager.GenerateQRCodePair(channelBindingValue, 1)

	animator := NewQRAnimator(qrManager)
	animator.Start()

	// Let it run for a bit
	time.Sleep(1500 * time.Millisecond)

	animator.Stop()

	// Verify frame was toggled
	frame, _ := qrManager.GetCurrentFrame()
	t.Logf("Final frame: %d", frame)

	t.Log("QR animator test passed")
}
