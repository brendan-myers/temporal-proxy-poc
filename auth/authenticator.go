package auth

import (
	"context"
	"fmt"
	"go.uber.org/zap"
	"temporal-sa/temporal-cloud-proxy/config"
	"time"
)

type (
	AuthenticatorFactory interface {
		NewAuthenticator(authConfig config.AuthConfig) (Authenticator, error)
	}

	Authenticator interface {
		Type() string
		Init(ctx context.Context, config map[string]interface{}) error
		Authenticate(ctx context.Context, credentials interface{}) (*AuthenticationResult, error)
		Close() error
	}

	AuthenticationResult struct {
		Authenticated bool
		Subject       string
		Claims        map[string]interface{}
		Expiration    time.Time
	}

	AuthenticatorConstructor func(config map[string]interface{}) (Authenticator, error)

	authenticatorFactory struct {
		providers map[string]AuthenticatorConstructor
	}
)

func newAuthenticatorFactoryProvider(ctx context.Context, _ *zap.Logger) (AuthenticatorFactory, error) {
	af := &authenticatorFactory{
		providers: make(map[string]AuthenticatorConstructor),
	}

	af.providers["spiffe"] = func(config map[string]interface{}) (Authenticator, error) {
		authenticator := &SpiffeAuthenticator{}
		err := authenticator.Init(ctx, config)
		return authenticator, err
	}

	af.providers["jwt"] = func(config map[string]interface{}) (Authenticator, error) {
		authenticator := &JwtAuthenticator{}
		err := authenticator.Init(ctx, config)
		return authenticator, err
	}

	return af, nil
}

func (a *authenticatorFactory) NewAuthenticator(authConfig config.AuthConfig) (Authenticator, error) {
	authenticator, ok := a.providers[authConfig.Type]
	if !ok {
		return nil, fmt.Errorf("authenticator not found for type %s", authConfig.Type)
	}

	return authenticator(authConfig.Config)
}
