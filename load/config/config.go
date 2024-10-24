package config

import (
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mkeeler/consul-load-test/metrics"
)

type GeneratorConfig struct {
	// Seed is a 64 bit integer value used to seed creation
	// of a load generators pseudo random number generator
	Seed uint64

	// Client is the Consul API client that should be used
	// for interacting with Consul
	Client *api.Client

	// MetricsServer is the metrics server that a load generator
	// may use for emitting metrics
	MetricsServer *metrics.MetricsServer

	// Logger is a logger.
	Logger hclog.Logger
}

func (gc GeneratorConfig) WithSeed(seed uint64) GeneratorConfig {
	// because the receiver is not a pointer value we can just
	// override and return.
	gc.Seed = seed
	return gc
}

func (gc GeneratorConfig) WithLogger(logger hclog.Logger) GeneratorConfig {
	// because the receiver is not a pointer value we can just
	// override and return.
	gc.Logger = logger
	return gc
}
