package petname

import (
	_ "embed"
	"strings"
)

//go:embed data/adjectives.txt
var adjectivesSrc string

//go:embed data/adverbs.txt
var adverbsSrc string

//go:embed data/names.txt
var namesSrc string

var (
	adjectives []string
	adverbs    []string
	names      []string
)

func init() {
	adjectives = strings.Fields(adjectivesSrc)
	adverbs = strings.Fields(adverbsSrc)
	names = strings.Fields(namesSrc)
}
