package main

import (
	"context"
	"go.uber.org/fx"
	"go.uber.org/zap"
	"os"
	"os/signal"
	"syscall"
	"temporal-sa/temporal-cloud-proxy/auth"
	"temporal-sa/temporal-cloud-proxy/config"
	"temporal-sa/temporal-cloud-proxy/metrics"
	"temporal-sa/temporal-cloud-proxy/proxy"
	"temporal-sa/temporal-cloud-proxy/transport"

	"github.com/urfave/cli/v2"
)

var configFilePath string

func run(args []string) error {
	app := buildCLIOptions()
	return app.Run(args)
}

func buildCLIOptions() *cli.App {
	app := &cli.App{
		Name:  "tclp",
		Usage: "Temporal Cloud Proxy",
		Flags: []cli.Flag{
			&cli.StringFlag{
				Name:        config.ConfigPathFlag,
				Usage:       "Path to yaml config file. Default is ./config.yaml",
				Aliases:     []string{"c"},
				Value:       config.DefaultConfigPath,
				Destination: &configFilePath,
			},
			&cli.StringFlag{
				Name:     config.LogLevelFlag,
				Usage:    "Set log level (debug, info, warn, error). Default level is info",
				Required: false,
			},
		},
		Action: startProxy,
	}

	return app
}

func startProxy(c *cli.Context) error {
	// var logCfg log.Config
	if logLevel := c.String(config.LogLevelFlag); len(logLevel) != 0 {
		// logCfg.Level = logLevel
	}

	app := fx.New(
		fx.Provide(
			zap.NewExample,
			func() *cli.Context { return c },
			func() context.Context { return c.Context },
		),

		config.Module,
		metrics.Module,
		auth.Module,
		// encryption.Module, // TODO
		proxy.Module,
		transport.Module,

		fx.Invoke(
			func(metrics.MetricsProvider) {},
			func(transport.TransportProvider) {},
		),
	)

	if err := app.Start(context.Background()); err != nil {
		return err
	}

	<-interruptCh()

	return app.Stop(context.Background())
}

func main() {
	if err := run(os.Args); err != nil {
		panic(err)
	}
}

func interruptCh() <-chan interface{} {
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	ret := make(chan interface{}, 1)
	go func() {
		s := <-c
		ret <- s
		close(ret)
		signal.Stop(c)
	}()

	return ret
}
