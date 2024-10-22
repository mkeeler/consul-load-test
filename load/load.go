package load

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/hashicorp/go-hclog"
	"github.com/mkeeler/consul-load-test/metrics"
)

func Load(ctx context.Context, client *api.Client, conf Config, metricsServer *metrics.MetricsServer) <-chan struct{} {
	logger := hclog.FromContext(ctx)
	done := make(chan struct{})

	go func() {
		defer close(done)

		var loadDone <-chan struct{}
		switch conf.Target {
		case TargetKV:
			loadDone = kvLoad(ctx, client, conf, metricsServer)
		case TargetPeering:
			loadDone = peeringLoad(ctx, client, conf, metricsServer)
		case TargetCatalog:
			loadDone = catalogLoad(ctx, client, conf, metricsServer)
		default:
			logger.Error("error: invalid load type:", conf.Target)
			return
		}

		// load will only be stopped at ctx, passed to the load method abvoe
		<-loadDone
	}()

	return done
}
