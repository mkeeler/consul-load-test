SHELL = bash
root_dir = $(shell pwd)

build:
	mkdir -p bin
	CGO_ENABLED=0 go build -o bin/ ./cmd/consul-load/... 

build-linux:
	mkdir -p bin
	CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o bin/consul-load-linux ./cmd/consul-load/...

.PHONY: build

docker-prometheus:
	docker run -d --name prometheus \
        --mount type=bind,source=${root_dir}/hack/local-prometheus.yaml,destination=/etc/prometheus/prometheus.yml \
        --publish published=19090,target=9090,protocol=tcp prom/prometheus:v2.36.2
