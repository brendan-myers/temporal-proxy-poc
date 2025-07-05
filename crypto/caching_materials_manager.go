package crypto

import (
	"context"
	"crypto/sha256"
	"fmt"
	"sort"
	"sync"
	"temporal-sa/temporal-cloud-proxy/metrics"
	"time"

	lru "github.com/hashicorp/golang-lru"
	"go.temporal.io/sdk/client"
)

// CachingConfig holds configuration for caching materials manager
type CachingConfig struct {
	MaxCache        int
	MaxAge          time.Duration
	MaxMessagesUsed int
}

// CachingMaterialsManager manages cryptographic materials with caching
type CachingMaterialsManager struct {
	cache           *lru.Cache
	mutex           sync.RWMutex
	maxAge          time.Duration
	maxMessagesUsed int
	underlyingMM    MaterialsManager
	metricsHandler  client.MetricsHandler
}

// NewCachingMaterialsManager creates a new caching materials manager
func NewCachingMaterialsManager(
	underlyingMM MaterialsManager,
	config CachingConfig,
	metricsHandler client.MetricsHandler,
) (*CachingMaterialsManager, error) {
	cache, err := lru.New(config.MaxCache)
	if err != nil {
		return nil, fmt.Errorf("failed to create cache: %v", err)
	}

	if metricsHandler == nil {
		metricsHandler = client.MetricsNopHandler
	}

	return &CachingMaterialsManager{
		cache:           cache,
		maxAge:          config.MaxAge,
		maxMessagesUsed: config.MaxMessagesUsed,
		underlyingMM:    underlyingMM,
		metricsHandler:  metricsHandler,
	}, nil
}

// GetMaterial retrieves cryptographic material, either from cache or by creating new ones
func (c *CachingMaterialsManager) GetMaterial(ctx context.Context, cryptoCtx CryptoContext) (*Material, error) {
	cacheKey := c.createCacheKey(cryptoCtx)

	// Try to get from cache first
	c.mutex.RLock()
	cachedValue, found := c.cache.Get(cacheKey)
	c.mutex.RUnlock()

	if found {
		material := cachedValue.(*Material)

		// Check if material is still valid
		if c.isMaterialValid(material) {
			c.mutex.Lock()
			material.UsageCount++
			c.mutex.Unlock()
			return material, nil
		}

		// Remove expired material
		c.mutex.Lock()
		c.cache.Remove(cacheKey)
		c.mutex.Unlock()
	}

	// Get new material from underlying MM with metrics
	start := time.Now()
	c.metricsHandler.Counter(metrics.MaterialsManagerGetRequests).Inc(1)
	material, err := c.underlyingMM.GetMaterial(ctx, cryptoCtx)
	c.metricsHandler.Timer(metrics.MaterialsManagerGetLatency).Record(time.Since(start))
	if err != nil {
		c.metricsHandler.Counter(metrics.MaterialsManagerGetErrors).Inc(1)
		return nil, err
	}
	c.metricsHandler.Counter(metrics.MaterialsManagerGetSuccess).Inc(1)

	// Initialize usage metadata
	material.CreatedAt = time.Now()
	material.UsageCount = 1

	// Cache the new material
	c.mutex.Lock()
	c.cache.Add(cacheKey, material)
	c.mutex.Unlock()

	return material, nil
}

// DecryptMaterial implements the CipherMaterialsManager interface
func (c *CachingMaterialsManager) DecryptMaterial(ctx context.Context, cryptoCtx CryptoContext, material *Material) (*Material, error) {
	// Create a hashed cache key that includes both context and encrypted key
	cacheKey := c.createDecryptionCacheKey(cryptoCtx, material.EncryptedKey)

	// Try to get from cache first
	c.mutex.RLock()
	cachedValue, found := c.cache.Get(cacheKey)
	c.mutex.RUnlock()

	if found {
		cachedMaterial := cachedValue.(*Material)

		// Check if material is still valid
		if c.isMaterialValid(cachedMaterial) {
			c.mutex.Lock()
			cachedMaterial.UsageCount++
			c.mutex.Unlock()
			return cachedMaterial, nil
		}

		// Remove expired material
		c.mutex.Lock()
		c.cache.Remove(cacheKey)
		c.mutex.Unlock()
	}

	// Get new material from underlying MM with metrics
	start := time.Now()
	c.metricsHandler.Counter(metrics.MaterialsManagerDecryptRequests).Inc(1)
	decryptedMaterial, err := c.underlyingMM.DecryptMaterial(ctx, cryptoCtx, material)
	c.metricsHandler.Timer(metrics.MaterialsManagerDecryptLatency).Record(time.Since(start))
	if err != nil {
		c.metricsHandler.Counter(metrics.MaterialsManagerDecryptErrors).Inc(1)
		return nil, err
	}
	c.metricsHandler.Counter(metrics.MaterialsManagerDecryptSuccess).Inc(1)

	// Initialize usage metadata
	decryptedMaterial.CreatedAt = time.Now()
	decryptedMaterial.UsageCount = 1

	// Cache the new material
	c.mutex.Lock()
	c.cache.Add(cacheKey, decryptedMaterial)
	c.mutex.Unlock()

	return decryptedMaterial, nil
}

// isMaterialValid checks if the material is still valid based on age and usage count
func (c *CachingMaterialsManager) isMaterialValid(material *Material) bool {
	// Check age
	if time.Since(material.CreatedAt) > c.maxAge {
		return false
	}

	// Check usage count
	if material.UsageCount >= c.maxMessagesUsed {
		return false
	}

	return true
}

// createCacheKey generates a hashed cache key from the crypto context
func (c *CachingMaterialsManager) createCacheKey(cryptoCtx CryptoContext) string {
	// Sort keys for consistent ordering
	keys := make([]string, 0, len(cryptoCtx))
	for k := range cryptoCtx {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Create a hash of the context
	h := sha256.New()
	for _, k := range keys {
		h.Write([]byte(k))
		h.Write([]byte{':'})
		h.Write([]byte(cryptoCtx[k]))
		h.Write([]byte{';'})
	}

	return fmt.Sprintf("%x", h.Sum(nil))
}

// createDecryptionCacheKey generates a hashed cache key that includes encrypted key
// Todo: Should encryption and decryption share the same cache key?
func (c *CachingMaterialsManager) createDecryptionCacheKey(cryptoCtx CryptoContext, encryptedKey []byte) string {
	// Get the context hash
	contextKey := c.createCacheKey(cryptoCtx)

	// Add the encrypted key to the hash
	h := sha256.New()
	h.Write([]byte(contextKey))
	h.Write([]byte{':'})
	h.Write(encryptedKey)

	return fmt.Sprintf("%x", h.Sum(nil))
}
