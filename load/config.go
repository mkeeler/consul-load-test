package load

import (
	"encoding/json"
	"fmt"
	"os"
)

const (
	defaultKVKeySegments  = 3
	defaultKVMinValueSize = 256
	defaultKVMaxValueSize = 4096
)

type Config struct {
	KV KVConfig
}

func (c *Config) Normalize() error {
	if err := c.KV.Normalize(); err != nil {
		return fmt.Errorf("error validating KV configuration: %w", err)
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

// type CatalogConfig struct {
// 	// NodeUpdateRate is the number of node updates per second
// 	// A node update may be to update an address or node meta
// 	NodeUpdateRate int

// 	// ServiceUpdateRate is the number of service updates per second
// 	// A service update may be to update the address, meta of an existing
// 	// service or to delete/add a service.
// 	ServiceUpdateRate int

// 	// NumNodes is the number of nodes to initialize and maintain
// 	NumNodes int
// 	// MinServicesPerNode is the minimum number of services to register
// 	// to a node
// 	MinServicesPerNode int
// 	// MaxServicesPerNode is the maximum number of services to register
// 	// to a node
// 	MaxServicesPerNode int
// 	// MinMetaPerNode is the minimum number of node meta entries to
// 	// attach to a node
// 	MinMetaPerNode int
// 	// MaxMetaPerNode is the maximum number of node meta entries to
// 	// attach to a node
// 	MaxMetaPerNode int
// 	// MinMetaPerService is the minimum number of meta entries to
// 	// attach to a service
// 	MinMetaPerService int
// 	// MaxMetaPerService is the maximum number of meta entries to
// 	// attach to a service
// 	MaxMetaPerService int
// }
