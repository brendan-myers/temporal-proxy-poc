package auth

import (
	"context"
	"fmt"
	"github.com/MicahParks/keyfunc"
	"github.com/golang-jwt/jwt/v4"
	"strings"
	"time"
)

type JwtAuthenticator struct {
	Audiences []string `yaml:"audiences"`
	JwksUrl   string   `yaml:"jwks-url"`
	jwks      *keyfunc.JWKS
}

func (j *JwtAuthenticator) Type() string {
	return "jwt"
}

func (j *JwtAuthenticator) Init(ctx context.Context, config map[string]interface{}) error {
	jwksUrl, ok := config["jwks-url"].(string)
	if !ok {
		return fmt.Errorf("jwks-url is required")
	}
	j.JwksUrl = jwksUrl

	jwks, err := keyfunc.Get(jwksUrl, keyfunc.Options{
		RefreshErrorHandler: func(err error) {
			fmt.Printf("JWKS refresh failed: %v\n", err)
		},
		RefreshInterval: time.Minute * 5,
	})
	if err != nil {
		return fmt.Errorf("failed to get JWKS: %w", err)
	}

	j.jwks = jwks

	if audiences, ok := config["audiences"].([]interface{}); ok {
		for _, a := range audiences {
			if audience, ok := a.(string); ok {
				j.Audiences = append(j.Audiences, audience)
			}
		}
	}

	return nil
}

func (j *JwtAuthenticator) Authenticate(ctx context.Context, credentials interface{}) (*AuthenticationResult, error) {
	jwtString, ok := credentials.(string)
	if !ok {
		return nil, fmt.Errorf("credentials must be a string token")
	}

	const prefix = "Bearer "
	jwtString = strings.TrimPrefix(jwtString, prefix)

	token, err := jwt.Parse(jwtString, j.jwks.Keyfunc)
	if err != nil {
		fmt.Printf("Error parsing JWT token: %v\n", err)
		return &AuthenticationResult{
			Authenticated: false,
		}, err
	}

	if !token.Valid {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid token signature")
	}

	claims := token.Claims.(jwt.MapClaims)
	var audClaim interface{}
	var validAud bool

	audClaim, ok = claims["aud"]
	if !ok {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("audience claim missing")
	}

	switch aud := audClaim.(type) {
	case string:
		// Single audience case
		for _, audience := range j.Audiences {
			if aud == audience {
				validAud = true
				break
			}
		}
	case []interface{}:
		// Multi-audience case
		for _, audItem := range aud {
			if audStr, ok := audItem.(string); ok {
				for _, audience := range j.Audiences {
					if audStr == audience {
						validAud = true
						break
					}
				}
			}
			if validAud {
				break
			}
		}
	default:
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid audience format")
	}

	if !validAud {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid audience: %v", audClaim)
	}

	sub, ok := claims["sub"].(string)
	if !ok {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid subject: %v", sub)
	}

	expFloat, ok := claims["exp"].(float64)
	if !ok {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("invalid expiry: %v", expFloat)
	}
	expiry := time.Unix(int64(expFloat), 0)

	if time.Now().After(expiry) {
		return &AuthenticationResult{
			Authenticated: false,
		}, fmt.Errorf("token expired")
	}

	return &AuthenticationResult{
		Authenticated: true,
		Subject:       sub,
		Claims:        claims,
		Expiration:    expiry,
	}, nil
}

func (j *JwtAuthenticator) Close() error {
	if j.jwks != nil {
		j.jwks.EndBackground()
	}
	return nil
}
