# Distributed Service Mesh Control Plane

Repo: https://github.com/saitejasrivilli/distributed-service-mesh-control-plane

An independent service-mesh control plane implementation: service registry,
Envoy xDS (CDS/EDS/LDS/RDS), dynamic traffic management, health-aware
discovery, and Kubernetes deployment. Built to close the specialized gap
between general distributed-systems experience and service-mesh/container-
networking infrastructure work.

**Status: v0.5.0 — control-plane foundation + service registry/discovery +
Envoy integration + dynamic xDS + service-to-service traffic management
(weighted/canary routing, retries, timeouts, circuit breaking).**
Health-aware reconciliation lands in v0.6.0 (see `CHANGELOG.md`).

This is an independent service-mesh implementation inspired by production
service-discovery and traffic-management architectures. It is not an
implementation of AWS ECS Service Connect, AWS Service Gateway, or AWS Cloud
Map.

## What's here

**v0.1.0**
- Go control-plane process with env-driven, validated configuration
- HTTP management API: `GET /healthz`, `GET /readyz`, `GET /metrics`
- Structured JSON logging with request correlation IDs
- Prometheus metrics (request count, latency histograms)
- Graceful shutdown on SIGINT/SIGTERM with bounded drain timeout
- Minimal demo backend (`cmd/demo-service`) for later Envoy sidecar wiring

**v0.2.0**
- In-memory, thread-safe service registry with health-aware, stale-filtered
  endpoint lookups (see `docs/adr/ADR-002-service-registry.md`)
- Management API: register/deregister/heartbeat/list instances

```
curl -X POST localhost:8080/v1/services -d '{
  "service_name":"backend-a","instance_id":"i1",
  "address":"10.0.0.5","port":9000
}'
curl localhost:8080/v1/services/backend-a
curl -X POST localhost:8080/v1/services/backend-a/instances/i1/heartbeat
curl "localhost:8080/v1/services/backend-a/instances?healthy=true"
curl -X DELETE localhost:8080/v1/services/backend-a/instances/i1
```

**v0.3.0**
- Envoy sidecar in front of each backend, static config (no xDS yet)
- Demonstrates client -> Envoy -> backend, and Envoy A -> Envoy B -> backend B
- Health-aware failure isolation and recovery without backend routing logic

```
docker compose -f deployments/docker/docker-compose.yml up --build
curl localhost:10000/echo    # client -> envoy-a -> backend-a
curl localhost:10000/via-b   # client -> envoy-a -> envoy-b -> backend-b
./scripts/envoy_smoke_test.sh   # full connectivity + failure/recovery test
```

**v0.4.0**
- Control plane generates Envoy config dynamically over gRPC ADS
  (CDS/EDS/LDS/RDS) — no static config, no Envoy restarts
- Registering/deregistering a service via the management API propagates to
  Envoy automatically

```
docker compose -f deployments/docker/docker-compose-xds.yml up --build
curl -X POST localhost:8080/v1/services -d '{"service_name":"backend-a","instance_id":"i1","address":"<backend-a-ip>","port":9000}'
curl localhost:20000/echo   # Envoy now serving backend-a — no restart happened
./scripts/xds_smoke_test.sh   # full dynamic add/remove test
```

**v0.5.0**
- Weighted/canary routing across service versions, retries, timeouts,
  circuit breaking — all dynamic, no Envoy restarts
- Measured (not invented) numbers: 90/10 canary split measured 86.5%/13.5%
  over 200 requests; live shift to 50/50 measured 46%/54% over the next 200
  requests; 300-request latency benchmark: p50=2.01ms p95=2.81ms
  p99=3.38ms, 0 errors (`test/benchmark/results/v0.5.0_latency.json`)

```
docker compose -f deployments/docker/docker-compose-traffic.yml up --build
curl -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":90},{"version":"v2","weight":10}]}'
curl -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":50},{"version":"v2","weight":50}]}'
./scripts/traffic_smoke_test.sh   # measures real canary distribution at both splits
```

See `docs/architecture/system.md` for the design and `docs/adr/` for decision
records.

## Running locally

```
git clone https://github.com/saitejasrivilli/distributed-service-mesh-control-plane.git
cd distributed-service-mesh-control-plane
go run ./cmd/control-plane
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics
```

## Quality gates

```
gofmt -l .
go vet ./...
go build ./...
go test ./...
go test -race ./...
golangci-lint run ./...
```

## Roadmap

v0.6.0 reconciliation/health · v0.7.0 Kubernetes · v0.8.0 observability ·
v0.9.0 scale/perf validation · v1.0.0 production-quality release.
