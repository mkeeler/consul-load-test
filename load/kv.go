package load

import (
	"context"
	"fmt"
	"math/rand"
	"time"

	petname "github.com/dustinkirkland/golang-petname"
	"github.com/hashicorp/consul/api"
	"github.com/mkeeler/consul-data/generate/generators"
	"github.com/mkeeler/consul-load-test/metrics"
	"golang.org/x/time/rate"
)

// KVConfig is the configuration for the KV load generator
type KVConfig struct {
	// NumKeys is the number of keys to initialize and maintain
	NumKeys int
	// UpdateRate is the number of KV updates per second
	UpdateRate rate.Limit

	// KeySegments is the number of segments for each pet name
	KeySegments int
	// KeySeparator is the character to separate the different key
	// segments
	KeySeparator string

	// MinValueSize is the minimum size of kv values
	MinValueSize int
	// MaxValueSize is the maximum size of kv values
	MaxValueSize int
}

func (c *KVConfig) Normalize() error {
	if c.NumKeys < 1 {
		return nil
	}

	if c.UpdateRate == 0.0 {
		return fmt.Errorf("invalid UpdateRate configuration: %v", c.UpdateRate)
	}

	if c.KeySegments < 1 {
		c.KeySegments = defaultKVKeySegments
	}

	if c.KeySeparator == "" {
		c.KeySeparator = "/"
	}

	if c.MinValueSize < 1 {
		c.MinValueSize = defaultKVMinValueSize
	}

	if c.MaxValueSize < 1 {
		c.MaxValueSize = defaultKVMaxValueSize
	}

	if c.MinValueSize > c.MaxValueSize {
		c.MaxValueSize = c.MinValueSize
	}

	return nil
}

func kvLoad(ctx context.Context, client *api.Client, conf Config, metricsServer *metrics.MetricsServer) <-chan struct{} {
	done := make(chan struct{})

	go func() {
		defer close(done)
		limiter := rate.NewLimiter(conf.KV.UpdateRate, int(conf.KV.UpdateRate*10))

		keys := make([]string, conf.KV.NumKeys)

		for i := 0; i < conf.KV.NumKeys; i++ {
			keys[i] = petname.Generate(conf.KV.KeySegments, conf.KV.KeySeparator)
		}

		valueGen := generators.RandomB64Generator(conf.KV.MinValueSize, conf.KV.MaxValueSize)

		for {
			err := limiter.Wait(ctx)
			if err != nil {
				return
			}

			key := keys[rand.Intn(len(keys))]

			value, err := valueGen()
			if err != nil {
				panic(err)
			}

			pair := api.KVPair{
				Key:   key,
				Value: []byte(value),
			}

			go sendKey(client, &pair, metricsServer)
		}
	}()
	return done
}

func sendKey(client *api.Client, pair *api.KVPair, metricsServer *metrics.MetricsServer) {
	start := time.Now()
	_, err := client.KV().Put(pair, nil)
	if metricsServer != nil {
		duration := time.Since(start)
		if err == nil {
			metricsServer.IncLatencyHistogram(duration, "kv", "success")
		} else {
			metricsServer.IncLatencyHistogram(duration, "kv", "error")
		}
	}
}
