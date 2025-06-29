package crypto

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// MockMaterialsManager implements MaterialsManager for testing
type MockMaterialsManager struct {
	callCount int
}

func NewMockMaterialsManager() *MockMaterialsManager {
	return &MockMaterialsManager{
		callCount: 0,
	}
}

func (m *MockMaterialsManager) GetMaterial(ctx context.Context, cryptoCtx CryptoContext) (*Material, error) {
	m.callCount++
	// Return a new material instance each time
	return &Material{
		PlaintextKey: []byte("mock-plaintext-key"),
		EncryptedKey: []byte("mock-encrypted-key"),
	}, nil
}

func (m *MockMaterialsManager) DecryptMaterial(ctx context.Context, cryptoCtx CryptoContext, material *Material) (*Material, error) {
	m.callCount++
	// Return a new material instance each time
	return &Material{
		PlaintextKey: []byte("mock-plaintext-key"),
		EncryptedKey: material.EncryptedKey,
	}, nil
}

func TestCachingMaterialsManager_GetMaterial(t *testing.T) {
	mockMM := NewMockMaterialsManager()
	cachingMM, err := NewCachingMaterialsManager(
		mockMM,
		10,            // maxCache
		5*time.Minute, // maxAge
		5,             // maxMessagesUsed
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	cryptoCtx := CryptoContext{"purpose": "test"}
	ctx := context.Background()

	// First call should get material from underlying MM
	material1, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material")
	assert.Equal(t, 1, material1.UsageCount, "Expected usage count to be 1")

	// Second call should get material from cache and increment usage count
	material2, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material from cache")
	assert.Equal(t, 2, material2.UsageCount, "Expected usage count to be 2")

	// Verify it's the same material instance (cached)
	assert.Same(t, material1, material2, "Expected to get the same material instance from cache")

	// Verify the mock was only called once (first time)
	assert.Equal(t, 1, mockMM.callCount, "Expected mock to be called only once")
}

func TestCachingMaterialsManager_MaterialExpiration(t *testing.T) {
	mockMM := NewMockMaterialsManager()
	cachingMM, err := NewCachingMaterialsManager(
		mockMM,
		10,                  // maxCache
		50*time.Millisecond, // very short maxAge for testing
		100,                 // high maxMessagesUsed to focus on age
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	cryptoCtx := CryptoContext{"purpose": "expiration-test"}
	ctx := context.Background()

	// Get initial material
	material1, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material")
	assert.Equal(t, 1, mockMM.callCount, "Expected mock to be called once")

	// Wait for material to expire
	time.Sleep(100 * time.Millisecond)

	// Get material again, should be a new instance due to expiration
	material2, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material after expiration")

	// Verify the mock was called again (new material created)
	assert.Equal(t, 2, mockMM.callCount, "Expected mock to be called again after expiration")

	// Verify it's a different material instance (new after expiration)
	assert.NotSame(t, material1, material2, "Expected to get a new material instance after expiration")

	// Verify the usage count is reset
	assert.Equal(t, 1, material2.UsageCount, "Expected new material usage count to be 1")
}

func TestCachingMaterialsManager_UsageLimit(t *testing.T) {
	mockMM := NewMockMaterialsManager()
	maxUsage := 3
	cachingMM, err := NewCachingMaterialsManager(
		mockMM,
		10,            // maxCache
		5*time.Minute, // long maxAge to focus on usage count
		maxUsage,      // low maxMessagesUsed for testing
	)
	require.NoError(t, err, "Failed to create caching materials manager")

	cryptoCtx := CryptoContext{"purpose": "usage-limit-test"}
	ctx := context.Background()

	// Get initial material
	material1, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material")
	assert.Equal(t, 1, mockMM.callCount, "Expected mock to be called once")

	// Use material up to max usage
	var material *Material
	for i := 2; i <= maxUsage; i++ {
		material, err = cachingMM.GetMaterial(ctx, cryptoCtx)
		require.NoError(t, err, "Failed to get material on usage %d", i)
		assert.Equal(t, i, material.UsageCount, "Expected usage count to be %d", i)

		// Should be the same material instance
		assert.Same(t, material1, material, "Expected to get the same material instance within usage limit")
	}

	// Verify the mock was still only called once
	assert.Equal(t, 1, mockMM.callCount, "Expected mock to be called only once before reaching limit")

	// Get material one more time, should be a new instance due to usage limit
	materialNew, err := cachingMM.GetMaterial(ctx, cryptoCtx)
	require.NoError(t, err, "Failed to get material after usage limit")

	// Verify the mock was called again (new material created)
	assert.Equal(t, 2, mockMM.callCount, "Expected mock to be called again after reaching usage limit")

	// Verify it's a different material instance
	assert.NotSame(t, material1, materialNew, "Expected to get a new material instance after usage limit")

	// Verify the usage count is reset
	assert.Equal(t, 1, materialNew.UsageCount, "Expected new material usage count to be 1")
}
