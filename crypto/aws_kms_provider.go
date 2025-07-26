package crypto

import (
	"context"
	"fmt"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/service/kms"
	"github.com/aws/aws-sdk-go/service/kms/kmsiface"
)

// AWWSKMSOptions contains configuration options for AWSKMSProvider
type AWSKMSOptions struct {
	// KeyID is the ARN or ID of the KMS key to use
	KeyID string

	// KeySpec is the type of key to generate (defaults to AES_256 if empty)
	KeySpec string
}

// AWSKMSProvider implements MaterialsManager using AWS KMS
type AWSKMSProvider struct {
	kmsClient kmsiface.KMSAPI
	keyID     string
	keySpec   string
}

// NewAWSKMSProvider creates a new KMS-based materials manager
func NewAWSKMSProvider(kmsClient kmsiface.KMSAPI, options AWSKMSOptions) *AWSKMSProvider {
	// Set default keySpec if not provided
	keySpec := options.KeySpec
	if keySpec == "" {
		keySpec = "AES_256"
	}

	return &AWSKMSProvider{
		kmsClient: kmsClient,
		keyID:     options.KeyID,
		keySpec:   keySpec,
	}
}

// GetMaterial generates new encryption materials using KMS
func (k *AWSKMSProvider) GetMaterial(ctx context.Context, cryptoCtx CryptoContext) (*Material, error) {
	// Convert CryptoContext to KMS encryption context
	encryptionContext := make(map[string]*string)
	for key, value := range cryptoCtx {
		valueCopy := value // Create a copy to avoid issues with loop variable
		encryptionContext[key] = &valueCopy
	}

	input := &kms.GenerateDataKeyInput{
		KeyId:             aws.String(k.keyID),
		KeySpec:           aws.String(k.keySpec),
		EncryptionContext: encryptionContext,
	}

	result, err := k.kmsClient.GenerateDataKeyWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to generate data key: %v", err)
	}

	return &Material{
		PlaintextKey: result.Plaintext,
		EncryptedKey: result.CiphertextBlob,
	}, nil
}

// DecryptMaterial decrypts the encrypted key using KMS
func (k *AWSKMSProvider) DecryptMaterial(ctx context.Context, cryptoCtx CryptoContext, material *Material) (*Material, error) {
	// Convert CryptoContext to KMS encryption context
	encryptionContext := make(map[string]*string)
	for key, value := range cryptoCtx {
		valueCopy := value // Create a copy to avoid issues with loop variable
		encryptionContext[key] = &valueCopy
	}

	input := &kms.DecryptInput{
		CiphertextBlob:    material.EncryptedKey,
		EncryptionContext: encryptionContext,
	}

	result, err := k.kmsClient.DecryptWithContext(ctx, input)
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %v", err)
	}

	return &Material{
		PlaintextKey: result.Plaintext,
		EncryptedKey: material.EncryptedKey,
	}, nil
}
