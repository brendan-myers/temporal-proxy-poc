package codec

import (
	"context"
	"fmt"
	"time"

	"temporal-sa/temporal-cloud-proxy/crypto"
	"temporal-sa/temporal-cloud-proxy/metrics"

	"github.com/aws/aws-sdk-go/service/kms"

	commonpb "go.temporal.io/api/common/v1"
	"go.temporal.io/sdk/client"
	"go.temporal.io/sdk/converter"
)

const (
	// MetadataEncodingEncrypted is "binary/encrypted"
	MetadataEncodingEncrypted = "binary/encrypted"
	// MetadataEncryptionKeyID is "encryption-key-id"
	MetadataEncryptionKeyID = "encryption-key-id"
	// MetadataEncryptedDataKey is "encrypted-data-key"
	MetadataEncryptedDataKey = "encrypted-data-key"

	// PurposeEncryptionKeyAuth is the purpose for encryption key authentication
	PurposeEncryptionKeyAuth = "encryption-key-auth"
	// PurposePayloadAuth is the purpose for payload authentication
	PurposePayloadAuth = "payload-auth"
)

// Codec implements PayloadCodec using the crypto package's cached material manager.
type Codec struct {
	KeyID          string
	Cipher         *crypto.Cipher
	CodecContext   map[string]string
	MetricsHandler client.MetricsHandler
}

// NewEncryptionCodecWithCaching creates a new encryption codec with configurable caching.
func NewEncryptionCodecWithCaching(
	kmsClient *kms.KMS,
	codecContext map[string]string,
	encryptionKeyID string,
	metricsHandler client.MetricsHandler,
	cachingConfig *crypto.CachingConfig,
) converter.PayloadCodec {
	// Set default caching config if not provided
	if cachingConfig == nil {
		cachingConfig = &crypto.CachingConfig{
			MaxCache:        100,
			MaxAge:          5 * time.Minute,
			MaxMessagesUsed: 100,
		}
	}

	// Create AWS KMS provider
	awsProvider := crypto.NewAWSKMSProvider(kmsClient, crypto.KMSOptions{
		KeyID:   encryptionKeyID,
		KeySpec: "AES_256",
	})

	// Create caching materials manager
	cachingMM, _ := crypto.NewCachingMaterialsManager(
		awsProvider,
		*cachingConfig,
		metricsHandler,
	)

	// Create cipher with caching materials manager
	cipher := crypto.NewCipher(cachingMM)

	return &Codec{
		KeyID:          encryptionKeyID,
		Cipher:         cipher,
		CodecContext:   codecContext,
		MetricsHandler: metricsHandler,
	}
}

// createCryptoContext creates a crypto context for the given purpose, encryption key ID and codec context
func (e *Codec) createCryptoContext(purpose, encryptionKeyID string, codecContext map[string]string) crypto.CryptoContext {
	cryptoContext := crypto.CryptoContext{
		"purpose":         purpose,
		"encryptionKeyID": encryptionKeyID,
	}

	// Add all codec context values to the crypto context
	for k, v := range codecContext {
		cryptoContext[k] = v
	}

	return cryptoContext
}

// Encode implements converter.PayloadCodec.Encode.
func (e *Codec) Encode(payloads []*commonpb.Payload) ([]*commonpb.Payload, error) {
	start := time.Now()
	e.MetricsHandler.Counter(metrics.EncryptRequests).Inc(1)

	result := make([]*commonpb.Payload, len(payloads))
	for i, p := range payloads {
		origBytes, err := p.Marshal()
		if err != nil {
			e.MetricsHandler.Counter(metrics.EncryptErrors).Inc(1)
			return payloads, err
		}

		keyContext := e.createCryptoContext(PurposeEncryptionKeyAuth, e.KeyID, e.CodecContext)

		// TODO: Reusing key context to auth payload is temporary until we can get additional payload metadata, e.g. workflow ID, run ID, etc.
		// Reference: https://temporaltechnologies.slack.com/archives/C04NYM5D3U6/p1750377050937099
		payloadContext := e.createCryptoContext(PurposePayloadAuth, e.KeyID, e.CodecContext)

		input := &crypto.EncryptInput{
			Plaintext:      origBytes,
			KeyContext:     keyContext,
			PayloadContext: payloadContext,
		}

		ciphertext, encryptedKey, err := e.Cipher.Encrypt(context.Background(), input)
		if err != nil {
			e.MetricsHandler.Counter(metrics.EncryptErrors).Inc(1)
			return payloads, err
		}

		result[i] = &commonpb.Payload{
			Metadata: map[string][]byte{
				converter.MetadataEncoding: []byte(MetadataEncodingEncrypted),
				MetadataEncryptionKeyID:    []byte(e.KeyID),
				MetadataEncryptedDataKey:   encryptedKey,
			},
			Data: ciphertext,
		}
	}

	e.MetricsHandler.Counter(metrics.EncryptSuccess).Inc(1)
	e.MetricsHandler.Timer(metrics.EncryptLatency).Record(time.Since(start))
	return result, nil
}

// Decode implements converter.PayloadCodec.Decode.
func (e *Codec) Decode(payloads []*commonpb.Payload) ([]*commonpb.Payload, error) {
	start := time.Now()
	e.MetricsHandler.Counter(metrics.DecryptRequests).Inc(1)

	result := make([]*commonpb.Payload, len(payloads))
	for i, p := range payloads {
		// Only if it's encrypted
		if string(p.Metadata[converter.MetadataEncoding]) != MetadataEncodingEncrypted {
			result[i] = p
			continue
		}

		keyID, ok := p.Metadata[MetadataEncryptionKeyID]
		if !ok {
			e.MetricsHandler.Counter(metrics.DecryptErrors).Inc(1)
			return payloads, fmt.Errorf("no encryption key id")
		}

		keyContext := e.createCryptoContext(PurposeEncryptionKeyAuth, string(keyID), e.CodecContext)
		payloadContext := e.createCryptoContext(PurposePayloadAuth, string(keyID), e.CodecContext)

		// Get the encrypted key from metadata
		encryptedKey, ok := p.Metadata[MetadataEncryptedDataKey]
		if !ok {
			e.MetricsHandler.Counter(metrics.DecryptErrors).Inc(1)
			return payloads, fmt.Errorf("no encrypted key in payload")
		}

		input := &crypto.DecryptInput{
			Ciphertext:     p.Data,
			EncryptedKey:   encryptedKey,
			KeyContext:     keyContext,
			PayloadContext: payloadContext,
		}

		decrypted, err := e.Cipher.Decrypt(context.Background(), input)
		if err != nil {
			e.MetricsHandler.Counter(metrics.DecryptErrors).Inc(1)
			return payloads, err
		}

		result[i] = &commonpb.Payload{}
		err = result[i].Unmarshal(decrypted)
		if err != nil {
			e.MetricsHandler.Counter(metrics.DecryptErrors).Inc(1)
			return payloads, err
		}
	}

	e.MetricsHandler.Counter(metrics.DecryptSuccess).Inc(1)
	e.MetricsHandler.Timer(metrics.DecryptLatency).Record(time.Since(start))
	return result, nil
}
