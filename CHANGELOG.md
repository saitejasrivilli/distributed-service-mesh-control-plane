# Changelog

## v0.3.0 — Envoy integration

- `Dockerfile`: multi-stage build producing `demo-service` and
  `control-plane` binaries on distroless base.
- `deployments/envoy/envoy-{a,b}.yaml`: static Envoy config — listener,
  route, cluster, active HTTP health check per backend.
- `deployments/docker/docker-compose.yml`: backend-a, backend-b, envoy-a,
  envoy-b wired on one bridge network.
- `scripts/envoy_smoke_test.sh`: end-to-end smoke test against real Docker
  containers (no mocks) — brings the stack up, verifies:
  - client -> Envoy -> backend
  - client -> Envoy A -> Envoy B -> backend B (sidecar chaining)
  - backend failure -> Envoy returns 503, marks endpoint unhealthy
  - backend recovery -> Envoy resumes routing traffic, 200
  - malformed Envoy config is rejected by `envoy --mode validate`
  then tears the stack down. All five scenarios pass against live
  containers as of this release.
- Backend containers (`cmd/demo-service`) implement zero routing logic —
  Envoy owns cluster selection and health-aware failure isolation.

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
