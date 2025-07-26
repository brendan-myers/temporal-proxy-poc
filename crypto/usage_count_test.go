package crypto

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestUsageCount(t *testing.T) {
	keyID := os.Getenv("KMS_KEY_ID")
	if keyID == "" {
		t.Skip("Skipping test: KMS_KEY_ID environment variable not set")
	}

	// Initialize AWS session and KMS client
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	kmsClient := kms.New(sess)

	// Create the AWS KMS provider
	awsProvider := NewAWSKMSProvider(kmsClient, AWSKMSOptions{KeyID: keyID})

	// Create the caching materials manager with specific usage limit
	maxUsage := 3
	cachingMM, err := NewCachingMaterialsManager(
		awsProvider,
		CachingConfig{
			MaxCache:        MaxCacheSize,
			MaxAge:          1 * time.Hour, // Long TTL to focus on usage count
			MaxMessagesUsed: maxUsage,      // Low usage count for testing
		},
		nil, // MetricsHandler
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	// Test context
	cryptoCtx := CryptoContext{"purpose": "usage-count-test"}
	ctx := context.Background()

	// Get the first material directly to inspect it
	material1, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get first material")

	// Verify initial usage count
	assert.Equal(t, 1, material1.UsageCount, "Initial usage count should be 1")

	// Use the material up to max usage
	for i := 2; i <= maxUsage; i++ {
		material, err := cachingMM.GetMaterial(ctx, cryptoCtx)
		require.NoError(t, err, "Failed to get material on iteration %d", i)

		// Verify it's the same material (by comparing encrypted key)
		assert.Equal(t, string(material1.EncryptedKey), string(material.EncryptedKey),
			"Iteration %d: Should get the same material", i)

		// Verify usage count increases
		assert.Equal(t, i, material.UsageCount, "Usage count should increment on each use")
	}

	// Get one more material - should be a new one due to max usage
	materialNew, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get new material after max usage")

	// Verify it's a different material
	assert.NotEqual(t, string(material1.EncryptedKey), string(materialNew.EncryptedKey),
		"Should get a new material after max usage")

	// Verify usage count is reset
	assert.Equal(t, 1, materialNew.UsageCount, "New material usage count should be 1")

	t.Log("Usage count limit correctly enforced")
}

func TestDecryptionWithDecryptMaterial(t *testing.T) {
	keyID := os.Getenv("KMS_KEY_ID")
	if keyID == "" {
		t.Skip("Skipping test: KMS_KEY_ID environment variable not set")
	}

	// Initialize AWS session and KMS client
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	kmsClient := kms.New(sess)

	// Create the AWS KMS provider
	awsProvider := NewAWSKMSProvider(kmsClient, AWSKMSOptions{KeyID: keyID})

	// Create the caching materials manager with specific usage limit
	maxUsage := 3 // This should not affect decryption
	cachingMM, err := NewCachingMaterialsManager(
		awsProvider,
		CachingConfig{
			MaxCache:        MaxCacheSize,
			MaxAge:          1 * time.Hour, // Long TTL to focus on usage count
			MaxMessagesUsed: maxUsage,      // Low usage count for testing
		},
		nil, // MetricsHandler
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	cipher := NewCipher(cachingMM)

	// Test data
	data := []byte("Test data for decryption usage count")
	cryptoCtx := CryptoContext{"purpose": "decryption-usage-count-test"}
	ctx := context.Background()

	// First encrypt some data to get encrypted key
	encryptInput := &EncryptInput{
		Plaintext:      data,
		KeyContext:     cryptoCtx,
		PayloadContext: cryptoCtx,
	}

	ciphertext, encryptedKey, err := cipher.Encrypt(ctx, encryptInput)
	require.NoError(t, err, "Failed to encrypt test data")

	// Create a material with just the encrypted key
	inputMaterial := &Material{
		EncryptedKey: encryptedKey,
	}

	// Use the material well beyond max usage
	// Since we don't enforce usage limits on decryption, this should work
	for i := 1; i <= maxUsage*2; i++ {
		_, err := cachingMM.DecryptMaterial(ctx, cryptoCtx, inputMaterial)
		require.NoError(t, err, "Failed to decrypt material on iteration %d", i)

		// We can't verify usage count since we're using DecryptMaterial directly
		// which doesn't use the same caching mechanism as GetDecryptionMaterial did
	}

	// Verify decryption still works after multiple uses
	decryptInput := &DecryptInput{
		Ciphertext:     ciphertext,
		EncryptedKey:   encryptedKey,
		KeyContext:     cryptoCtx,
		PayloadContext: cryptoCtx,
	}

	_, err = cipher.Decrypt(ctx, decryptInput)
	require.NoError(t, err, "Decryption failed after multiple uses")

	t.Log("Decryption works with DecryptMaterial")
}
