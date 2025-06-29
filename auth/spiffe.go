package auth

import (
	"context"
	"fmt"
	"log"
	"strings"

	"github.com/spiffe/go-spiffe/v2/svid/jwtsvid"
	"github.com/spiffe/go-spiffe/v2/workloadapi"
)

type SpiffeAuthenticator struct {
	TrustDomain string   `yaml:"trust_domain"`
	Audiences   []string `yaml:"audiences"`
	Endpoint    string   `yaml:"endpoint"`
	jwtSource   *workloadapi.JWTSource
}

func (s *SpiffeAuthenticator) Type() string {
	return "spiffe"
}

func (s *SpiffeAuthenticator) Init(ctx context.Context, config map[string]interface{}) error {
	trustDomain, ok := config["trust_domain"].(string)
	if !ok {
		return fmt.Errorf("trust_domain is required")
	}
	s.TrustDomain = trustDomain

	endpoint, ok := config["endpoint"].(string)
	if !ok {
		return fmt.Errorf("endpoint is required")
	}
	s.Endpoint = endpoint

	if audiencesRaw, ok := config["audiences"].([]interface{}); ok {
		for _, a := range audiencesRaw {
			if audience, ok := a.(string); ok {
				s.Audiences = append(s.Audiences, audience)
			}
		}
	}

	clientOptions := workloadapi.WithClientOptions(workloadapi.WithAddr(s.Endpoint))
	jwtSource, err := workloadapi.NewJWTSource(ctx, clientOptions)
	if err != nil {
		return fmt.Errorf("failed to initialise JWT source: %w", err)
	}

	s.jwtSource = jwtSource

	return nil
}

func (s *SpiffeAuthenticator) Authenticate(ctx context.Context, credentials interface{}) (*AuthenticationResult, error) {
	token, ok := credentials.(string)
	if !ok {
		return nil, fmt.Errorf("credentials must be a string token")
	}

	const prefix = "Bearer "
	if strings.HasPrefix(token, prefix) {
		token = strings.TrimPrefix(token, prefix)
	}

	svid, err := jwtsvid.ParseAndValidate(token, s.jwtSource, s.Audiences)
	if err != nil {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid token: %w", err)
	}

	claims := make(map[string]interface{})
	for k, v := range svid.Claims {
		claims[k] = v
	}

	log.Default().Println("sub", svid.ID)

	return &AuthenticationResult{
		Authenticated: true,
		Subject:       svid.ID.String(),
		Claims:        claims,
		Expiration:    svid.Expiry,
	}, nil
}

func (s *SpiffeAuthenticator) Refresh(ctx context.Context, result *AuthenticationResult) (*AuthenticationResult, error) {
	return result, fmt.Errorf("refresh not applicable for SPIFFE JWT-SWIDs")
}

func (s *SpiffeAuthenticator) Close() error {
	if s.jwtSource != nil {
		return s.jwtSource.Close()
	}
	return nil
}
