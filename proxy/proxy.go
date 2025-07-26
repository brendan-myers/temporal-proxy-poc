package proxy

import (
	"context"
	"crypto/tls"
	"errors"
	"fmt"
	"os"
	"sync"
	"temporal-sa/temporal-cloud-proxy/auth"
	"temporal-sa/temporal-cloud-proxy/codec"
	"temporal-sa/temporal-cloud-proxy/config"
	"temporal-sa/temporal-cloud-proxy/metrics"
	"time"

	"go.opentelemetry.io/otel/attribute"
	"go.temporal.io/sdk/converter"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"
)

type (
	ProxyProvider interface {
		GetConnectionMux() grpc.ClientConnInterface
		Start() error
		Stop() error
	}

	proxyServer struct {
		grpc.ClientConnInterface
		connectionMux  map[string]namespaceConnection
		mu             sync.RWMutex
		logger         *zap.Logger
		metricsHandler metrics.MetricsHandler
	}

	namespaceConnection struct {
		conn           *grpc.ClientConn
		auth           *auth.Authenticator
		metricsHandler metrics.MetricsHandler
	}
)

func newProxyProvider(configProvider config.ConfigProvider, logger *zap.Logger,
	authFactory auth.AuthenticatorFactory, codecFactory codec.EncryptionCodecFactory) (ProxyProvider, error) {
	proxy := &proxyServer{
		connectionMux:  make(map[string]namespaceConnection),
		logger:         logger,
		metricsHandler: metrics.NewMetricsHandler(metrics.MetricsHandlerOptions{}),
	}

	for _, w := range configProvider.GetProxyConfig().Workloads {
		logger.Debug("adding namespace connection",
			zap.String("workload-id", w.WorkloadId),
			zap.String("namespace", w.TemporalCloud.Namespace),
		)

		nsConn := &namespaceConnection{
			metricsHandler: metrics.NewMetricsHandler(metrics.MetricsHandlerOptions{
				InitialAttributes: attribute.NewSet(
					attribute.String("workload_id", w.WorkloadId),
					attribute.String("namespace", w.TemporalCloud.Namespace),
				),
			}),
		}

		// configure worker auth
		if w.Authentication == nil {
			logger.Warn("workload configured without worker authentication",
				zap.String("workload-id", w.WorkloadId))
		}
		if w.Authentication != nil {
			authenticator, err := authFactory.NewAuthenticator(*w.Authentication)
			if err != nil {
				logger.Error("failed to create authenticator",
					zap.String("workload-id", w.WorkloadId), zap.Error(err))
				return nil, fmt.Errorf(
					"failed to create authenticator for workload %s, %w", w.WorkloadId, err)
			}
			nsConn.auth = &authenticator
		}

		var grpcInterceptors []grpc.UnaryClientInterceptor

		// configure encryption/decryption codec
		if w.Encryption == nil {
			logger.Warn("workload configured without payload encryption",
				zap.String("workload-id", w.WorkloadId))
		}
		if w.Encryption != nil {
			// configure encryption metrics handler
			attributes := []attribute.KeyValue{
				attribute.String("workload_id", w.WorkloadId),
				attribute.String("namespace", w.TemporalCloud.Namespace),
				attribute.String("host_port", w.TemporalCloud.HostPort),
			}
			if w.Authentication != nil {
				attributes = append(attributes, attribute.String("auth_type", w.Authentication.Type))
			}

			metricsHandler := metrics.NewMetricsHandler(metrics.MetricsHandlerOptions{
				InitialAttributes: attribute.NewSet(
					attributes...,
					// Note: encryption codec adds additional attributes
				),
			})

			encryptionCodec, err := codecFactory.NewEncryptionCodec(codec.EncryptionCodecOptions{
				LocalEncryptionConfig: *w.Encryption,
				MetricsHandler:        &metricsHandler,
				CodecContext: map[string]string{
					"namespace": w.TemporalCloud.Namespace,
				},
			})
			if err != nil {
				logger.Error("failed to create encryption codec",
					zap.String("workload-id", w.WorkloadId), zap.Error(err))
				return nil, fmt.Errorf(
					"failed to create encryption codec for workload %s, %w", w.WorkloadId, err)
			}

			if encryptionCodec != nil {
				encryptionInterceptor, err := converter.NewPayloadCodecGRPCClientInterceptor(
					converter.PayloadCodecGRPCClientInterceptorOptions{
						Codecs: []converter.PayloadCodec{encryptionCodec},
					},
				)
				if err != nil {
					logger.Error("failed to create client interceptor",
						zap.String("workload-id", w.WorkloadId), zap.Error(err))
					return nil, fmt.Errorf(
						"failed to create client interceptor for workload %s, %w", w.WorkloadId, err)
				}
				if encryptionInterceptor != nil {
					grpcInterceptors = append(grpcInterceptors, encryptionInterceptor)
				}
			}
		}

		// set api key or mTLS auth on the namespace connection
		tlsConfig, authInterceptor, err := setNamespaceAuth(w, logger)
		if err != nil {
			logger.Error("failed to set namespace auth",
				zap.String("workload-id", w.WorkloadId), zap.Error(err))
			return nil, fmt.Errorf(
				"failed to set namespace auth for workload %s, %w", w.WorkloadId, err)
		}
		if authInterceptor != nil {
			grpcInterceptors = append(grpcInterceptors, authInterceptor)
		}

		conn, err := grpc.NewClient(
			w.TemporalCloud.HostPort,
			grpc.WithTransportCredentials(credentials.NewTLS(
				tlsConfig,
			)),
			grpc.WithChainUnaryInterceptor(grpcInterceptors...),
		)
		if err != nil {
			logger.Error("failed to create grpc client",
				zap.String("workload-id", w.WorkloadId), zap.Error(err))
			return nil, fmt.Errorf(
				"failed to create grpc client for workload %s, %w", w.WorkloadId, err)
		}

		nsConn.conn = conn

		proxy.mu.Lock()
		proxy.connectionMux[w.WorkloadId] = *nsConn
		proxy.mu.Unlock()
	}

	return proxy, nil
}

func (n *namespaceConnection) GetConnection() grpc.ClientConnInterface {
	return n.conn
}

func (n *namespaceConnection) GetAuthenticator() auth.Authenticator {
	if n.auth == nil {
		return nil
	}
	return *n.auth
}

func (n *namespaceConnection) Close() error {
	var errs []error

	if err := n.conn.Close(); err != nil {
		errs = append(errs, err)
	}
	if n.auth != nil {
		if err := n.GetAuthenticator().Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func (p *proxyServer) GetConnectionMux() grpc.ClientConnInterface {
	return p
}

func (p *proxyServer) Start() error {
	return nil
}

func (p *proxyServer) Stop() error {
	p.mu.Lock()
	defer p.mu.Unlock()

	var errs []error

	for _, conn := range p.connectionMux {
		if err := conn.Close(); err != nil {
			errs = append(errs, err)
		}
	}

	return errors.Join(errs...)
}

func setNamespaceAuth(workloadConfig config.WorkloadConfig, logger *zap.Logger) (*tls.Config, grpc.UnaryClientInterceptor, error) {
	tlsConfig := &tls.Config{}
	var grpcInterceptor grpc.UnaryClientInterceptor

	if apiKeyConfig := workloadConfig.TemporalCloud.Authentication.ApiKey; apiKeyConfig != nil {
		//
		//	Configure API key auth
		//
		if apiKeyConfig.Value != "" && apiKeyConfig.EnvVar != "" {
			logger.Warn("both value and envvar provider for api key; using value",
				zap.String("workload-id", workloadConfig.WorkloadId))
		}

		apiKey := ""
		if apiKeyConfig.Value != "" {
			apiKey = apiKeyConfig.Value
		} else if apiKeyConfig.EnvVar != "" {
			apiKey = os.Getenv(apiKeyConfig.EnvVar)
		}

		if apiKey == "" {
			return nil, nil, fmt.Errorf("no api key provided")
		}

		grpcInterceptor =
			func(ctx context.Context, method string, req any, reply any, cc *grpc.ClientConn, invoker grpc.UnaryInvoker, opts ...grpc.CallOption) error {
				md, ok := metadata.FromIncomingContext(ctx)

				if ok {
					md = md.Copy()
					md.Delete("authorization")
					md.Delete("temporal-namespace")

					ctx = metadata.NewOutgoingContext(ctx, md)
					ctx = metadata.AppendToOutgoingContext(ctx, "temporal-namespace", workloadConfig.TemporalCloud.Namespace)
					ctx = metadata.AppendToOutgoingContext(ctx, "authorization", "Bearer "+apiKey)
				}

				return invoker(ctx, method, req, reply, cc, opts...)
			}
	} else if workloadConfig.TemporalCloud.Authentication.TLS != nil {
		//
		//	Configure mTLS auth
		//
		cert, err := tls.LoadX509KeyPair(workloadConfig.TemporalCloud.Authentication.TLS.CertFile,
			workloadConfig.TemporalCloud.Authentication.TLS.KeyFile)
		if err != nil {
			return nil, nil, err
		}

		tlsConfig.Certificates = []tls.Certificate{cert}
	} else {
		// Passthrough. Useful if the client/worker is setting the API. Note: will not work with
		// mTLS configured at the client/worker.
	}

	return tlsConfig, grpcInterceptor, nil
}

// Invoke implements the grpc.ClientConnInterface Invoke method
func (p *proxyServer) Invoke(ctx context.Context, method string, args interface{}, reply interface{}, opts ...grpc.CallOption) error {
	start := time.Now()
	p.metricsHandler.Counter(metrics.ProxyRequestTotal).Inc(1)

	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		p.metricsHandler.WithTags(map[string]string{"error": "unable to read metadata"}).Counter(metrics.ProxyRequestErrors).Inc(1)
		return status.Errorf(codes.InvalidArgument, "unable to read metadata")
	}

	workloadId := md.Get("workload-id")

	if len(workloadId) <= 0 {
		p.metricsHandler.WithTags(map[string]string{"error": "metadata missing workload-id"}).Counter(metrics.ProxyRequestErrors).Inc(1)
		return status.Error(codes.InvalidArgument, "metadata missing workload-id")
	}
	if len(workloadId) != 1 {
		p.metricsHandler.WithTags(map[string]string{"error": "metadata contains multiple workload-id entries"}).Counter(metrics.ProxyRequestErrors).Inc(1)
		return status.Error(codes.InvalidArgument, "metadata contains multiple workload-id entries")
	}

	p.mu.RLock()
	namespace, exists := p.connectionMux[workloadId[0]]
	p.mu.RUnlock()

	if !exists {
		p.logger.Warn("invalid workload-id", zap.String("workload-id", workloadId[0]))
		p.metricsHandler.WithTags(map[string]string{"error": "invalid workload-id"}).Counter(metrics.ProxyRequestErrors).Inc(1)
		return status.Errorf(codes.InvalidArgument, "invalid workload-id")
	}

	if namespace.GetAuthenticator() != nil {
		authorization := md.Get("authorization")

		if len(authorization) < 1 {
			namespace.metricsHandler.WithTags(map[string]string{"error": "metadata is missing authorization"}).Counter(metrics.ProxyRequestErrors).Inc(1)
			return status.Error(codes.InvalidArgument, "metadata is missing authorization")
		} else if len(authorization) > 1 {
			namespace.metricsHandler.WithTags(map[string]string{"error": "metadata contains multiple authorization entries"}).Counter(metrics.ProxyRequestErrors).Inc(1)
			return status.Error(codes.InvalidArgument, "metadata contains multiple authorization entries")
		}

		result, err := namespace.GetAuthenticator().Authenticate(ctx, authorization[0])
		if err != nil {
			namespace.metricsHandler.WithTags(map[string]string{"error": "failed to authenticate"}).Counter(metrics.ProxyRequestErrors).Inc(1)
			return status.Errorf(codes.Unknown, "failed to authenticate: %v", err)
		}
		if !result.Authenticated {
			namespace.metricsHandler.WithTags(map[string]string{"error": "invalid token"}).Counter(metrics.ProxyRequestErrors).Inc(1)
			return status.Errorf(codes.Unauthenticated, "invalid token")
		}
	}

	p.logger.Debug("invoking method",
		zap.String("workload-id", workloadId[0]),
		zap.String("method", method),
		zap.Any("args", args),
		zap.Any("md", md),
	)

	namespace.metricsHandler.Counter(metrics.ProxyRequestSuccess).Inc(1)
	namespace.metricsHandler.Timer(metrics.ProxyLatency).Record(time.Since(start))
	return namespace.GetConnection().Invoke(ctx, method, args, reply, opts...)
}

// NewStream implements the grpc.ClientConnInterface NewStream method
func (p *proxyServer) NewStream(ctx context.Context, desc *grpc.StreamDesc, method string, opts ...grpc.CallOption) (grpc.ClientStream, error) {
	return nil, status.Error(codes.Unimplemented, "streams not supported")
}
