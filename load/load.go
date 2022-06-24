package load

import (
	"context"

	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-load-test/metrics"
)

func Load(ctx context.Context, client *api.Client, conf Config, metricsServer *metrics.MetricsServer) {
	go kvLoad(ctx, client, conf, metricsServer)
	<-ctx.Done()
}
