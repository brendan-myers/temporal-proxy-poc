package auth

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"

	"temporal-sa/temporal-cloud-proxy/config"
)

func TestNewAuthenticatorFactoryProvider(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	factory, err := newAuthenticatorFactoryProvider(ctx, logger)

	assert.NoError(t, err)
	assert.NotNil(t, factory)
}

func TestAuthenticatorFactory_NewAuthenticator_UnsupportedType(t *testing.T) {
	ctx := context.Background()
	logger := zap.NewNop()

	factory, err := newAuthenticatorFactoryProvider(ctx, logger)
	require.NoError(t, err)

	authConfig := config.AuthConfig{
		Type: "unsupported-type",
		Config: map[string]interface{}{
			"some-config": "value",
		},
	}

	authenticator, err := factory.NewAuthenticator(authConfig)

	assert.Error(t, err)
	assert.Contains(t, err.Error(), "authenticator not found for type unsupported-type")
	assert.Nil(t, authenticator)
}

func TestAuthenticationResult(t *testing.T) {
	// Test the AuthenticationResult struct
	result := &AuthenticationResult{
		Authenticated: true,
		Subject:       "test-subject",
		Claims: map[string]interface{}{
			"iss": "test-issuer",
			"sub": "test-subject",
			"aud": "test-audience",
		},
	}

	assert.True(t, result.Authenticated)
	assert.Equal(t, "test-subject", result.Subject)
	assert.Equal(t, "test-issuer", result.Claims["iss"])
	assert.Equal(t, "test-subject", result.Claims["sub"])
	assert.Equal(t, "test-audience", result.Claims["aud"])
}

// Note: JWT and SPIFFE authenticator tests are in their respective _test.go files
// (jwt_test.go and spiffe_test.go) where they can properly mock external dependencies
// without making real network calls or requiring actual SPIFFE infrastructure.
//
// This file focuses on testing the factory pattern and basic functionality
// that doesn't require external dependencies.

func TestAuthenticatorFactory_FactoryPattern(t *testing.T) {
	// Test that the factory correctly registers authenticator constructors
	ctx := context.Background()
	logger := zap.NewNop()

	factory, err := newAuthenticatorFactoryProvider(ctx, logger)
	require.NoError(t, err)

	// Cast to concrete type to access internal state for testing
	concreteFactory, ok := factory.(*authenticatorFactory)
	require.True(t, ok, "Factory should be of type *authenticatorFactory")

	// Verify that JWT and SPIFFE providers are registered
	assert.Contains(t, concreteFactory.providers, "jwt", "JWT provider should be registered")
	assert.Contains(t, concreteFactory.providers, "spiffe", "SPIFFE provider should be registered")
	assert.Len(t, concreteFactory.providers, 2, "Should have exactly 2 providers registered")
}
