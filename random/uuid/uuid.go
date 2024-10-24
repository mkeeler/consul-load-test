package uuid

import (
	"fmt"

	"github.com/mkeeler/consul-load-test/random/bytes"
	"github.com/mkeeler/consul-load-test/random/internal/options"
)

type UUIDGenerator struct {
	bytes *bytes.BytesGenerator
}

func NewUUIDGenerator(opts ...options.GeneratorOption) *UUIDGenerator {
	gopts := options.ApplyGeneratorOptions(opts)
	return &UUIDGenerator{
		bytes: bytes.NewBytesGenerator(gopts.RNG.Uint64()),
	}
}

func (g *UUIDGenerator) Generate() string {
	buf := g.bytes.Generate(16)

	return fmt.Sprintf("%x-%x-%x-%x-%x",
		buf[0:4],
		buf[4:6],
		buf[6:8],
		buf[8:10],
		buf[10:16])
}
