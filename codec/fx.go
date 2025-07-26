package codec

import "go.uber.org/fx"

var Module = fx.Provide(
	newCodecFactoryProvider,
)
