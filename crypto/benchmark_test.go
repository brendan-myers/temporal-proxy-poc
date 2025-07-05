package crypto

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/stretchr/testify/require"
)

const (
	MaxCacheSize = 128
	MaxKeyUsage  = 100
	KeyTTL       = 5 * time.Minute
)

func setupManagers(b testing.TB) (*CachingMaterialsManager, *CachingMaterialsManager) {
	keyID := os.Getenv("KMS_KEY_ID")
	if keyID == "" {
		b.Skip("Skipping benchmark: KMS_KEY_ID environment variable not set")
	}

	// Initialize AWS session and KMS client
	sess := session.Must(session.NewSession(&aws.Config{
		Region: aws.String(os.Getenv("AWS_REGION")),
	}))
	kmsClient := kms.New(sess)

	// Create the AWS KMS provider
	awsProvider := NewAWSKMSProvider(kmsClient, KMSOptions{KeyID: keyID})

	// Create the caching materials manager
	cachingMM, err := NewCachingMaterialsManager(
		awsProvider,
		CachingConfig{
			MaxCache:        MaxCacheSize,
			MaxAge:          KeyTTL,
			MaxMessagesUsed: MaxKeyUsage,
		},
		nil, // MetricsHandler
	)
	require.NoError(b, err, "Failed to create caching materials manager")

	// Create a non-caching materials manager
	noCacheMM, err := NewCachingMaterialsManager(
		awsProvider,
		CachingConfig{
			MaxCache:        1, // minimal cache size
			MaxAge:          0, // zero TTL forces refresh
			MaxMessagesUsed: 1, // single use forces refresh
		},
		nil, // MetricsHandler
	)
	require.NoError(b, err, "Failed to create no-cache materials manager")

	return cachingMM, noCacheMM
}

func BenchmarkEncryption(b *testing.B) {
	cachingMM, noCacheMM := setupManagers(b)
	data := []byte("This is a sample text that will be encrypted for benchmarking with context")
	keyCtx := CryptoContext{"purpose": "encryption", "keyId": "benchmark"}
	payloadCtx := CryptoContext{"purpose": "authentication", "userId": "benchmark"}

	cachingCipher := NewCipher(cachingMM)
	noCacheCipher := NewCipher(noCacheMM)

	encryptInput := &EncryptInput{
		Plaintext:      data,
		KeyContext:     keyCtx,
		PayloadContext: payloadCtx,
	}

	b.Run("WithCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			_, _, err := cachingCipher.Encrypt(ctx, encryptInput)
			require.NoError(b, err, "Encryption failed")
		}
	})

	b.Run("WithoutCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			_, _, err := noCacheCipher.Encrypt(ctx, encryptInput)
			require.NoError(b, err, "Encryption failed")
		}
	})
}

func BenchmarkDecryption(b *testing.B) {
	cachingMM, noCacheMM := setupManagers(b)
	data := []byte("This is a sample text that will be encrypted for benchmarking with context")
	keyCtx := CryptoContext{"purpose": "encryption", "keyId": "benchmark"}
	payloadCtx := CryptoContext{"purpose": "authentication", "userId": "benchmark"}

	cachingCipher := NewCipher(cachingMM)
	noCacheCipher := NewCipher(noCacheMM)

	encryptInput := &EncryptInput{
		Plaintext:      data,
		KeyContext:     keyCtx,
		PayloadContext: payloadCtx,
	}

	// Pre-encrypt data for decryption benchmarks
	ctx := context.Background()
	ciphertext, encryptedKey, err := cachingCipher.Encrypt(ctx, encryptInput)
	require.NoError(b, err, "Pre-encryption failed")

	decryptInput := &DecryptInput{
		Ciphertext:     ciphertext,
		EncryptedKey:   encryptedKey,
		KeyContext:     keyCtx,
		PayloadContext: payloadCtx,
	}

	b.Run("WithCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			_, err := cachingCipher.Decrypt(ctx, decryptInput)
			require.NoError(b, err, "Decryption failed")
		}
	})

	b.Run("WithoutCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			ctx := context.Background()
			_, err := noCacheCipher.Decrypt(ctx, decryptInput)
			require.NoError(b, err, "Decryption failed")
		}
	})
}

func BenchmarkFullCycle(b *testing.B) {
	cachingMM, noCacheMM := setupManagers(b)
	data := []byte("This is a sample text that will be encrypted for benchmarking with context")
	keyCtx := CryptoContext{"purpose": "encryption", "keyId": "benchmark"}
	payloadCtx := CryptoContext{"purpose": "authentication", "userId": "benchmark"}

	cachingCipher := NewCipher(cachingMM)
	noCacheCipher := NewCipher(noCacheMM)

	b.Run("WithCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Encrypt
			ctx := context.Background()
			encryptInput := &EncryptInput{
				Plaintext:      data,
				KeyContext:     keyCtx,
				PayloadContext: payloadCtx,
			}

			ciphertext, encryptedKey, err := cachingCipher.Encrypt(ctx, encryptInput)
			require.NoError(b, err, "Encryption failed")

			// Decrypt
			decryptInput := &DecryptInput{
				Ciphertext:     ciphertext,
				EncryptedKey:   encryptedKey,
				KeyContext:     keyCtx,
				PayloadContext: payloadCtx,
			}

			_, err = cachingCipher.Decrypt(ctx, decryptInput)
			require.NoError(b, err, "Decryption failed")
		}
	})

	b.Run("WithoutCache", func(b *testing.B) {
		b.ResetTimer()
		for i := 0; i < b.N; i++ {
			// Encrypt
			ctx := context.Background()
			encryptInput := &EncryptInput{
				Plaintext:      data,
				KeyContext:     keyCtx,
				PayloadContext: payloadCtx,
			}

			ciphertext, encryptedKey, err := noCacheCipher.Encrypt(ctx, encryptInput)
			require.NoError(b, err, "Encryption failed")

			// Decrypt
			decryptInput := &DecryptInput{
				Ciphertext:     ciphertext,
				EncryptedKey:   encryptedKey,
				KeyContext:     keyCtx,
				PayloadContext: payloadCtx,
			}

			_, err = noCacheCipher.Decrypt(ctx, decryptInput)
			require.NoError(b, err, "Decryption failed")
		}
	})
}

func TestCachingBehavior(t *testing.T) {
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
	awsProvider := NewAWSKMSProvider(kmsClient, KMSOptions{KeyID: keyID})

	// Create the caching materials manager with short TTL for testing
	cachingMM, err := NewCachingMaterialsManager(
		awsProvider,
		CachingConfig{
			MaxCache:        MaxCacheSize,
			MaxAge:          100 * time.Millisecond, // Very short TTL for testing
			MaxMessagesUsed: 5,                      // Low usage count for testing
		},
		nil, // MetricsHandler
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	cipher := NewCipher(cachingMM)

	// Test encryption caching
	t.Run("EncryptionCaching", func(t *testing.T) {
		data := []byte("Test data with context")
		keyCtx := CryptoContext{"purpose": "encryption", "keyId": "test"}
		payloadCtx := CryptoContext{"purpose": "authentication", "userId": "test"}

		encryptInput := &EncryptInput{
			Plaintext:      data,
			KeyContext:     keyCtx,
			PayloadContext: payloadCtx,
		}

		// First encryption - should generate a new key
		startFirst := time.Now()
		ctx := context.Background()
		ciphertext1, encryptedKey1, err := cipher.Encrypt(ctx, encryptInput)
		require.NoError(t, err, "First encryption failed")
		firstDuration := time.Since(startFirst)

		// Second encryption - should use cached key (faster)
		startSecond := time.Now()
		_, _, err = cipher.Encrypt(ctx, encryptInput)
		require.NoError(t, err, "Second encryption failed")
		secondDuration := time.Since(startSecond)

		t.Logf("First encryption (no cache): %v", firstDuration)
		t.Logf("Second encryption (with cache): %v", secondDuration)

		// Test decryption caching
		decryptInput := &DecryptInput{
			Ciphertext:     ciphertext1,
			EncryptedKey:   encryptedKey1,
			KeyContext:     keyCtx,
			PayloadContext: payloadCtx,
		}

		// First decryption - should decrypt key
		startFirstDecrypt := time.Now()
		_, err = cipher.Decrypt(ctx, decryptInput)
		require.NoError(t, err, "First decryption failed")
		firstDecryptDuration := time.Since(startFirstDecrypt)

		// Second decryption - should use cached key (faster)
		startSecondDecrypt := time.Now()
		_, err = cipher.Decrypt(ctx, decryptInput)
		require.NoError(t, err, "Second decryption failed")
		secondDecryptDuration := time.Since(startSecondDecrypt)

		t.Logf("First decryption (no cache): %v", firstDecryptDuration)
		t.Logf("Second decryption (with cache): %v", secondDecryptDuration)
	})
}
