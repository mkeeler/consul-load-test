package bytes

import (
	"math/rand/v2"
)

type BytesGenerator struct {
	dataSource *rand.ChaCha8
}

func chacha8Seed(rng *rand.Rand) [32]uint8 {
	var seed [32]uint8
	for i := 0; i < 4; i++ {
		val := rng.Uint64()
		seed[(i * 8)] = uint8(val >> 56 & 0xFF)
		seed[(i*8)+1] = uint8(val >> 48 & 0xFF)
		seed[(i*8)+2] = uint8(val >> 40 & 0xFF)
		seed[(i*8)+3] = uint8(val >> 32 & 0xFF)
		seed[(i*8)+4] = uint8(val >> 24 & 0xFF)
		seed[(i*8)+5] = uint8(val >> 16 & 0xFF)
		seed[(i*8)+6] = uint8(val >> 8 & 0xFF)
		seed[(i*8)+7] = uint8(val & 0xFF)
	}
	return seed
}

func NewBytesGenerator(seed uint64) *BytesGenerator {
	rng := rand.New(rand.NewPCG(seed, 0))

	return &BytesGenerator{
		dataSource: rand.NewChaCha8(chacha8Seed(rng)),
	}
}

func (g *BytesGenerator) Generate(numBytes int) []byte {
	buf := make([]byte, numBytes)
	_, _ = g.dataSource.Read(buf)
	return buf
}
