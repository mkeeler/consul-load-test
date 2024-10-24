package catalog

import (
	"context"
	"errors"

	"github.com/hashicorp/consul/api"
	"golang.org/x/sync/errgroup"
)

func (lg *LoadGenerator) Run(ctx context.Context) error {
	grp, grpCtx := errgroup.WithContext(ctx)

	if lg.updateNodes {
		grp.Go(func() error {
			return lg.runNodeUpdates(grpCtx)
		})
	}

	if lg.updateServices {
		grp.Go(func() error {
			return lg.runServiceUpdates(grpCtx)
		})
	}

	if lg.updateChecks {
		grp.Go(func() error {
			return lg.runCheckUpdates(grpCtx)
		})
	}

	return grp.Wait()
}

func (lg *LoadGenerator) runNodeUpdates(ctx context.Context) error {
	lg.Logger.Info("running periodic node updates")
	defer lg.Logger.Info("periodic node updates has finished")

	nodeLimiter := newWrappedLimiter(lg.conf.NodeUpdateRate, int(lg.conf.NodeUpdateRate*10))

	for {
		err := nodeLimiter.Wait(ctx)
		if err != nil {
			return nilContextError(err)
		}

		// pick a random node to update
		node := lg.nodes[lg.nodeRng.IntN(len(lg.nodes))]
		err = lg.registerNode(ctx, node)
		if err != nil {
			return err
		}
	}
}

func (lg *LoadGenerator) runServiceUpdates(ctx context.Context) error {
	lg.Logger.Info("running periodic service updates", "rate", lg.conf.ServiceUpdateRate)
	defer lg.Logger.Info("periodic service updates has finished")

	serviceLimiter := newWrappedLimiter(lg.conf.ServiceUpdateRate, int(lg.conf.ServiceUpdateRate*10))

	for {
		err := serviceLimiter.Wait(ctx)
		if err != nil {
			lg.Logger.Info("service limiter return an error", "error", err)
			return nilContextError(err)
		}

		// pick a random service instance to update
		instance := lg.serviceInstances[lg.serviceRng.IntN(len(lg.serviceInstances))]
		err = lg.registerService(ctx, instance)
		if err != nil {
			lg.Logger.Error("error updating service", "error", err)
			return err
		}
	}
}

func (lg *LoadGenerator) runCheckUpdates(ctx context.Context) error {
	lg.Logger.Info("running periodic check updates")
	defer lg.Logger.Info("periodic check updates has finished")

	checkLimiter := newWrappedLimiter(lg.conf.CheckUpdateRate, int(lg.conf.CheckUpdateRate*10))

	for {
		err := checkLimiter.Wait(ctx)
		if err != nil {
			return nilContextError(err)
		}

		// pick a random service instance to update
		check := lg.checks[lg.checkRng.IntN(len(lg.checks))]
		err = lg.registerCheck(ctx, check, api.HealthAny)
		if err != nil {
			return err
		}
	}
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
