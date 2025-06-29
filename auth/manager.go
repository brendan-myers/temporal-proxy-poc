package auth

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

type AuthManager struct {
	authenticators map[string]Authenticator
	mu             sync.RWMutex
}

func NewAuthManager() *AuthManager {
	return &AuthManager{
		authenticators: make(map[string]Authenticator),
	}
}

func (am *AuthManager) RegisterAuthenticator(auth Authenticator) error {
	am.mu.Lock()
	defer am.mu.Unlock()

	typ := auth.Type()
	if _, exists := am.authenticators[typ]; exists {
		return fmt.Errorf("authenticator with type %s already registered", typ)
	}

	am.authenticators[typ] = auth
	return nil
}

func (am *AuthManager) GetAuthenticator(name string) (Authenticator, error) {
	am.mu.RLock()
	defer am.mu.RUnlock()

	auth, exists := am.authenticators[name]
	if !exists {
		return nil, fmt.Errorf("authenticator with name %s not found", name)
	}

	return auth, nil
}

func (am *AuthManager) Authenticate(ctx context.Context, name string, credentials interface{}) (*AuthenticationResult, error) {
	auth, err := am.GetAuthenticator(name)
	if err != nil {
		return nil, err
	}

	return auth.Authenticate(ctx, credentials)
}

func (am *AuthManager) Close() error {
	am.mu.Lock()
	defer am.mu.Unlock()

	var errs []error
	for name, auth := range am.authenticators {
		if err := auth.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close authenticator %s: %w", name, err))
		}
	}

	if len(errs) > 0 {
		return errors.Join(errs...)
	}

	return nil
}
