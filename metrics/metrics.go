package metrics

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	"github.com/prometheus/client_golang/api/prometheus/v1"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"github.com/prometheus/common/model"
)

// Follow Prometheus naming practices
// https://prometheus.io/docs/practices/naming/
var (
	requestTypeLabels = []string{"requestType", "status"}
)

var (
	MetricRequestLatency = prometheus.NewHistogramVec(
		prometheus.HistogramOpts{
			Name:    "consul_load_request_latency_seconds",
			Help:    "consul request latency (seconds).",
			Buckets: []float64{0.02, 0.05, .1, .2, .4, .8, 1, 30, 60, 120, 180},
		},
		requestTypeLabels,
	)
)

type MetricsServer struct {
	*http.Server

	latencyHistogram *prometheus.HistogramVec
}

const (
	// MetricsPath is the endpoint to collect load test metrics
	MetricsPath = "/metrics"
)

type ServerConfig struct {
	Addr string
}

// NewMetricsServer returns a new prometheus server which collects load test metrics
func NewMetricsServer(cfg ServerConfig) *MetricsServer {
	mux := http.NewServeMux()

	reg := prometheus.NewRegistry()

	reg.MustRegister(MetricRequestLatency)

	mux.Handle(MetricsPath, promhttp.HandlerFor(prometheus.Gatherers{
		reg,
	}, promhttp.HandlerOpts{}))
	return &MetricsServer{
		Server: &http.Server{
			Addr:    cfg.Addr,
			Handler: mux,
		},
		latencyHistogram: MetricRequestLatency,
	}
}

// IncLatencyHistogram add an observed measurment of request latency
func (m *MetricsServer) IncLatencyHistogram(duration time.Duration, lvs ...string) {
	m.latencyHistogram.WithLabelValues(lvs...).Observe(duration.Seconds())
}

func KVLoadReport(addr string, duration time.Duration) error {
	client, err := api.NewClient(api.Config{
		Address: addr,
	})
	if err != nil {
		fmt.Printf("Error creating client: %v\n", err)
		os.Exit(1)
	}

	v1api := v1.NewAPI(client)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	interval := duration.Round(time.Second).String()
	getPercentile := func(percentile string) error {

		queryLatencyPercentile := "histogram_quantile(%s, sum(rate(consul_load_request_latency_seconds_bucket[%s])) by (le))"

		query := fmt.Sprintf(queryLatencyPercentile, percentile, interval)
		result, warnings, err := v1api.Query(ctx, query, time.Now())
		if err != nil {
			fmt.Printf("Error querying Prometheus: %v\n", err)
			os.Exit(1)
		}
		if len(warnings) > 0 {
			fmt.Printf("Warnings: %v\n", warnings)
		}

		vec, ok := result.(model.Vector)
		if !ok {
			return fmt.Errorf("unsupported result format: %s", result.Type().String())
		}
		if vec.Len() == 0 {
			fmt.Println("Not enough samples")
			return nil
		}
		fmt.Printf("%sth percentile latency (second): %0.3f\n", percentile, vec[0].Value)
		return nil
	}

	err = getPercentile("0.5")
	if err != nil {
		return err
	}
	err = getPercentile("0.9")
	if err != nil {
		return err
	}

	return nil
}
