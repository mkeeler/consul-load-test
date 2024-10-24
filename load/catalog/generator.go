package catalog

import (
	"context"
	"math/rand/v2"

	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/load/config"
	"github.com/mkeeler/consul-load-test/random/petname"
)

var statusList = []string{api.HealthPassing, api.HealthCritical, api.HealthWarning}

type LoadGenerator struct {
	config.GeneratorConfig
	conf UserConfig

	nodes       []*node
	nodeRng     *rand.Rand
	nodeMetaGen *petname.PetnameGenerator

	serviceInstances []*serviceInstance
	serviceRng       *rand.Rand
	serviceMetaGen   *petname.PetnameGenerator

	checks   []*check
	checkRng *rand.Rand

	updateNodes    bool
	updateServices bool
	updateChecks   bool
}

func NewLoadGenerator(conf Config) *LoadGenerator {
	rng := seededRng(conf.Seed)

	return &LoadGenerator{
		conf:            conf.UserConfig,
		GeneratorConfig: conf.GeneratorConfig.WithLogger(conf.Logger.Named("catalog")),
		updateNodes:     conf.NodeUpdateRate != 0.0,
		nodeRng:         seededRng(rng.Uint64()),
		updateServices:  conf.ServiceUpdateRate != 0.0,
		serviceRng:      seededRng(rng.Uint64()),
		updateChecks:    conf.CheckUpdateRate != 0.0,
		checkRng:        seededRng(rng.Uint64()),
	}

}

func (lg *LoadGenerator) registerService(ctx context.Context, instance *serviceInstance) error {
	reg := &api.CatalogRegistration{
		ID:             instance.node.id,
		Node:           instance.node.name,
		SkipNodeUpdate: true,
		Service: &api.AgentService{
			ID:      instance.serviceID,
			Service: instance.serviceName,
			Meta:    generateMeta(lg.serviceRng, lg.conf.MinMetaPerService, lg.conf.MaxMetaPerService),
			Port:    8443,
		},
	}

	opts := &api.WriteOptions{}
	_, err := lg.Client.Catalog().Register(reg, opts.WithContext(ctx))
	return err
}

func (lg *LoadGenerator) registerCheck(ctx context.Context, check *check, status string) error {
	if status == api.HealthAny {
		status = statusList[lg.checkRng.IntN(len(statusList))]
	}

	reg := &api.CatalogRegistration{
		ID:             check.node.id,
		Node:           check.node.name,
		SkipNodeUpdate: true,
		Check: &api.AgentCheck{
			CheckID:   check.checkID,
			Status:    status,
			Type:      "synthetic",
			ServiceID: check.serviceInstance.getID(),
		},
	}

	opts := &api.WriteOptions{}
	_, err := lg.Client.Catalog().Register(reg, opts.WithContext(ctx))
	return err
}

func (lg *LoadGenerator) registerNode(ctx context.Context, node *node) error {
	reg := &api.CatalogRegistration{
		ID:       node.id,
		Node:     node.name,
		Address:  node.ip,
		NodeMeta: generateMeta(lg.nodeRng, lg.conf.MinMetaPerNode, lg.conf.MaxMetaPerNode),
	}

	opts := &api.WriteOptions{}
	_, err := lg.Client.Catalog().Register(reg, opts.WithContext(ctx))
	return err
}
