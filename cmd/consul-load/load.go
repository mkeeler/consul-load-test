package main

import (
	"context"
	"flag"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/hashicorp/go-hclog"
	"github.com/mitchellh/cli"
	"github.com/mkeeler/consul-load-test/load"
	"github.com/mkeeler/consul-load-test/metrics"
)

const (
	txnMaxOps = 64
)

type loadCommand struct {
	ui          cli.Ui
	configPath  string
	randSeed    int64
	quiet       bool
	timeout     time.Duration
	metricsPort int
	reportAddr  string
	levelString string

	flags *flag.FlagSet
	http  *HTTPFlags
	help  string
}

func newLoadCommand(ui cli.Ui) cli.Command {
	c := &loadCommand{
		ui: ui,
	}

	levelChoices := strings.Join([]string{
		hclog.Off.String(),
		hclog.Trace.String(),
		hclog.Debug.String(),
		hclog.Info.String(),
		hclog.Warn.String(),
		hclog.Error.String(),
	}, ", ")

	flags := flag.NewFlagSet("", flag.ContinueOnError)

	flags.BoolVar(&c.quiet, "quiet", false, "Whether to suppress output of handling of individual resources")
	flags.Int64Var(&c.randSeed, "seed", 0, "Value to use to seed the pseudo-random number generator with instead of the current time")
	flags.StringVar(&c.configPath, "config", "", "Path to the configuration to use for generating data")
	flags.DurationVar(&c.timeout, "timeout", 5*time.Minute, "How long to run the load generation for")
	flags.IntVar(&c.metricsPort, "metrics-port", 0, "listening port for metrics path /metrics (default: disabled)")
	flags.StringVar(&c.reportAddr, "report-addr", "", "address to retrieve performance measurement (default: disabled)")
	flags.StringVar(&c.levelString, "log-level", hclog.Info.String(), fmt.Sprintf("Log level. Must be one of [%s]", levelChoices))

	c.http = &HTTPFlags{}
	c.http.MergeAll(flags)

	c.flags = flags
	c.help = genUsage(`Usage: consul-load [OPTIONS]
	
	Generate load on Consul
	
	This command will take in its configuration and generate the corresponding load on 
	a Consul cluster.`, c.flags)

	return c
}

func (c *loadCommand) Run(args []string) int {
	if err := c.flags.Parse(args); err != nil {
		c.ui.Error(fmt.Sprintf("Failed to parse command line arguments: %v", err))
		return 1
	}

	level := hclog.LevelFromString(c.levelString)
	if level == hclog.NoLevel {
		c.ui.Error(fmt.Sprintf("Invalid log level choice: %s", c.levelString))
		return 1
	}

	if c.configPath == "" {
		c.ui.Error(fmt.Sprintf("Must supply a configuration"))
		return 1
	}

	conf, err := load.ReadConfig(c.configPath)
	if err != nil {
		c.ui.Error(fmt.Sprintf("Error reading config: %v", err))
		return 1
	}

	if c.randSeed == 0 {
		c.randSeed = time.Now().UnixNano()
	}
	rand.Seed(c.randSeed)

	client, err := c.http.APIClient()
	if err != nil {
		c.ui.Error(fmt.Sprintf("Error creating API client: %v", err))
		return 1
	}

	// wait for signal
	signalCh := make(chan os.Signal, 10)
	signal.Notify(signalCh, os.Interrupt, syscall.SIGTERM, syscall.SIGPIPE)

	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)

	logger := hclog.New(&hclog.LoggerOptions{
		Name:            "load",
		Level:           level,
		Output:          uiLogWriter(c.ui),
		IncludeLocation: false,
	})

	ctx = hclog.WithContext(ctx, logger)

	var metricsServer *metrics.MetricsServer
	if c.metricsPort != 0 {
		listenAddr := "0.0.0.0:%d"
		metricsAddr := fmt.Sprintf(listenAddr, c.metricsPort)
		metricsServer = metrics.NewMetricsServer(metrics.ServerConfig{
			Addr: metricsAddr,
		})
		go func() {
			logger.Info("Starting Metric Server", "address", metricsServer.Addr)
			if err := metricsServer.ListenAndServe(); err != http.ErrServerClosed {
				logger.Error("error starting metric server", "error", err)
			}
		}()
	}

	go func() {
		shutdownMetricsServer := func() {
			if metricsServer != nil {
				metricsServer.Shutdown(ctx)
			}
		}
		for {
			var sig os.Signal
			select {
			case s := <-signalCh:
				sig = s
			case <-ctx.Done():
				shutdownMetricsServer()
				return
			}

			switch sig {
			case syscall.SIGPIPE:
				continue
			default:
				logger.Info("Shutting down")
				shutdownMetricsServer()
				cancel()
				return
			}
		}
	}()

	start := time.Now()
	loadDone := load.Load(ctx, client, conf, metricsServer)
	logger.Info("Load started")

	<-loadDone
	logger.Info("Load completed")
	if metricsServer != nil && c.reportAddr != "" {
		metrics.KVLoadReport(c.reportAddr, time.Since(start))
	}
	return 0
}

func (c *loadCommand) Synopsis() string {
	return "Generate load on Consul"
}

func (c *loadCommand) Help() string {
	return c.help
}

func uiLogWriter(ui cli.Ui) io.Writer {
	return hclog.NewLeveledWriter(
		uiWriter(ui.Output),
		map[hclog.Level]io.Writer{
			hclog.Info:  uiWriter(ui.Info),
			hclog.Error: uiWriter(ui.Error),
			hclog.Warn:  uiWriter(ui.Warn),
		},
	)
}

type uiWriter func(string)

func (write uiWriter) Write(p []byte) (n int, err error) {
	// trim the newline as the cli.Ui will add it on for us.
	write(strings.TrimRight(string(p), "\n"))
	return len(p), nil
}
