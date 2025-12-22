package mitid

import (
	"math/big"
	"testing"
)

func TestSRPPrimeN(t *testing.T) {
	// Verify N is the expected 3072-bit prime (MitID uses a larger prime)
	if srpN == nil {
		t.Fatal("srpN is nil")
	}

	// Check bit length - MitID uses a 3072-bit prime
	bitLen := srpN.BitLen()
	if bitLen < 3060 || bitLen > 3080 {
		t.Errorf("srpN bit length = %d, expected ~3072", bitLen)
	}

	// Check it's the expected value (first few digits)
	nStr := srpN.String()
	expectedPrefix := "498331309206949039885270069250879547356725142258624480669494087724266457318990319293779744699206881809998695805499801233172086913629678093600950870048778996242916151585354155671959334695992953115070645733842905892650581784752485586225933343823975647446475997418998423140917075836068639262563563208439563914"
	if len(nStr) < len(expectedPrefix) || nStr[:len(expectedPrefix)] != expectedPrefix {
		t.Error("srpN does not match expected value")
	}
}

func TestSRPStage1(t *testing.T) {
	srp := NewSRP()

	aHex, err := srp.Stage1()
	if err != nil {
		t.Fatalf("Stage1 failed: %v", err)
	}

	// A should be a valid hex string
	if len(aHex) == 0 {
		t.Error("Stage1 returned empty A")
	}

	// Parse as big int to verify it's valid hex
	A := new(big.Int)
	if _, ok := A.SetString(aHex, 16); !ok {
		t.Error("Stage1 returned invalid hex")
	}

	// A should be non-zero
	if A.Sign() == 0 {
		t.Error("Stage1 returned zero A")
	}

	// A should be less than N
	if A.Cmp(srp.N) >= 0 {
		t.Error("Stage1 returned A >= N")
	}

	// Internal state should be set
	if srp.a == nil || srp.A == nil {
		t.Error("Stage1 did not set internal state")
	}
}

func TestSRPStage1Randomness(t *testing.T) {
	srp1 := NewSRP()
	srp2 := NewSRP()

	a1, _ := srp1.Stage1()
	a2, _ := srp2.Stage1()

	// Two calls should produce different A values (with very high probability)
	if a1 == a2 {
		t.Error("Stage1 produced identical A values - randomness failure")
	}
}

func TestSRPStage3ValidatesB(t *testing.T) {
	srp := NewSRP()
	srp.Stage1()

	// Test with B = 0
	_, err := srp.Stage3("salt123", "0", "password", "sessionId")
	if err == nil {
		t.Error("Stage3 should reject B = 0")
	}

	// Test with B = N (which means B mod N = 0)
	_, err = srp.Stage3("salt123", srpN.Text(16), "password", "sessionId")
	if err == nil {
		t.Error("Stage3 should reject B = N (B mod N = 0)")
	}
}

func TestSRPStage3And5(t *testing.T) {
	// This test verifies that a complete SRP exchange produces valid M1 and K
	srp := NewSRP()

	// Stage 1: Generate A
	aHex, err := srp.Stage1()
	if err != nil {
		t.Fatalf("Stage1 failed: %v", err)
	}

	// Simulate a server response (we can't verify against real server,
	// but we can verify the math doesn't panic and produces output)
	// Use a simple B value that passes validation
	B := new(big.Int)
	B.SetString("1234567890abcdef1234567890abcdef1234567890abcdef", 16)

	srpSalt := "somesaltvalue"
	password := "testpassword"
	sessionID := "test-session-id"

	// Stage 3: Compute M1
	m1Hex, err := srp.Stage3(srpSalt, B.Text(16), password, sessionID)
	if err != nil {
		t.Fatalf("Stage3 failed: %v", err)
	}

	// M1 should be a valid hex string
	if len(m1Hex) != 64 { // SHA256 produces 32 bytes = 64 hex chars
		t.Errorf("M1 length = %d, expected 64", len(m1Hex))
	}

	// K should be set
	if len(srp.KBits) != 32 {
		t.Errorf("K length = %d, expected 32", len(srp.KBits))
	}

	// Verify A is preserved
	if srp.A.Text(16) != aHex {
		t.Error("A was modified during Stage3")
	}
}

func TestSRPComputeLittleK(t *testing.T) {
	srp := NewSRP()

	// computeLittleK should be deterministic
	k1 := srp.computeLittleK()
	k2 := srp.computeLittleK()

	if k1.Cmp(k2) != 0 {
		t.Error("computeLittleK should be deterministic")
	}

	// k should be non-zero
	if k1.Sign() == 0 {
		t.Error("computeLittleK returned zero")
	}
}

func TestSRPAuthEncDec(t *testing.T) {
	srp := NewSRP()
	srp.Stage1()

	// Create a valid session by running Stage3
	B := new(big.Int)
	B.SetString("abcdef1234567890abcdef1234567890abcdef1234567890", 16)
	srp.Stage3("salt", B.Text(16), "pass", "session")

	// Now test AuthEnc/AuthDec
	plaintext := []byte("Test message for encryption")

	encrypted, err := srp.AuthEnc(plaintext)
	if err != nil {
		t.Fatalf("AuthEnc failed: %v", err)
	}

	decrypted, err := srp.AuthDec(encrypted)
	if err != nil {
		t.Fatalf("AuthDec failed: %v", err)
	}

	if string(decrypted) != string(plaintext) {
		t.Errorf("AuthDec = %q, want %q", string(decrypted), string(plaintext))
	}
}

func TestSRPGetSessionKey(t *testing.T) {
	srp := NewSRP()
	srp.Stage1()

	// Before Stage3, K should be nil
	if srp.GetSessionKey() != nil {
		t.Error("K should be nil before Stage3")
	}

	// Run Stage3
	B := new(big.Int)
	B.SetString("deadbeef1234567890abcdef1234567890abcdef1234", 16)
	srp.Stage3("salt", B.Text(16), "pass", "session")

	// After Stage3, K should be set
	k := srp.GetSessionKey()
	if k == nil {
		t.Error("K should be set after Stage3")
	}
	if len(k) != 32 {
		t.Errorf("K length = %d, expected 32", len(k))
	}
}

func TestSRPStage5Verification(t *testing.T) {
	srp := NewSRP()
	srp.Stage1()

	B := new(big.Int)
	B.SetString("cafe1234567890abcdef1234567890abcdef1234567890", 16)
	srp.Stage3("salt", B.Text(16), "pass", "session")

	// Stage5 with wrong M2 should return false
	if srp.Stage5("wrongm2value") {
		t.Error("Stage5 should return false for invalid M2")
	}

	// Note: We can't easily test successful verification without a real server
	// because we'd need to compute the correct M2 based on server-side values
}

func TestNewSRPCreatesInstance(t *testing.T) {
	srp := NewSRP()

	if srp == nil {
		t.Fatal("NewSRP returned nil")
	}

	if srp.N == nil {
		t.Error("N is nil")
	}

	if srp.g == nil {
		t.Error("g is nil")
	}

	if srp.g.Int64() != 2 {
		t.Errorf("g = %d, expected 2", srp.g.Int64())
	}
}

// Benchmark SRP operations
func BenchmarkSRPStage1(b *testing.B) {
	for i := 0; i < b.N; i++ {
		srp := NewSRP()
		srp.Stage1()
	}
}

func BenchmarkSRPStage3(b *testing.B) {
	B := new(big.Int)
	B.SetString("1234567890abcdef1234567890abcdef1234567890abcdef", 16)

	for i := 0; i < b.N; i++ {
		srp := NewSRP()
		srp.Stage1()
		srp.Stage3("salt", B.Text(16), "password", "sessionId")
	}
}
