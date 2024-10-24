package kv

import (
	"context"
	"math/rand/v2"
	"time"

	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/load/config"
	"github.com/mkeeler/consul-load-test/random/base64"
	"github.com/mkeeler/consul-load-test/random/options"
	"github.com/mkeeler/consul-load-test/random/petname"
	"golang.org/x/time/rate"
)

type LoadGenerator struct {
	config.GeneratorConfig
	conf UserConfig
	rng  *rand.Rand

	keys     []string
	valueGen *base64.RandomB64Generator
}

func NewLoadGenerator(conf Config) *LoadGenerator {
	return &LoadGenerator{
		GeneratorConfig: conf.GeneratorConfig,
		conf:            conf.UserConfig,
		rng:             rand.New(rand.NewPCG(conf.Seed, 0)),
	}
}

func (lg *LoadGenerator) Initialize(ctx context.Context) error {
	lg.keys = make([]string, lg.conf.NumKeys)

	petnames := petname.NewPetnameGenerator(lg.conf.KeySegments, lg.conf.KeySeparator, options.WithSeed(lg.rng.Uint64()))

	for i := 0; i < lg.conf.NumKeys; i++ {
		lg.keys[i] = petnames.Generate()
	}

	lg.valueGen = base64.NewRandomB64Generator(lg.conf.MinValueSize, lg.conf.MaxValueSize, options.WithSeed(lg.rng.Uint64()))

	return nil
}

func (lg *LoadGenerator) Run(ctx context.Context) error {
	limiter := rate.NewLimiter(lg.conf.UpdateRate, int(lg.conf.UpdateRate*10))

	for {
		err := limiter.Wait(ctx)
		if err != nil {
			return err
		}

		key := lg.keys[lg.rng.IntN(len(lg.keys))]

		value := lg.valueGen.Generate()

		pair := api.KVPair{
			Key:   key,
			Value: []byte(value),
		}

		go lg.sendKey(pair)
	}
}

func (lg *LoadGenerator) sendKey(pair api.KVPair) {
	start := time.Now()
	_, err := lg.Client.KV().Put(&pair, nil)
	if lg.MetricsServer != nil {
		duration := time.Since(start)
		if err == nil {
			lg.MetricsServer.IncLatencyHistogram(duration, "kv", "success")
		} else {
			lg.MetricsServer.IncLatencyHistogram(duration, "kv", "error")
		}
	}
}
