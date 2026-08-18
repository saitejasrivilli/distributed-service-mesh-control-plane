# System Architecture — v0.1.0 / v0.2.0

## Scope

v0.1.0 is the control-plane process skeleton: config loading/validation,
structured logging with correlation IDs, Prometheus metrics, graceful
shutdown, and health/readiness endpoints.

v0.2.0 adds the in-memory service registry and its management API
(register/deregister/heartbeat/list). No xDS or Envoy integration yet —
those arrive in v0.3.0+.

## Service registry (v0.2.0)

`internal/registry.Registry` is the storage interface; `InMemory` is the
current implementation, a mutex-guarded map keyed by
`namespace + "/" + serviceName`. See ADR-002 for the health/staleness model
and why persistence was deliberately deferred.

Management endpoints (`internal/api/registry_handlers.go`):

```
POST   /v1/services                                  register an instance
DELETE /v1/services/{name}/instances/{id}             deregister an instance
POST   /v1/services/{name}/instances/{id}/heartbeat   refresh liveness
GET    /v1/services/{name}                            all instances
GET    /v1/services/{name}/instances?healthy=true     healthy-only, stale-filtered
```

All registry endpoints accept an optional `?namespace=` query param
(`GET`/`DELETE`) or `namespace` body field (`POST`), defaulting to
`"default"`.

## Envoy data plane (v0.3.0)

v0.3.0 introduces the actual data plane: Envoy runs as a sidecar in front of
each backend, using **static** config (no xDS yet — that's v0.4.0). This
release proves the architecture works before the control plane starts
generating configuration dynamically.

```
client -> envoy-a (:10000) -> backend-a (:9000)
client -> envoy-a (:10000, /via-b) -> envoy-b (:10001) -> backend-b (:9000)
```

Files:

```
Dockerfile                          multi-stage build for demo-service + control-plane binaries
deployments/envoy/envoy-a.yaml      static listener/cluster config for envoy-a sidecar
deployments/envoy/envoy-b.yaml      static listener/cluster config for envoy-b sidecar
deployments/docker/docker-compose.yml   backend-a, backend-b, envoy-a, envoy-b on one bridge network
scripts/envoy_smoke_test.sh         brings the stack up, runs every connectivity/failure
                                     scenario below against real containers, tears it down
```

Verified scenarios (via `scripts/envoy_smoke_test.sh`, exercised against real
Docker containers, not mocked):

- Client -> Envoy -> backend returns the backend's response.
- Client -> Envoy A -> Envoy B -> backend B (`/via-b` route) returns
  backend B's response, proving Envoy-to-Envoy sidecar chaining.
- Backend containers implement no routing logic themselves — Envoy owns
  cluster selection, health checking (active HTTP health checks against
  `/healthz`), and failure isolation.
- Killing `backend-a` causes Envoy to return `503` and mark the endpoint
  unhealthy (visible via `/clusters` admin endpoint); restarting the
  container brings the endpoint back to `200` once the health check passes.
- Malformed Envoy config (unknown field) is rejected by `envoy --mode
  validate` with a non-zero exit and a descriptive protobuf error — Envoy
  never silently starts with bad config.

## xDS control plane (v0.4.0)

```
Service Registry -> Reconciler (periodic + on-demand) -> xds.BuildSnapshot
  -> versioned cachev3.Snapshot -> SnapshotCache -> gRPC ADS server -> Envoy
```

```
internal/xds/snapshot.go        pure function: registry state -> CDS/EDS/LDS/RDS resources
internal/xds/server.go          gRPC server exposing ADS (+ per-type CDS/EDS/LDS/RDS services)
internal/reconciliation         periodic + on-demand reconcile loop, monotonic versioning
deployments/envoy/envoy-dynamic.yaml   ADS bootstrap (no static clusters/listeners)
deployments/docker/docker-compose-xds.yml   control-plane + envoy-dynamic + backend-a
scripts/xds_smoke_test.sh       automated proof of dynamic add/remove, zero Envoy restarts
```

See ADR-004 for why go-control-plane's `SnapshotCache` was used over
hand-rolling the protocol, and for the deterministic port-assignment and
versioning rules. Verified live: registering a backend causes Envoy to
receive a new cluster/endpoint/listener/route and start serving traffic
through it immediately (`cds: add 1 cluster(s)`, `lds: add/update
listener`) with no process restart; deregistering the last instance of a
service causes Envoy to remove the listener entirely.

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
