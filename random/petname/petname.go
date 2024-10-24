package petname

import (
	"strings"

	"github.com/mkeeler/consul-load-test/random/internal/options"
)

type PetnameGenerator struct {
	*options.GeneratorOptions
	words     int
	separator string
}

func NewPetnameGenerator(words int, separator string, opts ...options.GeneratorOption) *PetnameGenerator {
	return &PetnameGenerator{
		GeneratorOptions: options.ApplyGeneratorOptions(opts),
		words:            words,
		separator:        separator,
	}
}

func (g *PetnameGenerator) Generate() string {
	switch g.words {
	case 1:
		return g.name()
	case 2:
		return g.adjective() + g.separator + g.name()
	default:
		parts := make([]string, g.words)
		for i := 0; i < g.words-2; i++ {
			parts[i] = g.adverb()
		}
		parts[g.words-2] = g.adjective()
		parts[g.words-1] = g.name()
		return strings.Join(parts, g.separator)
	}
}

func (g *PetnameGenerator) name() string {
	return names[g.RNG.IntN(len(names))]
}

func (g *PetnameGenerator) adjective() string {
	return adjectives[g.RNG.IntN(len(adjectives))]
}

func (g *PetnameGenerator) adverb() string {
	return adverbs[g.RNG.IntN(len(adverbs))]
}
