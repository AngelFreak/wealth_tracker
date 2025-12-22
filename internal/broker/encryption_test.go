package broker

import (
	"testing"
)

func TestNewEncryptor_ValidSecret(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, err := NewEncryptor(secret)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v, want nil", err)
	}
	if enc == nil {
		t.Fatal("NewEncryptor() returned nil")
	}
}

func TestNewEncryptor_ShortSecret(t *testing.T) {
	secret := "short"
	_, err := NewEncryptor(secret)
	if err != ErrInvalidKey {
		t.Errorf("NewEncryptor() error = %v, want %v", err, ErrInvalidKey)
	}
}

func TestEncryptor_RoundTrip(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, err := NewEncryptor(secret)
	if err != nil {
		t.Fatalf("NewEncryptor() error = %v", err)
	}

	testCases := []struct {
		name      string
		plaintext string
		userID    int64
	}{
		{"simple password", "mypassword123", 1},
		{"complex password", "P@ssw0rd!#$%^&*()", 2},
		{"unicode password", "пароль密码🔐", 3},
		{"empty password", "", 4},
		{"long password", "this-is-a-very-long-password-that-should-still-work-correctly-even-with-many-characters", 5},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Encrypt
			ciphertext, nonce, err := enc.Encrypt(tc.plaintext, tc.userID)
			if err != nil {
				t.Fatalf("Encrypt() error = %v", err)
			}

			// Verify ciphertext is different from plaintext
			if tc.plaintext != "" && string(ciphertext) == tc.plaintext {
				t.Error("ciphertext should not equal plaintext")
			}

			// Decrypt
			decrypted, err := enc.Decrypt(ciphertext, nonce, tc.userID)
			if err != nil {
				t.Fatalf("Decrypt() error = %v", err)
			}

			// Verify round-trip
			if decrypted != tc.plaintext {
				t.Errorf("Decrypt() = %q, want %q", decrypted, tc.plaintext)
			}
		})
	}
}

func TestEncryptor_DifferentUsersGetDifferentKeys(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, _ := NewEncryptor(secret)

	plaintext := "same-password"
	userID1 := int64(1)
	userID2 := int64(2)

	// Encrypt same plaintext for different users
	ciphertext1, nonce1, _ := enc.Encrypt(plaintext, userID1)
	ciphertext2, nonce2, _ := enc.Encrypt(plaintext, userID2)

	// Verify decryption works for correct user
	decrypted1, err := enc.Decrypt(ciphertext1, nonce1, userID1)
	if err != nil || decrypted1 != plaintext {
		t.Errorf("Decrypt with correct userID failed")
	}

	// Verify decryption fails with wrong user
	_, err = enc.Decrypt(ciphertext1, nonce1, userID2)
	if err == nil {
		t.Error("Decrypt with wrong userID should fail")
	}

	// Verify ciphertexts are different (due to random nonce and different keys)
	// Note: Even with same plaintext, different nonces and keys produce different ciphertexts
	_ = nonce2 // nonce2 is different but we only need to verify ciphertexts differ
	if string(ciphertext1) == string(ciphertext2) {
		t.Error("ciphertexts should be different for different users")
	}
}

func TestEncryptor_DifferentEncryptionsProduceDifferentCiphertexts(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, _ := NewEncryptor(secret)

	plaintext := "test-password"
	userID := int64(1)

	// Encrypt same plaintext twice
	ciphertext1, nonce1, _ := enc.Encrypt(plaintext, userID)
	ciphertext2, nonce2, _ := enc.Encrypt(plaintext, userID)

	// Nonces should be different (random)
	if string(nonce1) == string(nonce2) {
		t.Error("nonces should be different for each encryption")
	}

	// Ciphertexts should be different (due to different nonces)
	if string(ciphertext1) == string(ciphertext2) {
		t.Error("ciphertexts should be different for each encryption")
	}

	// Both should still decrypt correctly
	decrypted1, _ := enc.Decrypt(ciphertext1, nonce1, userID)
	decrypted2, _ := enc.Decrypt(ciphertext2, nonce2, userID)

	if decrypted1 != plaintext || decrypted2 != plaintext {
		t.Error("both ciphertexts should decrypt to original plaintext")
	}
}

func TestEncryptor_DecryptInvalidInputs(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, _ := NewEncryptor(secret)

	testCases := []struct {
		name       string
		ciphertext []byte
		nonce      []byte
		wantErr    error
	}{
		{"nil ciphertext", nil, []byte("123456789012"), ErrInvalidCiphertext},
		{"empty ciphertext", []byte{}, []byte("123456789012"), ErrInvalidCiphertext},
		{"nil nonce", []byte("ciphertext"), nil, ErrInvalidCiphertext},
		{"empty nonce", []byte("ciphertext"), []byte{}, ErrInvalidCiphertext},
		{"wrong nonce size", []byte("ciphertext"), []byte("short"), ErrInvalidCiphertext},
		{"corrupted ciphertext", []byte("corrupted"), make([]byte, 12), ErrDecryptionFailed},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := enc.Decrypt(tc.ciphertext, tc.nonce, 1)
			if err != tc.wantErr {
				t.Errorf("Decrypt() error = %v, want %v", err, tc.wantErr)
			}
		})
	}
}

func TestEncryptor_DeriveKey_Deterministic(t *testing.T) {
	secret := "this-is-a-valid-32-character-key"
	enc, _ := NewEncryptor(secret)

	userID := int64(42)

	// Derive key multiple times
	key1 := enc.DeriveKey(userID)
	key2 := enc.DeriveKey(userID)

	// Keys should be identical
	if string(key1) != string(key2) {
		t.Error("DeriveKey should be deterministic for same inputs")
	}

	// Key should be 32 bytes (AES-256)
	if len(key1) != KeySize {
		t.Errorf("DeriveKey() length = %d, want %d", len(key1), KeySize)
	}
}
