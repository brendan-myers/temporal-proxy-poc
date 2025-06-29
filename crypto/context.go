package crypto

import (
	"encoding/json"
	"sort"
)

// CryptoContext contains information used for key derivation and authentication
type CryptoContext map[string]string

// ContextToBytes converts a CryptoContext to a deterministic byte array
// for use as authentication data in GCM encryption/decryption
func ContextToBytes(ctx CryptoContext) []byte {
	// Sort keys for deterministic ordering
	keys := make([]string, 0, len(ctx))
	for k := range ctx {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	// Build a map with sorted keys for JSON marshaling
	sortedMap := make(map[string]string)
	for _, k := range keys {
		sortedMap[k] = ctx[k]
	}

	// Marshal to JSON for a consistent binary representation
	data, err := json.Marshal(sortedMap)
	if err != nil {
		// If marshaling fails, return an empty slice rather than crashing
		// This should never happen with simple string maps
		return []byte{}
	}

	return data
}
