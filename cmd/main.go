package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"os"
	"strconv"
	"temporal-sa/temporal-cloud-proxy/auth"
	"temporal-sa/temporal-cloud-proxy/proxy"
	"temporal-sa/temporal-cloud-proxy/utils"

	"github.com/urfave/cli/v2"
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

			lis, err := net.Listen("tcp", cfg.Server.Host+":"+strconv.Itoa(cfg.Server.Port))
			if err != nil {
				return err
			}

			fmt.Printf("listening on %s:%d\n", cfg.Server.Host, cfg.Server.Port)

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
		fmt.Println(
			"Configuring connection from",
			t.Source+":"+strconv.Itoa(cfg.Server.Port),
			"to",
			t.Target,
		)

		fmt.Println(cfg, t.Authentication)

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

		err := proxyConns.AddConn(proxy.AddConnInput{
			Source:          t.Source + ":" + strconv.Itoa(cfg.Server.Port),
			Target:          t.Target,
			TLSCertPath:     t.TLS.CertFile,
			TLSKeyPath:      t.TLS.KeyFile,
			EncryptionKeyID: t.EncryptionKey,
			Namespace:       t.Namespace,
			AuthManager:     authManager,
			AuthType:        authType,
		})

		if err != nil {
			return err
		}
	}

	return nil
}
