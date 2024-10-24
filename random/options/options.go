package options

import (
	"math/rand/v2"

	"github.com/mkeeler/consul-load-test/random/internal/options"
)

type GeneratorOption = options.GeneratorOption

func WithRand(rng *rand.Rand) GeneratorOption {
	return func(opts *options.GeneratorOptions) {
		opts.RNG = rng
	}
}

func WithSeed(seed uint64) GeneratorOption {
	return func(opts *options.GeneratorOptions) {
		opts.RNG = rand.New(rand.NewPCG(seed, 0))
	}
}
