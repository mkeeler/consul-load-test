package catalog

import (
	"fmt"

	"github.com/mkeeler/consul-load-test/load/config"
	"golang.org/x/time/rate"
)

type performRateLimiting bool

const (
	disableRateLimiting performRateLimiting = false
	enableRateLimiting                      = true
)

type UserConfig struct {
	// NodeUpdateRate is the number of node updates per second
	// A node update may be to update an address or node meta
	NodeUpdateRate rate.Limit

	// ServiceUpdateRate is the number of service updates per second
	// A service update may be to update the address, meta of an existing
	// service or to delete/add a service.
	ServiceUpdateRate rate.Limit

	// CheckUpdateRate is the number of check updates per second
	// A check update may be to add/remove a check, update a checks status
	// or to change check output
	CheckUpdateRate rate.Limit

	// NumNodes is the number of nodes to initialize and maintain
	NumNodes int

	// NumServices is the total number of services (not instances) to allocate names
	// from
	NumServices int

	// MinServicesPerNode is the minimum number of services to register
	// to a node
	MinServicesPerNode int
	// MaxServicesPerNode is the maximum number of services to register
	// to a node
	MaxServicesPerNode int
	// MinChecksPerService is the minimum number of checks to register
	// to a service
	MinChecksPerNode int
	// MaxChecksPerService is the maximum number of checks to register
	// to a service
	MaxChecksPerNode int
	// MinChecksPerService is the minimum number of checks to register
	// to a service
	MinChecksPerService int
	// MaxChecksPerService is the maximum number of checks to register
	// to a service
	MaxChecksPerService int
	// MinMetaPerNode is the minimum number of node meta entries to
	// attach to a node
	MinMetaPerNode int
	// MaxMetaPerNode is the maximum number of node meta entries to
	// attach to a node
	MaxMetaPerNode int
	// MinMetaPerService is the minimum number of meta entries to
	// attach to a service
	MinMetaPerService int
	// MaxMetaPerService is the maximum number of meta entries to
	// attach to a service
	MaxMetaPerService int
}

func (c *UserConfig) Normalize() error {
	if c.NumNodes < 1 {
		return nil
	}

	if c.NumServices < 1 {
		return nil
	}

	if c.MinServicesPerNode < 0 {
		return fmt.Errorf("MinServicesPerNode cannot be negative")
	}

	if c.MaxServicesPerNode < 0 {
		return fmt.Errorf("MaxServicesPerNode cannot be negative")
	}

	if c.MaxServicesPerNode < c.MinServicesPerNode {
		return fmt.Errorf("MaxServicesPerNode cannot be less than MinServicesPerNode")
	}

	if c.MinMetaPerNode < 0 {
		return fmt.Errorf("MinMetaPerNode cannot be negative")
	}

	if c.MaxMetaPerNode < 0 {
		return fmt.Errorf("MaxMetaPerNode cannot be negative")
	}

	if c.MaxMetaPerNode < c.MinMetaPerNode {
		return fmt.Errorf("MaxMetaPerNode cannot be less than MinMetaPerNode")
	}

	if c.MinMetaPerService < 0 {
		return fmt.Errorf("MinMetaPerService cannot be negative")
	}

	if c.MaxMetaPerService < 0 {
		return fmt.Errorf("MaxMetaPerService cannot be negative")
	}

	if c.MaxMetaPerService < c.MinMetaPerService {
		return fmt.Errorf("MaxMetaPerService cannot be less than MinMetaPerService")
	}

	if c.MinChecksPerNode < 0 {
		return fmt.Errorf("MinChecksPerNode cannot be negative")
	}

	if c.MaxChecksPerNode < 0 {
		return fmt.Errorf("MaxChecksPerNode cannot be negative")
	}

	if c.MaxChecksPerNode < c.MinChecksPerNode {
		return fmt.Errorf("MaxChecksPerNode cannot be less than MinChecksPerNode")
	}

	if c.MinChecksPerService < 0 {
		return fmt.Errorf("MinChecksPerService cannot be negative")
	}

	if c.MaxChecksPerService < 0 {
		return fmt.Errorf("MaxChecksPerService cannot be negative")
	}

	if c.MaxChecksPerService < c.MinChecksPerService {
		return fmt.Errorf("MaxChecksPerService cannot be less than MinChecksPerService")
	}

	return nil
}

type Config struct {
	UserConfig
	config.GeneratorConfig
}
