package proxy

import "go.uber.org/fx"

var Module = fx.Provide(
	newProxyProvider,
)
