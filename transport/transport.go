package transport

import (
	"context"
	"fmt"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"net"
	"temporal-sa/temporal-cloud-proxy/config"
	"temporal-sa/temporal-cloud-proxy/proxy"
)

type (
	TransportProvider interface {
		Start() error
		Stop() error
	}

	grpcTransportProvider struct {
		host       string
		port       int
		grpcServer *grpc.Server
		logger     *zap.Logger
	}
)

func newTransportProvider(lc fx.Lifecycle, configProvider config.ConfigProvider, logger *zap.Logger,
	proxyProvider proxy.ProxyProvider) (TransportProvider, error) {

	transportManager := &grpcTransportProvider{
		host:   configProvider.GetProxyConfig().Server.Host,
		port:   configProvider.GetProxyConfig().Server.Port,
		logger: logger,
	}

	workflowClient := workflowservice.NewWorkflowServiceClient(proxyProvider.GetConnectionMux())

	handler, err := client.NewWorkflowServiceProxyServer(
		client.WorkflowServiceProxyOptions{Client: workflowClient},
	)
	if err != nil {
		return nil, err
	}

	transportManager.grpcServer = grpc.NewServer()
	workflowservice.RegisterWorkflowServiceServer(transportManager.grpcServer, handler)

	lc.Append(fx.Hook{
		OnStart: func(ctx context.Context) error {
			return transportManager.Start()
		},
		OnStop: func(ctx context.Context) error {
			return transportManager.Stop()
		},
	})

	return transportManager, nil
}

func (t *grpcTransportProvider) Start() error {
	lis, err := net.Listen("tcp", t.getHostPort())
	if err != nil {
		return err
	}

	t.logger.Info(
		"proxy started",
		zap.String("host", t.host),
		zap.Int("port", t.port),
	)

	go t.grpcServer.Serve(lis)

	return nil
}

func (t *grpcTransportProvider) Stop() error {
	t.grpcServer.GracefulStop()
	return nil
}

func (t *grpcTransportProvider) getHostPort() string {
	return fmt.Sprintf("%s:%d", t.host, t.port)
}
