# System Architecture — v0.1.0

## Scope of this release

v0.1.0 is the control-plane process skeleton only. No registry, no xDS, no
Envoy integration yet — those arrive in v0.2.0+. This release exists to prove
out the operational foundation every later release depends on: config
loading/validation, structured logging with correlation IDs, Prometheus
metrics, graceful shutdown, and health/readiness endpoints.

## Components

```
cmd/control-plane      entrypoint: loads config, wires dependencies, handles OS signals
internal/config        env-var driven config with explicit Validate()
internal/logging       slog-based structured JSON logging + correlation ID propagation
internal/metrics       Prometheus collectors (HTTP request count/latency)
internal/api           HTTP management API: /healthz, /readyz, /metrics
internal/controlplane  wires config+logging+metrics+api together, owns Run() lifecycle
cmd/demo-service       minimal backend, used by later releases to exercise mesh routing
```

## Request lifecycle

1. Request arrives at `internal/api.Server`.
2. `withCorrelationID` middleware assigns/propagates `X-Correlation-ID`.
3. `withMetrics` middleware records request count and latency histograms.
4. Handler runs; logger pulled from context carries the correlation ID.

## Lifecycle management

`controlplane.ControlPlane.Run(ctx)`:
- Starts the HTTP server in a goroutine, marks readiness true.
- Blocks on `ctx.Done()` (wired to SIGINT/SIGTERM in `main.go`) or a server error.
- On cancellation: flips readiness false immediately (so a load balancer stops
  routing new traffic), then calls `Shutdown` bounded by `ShutdownTimeout`.

## Why these choices

- **In-process readiness flag, not external dependency checks**: v0.1.0 has no
  downstream dependencies (no registry, no xDS) to check yet. The readiness
  abstraction (`ReadinessChecker` interface) exists so v0.2.0+ can wire in
  real checks (registry reachable, xDS snapshot loaded) without touching the
  HTTP layer.
- **Custom Prometheus registry per `ControlPlane` instance, not the global
  default registry**: avoids "duplicate collector registration" panics when
  multiple instances exist in the same process (this bit us in testing —
  tests constructing more than one `ControlPlane` panicked against the global
  registry).
- **slog over a third-party logging library**: standard library, structured
  JSON output, sufficient for this project's needs.
