// Package broker provides broker API integration functionality.
package broker

import (
	"crypto/aes"
	"crypto/cipher"
	"crypto/rand"
	"crypto/sha256"
	"errors"
	"fmt"

	"golang.org/x/crypto/pbkdf2"
)

const (
	// KeySize is the size of the AES-256 key in bytes.
	KeySize = 32
	// SaltSize is the size of the salt for PBKDF2.
	SaltSize = 16
	// NonceSize is the size of the GCM nonce.
	NonceSize = 12
	// PBKDF2Iterations is the number of iterations for key derivation.
	PBKDF2Iterations = 100000
)

var (
	ErrInvalidKey       = errors.New("invalid encryption key: must be at least 32 characters")
	ErrInvalidCiphertext = errors.New("invalid ciphertext")
	ErrDecryptionFailed = errors.New("decryption failed")
)

// Encryptor handles credential encryption and decryption.
type Encryptor struct {
	masterKey []byte
}

// NewEncryptor creates a new Encryptor with the given master secret.
// The secret should be at least 32 characters for security.
func NewEncryptor(secret string) (*Encryptor, error) {
	if len(secret) < 32 {
		return nil, ErrInvalidKey
	}
	// Use SHA-256 to normalize the key length
	hash := sha256.Sum256([]byte(secret))
	return &Encryptor{masterKey: hash[:]}, nil
}

// DeriveKey derives a unique encryption key using PBKDF2 with the user ID as additional salt.
func (e *Encryptor) DeriveKey(userID int64) []byte {
	// Combine master key with user ID for user-specific derivation
	salt := fmt.Sprintf("user:%d", userID)
	return pbkdf2.Key(e.masterKey, []byte(salt), PBKDF2Iterations, KeySize, sha256.New)
}

// Encrypt encrypts plaintext using AES-256-GCM with a user-specific key.
// Returns the ciphertext and the nonce (IV) used for encryption.
func (e *Encryptor) Encrypt(plaintext string, userID int64) (ciphertext, nonce []byte, err error) {
	key := e.DeriveKey(userID)

	block, err := aes.NewCipher(key)
	if err != nil {
		return nil, nil, fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return nil, nil, fmt.Errorf("creating GCM: %w", err)
	}

	nonce = make([]byte, gcm.NonceSize())
	if _, err := rand.Read(nonce); err != nil {
		return nil, nil, fmt.Errorf("generating nonce: %w", err)
	}

	ciphertext = gcm.Seal(nil, nonce, []byte(plaintext), nil)
	return ciphertext, nonce, nil
}

// Decrypt decrypts ciphertext using AES-256-GCM with a user-specific key.
func (e *Encryptor) Decrypt(ciphertext, nonce []byte, userID int64) (string, error) {
	if len(ciphertext) == 0 || len(nonce) == 0 {
		return "", ErrInvalidCiphertext
	}

	key := e.DeriveKey(userID)

	block, err := aes.NewCipher(key)
	if err != nil {
		return "", fmt.Errorf("creating cipher: %w", err)
	}

	gcm, err := cipher.NewGCM(block)
	if err != nil {
		return "", fmt.Errorf("creating GCM: %w", err)
	}

	if len(nonce) != gcm.NonceSize() {
		return "", ErrInvalidCiphertext
	}

	plaintext, err := gcm.Open(nil, nonce, ciphertext, nil)
	if err != nil {
		return "", ErrDecryptionFailed
	}

	return string(plaintext), nil
}
