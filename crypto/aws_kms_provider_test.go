package crypto

import (
	"context"
	"errors"
	"testing"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/request"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/kms/kmsiface"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockKMSClient implements kmsiface.KMSAPI for testing
type MockKMSClient struct {
	kmsiface.KMSAPI
	generateDataKeyOutput *kms.GenerateDataKeyOutput
	generateDataKeyError  error
	decryptOutput         *kms.DecryptOutput
	decryptError          error
	lastEncryptionContext map[string]*string
	lastKeySpec           string
	lastKeyId             string
}

func (m *MockKMSClient) GenerateDataKeyWithContext(ctx context.Context, input *kms.GenerateDataKeyInput, opts ...request.Option) (*kms.GenerateDataKeyOutput, error) {
	m.lastEncryptionContext = input.EncryptionContext
	m.lastKeySpec = *input.KeySpec
	m.lastKeyId = *input.KeyId
	return m.generateDataKeyOutput, m.generateDataKeyError
}

func (m *MockKMSClient) GenerateDataKey(input *kms.GenerateDataKeyInput) (*kms.GenerateDataKeyOutput, error) {
	return m.GenerateDataKeyWithContext(context.Background(), input)
}

func (m *MockKMSClient) DecryptWithContext(ctx context.Context, input *kms.DecryptInput, opts ...request.Option) (*kms.DecryptOutput, error) {
	m.lastEncryptionContext = input.EncryptionContext
	return m.decryptOutput, m.decryptError
}

func (m *MockKMSClient) Decrypt(input *kms.DecryptInput) (*kms.DecryptOutput, error) {
	return m.DecryptWithContext(context.Background(), input)
}

func TestNewAWSKMSProvider(t *testing.T) {
	tests := []struct {
		name     string
		options  AWSKMSOptions
		expected string
	}{
		{
			name:     "Default KeySpec",
			options:  AWSKMSOptions{KeyID: "test-key-id"},
			expected: "AES_256",
		},
		{
			name:     "Custom KeySpec",
			options:  AWSKMSOptions{KeyID: "test-key-id", KeySpec: "RSA_2048"},
			expected: "RSA_2048",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKMS := &MockKMSClient{}
			provider := NewAWSKMSProvider(mockKMS, tt.options)

			// We can't directly access private fields, so we'll test functionality instead
			// by making a call that uses the key spec
			cryptoCtx := CryptoContext{"purpose": "test"}
			mockKMS.generateDataKeyOutput = &kms.GenerateDataKeyOutput{
				Plaintext:      []byte("test-plaintext"),
				CiphertextBlob: []byte("test-ciphertext"),
			}

			ctx := context.Background()
			_, err := provider.GetMaterial(ctx, cryptoCtx)
			require.NoError(t, err)

			assert.Equal(t, tt.options.KeyID, mockKMS.lastKeyId)
			assert.Equal(t, tt.expected, mockKMS.lastKeySpec)
		})
	}
}

func TestAWSKMSProvider_GetMaterial(t *testing.T) {
	tests := []struct {
		name              string
		context           CryptoContext
		mockOutput        *kms.GenerateDataKeyOutput
		mockError         error
		expectedPlaintext []byte
		expectedError     bool
	}{
		{
			name:    "Success",
			context: CryptoContext{"purpose": "test"},
			mockOutput: &kms.GenerateDataKeyOutput{
				Plaintext:      []byte("test-plaintext"),
				CiphertextBlob: []byte("test-ciphertext"),
			},
			mockError:         nil,
			expectedPlaintext: []byte("test-plaintext"),
			expectedError:     false,
		},
		{
			name:              "KMS Error",
			context:           CryptoContext{"purpose": "test"},
			mockOutput:        nil,
			mockError:         errors.New("KMS error"),
			expectedPlaintext: nil,
			expectedError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKMS := &MockKMSClient{
				generateDataKeyOutput: tt.mockOutput,
				generateDataKeyError:  tt.mockError,
			}

			provider := NewAWSKMSProvider(mockKMS, AWSKMSOptions{KeyID: "test-key-id"})
			ctx := context.Background()
			material, err := provider.GetMaterial(ctx, tt.context)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, material)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPlaintext, material.PlaintextKey)
				assert.Equal(t, tt.mockOutput.CiphertextBlob, material.EncryptedKey)

				// Verify encryption context was passed correctly
				for k, v := range tt.context {
					assert.Equal(t, v, *mockKMS.lastEncryptionContext[k])
				}

				// Verify key spec and key ID
				assert.Equal(t, "AES_256", mockKMS.lastKeySpec)
				assert.Equal(t, "test-key-id", mockKMS.lastKeyId)
			}
		})
	}
}

func TestAWSKMSProvider_DecryptMaterial(t *testing.T) {
	tests := []struct {
		name              string
		context           CryptoContext
		encryptedKey      []byte
		mockOutput        *kms.DecryptOutput
		mockError         error
		expectedPlaintext []byte
		expectedError     bool
	}{
		{
			name:         "Success",
			context:      CryptoContext{"purpose": "test"},
			encryptedKey: []byte("test-encrypted-key"),
			mockOutput: &kms.DecryptOutput{
				Plaintext: []byte("test-plaintext"),
				KeyId:     aws.String("test-key-id"),
			},
			mockError:         nil,
			expectedPlaintext: []byte("test-plaintext"),
			expectedError:     false,
		},
		{
			name:              "KMS Error",
			context:           CryptoContext{"purpose": "test"},
			encryptedKey:      []byte("test-encrypted-key"),
			mockOutput:        nil,
			mockError:         errors.New("KMS error"),
			expectedPlaintext: nil,
			expectedError:     true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			mockKMS := &MockKMSClient{
				decryptOutput: tt.mockOutput,
				decryptError:  tt.mockError,
			}

			provider := NewAWSKMSProvider(mockKMS, AWSKMSOptions{KeyID: "test-key-id"})
			inputMaterial := &Material{
				EncryptedKey: tt.encryptedKey,
			}
			ctx := context.Background()
			material, err := provider.DecryptMaterial(ctx, tt.context, inputMaterial)

			if tt.expectedError {
				assert.Error(t, err)
				assert.Nil(t, material)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.expectedPlaintext, material.PlaintextKey)
				assert.Equal(t, tt.encryptedKey, material.EncryptedKey)

				// Verify encryption context was passed correctly
				for k, v := range tt.context {
					assert.Equal(t, v, *mockKMS.lastEncryptionContext[k])
				}
			}
		})
	}
}

func TestAWSKMSProvider_EncryptionContextHandling(t *testing.T) {
	// Test that empty context works
	emptyContext := CryptoContext{}

	mockKMS := &MockKMSClient{
		generateDataKeyOutput: &kms.GenerateDataKeyOutput{
			Plaintext:      []byte("test-plaintext"),
			CiphertextBlob: []byte("test-ciphertext"),
		},
	}

	provider := NewAWSKMSProvider(mockKMS, AWSKMSOptions{KeyID: "test-key-id"})
	ctx := context.Background()
	_, err := provider.GetMaterial(ctx, emptyContext)
	require.NoError(t, err)
	assert.Empty(t, mockKMS.lastEncryptionContext)

	// Test that complex context is handled correctly
	complexContext := CryptoContext{
		"key1": "value1",
		"key2": "value2",
		"key3": "value3",
	}

	_, err = provider.GetMaterial(ctx, complexContext)
	require.NoError(t, err)

	assert.Equal(t, 3, len(mockKMS.lastEncryptionContext))
	assert.Equal(t, "value1", *mockKMS.lastEncryptionContext["key1"])
	assert.Equal(t, "value2", *mockKMS.lastEncryptionContext["key2"])
	assert.Equal(t, "value3", *mockKMS.lastEncryptionContext["key3"])
}
