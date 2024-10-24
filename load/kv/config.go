package kv

import (
	"fmt"

	"github.com/mkeeler/consul-load-test/load/config"
	"golang.org/x/time/rate"
)

const (
	defaultKVKeySegments  = 3
	defaultKVMinValueSize = 256
	defaultKVMaxValueSize = 4096
)

// UserConfig is the configuration for the KV load generator
type UserConfig struct {
	// NumKeys is the number of keys to initialize and maintain
	NumKeys int
	// UpdateRate is the number of KV updates per second
	UpdateRate rate.Limit

	// KeySegments is the number of segments for each pet name
	KeySegments int
	// KeySeparator is the character to separate the different key
	// segments
	KeySeparator string

	// MinValueSize is the minimum size of kv values
	MinValueSize int
	// MaxValueSize is the maximum size of kv values
	MaxValueSize int
}

func (c *UserConfig) Normalize() error {
	if c.NumKeys < 1 {
		return nil
	}

	if c.UpdateRate == 0.0 {
		return fmt.Errorf("invalid UpdateRate configuration: %v", c.UpdateRate)
	}

	if c.KeySegments < 1 {
		c.KeySegments = defaultKVKeySegments
	}

	if c.KeySeparator == "" {
		c.KeySeparator = "/"
	}

	if c.MinValueSize < 1 {
		c.MinValueSize = defaultKVMinValueSize
	}

	if c.MaxValueSize < 1 {
		c.MaxValueSize = defaultKVMaxValueSize
	}

	if c.MinValueSize > c.MaxValueSize {
		c.MaxValueSize = c.MinValueSize
	}

	return nil
}

type Config struct {
	UserConfig
	config.GeneratorConfig
}
