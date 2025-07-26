package metrics

import (
	"context"
	"errors"
	"fmt"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"net/http"
	"temporal-sa/temporal-cloud-proxy/config"
)

type (
	MetricsProvider interface {
		Start() error
		Stop() error
	}

	httpPromMetricsProvider struct {
		host   string
		port   int
		path   string
		server *http.Server
		logger *zap.Logger
	}
)

func newMetricsProvider(lc fx.Lifecycle, configProvider config.ConfigProvider, logger *zap.Logger) MetricsProvider {
	provider := &httpPromMetricsProvider{
		host:   configProvider.GetProxyConfig().Server.Host,
		port:   configProvider.GetProxyConfig().Metrics.Port,
		path:   DefaultPrometheusPath,
		logger: logger,
	}

	// Initialize metrics
	_, err := InitPrometheus()
	if err != nil {
		logger.Fatal("failed to initialize prometheus provider", zap.Error(err))
	}

	provider.server = &http.Server{Addr: provider.getHostPort()}
	http.Handle(provider.path, promhttp.Handler())

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return provider.Start()
		},
		OnStop: func(ctx context.Context) error {
			return provider.Stop()
		},
	})

	return provider
}

func (h *httpPromMetricsProvider) Start() error {
	go func() {
		h.logger.Info("metrics server started", zap.String("endpoint", h.getHostPortPath()))
		if err := h.server.ListenAndServe(); !errors.Is(err, http.ErrServerClosed) {
			h.logger.Fatal("metrics server error: %v", zap.Error(err))
		}
	}()

	return nil
}

func (h *httpPromMetricsProvider) Stop() error {
	return nil
}

func (h *httpPromMetricsProvider) getHostPort() string {
	return fmt.Sprintf("%s:%d", h.host, h.port)
}

func (h *httpPromMetricsProvider) getHostPortPath() string {
	return fmt.Sprintf("%s:%d%s", h.host, h.port, h.path)
}
