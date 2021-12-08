package main

import (
	"context"
	"flag"
	"fmt"
	"math/rand"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/mitchellh/cli"
	"github.com/mkeeler/consul-load-test/load"
)

const (
	txnMaxOps = 64
)

type loadCommand struct {
	ui         cli.Ui
	configPath string
	randSeed   int64
	quiet      bool
	timeout    time.Duration

	flags *flag.FlagSet
	http  *HTTPFlags
	help  string
}

func newLoadCommand(ui cli.Ui) cli.Command {
	c := &loadCommand{
		ui: ui,
	}

	flags := flag.NewFlagSet("", flag.ContinueOnError)

	flags.BoolVar(&c.quiet, "quiet", false, "Whether to suppress output of handling of individual resources")
	flags.Int64Var(&c.randSeed, "seed", 0, "Value to use to seed the pseudo-random number generator with instead of the current time")
	flags.StringVar(&c.configPath, "config", "", "Path to the configuration to use for generating data")
	flags.DurationVar(&c.timeout, "timeout", 5*time.Minute, "How long to run the load generation for")

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

	go func() {
		for {
			var sig os.Signal
			select {
			case s := <-signalCh:
				sig = s
			case <-ctx.Done():
				return
			}

			switch sig {
			case syscall.SIGPIPE:
				continue
			default:
				cancel()
				return
			}
		}
	}()

	load.Load(ctx, client, conf)

	return 0
}

func (c *loadCommand) Synopsis() string {
	return "Generate load on Consul"
}

func (c *loadCommand) Help() string {
	return c.help
}
