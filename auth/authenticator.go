package auth

import (
	"context"
	"time"
)

type AuthenticationResult struct {
	Authenticated bool
	Subject       string
	Claims        map[string]interface{}
	Expiration    time.Time
}

type Authenticator interface {
	Type() string
	Init(ctx context.Context, config map[string]interface{}) error
	Authenticate(ctx context.Context, credentials interface{}) (*AuthenticationResult, error)
	Refresh(ctx context.Context, result *AuthenticationResult) (*AuthenticationResult, error)
	Close() error
}
