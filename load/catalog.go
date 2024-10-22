package load

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"sync/atomic"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-uuid"
	"github.com/mkeeler/consul-data/generate/generators"
	"github.com/mkeeler/consul-load-test/metrics"
	"golang.org/x/sync/errgroup"
	"golang.org/x/time/rate"
)

type performRateLimiting bool

const (
	disableRateLimiting performRateLimiting = false
	enableRateLimiting                      = true
)

type CatalogConfig struct {
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

func (c *CatalogConfig) Normalize() error {
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

func catalogLoad(ctx context.Context, client *api.Client, conf Config, _ *metrics.MetricsServer) <-chan struct{} {
	logger := hclog.FromContext(ctx).Named("catalog")
	ctx = hclog.WithContext(ctx, logger)

	done := make(chan struct{})

	updater := updater{
		client:         client,
		conf:           conf.Catalog,
		updateNodes:    conf.Catalog.NodeUpdateRate != 0.0,
		updateServices: conf.Catalog.ServiceUpdateRate != 0.0,
		updateChecks:   conf.Catalog.CheckUpdateRate != 0.0,
	}

	logger.Trace("generating node names", "nodes", conf.Catalog.NumNodes)
	nodeNames := make([]string, conf.Catalog.NumNodes)
	for i := 0; i < conf.Catalog.NumNodes; i++ {
		nodeNames[i] = petname.Generate(2, "-")
	}
	if updater.updateNodes {
		updater.nodeLimiter = &wrappedLimiter{limiter: rate.NewLimiter(conf.Catalog.NodeUpdateRate, int(conf.Catalog.NodeUpdateRate*10))}
	}

	logger.Trace("generating service names", "nodes", conf.Catalog.NumServices)
	serviceNames := make([]string, conf.Catalog.NumServices)
	for i := 0; i < conf.Catalog.NumServices; i++ {
		serviceNames[i] = petname.Generate(3, "-")
	}
	if updater.updateServices {
		updater.serviceLimiter = &wrappedLimiter{limiter: rate.NewLimiter(conf.Catalog.ServiceUpdateRate, int(conf.Catalog.ServiceUpdateRate*10))}
	}

	if updater.updateChecks {
		updater.checkLimiter = &wrappedLimiter{limiter: rate.NewLimiter(conf.Catalog.CheckUpdateRate, int(conf.Catalog.CheckUpdateRate*10))}
	}

	go func() {
		logger.Info("Starting catalog load", "num-nodes", len(nodeNames))
		defer logger.Info("Stopped catalog load")
		defer close(done)

		done := make(chan struct{}, len(nodeNames))
		grp, grpCtx := errgroup.WithContext(ctx)
		for _, nodeName := range nodeNames {
			nodeName := nodeName
			grp.Go(func() error {
				return updater.run(grpCtx, nodeName, serviceNames, done)
			})
		}

		for finishedInit := 0; finishedInit < len(nodeNames); finishedInit++ {
			select {
			case <-ctx.Done():
				break
			case <-grpCtx.Done():
				break
			case <-done:
			}
		}
		logger.Info("Finished catalog load initialization")

		err := nilContextError(grp.Wait())
		if err != nil {
			logger.Error("error running catalog load", "error", err)
		}
	}()

	return done
}

type updater struct {
	client         *api.Client
	conf           *CatalogConfig
	updateNodes    bool
	nodeLimiter    *wrappedLimiter
	updateServices bool
	serviceLimiter *wrappedLimiter
	updateChecks   bool
	checkLimiter   *wrappedLimiter
}

func (u *updater) run(ctx context.Context, nodeName string, serviceNames []string, notifyInitDone <-chan struct{}) error {
	logger := hclog.FromContext(ctx).Named("node").With("node-name", nodeName)
	ctx = hclog.WithContext(ctx, logger)
	logger.Debug("started node updater")
	defer logger.Debug("stopped node updater")

	ip, err := generators.RandomTestingIPGenerator()()
	if err != nil {
		logger.Error("failed to generate random IP", "error", err)
		return fmt.Errorf("failed to generate random IP: %w", err)
	}

	nodeID, err := uuid.GenerateUUID()
	if err != nil {
		logger.Error("failed to generate random uuid", "error", err)
		return fmt.Errorf("failed to generate random uuid: %w", err)
	}

	logger.Debug("registering node for the first time")
	err = u.registerNode(ctx, nodeName, nodeID, ip.String(), disableRateLimiting)
	if err != nil {
		logger.Error("error performing initial node registration", "error", err)
		return err
	}

	grp, grpCtx := errgroup.WithContext(ctx)

	// start the various go routines for updating the services for this node
	svcCount := randInterval(u.conf.MinServicesPerNode, u.conf.MaxServicesPerNode)
	logger.Debug("starting all service instance updaters", "count", svcCount)
	for i := 0; i < svcCount; i++ {
		svcName := serviceNames[rand.Intn(len(serviceNames))]
		svcID := fmt.Sprintf("%s-%d", svcName, i)

		grp.Go(func() error {
			return u.runInstanceUpdate(grpCtx, nodeName, nodeID, svcName, svcID)
		})
	}

	if u.updateNodes {
		grp.Go(func() error {
			return u.runNodeMetaUpdate(grpCtx, nodeName, nodeID, ip.String())
		})
	}

	return grp.Wait()
}

func (u *updater) runInstanceUpdate(ctx context.Context, nodeName, nodeID, serviceName, serviceID string) error {
	logger := hclog.FromContext(ctx).Named("service-instance").With("service-id", serviceID)
	ctx = hclog.WithContext(ctx, logger)
	logger.Debug("started service instance updater")
	defer logger.Debug("stopped service instance updater")

	logger.Debug("registering service instance for the first time")
	err := u.registerService(ctx, nodeName, nodeID, serviceName, serviceID, disableRateLimiting)
	if err != nil {
		logger.Error("error performing initial service registration", "error", err)
		return err
	}

	grp, grpCtx := errgroup.WithContext(ctx)

	// start the various go routines for updating the services for this node
	checkCount := randInterval(u.conf.MinChecksPerService, u.conf.MaxChecksPerService)
	logger.Debug("starting all check updaters", "count", checkCount)
	for i := 0; i < checkCount; i++ {
		checkID := fmt.Sprintf("check-%d", i)

		grp.Go(func() error {
			return u.runCheckUpdate(grpCtx, nodeName, nodeID, serviceID, checkID)
		})
	}

	if u.updateServices {
		grp.Go(func() error {
			return u.runServiceMetaUpdate(grpCtx, nodeName, nodeID, serviceName, serviceID)
		})
	}

	return grp.Wait()
}

func (u *updater) runCheckUpdate(ctx context.Context, nodeName, nodeID, serviceID, checkID string) error {
	logger := hclog.FromContext(ctx).Named("check").With("check-id", checkID)
	ctx = hclog.WithContext(ctx, logger)
	logger.Debug("started check updater")
	defer logger.Debug("stopped check updater")

	logger.Debug("registering check for the first time")
	err := u.registerCheck(ctx, nodeName, nodeID, serviceID, checkID, disableRateLimiting)
	if err != nil {
		logger.Error("failed to register check", "error", err)
		return fmt.Errorf("failed to update check: %w", err)
	}

	if u.updateChecks {
		for {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
				logger.Debug("registering check")
				err := u.registerCheck(ctx, nodeName, nodeID, serviceID, checkID, enableRateLimiting)
				if err != nil {
					logger.Error("failed to register check", "error", err)
					return fmt.Errorf("failed to update check: %w", err)
				}
			}
		}
	}
	return nil
}

func (u *updater) runNodeMetaUpdate(ctx context.Context, nodeName, nodeID, address string) error {
	logger := hclog.FromContext(ctx).Named("meta")
	ctx = hclog.WithContext(ctx, logger)
	logger.Debug("started node meta updater")
	defer logger.Debug("stopped node meta updater")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			logger.Debug("registering node")
			err := u.registerNode(ctx, nodeName, nodeID, address, enableRateLimiting)
			if err != nil {
				logger.Debug("failed to register node", "error", err)
				return fmt.Errorf("failed to update node meta for node %s: %w", nodeName, err)
			}
		}
	}
}

func (u *updater) runServiceMetaUpdate(ctx context.Context, nodeName, nodeID, serviceName, serviceID string) error {
	logger := hclog.FromContext(ctx).Named("meta")
	ctx = hclog.WithContext(ctx, logger)
	logger.Debug("started service meta updater")
	defer logger.Debug("stopped service meta updater")

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		default:
			logger.Debug("registering node")
			err := u.registerService(ctx, nodeName, nodeID, serviceName, serviceID, enableRateLimiting)
			if err != nil {
				logger.Debug("failed to register node", "error", err)
				return fmt.Errorf("failed to update service meta for node %s: %w", nodeName, err)
			}
		}
	}
}

var statusList = []string{api.HealthPassing, api.HealthCritical, api.HealthWarning}

func (u *updater) registerCheck(ctx context.Context, nodeName, nodeID, serviceID, checkID string, rateLimit performRateLimiting) error {
	status := api.HealthPassing
	if rateLimit {
		status = statusList[rand.Intn(len(statusList))]
		err := u.checkLimiter.Wait(ctx)
		if err != nil {
			return nilContextError(err)
		}
	}

	reg := &api.CatalogRegistration{
		ID:             nodeID,
		Node:           nodeName,
		SkipNodeUpdate: true,
		Check: &api.AgentCheck{
			CheckID:   checkID,
			Status:    status,
			ServiceID: serviceID,
			Type:      "ttl",
		},
	}

	_, err := u.client.Catalog().Register(reg, nil)
	return err
}

func (u *updater) registerService(ctx context.Context, nodeName, nodeID, serviceName, serviceID string, rateLimit performRateLimiting) error {
	if rateLimit {
		err := u.serviceLimiter.Wait(ctx)
		if err != nil {
			return nilContextError(err)
		}
	}

	meta, err := generateMeta(u.conf.MinMetaPerService, u.conf.MaxMetaPerService)
	if err != nil {
		return err
	}

	reg := &api.CatalogRegistration{
		ID:             nodeID,
		Node:           nodeName,
		SkipNodeUpdate: true,
		Service: &api.AgentService{
			ID:      serviceID,
			Service: serviceName,
			Meta:    meta,
			Port:    8443,
		},
	}

	_, err = u.client.Catalog().Register(reg, nil)
	return err
}

func (u *updater) registerNode(ctx context.Context, nodeName, nodeID, address string, rateLimit performRateLimiting) error {
	if rateLimit {
		err := u.nodeLimiter.Wait(ctx)
		if err != nil {
			return nilContextError(err)
		}
	}

	meta, err := generateMeta(u.conf.MinMetaPerNode, u.conf.MaxMetaPerNode)
	if err != nil {
		return err
	}

	reg := &api.CatalogRegistration{
		ID:       nodeID,
		Node:     nodeName,
		Address:  address,
		NodeMeta: meta,
	}

	_, err = u.client.Catalog().Register(reg, nil)
	return err
}

func generateMeta(min, max int) (map[string]string, error) {
	generator := generators.PetNameGenerator("", 2, "-")
	numMeta := randInterval(min, max)
	newMeta := make(map[string]string)
	for i := 0; i < numMeta; i++ {
		value, err := generator()
		if err != nil {
			return nil, fmt.Errorf("failed to generate node metadata value: %w", err)
		}
		newMeta[fmt.Sprintf("meta-%d", i)] = value
	}
	return newMeta, nil
}

func randInterval(min, max int) int {
	if min == max {
		return min
	}

	return min + rand.Intn(max-min)
}

func nilContextError(err error) error {
	if errors.Is(err, context.Canceled) {
		return nil
	}

	if errors.Is(err, context.DeadlineExceeded) {
		return nil
	}

	return err
}

type wrappedLimiter struct {
	limiter *rate.Limiter
}

func (wl *wrappedLimiter) Wait(ctx context.Context) error {
	err := wl.limiter.Wait(ctx)
	if err == nil {
		return nil
	}
	// The rate limiter has some nasty behavior where if it detects the limiter would wait
	// longer than the context's deadline it goes ahead and returns an error. This would
	// make needing to differentiate this error from others all over the place annoying
	// as we wouldn't want to cancel all the various contexts caused by one routine
	// in an errgroup exiting with this error. Therefore we do the more desirable thing
	// and detect the error and wait for the context to be finished.
	if strings.Contains(err.Error(), "Wait(n=1) would exceed context deadline") {
		<-ctx.Done()
		return ctx.Err()
	}
	return err
}

type initTracker struct {
	checkFinished chan struct{}
	finished      chan struct{}
	outstanding   atomic.Int32
}

func newInitTracker() *initTracker {
	return &initTracker{
		checkFinished: make(chan struct{}),
		finished:      make(chan struct{}),
	}
}

func (it *initTracker) run(ctx context.Context) error {
	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-it.checkFinished:
			if it.outstanding.Load() < 1 {
				close(it.finished)
				return nil
			}
		}
	}
}

func (it *initTracker) begin(num int32) {
	it.outstanding.Add(num)
}

func (it *initTracker) complete(num int32) {
	it.outstanding.Add(-num)
	select {
	case it.checkFinished <- struct{}{}:
	default:
	}
}

func (it *initTracker) initFinished() <-chan struct{} {
	return it.finished
}
