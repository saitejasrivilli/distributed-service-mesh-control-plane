# Distributed Service Mesh Control Plane

Repo: https://github.com/saitejasrivilli/distributed-service-mesh-control-plane

An independent service-mesh control plane implementation: service registry,
Envoy xDS (CDS/EDS/LDS/RDS), dynamic traffic management, health-aware
discovery, and Kubernetes deployment. Built to close the specialized gap
between general distributed-systems experience and service-mesh/container-
networking infrastructure work.

**Status: v0.1.0 — control-plane foundation only.** Registry, Envoy
integration, and xDS land in subsequent releases (see `CHANGELOG.md`).

This is an independent service-mesh implementation inspired by production
service-discovery and traffic-management architectures. It is not an
implementation of AWS ECS Service Connect, AWS Service Gateway, or AWS Cloud
Map.

## What's here in v0.1.0

- Go control-plane process with env-driven, validated configuration
- HTTP management API: `GET /healthz`, `GET /readyz`, `GET /metrics`
- Structured JSON logging with request correlation IDs
- Prometheus metrics (request count, latency histograms)
- Graceful shutdown on SIGINT/SIGTERM with bounded drain timeout
- Minimal demo backend (`cmd/demo-service`) for later Envoy sidecar wiring

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

v0.2.0 service registry/discovery · v0.3.0 Envoy integration · v0.4.0 xDS
control plane · v0.5.0 traffic management · v0.6.0 reconciliation · v0.7.0
Kubernetes · v0.8.0 observability · v0.9.0 scale/perf validation · v1.0.0
production-quality release.
