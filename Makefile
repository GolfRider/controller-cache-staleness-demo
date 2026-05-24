# Staleness demo — one target per step. Run in order.
IMG ?= staleness-demo:dev

.PHONY: tidy build crd sample naive gated inject clear burst help

help:
	@grep -E '^[a-zA-Z_-]+:.*?## .*$$' $(MAKEFILE_LIST) | awk 'BEGIN{FS=":.*?## "}{printf "  %-10s %s\n",$$1,$$2}'

tidy: ## resolve deps (run first on laptop)
	go mod tidy

build: ## compile both controllers into one binary
	go build -o bin/controller ./cmd

crd: ## install the FakeWorkload CRD
	kubectl apply -f config/crd/fakeworkload.yaml

sample: ## create the demo FakeWorkload (replicas: 5)
	kubectl apply -f config/sample-workload.yaml

naive: ## run the NAIVE controller (metrics :8080, lag-ctl :9090)
	go run ./cmd --mode=naive --metrics-bind-address=:8080 --lag-control-address=:9090

gated: ## run the GATED controller (metrics :8081, lag-ctl :9091)
	go run ./cmd --mode=gated --metrics-bind-address=:8081 --lag-control-address=:9091

inject: ## inject simulated lag on BOTH controllers (window=3, ~2s)
	./hack/inject-lag.sh 3 2.0 localhost:9090
	./hack/inject-lag.sh 3 2.0 localhost:9091

clear: ## clear lag on both (cache fresh again)
	./hack/inject-lag.sh 0 0 localhost:9090
	./hack/inject-lag.sh 0 0 localhost:9091

burst: ## real-churn context shot: kill 200 managed pods at once
	./hack/burst.sh 200
