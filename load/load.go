package load

import (
	"context"
	"math/rand/v2"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mkeeler/consul-load-test/load/catalog"
	"github.com/mkeeler/consul-load-test/load/config"
	"github.com/mkeeler/consul-load-test/load/kv"
	"github.com/mkeeler/consul-load-test/load/peering"
	"github.com/mkeeler/consul-load-test/metrics"
	"golang.org/x/sync/errgroup"
)

type SubLoadGenerator interface {
	Initialize(ctx context.Context) error
	Run(ctx context.Context) error
}

type LoadGenerator struct {
	conf       Config
	generators []SubLoadGenerator
}

func NewLoadGenerator(client *api.Client, logger hclog.Logger, conf Config, metricsServer *metrics.MetricsServer) *LoadGenerator {
	lg := &LoadGenerator{
		conf: conf,
	}

	rng := rand.New(rand.NewPCG(uint64(conf.Seed), 0))

	// Configure the base generator runtime configuration. The Seed value will be populated prior to creating
	// each sub-load generator. It will be reset once for each type of possible load that can be run regardless
	// of whether that type of load is enabled. That will ensure that the values used with a particular seed
	// remain predictable across runs even if additional load of another variety is added.
	gc := config.GeneratorConfig{
		Client:        client,
		MetricsServer: metricsServer,
		Logger:        logger,
	}

	gc.Seed = rng.Uint64()
	if lg.conf.Catalog != nil {
		catalogConfig := catalog.Config{
			UserConfig:      *lg.conf.Catalog,
			GeneratorConfig: gc,
		}
		lg.generators = append(lg.generators, catalog.NewLoadGenerator(catalogConfig))
	}

	gc.Seed = rng.Uint64()
	if lg.conf.KV != nil {
		kvConfig := kv.Config{
			UserConfig:      *lg.conf.KV,
			GeneratorConfig: gc,
		}
		lg.generators = append(lg.generators, kv.NewLoadGenerator(kvConfig))

	}

	gc.Seed = rng.Uint64()
	if lg.conf.Peering != nil {
		peeringConfig := peering.Config{
			UserConfig:      *lg.conf.Peering,
			GeneratorConfig: gc,
		}

		lg.generators = append(lg.generators, peering.NewLoadGenerator(peeringConfig))
	}

	return lg
}

func (lg *LoadGenerator) Run(ctx context.Context) error {
	for _, subGen := range lg.generators {
		err := subGen.Initialize(ctx)
		if err != nil {
			return err
		}
	}

	grp, grpCtx := errgroup.WithContext(ctx)
	for _, subGen := range lg.generators {
		grp.Go(func() error {
			return subGen.Run(grpCtx)
		})
	}

	return grp.Wait()
}
