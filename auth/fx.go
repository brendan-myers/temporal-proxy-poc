package auth

import "go.uber.org/fx"

var Module = fx.Provide(
	newAuthenticatorFactoryProvider,
)
