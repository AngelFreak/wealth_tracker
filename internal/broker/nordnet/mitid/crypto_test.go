package mitid

import (
	"bytes"
	"encoding/hex"
	"testing"
)

func TestPKCS7Padding(t *testing.T) {
	tests := []struct {
		name      string
		input     []byte
		blockSize int
		expected  []byte
	}{
		{
			name:      "pad 15 bytes to 16",
			input:     []byte("15byteslong...."),
			blockSize: 16,
			expected:  append([]byte("15byteslong...."), 0x01),
		},
		{
			name:      "pad 8 bytes to 16",
			input:     []byte("8bytes!"),
			blockSize: 16,
			expected:  append([]byte("8bytes!"), bytes.Repeat([]byte{0x09}, 9)...),
		},
		{
			name:      "full block adds full padding",
			input:     []byte("exactly16bytes!!"),
			blockSize: 16,
			expected:  append([]byte("exactly16bytes!!"), bytes.Repeat([]byte{0x10}, 16)...),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PKCS7Pad(tt.input, tt.blockSize)
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("PKCS7Pad(%v, %d) = %v, want %v", tt.input, tt.blockSize, result, tt.expected)
			}
		})
	}
}

func TestPKCS7Unpad(t *testing.T) {
	tests := []struct {
		name     string
		input    []byte
		expected []byte
		hasError bool
	}{
		{
			name:     "unpad single byte padding",
			input:    append([]byte("15byteslong...."), 0x01),
			expected: []byte("15byteslong...."),
			hasError: false,
		},
		{
			name:     "unpad 9 bytes",
			input:    append([]byte("8bytes!"), bytes.Repeat([]byte{0x09}, 9)...),
			expected: []byte("8bytes!"),
			hasError: false,
		},
		{
			name:     "empty input",
			input:    []byte{},
			expected: nil,
			hasError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := PKCS7Unpad(tt.input)
			if tt.hasError && err == nil {
				t.Errorf("PKCS7Unpad(%v) expected error, got nil", tt.input)
			}
			if !tt.hasError && err != nil {
				t.Errorf("PKCS7Unpad(%v) unexpected error: %v", tt.input, err)
			}
			if !bytes.Equal(result, tt.expected) {
				t.Errorf("PKCS7Unpad(%v) = %v, want %v", tt.input, result, tt.expected)
			}
		})
	}
}

func TestAESGCMRoundtrip(t *testing.T) {
	key := make([]byte, 32)
	for i := range key {
		key[i] = byte(i)
	}

	plaintext := []byte("Hello, MitID World! This is a test message for AES-GCM encryption.")

	encrypted, err := AESGCMEncrypt(plaintext, key)
	if err != nil {
		t.Fatalf("AESGCMEncrypt failed: %v", err)
	}

	decrypted, err := AESGCMDecrypt(encrypted, key)
	if err != nil {
		t.Fatalf("AESGCMDecrypt failed: %v", err)
	}

	if !bytes.Equal(decrypted, plaintext) {
		t.Errorf("Roundtrip failed: got %v, want %v", decrypted, plaintext)
	}
}

func TestAESGCMDecryptFailsWithWrongKey(t *testing.T) {
	key1 := make([]byte, 32)
	key2 := make([]byte, 32)
	for i := range key1 {
		key1[i] = byte(i)
		key2[i] = byte(i + 1)
	}

	plaintext := []byte("Secret message")

	encrypted, err := AESGCMEncrypt(plaintext, key1)
	if err != nil {
		t.Fatalf("AESGCMEncrypt failed: %v", err)
	}

	_, err = AESGCMDecrypt(encrypted, key2)
	if err == nil {
		t.Error("AESGCMDecrypt with wrong key should fail")
	}
}

func TestSHA256Hash(t *testing.T) {
	// Test vector: SHA256("test")
	input := []byte("test")
	expected := "9f86d081884c7d659a2feaa0c55ad015a3bf4f1b2b0b822cd15d6c15b0f00a08"

	result := SHA256Hash(input)
	if result != expected {
		t.Errorf("SHA256Hash(%q) = %q, want %q", string(input), result, expected)
	}
}

func TestHMACSHA256(t *testing.T) {
	// Test vector
	key := []byte("key")
	data := []byte("The quick brown fox jumps over the lazy dog")
	expected := "f7bc83f430538424b13298e6aa6fb143ef4d59a14946175997479dbc2d1a3cd8"

	result := HMACSHA256(key, data)
	if result != expected {
		t.Errorf("HMACSHA256 = %q, want %q", result, expected)
	}
}

func TestHexConversion(t *testing.T) {
	original := []byte{0xDE, 0xAD, 0xBE, 0xEF}
	hexStr := BytesToHex(original)

	if hexStr != "deadbeef" {
		t.Errorf("BytesToHex = %q, want %q", hexStr, "deadbeef")
	}

	back, err := HexToBytes(hexStr)
	if err != nil {
		t.Fatalf("HexToBytes failed: %v", err)
	}

	if !bytes.Equal(back, original) {
		t.Errorf("HexToBytes = %v, want %v", back, original)
	}
}

func TestBase64Encoding(t *testing.T) {
	original := []byte("Hello, World!")
	encoded := Base64Encode(original)

	decoded, err := Base64Decode(encoded)
	if err != nil {
		t.Fatalf("Base64Decode failed: %v", err)
	}

	if !bytes.Equal(decoded, original) {
		t.Errorf("Base64 roundtrip failed: got %v, want %v", decoded, original)
	}
}

func TestDerivePinKey(t *testing.T) {
	// Test that DerivePinKey produces consistent output
	sessionKey := make([]byte, 32)
	for i := range sessionKey {
		sessionKey[i] = byte(i)
	}

	key1 := DerivePinKey(sessionKey)
	key2 := DerivePinKey(sessionKey)

	if !bytes.Equal(key1, key2) {
		t.Error("DerivePinKey should produce consistent output")
	}

	if len(key1) != 32 {
		t.Errorf("DerivePinKey should return 32 bytes, got %d", len(key1))
	}
}

func TestCreateFlowValueProofKey(t *testing.T) {
	sessionKey, _ := hex.DecodeString("0123456789abcdef0123456789abcdef0123456789abcdef0123456789abcdef")

	key1 := CreateFlowValueProofKey("flowValues", sessionKey)
	key2 := CreateFlowValueProofKey("flowValues", sessionKey)

	if !bytes.Equal(key1, key2) {
		t.Error("CreateFlowValueProofKey should produce consistent output")
	}

	if len(key1) != 32 {
		t.Errorf("CreateFlowValueProofKey should return 32 bytes, got %d", len(key1))
	}

	// Different prefix should produce different key
	key3 := CreateFlowValueProofKey("OTP123456", sessionKey)
	if bytes.Equal(key1, key3) {
		t.Error("Different prefixes should produce different keys")
	}
}

func TestGenerateRandomBytes(t *testing.T) {
	bytes1, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	bytes2, err := GenerateRandomBytes(32)
	if err != nil {
		t.Fatalf("GenerateRandomBytes failed: %v", err)
	}

	if len(bytes1) != 32 {
		t.Errorf("GenerateRandomBytes returned %d bytes, want 32", len(bytes1))
	}

	// Random bytes should be different
	if bytes.Equal(bytes1, bytes2) {
		t.Error("GenerateRandomBytes should produce different values on each call")
	}
}
