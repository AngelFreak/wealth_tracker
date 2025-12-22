package mitid

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"fmt"
	"io"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// AES block size for GCM mode.
	aesBlockSize = 16

	// PBKDF2 iteration count used by MitID.
	pbkdf2Iterations = 20000

	// PBKDF2 key length.
	pbkdf2KeyLen = 32
)

// PKCS7Pad pads data to a multiple of blockSize using PKCS#7.
func PKCS7Pad(data []byte, blockSize int) []byte {
	padding := blockSize - (len(data) % blockSize)
	padBytes := make([]byte, padding)
	for i := range padBytes {
		padBytes[i] = byte(padding)
	}
	return append(data, padBytes...)
}

// PKCS7Unpad removes PKCS#7 padding.
func PKCS7Unpad(data []byte) ([]byte, error) {
	if len(data) == 0 {
		return nil, fmt.Errorf("empty data")
	}
	padding := int(data[len(data)-1])
	if padding > len(data) || padding == 0 {
		return nil, fmt.Errorf("invalid padding")
	}
	return data[:len(data)-padding], nil
}

// AESGCMEncrypt encrypts plaintext using AES-GCM with a random 16-byte IV.
// Returns: IV || ciphertext || tag (base64 encoded).
// Note: MitID uses 16-byte IVs which requires NewGCMWithNonceSize.
func AESGCMEncrypt(plaintext, key []byte) (string, error) {
	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	// Use 16-byte nonce to match MitID/Python implementation
	gcm, err := cipher.NewGCMWithNonceSize(block, aesBlockSize)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	// Generate random 16-byte IV
	iv := make([]byte, aesBlockSize)
	if _, err := io.ReadFull(rand.Reader, iv); err != nil {
		return "", fmt.Errorf("generating IV: %w", err)
	}

	// Encrypt and authenticate
	ciphertext := gcm.Seal(nil, iv, plaintext, nil)

	// Return IV || ciphertext || tag (tag is appended by Seal)
	result := append(iv, ciphertext...)
	return base64.StdEncoding.EncodeToString(result), nil
}

// AESGCMDecrypt decrypts base64-encoded data in format: IV || ciphertext || tag.
// Note: MitID uses 16-byte IVs which requires NewGCMWithNonceSize.
func AESGCMDecrypt(encryptedB64 string, key []byte) ([]byte, error) {
	encrypted, err := base64.StdEncoding.DecodeString(encryptedB64)
	if err != nil {
		return nil, fmt.Errorf("decoding base64: %w", err)
	}

	if len(encrypted) < aesBlockSize*2 {
		return nil, fmt.Errorf("encrypted data too short")
	}

	// Extract IV (16 bytes), ciphertext, and tag
	iv := encrypted[:aesBlockSize]
	ciphertextWithTag := encrypted[aesBlockSize:]

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, fmt.Errorf("creating cipher: %w", err)
	}

	// Use 16-byte nonce to match MitID/Python implementation
	gcm, err := cipher.NewGCMWithNonceSize(block, aesBlockSize)
	if err != nil {
		return nil, fmt.Errorf("creating GCM: %w", err)
	}

	// Decrypt and verify
	plaintext, err := gcm.Open(nil, iv, ciphertextWithTag, nil)
	if err != nil {
		return nil, fmt.Errorf("decrypting: %w", err)
	}

	return plaintext, nil
}

// DeriveKeyPBKDF2 derives a key using PBKDF2-HMAC-SHA256.
func DeriveKeyPBKDF2(password string, saltHex string) ([]byte, error) {
	salt, err := hex.DecodeString(saltHex)
	if err != nil {
		return nil, fmt.Errorf("decoding salt: %w", err)
	}

	key := pbkdf2.Key([]byte(password), salt, pbkdf2Iterations, pbkdf2KeyLen, sha256.New)
	return key, nil
}

// HMACSHA256 computes HMAC-SHA256 and returns the hex-encoded result.
func HMACSHA256(key, data []byte) string {
	h := hmac.New(sha256.New, key)
	h.Write(data)
	return hex.EncodeToString(h.Sum(nil))
}

// SHA256Hash computes SHA256 hash of data and returns hex-encoded result.
func SHA256Hash(data []byte) string {
	h := sha256.Sum256(data)
	return hex.EncodeToString(h[:])
}

// SHA256HashBytes computes SHA256 hash of data and returns raw bytes.
func SHA256HashBytes(data []byte) []byte {
	h := sha256.Sum256(data)
	return h[:]
}

// HexToBytes converts a hex string to bytes.
func HexToBytes(hexStr string) ([]byte, error) {
	return hex.DecodeString(hexStr)
}

// BytesToHex converts bytes to a hex string.
func BytesToHex(data []byte) string {
	return hex.EncodeToString(data)
}

// GenerateRandomBytes generates cryptographically secure random bytes.
func GenerateRandomBytes(n int) ([]byte, error) {
	b := make([]byte, n)
	if _, err := io.ReadFull(rand.Reader, b); err != nil {
		return nil, err
	}
	return b, nil
}

// GenerateRandomHex generates a random hex string of specified byte length.
func GenerateRandomHex(byteLen int) (string, error) {
	b, err := GenerateRandomBytes(byteLen)
	if err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

// Base64Encode encodes bytes to standard base64.
func Base64Encode(data []byte) string {
	return base64.StdEncoding.EncodeToString(data)
}

// Base64Decode decodes standard base64 to bytes.
func Base64Decode(s string) ([]byte, error) {
	return base64.StdEncoding.DecodeString(s)
}

// Base64URLEncode encodes bytes to URL-safe base64 without padding.
func Base64URLEncode(data []byte) string {
	return base64.RawURLEncoding.EncodeToString(data)
}

// Base64URLDecode decodes URL-safe base64 (with or without padding).
func Base64URLDecode(s string) ([]byte, error) {
	// Try without padding first
	data, err := base64.RawURLEncoding.DecodeString(s)
	if err != nil {
		// Try with padding
		return base64.URLEncoding.DecodeString(s)
	}
	return data, nil
}

// DerivePinKey derives the PIN key from session key K.
// pin_key = SHA256(hex(K) + "PIN")
func DerivePinKey(sessionKeyK []byte) []byte {
	data := append([]byte(hex.EncodeToString(sessionKeyK)), []byte("PIN")...)
	return SHA256HashBytes(data)
}

// CreateFlowValueProofKey creates the key for HMAC flow value proof.
// key = SHA256(prefix + hex(K))
func CreateFlowValueProofKey(prefix string, sessionKeyK []byte) []byte {
	data := prefix + hex.EncodeToString(sessionKeyK)
	return SHA256HashBytes([]byte(data))
}
