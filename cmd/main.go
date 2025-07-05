package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"temporal-sa/temporal-cloud-proxy/auth"
	"temporal-sa/temporal-cloud-proxy/crypto"
	"temporal-sa/temporal-cloud-proxy/metrics"
	"temporal-sa/temporal-cloud-proxy/proxy"
	"temporal-sa/temporal-cloud-proxy/utils"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/urfave/cli/v2"
	"go.opentelemetry.io/otel/attribute"
	"go.temporal.io/api/workflowservice/v1"
	"go.temporal.io/sdk/client"
	"google.golang.org/grpc"
)

var configFilePath string

// TODO: graceful shutdown
func main() {
	app := &cli.App{
		Name:  "tclp",
		Usage: "Temporal Cloud Proxy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        "config",
				Usage:       "config file",
				Aliases:     []string{"c"},
				Value:       "config.yaml",
				Destination: &configFilePath,
			},
		},
		Action: func(*cli.Context) error {
			configManager, err := utils.NewConfigManager(configFilePath)
			if err != nil {
				return err
			}
			defer configManager.Close()

			cfg := configManager.GetConfig()

			proxyConns := proxy.NewConn()
			defer proxyConns.CloseAll()

			if err := configureProxy(proxyConns, cfg); err != nil {
				return err
			}

			workflowClient := workflowservice.NewWorkflowServiceClient(proxyConns)

			handler, err := client.NewWorkflowServiceProxyServer(
				client.WorkflowServiceProxyOptions{Client: workflowClient},
			)
			if err != nil {
				return err
			}

			grpcServer := grpc.NewServer()
			workflowservice.RegisterWorkflowServiceServer(grpcServer, handler)

			// Initialize metrics
			metrics.InitPrometheus()
			metricsServer := &http.Server{Addr: ":" + strconv.Itoa(cfg.Metrics.Port)}
			http.Handle(metrics.DefaultPrometheusPath, promhttp.Handler())
			go func() {
				fmt.Printf("Metrics is exposed at %s:%d%s\n", cfg.Server.Host, cfg.Metrics.Port, metrics.DefaultPrometheusPath)
				if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
					log.Printf("metrics server error: %v", err)
				}
			}()

			c := make(chan os.Signal, 1)
			signal.Notify(c, os.Interrupt, syscall.SIGTERM)
			go func() {
				<-c
				fmt.Println("\nShutting down gracefully...")
				grpcServer.GracefulStop()
				os.Exit(0)
			}()

			lis, err := net.Listen("tcp", cfg.Server.Host+":"+strconv.Itoa(cfg.Server.Port))
			if err != nil {
				return err
			}

			fmt.Printf("Proxy is listening on %s:%d\n", cfg.Server.Host, cfg.Server.Port)

			err = grpcServer.Serve(lis)

			return err
		},
	}

	if err := app.Run(os.Args); err != nil {
		log.Fatalln(err)
	}
}

func configureProxy(proxyConns *proxy.Conn, cfg *utils.Config) error {
	ctx := context.TODO()

	for _, t := range cfg.Targets {
		var authManager *auth.AuthManager
		var authType string

		if t.Authentication != nil {
			authManager = auth.NewAuthManager()
			authType = t.Authentication.Type

			switch authType {
			case "spiffe":
				spiffeAuth := &auth.SpiffeAuthenticator{
					TrustDomain: t.Authentication.Config["trust_domain"].(string),
					Endpoint:    t.Authentication.Config["endpoint"].(string),
				}

				if audiences, ok := t.Authentication.Config["audiences"].([]interface{}); ok {
					for _, a := range audiences {
						if audience, ok := a.(string); ok {
							spiffeAuth.Audiences = append(spiffeAuth.Audiences, audience)
						}
					}
				}

				if err := spiffeAuth.Init(ctx, t.Authentication.Config); err != nil {
					return fmt.Errorf("failed to initialize spiffe authenticator: %w", err)
				}

				if err := authManager.RegisterAuthenticator(spiffeAuth); err != nil {
					return err
				}

			default:
				return fmt.Errorf("unsupported authentication type: %s", authType)
			}
		}

		metricsHandler := metrics.NewMetricsHandler(metrics.MetricsHandlerOptions{
			// Todo: do we need these many attributes?
			InitialAttributes: attribute.NewSet(
				attribute.String("source", t.Source),
				attribute.String("target", t.Target),
				attribute.String("namespace", t.Namespace),
				attribute.String("auth_type", authType),
				attribute.String("encryption_key", t.EncryptionKey),
			),
		})

		// Parse global caching config
		var cachingConfig *crypto.CachingConfig
		if cfg.Encryption.Caching.MaxCache > 0 || cfg.Encryption.Caching.MaxAge != "" || cfg.Encryption.Caching.MaxUsage > 0 {
			cachingConfig = &crypto.CachingConfig{
				MaxCache:        cfg.Encryption.Caching.MaxCache,
				MaxMessagesUsed: cfg.Encryption.Caching.MaxUsage,
			}
			if cfg.Encryption.Caching.MaxAge != "" {
				if duration, err := time.ParseDuration(cfg.Encryption.Caching.MaxAge); err == nil {
					cachingConfig.MaxAge = duration
				}
			}
		}

		err := proxyConns.AddConn(proxy.AddConnInput{
			Source:              t.Source,
			Target:              t.Target,
			TLSCertPath:         t.TLS.CertFile,
			TLSKeyPath:          t.TLS.KeyFile,
			EncryptionKeyID:     t.EncryptionKey,
			Namespace:           t.Namespace,
			AuthManager:         authManager,
			AuthType:            authType,
			MetricsHandler:      metricsHandler,
			CryptoCachingConfig: cachingConfig,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
