package crypto

import (
	"bytes"
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockCachingMaterialsManager implements a simple materials manager for testing
type MockCachingMaterialsManager struct {
	material *Material
}

func NewMockCachingMaterialsManager() *MockCachingMaterialsManager {
	return &MockCachingMaterialsManager{
		material: &Material{
			PlaintextKey: []byte("0123456789ABCDEF0123456789ABCDEF"), // 32-byte AES-256 key
			EncryptedKey: []byte("encrypted-key-for-testing"),
		},
	}
}

func (m *MockCachingMaterialsManager) GetMaterial(ctx context.Context, cryptoCtx CryptoContext) (*Material, error) {
	return m.material, nil
}

func (m *MockCachingMaterialsManager) GetDecryptionMaterial(ctx context.Context, cryptoCtx CryptoContext, encryptedKey []byte) (*Material, error) {
	// Verify the encrypted key matches what we expect
	if !bytes.Equal(encryptedKey, m.material.EncryptedKey) {
		return nil, nil
	}
	return m.material, nil
}

func (m *MockCachingMaterialsManager) DecryptMaterial(ctx context.Context, cryptoCtx CryptoContext, material *Material) (*Material, error) {
	// Verify the encrypted key matches what we expect
	if !bytes.Equal(material.EncryptedKey, m.material.EncryptedKey) {
		return nil, nil
	}
	return m.material, nil
}

func TestEncryptDecrypt(t *testing.T) {
	tests := []struct {
		name           string
		plaintext      []byte
		keyContext     CryptoContext
		payloadContext CryptoContext
		shouldFail     bool
	}{
		{
			name:           "Basic encryption and decryption",
			plaintext:      []byte("This is a test message"),
			keyContext:     CryptoContext{"purpose": "test"},
			payloadContext: CryptoContext{"purpose": "test"},
			shouldFail:     false,
		},
		{
			name:           "Empty plaintext",
			plaintext:      []byte{},
			keyContext:     CryptoContext{"purpose": "test"},
			payloadContext: CryptoContext{"purpose": "test"},
			shouldFail:     false,
		},
		{
			name:           "Different contexts",
			plaintext:      []byte("Message with different contexts"),
			keyContext:     CryptoContext{"purpose": "encryption"},
			payloadContext: CryptoContext{"purpose": "authentication", "user": "test"},
			shouldFail:     false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock materials manager
			mm := NewMockCachingMaterialsManager()
			cipher := NewCipher(mm)

			// Encrypt
			ctx := context.Background()
			encryptInput := &EncryptInput{
				Plaintext:      tt.plaintext,
				KeyContext:     tt.keyContext,
				PayloadContext: tt.payloadContext,
			}

			ciphertext, encryptedKey, err := cipher.Encrypt(ctx, encryptInput)
			if tt.shouldFail {
				assert.Error(t, err)
				return
			}
			require.NoError(t, err)
			assert.NotNil(t, ciphertext)
			assert.NotEmpty(t, ciphertext)
			assert.Equal(t, mm.material.EncryptedKey, encryptedKey)

			// Decrypt
			decryptInput := &DecryptInput{
				Ciphertext:     ciphertext,
				EncryptedKey:   encryptedKey,
				KeyContext:     tt.keyContext,
				PayloadContext: tt.payloadContext,
			}

			decrypted, err := cipher.Decrypt(ctx, decryptInput)
			require.NoError(t, err)

			// Handle nil vs empty slice comparison
			if len(tt.plaintext) == 0 {
				assert.Empty(t, decrypted, "Expected empty decrypted result")
			} else {
				assert.Equal(t, tt.plaintext, decrypted)
			}
		})
	}
}

func TestDecryptWithWrongContext(t *testing.T) {
	// Create mock materials manager
	mm := NewMockCachingMaterialsManager()
	cipher := NewCipher(mm)

	// Original contexts
	keyContext := CryptoContext{"purpose": "test"}
	payloadContext := CryptoContext{"purpose": "test", "user": "alice"}

	// Encrypt with original contexts
	plaintext := []byte("Secret message")
	ctx := context.Background()

	encryptInput := &EncryptInput{
		Plaintext:      plaintext,
		KeyContext:     keyContext,
		PayloadContext: payloadContext,
	}

	ciphertext, encryptedKey, err := cipher.Encrypt(ctx, encryptInput)
	require.NoError(t, err)

	// Try to decrypt with wrong payload context
	wrongPayloadContext := CryptoContext{"purpose": "test", "user": "bob"}

	decryptInput := &DecryptInput{
		Ciphertext:     ciphertext,
		EncryptedKey:   encryptedKey,
		KeyContext:     keyContext,
		PayloadContext: wrongPayloadContext,
	}

	_, err = cipher.Decrypt(ctx, decryptInput)
	assert.Error(t, err, "Decryption should fail with wrong authentication context")
}

func TestDecryptWithTamperedCiphertext(t *testing.T) {
	// Create mock materials manager
	mm := NewMockCachingMaterialsManager()
	cipher := NewCipher(mm)

	// Contexts
	keyContext := CryptoContext{"purpose": "test"}
	payloadContext := CryptoContext{"purpose": "test"}

	// Encrypt
	plaintext := []byte("Secret message")
	ctx := context.Background()

	encryptInput := &EncryptInput{
		Plaintext:      plaintext,
		KeyContext:     keyContext,
		PayloadContext: payloadContext,
	}

	ciphertext, encryptedKey, err := cipher.Encrypt(ctx, encryptInput)
	require.NoError(t, err)

	// Tamper with the ciphertext (change the last byte)
	if len(ciphertext) > 0 {
		tamperedCiphertext := make([]byte, len(ciphertext))
		copy(tamperedCiphertext, ciphertext)
		tamperedCiphertext[len(tamperedCiphertext)-1] ^= 0x01 // Flip a bit

		// Try to decrypt the tampered ciphertext
		decryptInput := &DecryptInput{
			Ciphertext:     tamperedCiphertext,
			EncryptedKey:   encryptedKey,
			KeyContext:     keyContext,
			PayloadContext: payloadContext,
		}

		_, err = cipher.Decrypt(ctx, decryptInput)
		assert.Error(t, err, "Decryption should fail with tampered ciphertext")
	}
}

func TestCiphertextTooShort(t *testing.T) {
	// Create mock materials manager
	mm := NewMockCachingMaterialsManager()
	cipher := NewCipher(mm)

	// Try to decrypt a ciphertext that's too short
	shortCiphertext := []byte{1, 2, 3} // Too short to contain a nonce
	ctx := context.Background()

	decryptInput := &DecryptInput{
		Ciphertext:     shortCiphertext,
		EncryptedKey:   mm.material.EncryptedKey,
		KeyContext:     CryptoContext{},
		PayloadContext: CryptoContext{},
	}

	_, err := cipher.Decrypt(ctx, decryptInput)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "ciphertext too short")
}
