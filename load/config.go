package load

import (
	"encoding/json"
	"fmt"
	"os"
	"time"

	"github.com/mkeeler/consul-load-test/load/catalog"
	"github.com/mkeeler/consul-load-test/load/kv"
	"github.com/mkeeler/consul-load-test/load/peering"
)

type Config struct {
	Seed    int64 
	KV      *kv.UserConfig
	Peering *peering.UserConfig
	Catalog *catalog.UserConfig
}

func (c *Config) Normalize() error {
	if c.Seed < 1 {
		c.Seed = time.Now().UnixNano()
	}

	if c.KV != nil {
		if err := c.KV.Normalize(); err != nil {
			return fmt.Errorf("error validating KV configuration: %w", err)
		}
	}

	if c.Peering != nil {
		if err := c.Peering.Normalize(); err != nil {
			return fmt.Errorf("error validating Peering configuration: %w", err)
		}
	}

	if c.Catalog != nil {
		if err := c.Catalog.Normalize(); err != nil {
			return fmt.Errorf("error validating Catalog configuration: %w", err)
		}
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
