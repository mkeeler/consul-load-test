package options

import (
	"math/rand/v2"
	"time"
)

type GeneratorOptions struct {
	RNG *rand.Rand
}

type GeneratorOption func(*GeneratorOptions)

func DefaultOptions() *GeneratorOptions {
	return &GeneratorOptions{
		RNG: rand.New(rand.NewPCG(uint64(time.Now().UnixNano()), 0)),
	}
}

func ApplyGeneratorOptions(opts []GeneratorOption) *GeneratorOptions {
	g := DefaultOptions()
	for _, opt := range opts {
		opt(g)
	}
	return g
}
