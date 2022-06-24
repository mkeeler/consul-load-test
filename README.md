

## Get started

1. Start a local prometheus server using docker

This step is optioal if you don't need any performance results on the client side.

```bash
make docker-prometheus
```

The launched promehteus container uses a static config file, `./hack/local-prometheus.yaml` and listens on
port 19090 on the host.

2. Start the load test

Prepare a load test config file, e.g.,:

```json
{
      "KV": {
            "NumKeys": 50,
            "UpdateRate": 2
      }
}
```

If a prometheus server is enabled like step 1, we can run the load test, listen on port 8080 for
metrics scrape, and retrieve the results from the prometheus server, e.g.,

```
 ./bin/consul-load gen -config ./hack/config.json  --http-addr=localhost:8500 -metrics-port=8080  -report-addr=http://localhost:19090   -timeout 10m
```

If metrics data is not needed, simply start the load test by

```
 ./bin/consul-load gen -config ./hack/config.json -timeout 10m
```
