package catalog

import (
	"context"
	"fmt"
	"sync"

	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/random/ip"
	"github.com/mkeeler/consul-load-test/random/options"
	"github.com/mkeeler/consul-load-test/random/petname"
	"github.com/mkeeler/consul-load-test/random/uuid"
)

type node struct {
	name string
	id   string
	ip   string
}

type serviceInstance struct {
	*node
	serviceName string
	serviceID   string
	kind        api.ServiceKind
	port        int
	proxy       *api.AgentServiceConnectProxyConfig
}

func (instance *serviceInstance) getID() string {
	if instance == nil {
		return ""
	}

	return instance.serviceID
}

type check struct {
	*node
	*serviceInstance
	checkID string
}

func (lg *LoadGenerator) Initialize(ctx context.Context) error {
	lg.initializeData()
	return lg.pushAllData(ctx)
}

func (lg *LoadGenerator) initializeData() {
	lg.Logger.Info("generating all Consul resources")
	defer lg.Logger.Info("finished generating all Consul resources")

	ipGen := ip.NewTestingIPv4Generator(options.WithSeed(lg.nodeRng.Uint64()))

	// generate the set of service names to allocate from
	serviceNameGen := petname.NewPetnameGenerator(3, "-", options.WithSeed(lg.serviceRng.Uint64()))
	serviceNames := make([]string, lg.conf.NumServices)
	lg.Logger.Trace("generating node names", "nodes", lg.conf.NumNodes)
	for i := 0; i < lg.conf.NumServices; i++ {
		serviceNames[i] = serviceNameGen.Generate()
	}

	// generate all the nodes, service instances and checks
	nodeNameGen := petname.NewPetnameGenerator(2, "-", options.WithSeed(lg.nodeRng.Uint64()))
	nodeIDGen := uuid.NewUUIDGenerator(options.WithSeed(lg.nodeRng.Uint64()))
	lg.Logger.Trace("generating nodes", "nodes", lg.conf.NumNodes)
	for i := 0; i < lg.conf.NumNodes; i++ {
		node := &node{
			name: nodeNameGen.Generate(),
			id:   nodeIDGen.Generate(),
			ip:   ipGen.GenerateIP().String(),
		}

		lg.nodes = append(lg.nodes, node)

		nodeCheckCount := randInterval(lg.nodeRng, lg.conf.MinChecksPerNode, lg.conf.MaxChecksPerNode)
		lg.Logger.Trace("generating node checks", "node", node.name, "checks", nodeCheckCount)
		for nodeCheckIdx := 0; nodeCheckIdx < nodeCheckCount; nodeCheckIdx++ {
			lg.checks = append(lg.checks, &check{
				node:    node,
				checkID: fmt.Sprintf("%s-check-%d", node.id, nodeCheckIdx),
			})
		}

		svcCount := randInterval(lg.serviceRng, lg.conf.MinServicesPerNode, lg.conf.MaxServicesPerNode)
		lg.Logger.Trace("generating node service instances", "node", node.name, "services", svcCount)
		for svcIndex := 0; svcIndex < svcCount; svcIndex++ {
			svcName := serviceNames[lg.serviceRng.IntN(len(serviceNames))]

			instance := &serviceInstance{
				node:        node,
				serviceName: svcName,
				serviceID:   fmt.Sprintf("%s-%d", svcName, svcIndex),
				port:        8443,
			}

			lg.serviceInstances = append(lg.serviceInstances, instance)

			svcCheckCount := randInterval(lg.checkRng, lg.conf.MinChecksPerService, lg.conf.MaxChecksPerService)
			lg.Logger.Trace("generating service instance checks", "node", node.name, "service-id", instance.serviceID, "checks", svcCheckCount)
			for svcCheckIdx := 0; svcCheckIdx < svcCheckCount; svcCheckIdx++ {
				lg.checks = append(lg.checks, &check{
					node:            node,
					serviceInstance: instance,
					checkID:         fmt.Sprintf("%s-check-%d", instance.serviceID, svcCheckIdx),
				})
			}

			// Generate the corresponding
			if lg.conf.IncludeConnectSidecars {
				proxyInstance := &serviceInstance{
					kind:        api.ServiceKindConnectProxy,
					node:        node,
					serviceName: fmt.Sprintf("%s-sidecar-proxy", instance.serviceName),
					serviceID:   fmt.Sprintf("%s-sidecar-proxy", instance.serviceID),
					port:        9443,
					proxy: &api.AgentServiceConnectProxyConfig{
						DestinationServiceName: instance.serviceName,
						DestinationServiceID:   instance.serviceID,
						LocalServicePort:       8443,
						LocalServiceAddress:    "127.0.0.1",
					},
				}
				lg.serviceInstances = append(lg.serviceInstances, proxyInstance)

				lg.checks = append(lg.checks, &check{
					node:            node,
					serviceInstance: proxyInstance,
					checkID:         fmt.Sprintf("%s-check", proxyInstance.serviceID),
				})
			}
		}
	}
}

func (lg *LoadGenerator) pushAllData(ctx context.Context) error {
	lg.Logger.Info("registering all Consul resources")
	defer lg.Logger.Info("finished registering all Consul resources")

	var wg sync.WaitGroup
	lg.Logger.Info("registering all nodes with Consul", "count", len(lg.nodes))
	for _, node := range lg.nodes {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lg.registerNode(ctx, node)
		}()
	}

	// wait for all nodes to be registered
	wg.Wait()

	if ctxIsDone(ctx) {
		return ctx.Err()
	}

	lg.Logger.Info("registering all service instances with Consul", "count", len(lg.serviceInstances))
	for _, svc := range lg.serviceInstances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lg.registerService(ctx, svc)
		}()
	}

	// wait for all services to be registered
	wg.Wait()

	if ctxIsDone(ctx) {
		return ctx.Err()
	}

	lg.Logger.Info("registering all checks with Consul", "count", len(lg.checks))
	for _, check := range lg.checks {
		wg.Add(1)
		go func() {
			defer wg.Done()
			lg.registerCheck(ctx, check, api.HealthPassing)
		}()
	}

	wg.Wait()

	if ctxIsDone(ctx) {
		return ctx.Err()
	}

	return nil
}

func ctxIsDone(ctx context.Context) bool {
	select {
	case <-ctx.Done():
		return true
	default:
		return false
	}
}
