# Changelog

## v0.2.0 — Service registry and discovery

- In-memory, thread-safe service registry (`internal/registry`) behind a
  `Registry` interface: register (idempotent), deregister, heartbeat,
  list services, get service, health-aware endpoint filtering with
  stale-instance exclusion.
- Namespace isolation, deterministic (instance-ID-ordered) listings.
- Management API: `POST /v1/services`, `DELETE
  /v1/services/{name}/instances/{id}`, `POST
  /v1/services/{name}/instances/{id}/heartbeat`, `GET /v1/services/{name}`,
  `GET /v1/services/{name}/instances`.
- Full test suite: registration, deregistration, heartbeat, stale expiration,
  duplicate registration, concurrent registration/deregistration, namespace
  isolation, healthy-endpoint filtering, restart-from-empty behavior, plus
  HTTP-layer integration tests for every endpoint. Passes `go vet`, `gofmt`,
  `golangci-lint`, and `go test -race`.

## v0.1.0 — Control-plane foundation

- Env-driven configuration (`internal/config`) with explicit validation.
- Structured JSON logging (`internal/logging`) with correlation ID propagation.
- Prometheus metrics (`internal/metrics`): HTTP request count and latency.
- HTTP management API (`internal/api`): `GET /healthz`, `GET /readyz`, `GET /metrics`.
- Lifecycle orchestration (`internal/controlplane`): graceful shutdown bounded
  by a configurable timeout, readiness flag flips false before drain begins.
- `cmd/control-plane` entrypoint wired to SIGINT/SIGTERM via
  `signal.NotifyContext`.
- `cmd/demo-service` minimal backend for future Envoy sidecar wiring.
- Full test suite: config validation, health/readiness/metrics endpoints,
  correlation ID propagation, concurrent request handling, graceful shutdown,
  context cancellation. Passes `go vet`, `gofmt`, `golangci-lint`, and
  `go test -race`.
