package load

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
)

const (
	defaultKVKeySegments  = 3
	defaultKVMinValueSize = 256
	defaultKVMaxValueSize = 4096
)

type Target int

const (
	TargetKV Target = iota
	TargetPeering
	TargetCatalog
)

func (t Target) GoString() string { return t.String() }

func (t Target) String() string {
	switch t {
	case TargetKV:
		return "kv"
	case TargetPeering:
		return "peering"
	case TargetCatalog:
		return "catalog"
	default:
		return ""
	}
}

type Config struct {
	KV      *KVConfig
	Peering *PeeringConfig
	Catalog *CatalogConfig
	Target  Target
}

func (c *Config) Normalize() error {
	if c.KV != nil && c.Peering != nil {
		return errors.New("error validating configuration, cannot configure both KV and Peering, must choose one or the other")
	}

	if c.KV != nil {
		if err := c.KV.Normalize(); err != nil {
			return fmt.Errorf("error validating KV configuration: %w", err)
		}
		c.Target = TargetKV
	}

	if c.Peering != nil {
		if err := c.Peering.Normalize(); err != nil {
			return fmt.Errorf("error validating Peering configuration: %w", err)
		}
		c.Target = TargetPeering
	}

	if c.Catalog != nil {
		if err := c.Catalog.Normalize(); err != nil {
			return fmt.Errorf("error validating Catalog configuration: %w", err)
		}
		c.Target = TargetCatalog
	}

	return nil
}

func ReadConfig(path string) (Config, error) {
	var conf Config
	fp, err := os.Open(path)
	if err != nil {
		return conf, fmt.Errorf("error opening config: %w", err)
	}

	dec := json.NewDecoder(fp)

	if err := dec.Decode(&conf); err != nil {
		return conf, fmt.Errorf("error decoding config: %w", err)
	}

	if err := conf.Normalize(); err != nil {
		return conf, fmt.Errorf("error normalizing config: %w", err)
	}

	return conf, nil
}


