# Generating Consul Peering Load

The `consul-load-test` tool can generate load to exercise the peering function of consul, specifically, peering replication. Peering is established between the acceptor cluster (by default, it is `localhost:8500`) and
the clusters specified in the load configuration file. Then, the tool will register service in the acceptor
cluster and measure the latency between service being exported and available in the peering cluster (by
issuing blocking query to`/health/service` ). The load test can be stopped by `ctrl-C` or timeout, e.g.,
`-timeout 1m`

## Steps

### Configuration

Prepare a load test config file `config-peering.json`, e.g.,:

```json
{
      "Peering": {
            "PeeringClusters": [
                  "172.16.1.100:8500",
                  "172.16.2.100:8500"
            ],
            "UpdateRate": 1
      }
}
```

Multiple clusters can be peered with the acceptor cluster simultaneously by adding the agent addresses to
`Peering.PeeringClusters` in the configuration file.

### Run the load test

```bash
./bin/consul-load gen -config ./hack/config-peering.json -timeout 5m
```



### Clean up

```bash
consul config delete -kind exported-services -name default
```

### Measuring results

Latency metrics can be scraped by a prometheus server when `-metrics-port` is set as follows.

```bash
./bin/consul-load gen -config ./hack/config-peering.json  --http-addr=localhost:8500 -metrics-port=8080 -timeout 10m
```