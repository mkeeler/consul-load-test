package peering

import (
	"fmt"
	"time"

	"github.com/mkeeler/consul-load-test/load/config"
	"golang.org/x/time/rate"
)

const (
	// acceptorPeerName is the peer name used by the peering clusters
	// the reverse will be <peering address>-<port>
	acceptorPeerName = "acceptor"
)

var defaultExportTimeout = time.Duration(180 * time.Second)

// UserConfig is the configuration for the Peering load generator
type UserConfig struct {
	// RegisterLimit is the time in seconds to wait between registering services
	RegisterLimit rate.Limit
	// ExportTimeout is the maximum time to wait for services to become exported to a remote peer
	ExportTimeout time.Duration
	// PeeringClusters holds the addresses for the servers that will be peered
	PeeringClusters []string
	// NumServices is the number of services to register and export during the load test
	NumServices int
}

// Normalize validates the Peering load test configuration. RegisterLimit and
// NumServices must not be zero, otherwise, an error is returned.
func (c *UserConfig) Normalize() error {
	if c.RegisterLimit == 0.0 {
		return fmt.Errorf("invalid RegisterLimit configuration: %v", c.RegisterLimit)
	}
	if c.NumServices == 0 {
		return fmt.Errorf("invalid NumServices configuration: %v", c.NumServices)
	}
	if c.ExportTimeout == 0 {
		c.ExportTimeout = defaultExportTimeout
	}
	return nil
}

type Config struct {
	UserConfig
	config.GeneratorConfig
}
