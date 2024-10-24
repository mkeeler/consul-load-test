package catalog

import (
	"fmt"
	"math/rand/v2"

	"github.com/mkeeler/consul-load-test/random/options"
	"github.com/mkeeler/consul-load-test/random/petname"
)

func seededRng(seed uint64) *rand.Rand {
	return rand.New(rand.NewPCG(seed, 0))
}

func randInterval(rng *rand.Rand, min, max int) int {
	if min == max {
		return min
	}

	return min + rng.IntN(max-min)
}

func generateMeta(rng *rand.Rand, min, max int) map[string]string {
	generator := petname.NewPetnameGenerator(2, "", options.WithRand(rng))
	numMeta := randInterval(rng, min, max)
	newMeta := make(map[string]string)
	for i := 0; i < numMeta; i++ {
		value := generator.Generate()
		newMeta[fmt.Sprintf("meta-%d", i)] = value
	}
	return newMeta
}
