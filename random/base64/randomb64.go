package base64

import (
	"encoding/base64"
	"math/rand/v2"

	"github.com/mkeeler/consul-load-test/random/bytes"
	internalopts "github.com/mkeeler/consul-load-test/random/internal/options"
	"github.com/mkeeler/consul-load-test/random/options"
)

type RandomB64Generator struct {
	*internalopts.GeneratorOptions
	bytes   *bytes.BytesGenerator
	rng     *rand.Rand
	minSize int
	maxSize int
}

func NewRandomB64Generator(minSize int, maxSize int, opts ...options.GeneratorOption) *RandomB64Generator {
	gopts := internalopts.ApplyGeneratorOptions(opts)

	return &RandomB64Generator{
		GeneratorOptions: gopts,
		bytes:            bytes.NewBytesGenerator(gopts.RNG.Uint64()),
		minSize:          minSize,
		maxSize:          maxSize,
	}
}

func (g *RandomB64Generator) Generate() string {
	raw := g.bytes.Generate(g.minSize + g.rng.IntN(g.maxSize-g.minSize))

	encoded := make([]byte, base64.StdEncoding.EncodedLen(len(raw)))
	base64.StdEncoding.Encode(encoded, raw)
	return string(encoded)
}
