package load

import (
	"context"
	"fmt"

	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/metrics"
)

func Load(ctx context.Context, client *api.Client, conf Config, metricsServer *metrics.MetricsServer) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)

		var loadDone <-chan struct{}
		switch conf.Target {
		case TargetKV:
			fmt.Println("Starting kv load")
			loadDone = kvLoad(ctx, client, conf, metricsServer)
		case TargetPeering:
			fmt.Println("Starting peering load")
			loadDone = peeringLoad(ctx, client, conf, metricsServer)
		default:
			fmt.Println("error: invalid load type:", conf.Target)
			return
		}

		// load will only be stopped at ctx, passed to the load method abvoe
		<-loadDone
	}()

	return done
}
