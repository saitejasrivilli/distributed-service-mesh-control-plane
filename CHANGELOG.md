# Changelog

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
