package load

import (
	"context"

	"github.com/hashicorp/consul/api"
)

func Load(ctx context.Context, client *api.Client, conf Config) {
	go kvLoad(ctx, client, conf)
	<-ctx.Done()
}
