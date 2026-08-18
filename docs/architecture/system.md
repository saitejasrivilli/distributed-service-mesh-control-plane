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

## Traffic management (v0.5.0)

```
internal/routing/route.go       Spec (splits/retry/timeout/circuit-breaker), validated Store
internal/api/routing_handlers.go   PUT/GET/DELETE /v1/routes/{service}
scripts/traffic_smoke_test.sh   measures ACTUAL canary traffic distribution, twice, at two splits
test/benchmark/results/v0.5.0_latency.json   measured p50/p95/p99/error-rate, not invented
```

`xds.BuildSnapshot` now takes a `*routing.Store`. Per service, each
configured version split becomes its own `service::version` EDS cluster;
with no configured route it falls back to one cluster per service (pre-
v0.5.0 behavior), so registry/xDS tests from earlier releases keep passing
unmodified. See ADR-007 for the validation rules and the real measured
canary-split numbers (90/10 measured as 86.5/13.5, then a live shift to
50/50 measured as 46/54, zero Envoy restarts).

## Health-aware reconciliation (v0.6.0)

```
internal/registry.SweepStale        transitions Healthy=true -> false past CP_STALE_AFTER, persisted
internal/reconciliation.Reconciler   sweeps stale instances + rebuilds/publishes snapshot, in one pass
                                     exponential backoff+jitter on repeated reconcile failure
docs/runbooks/                      backend-failure, control-plane-failure, stale-endpoint, config-rejection
scripts/health_reconciliation_smoke_test.sh   live proof against real Docker containers
```

Core invariant carried forward from v0.4.0 and made explicit in
`docs/runbooks/config-rejection.md`: **invalid configuration is never
published to Envoy.** Route specs are validated before ever reaching the
routing store (`routing.Spec.Validate`); snapshots are checked for
consistency before publish (`xds.BuildSnapshot` -> `snap.Consistent()`); on
either failure, the previous snapshot remains authoritative.

See ADR-006 for why the health sweep and snapshot rebuild happen in one
reconcile pass (not two independently-scheduled loops), and
`docs/runbooks/control-plane-failure.md` for the honest single-point-of-
failure trade-off of a centralized coordinator (a dedicated ADR for
failure handling is planned as this project's runbooks mature).

## Kubernetes deployment (v0.7.0)

```
cmd/k8s-watcher                 bridges Kubernetes Endpoints -> registry HTTP API (no hardcoded pod IPs)
deployments/kubernetes/control-plane.yaml   Deployment + Service, readiness/liveness probes, resource limits
deployments/kubernetes/backend-a.yaml       Deployment (3 replicas) + Service
deployments/kubernetes/k8s-watcher.yaml     ServiceAccount/Role/RoleBinding (get/list/watch endpoints only) + Deployment
deployments/kubernetes/envoy-dynamic.yaml   ConfigMap (ADS bootstrap) + Deployment + Service
scripts/k8s_smoke_test.sh       builds+loads image into kind, deploys, proves discovery + 3->5->2 scaling live
```

`k8s-watcher` polls the `backend-a` Service's `Endpoints` every 2s and
converges the registry to match — registering new pod IPs, heartbeating
known ones, deregistering pods that disappeared. Instance IDs are pod
names, so registry state is directly traceable to `kubectl get pods`. See
ADR-009 for why polling was chosen over the watch API for this release, and
why this is a separate bridge process rather than baking Kubernetes
awareness into `internal/registry` directly (keeps the registry
platform-agnostic — the same HTTP API also serves the plain Docker Compose
demos from v0.1.0-v0.6.0).

Verified live against a real `kind` cluster: 3 replicas discovered with
zero hardcoded IPs, scaled to 5 then to 2, registry instance count and
Envoy traffic tracking correctly at every step with zero restarts anywhere
in the chain (control plane, Envoy, or backends).

## Observability (v0.8.0)

```
internal/metrics                control-plane metrics: services/endpoints/envoy-connections/
                                 xds-updates+failures/config-version/reconciliation attempts+
                                 failures+duration/stale-transitions, all updated inside Reconcile
internal/xds/tracker.go          ConnectionTracker: tracks open xDS streams + node IDs via serverv3.Callbacks
internal/api/debug_handlers.go  GET /v1/debug/services/{name}, /v1/debug/envoys, /v1/debug/config/{service}
docker-compose.yml               root-level: control-plane + backend-a + envoy-dynamic + Prometheus + Grafana
prometheus.yml                   scrapes control-plane:8080/metrics and envoy-dynamic:9901/stats/prometheus
deployments/docker/grafana/      auto-provisioned datasource + dashboard (no manual setup)
```

`/v1/debug/config/{service}` deliberately reads the reconciler's last
*published* snapshot, not current registry state — the two can differ for
a few seconds around a change, and that gap is exactly what the endpoint
is for. See ADR-010 for the full design and the live verification numbers
(real Prometheus scrape targets up, real metric values, real Grafana
dashboard auto-provisioned).

## Scale and performance validation (v0.9.0)

```
internal/xds/snapshot_bench_test.go   go benchmarks: snapshot generation at 10/100 services, up to 1000 endpoints
test/benchmark/xds_scale_test.go      real gRPC ADS clients (10/25/50) against a real running control plane
test/benchmark/churn_test.go          100 services/1000 endpoints + 2s of continuous registration churn
test/benchmark/results/v0.9.0_scale.json   every number below, measured, not invented
internal/api pprof endpoints          GET /debug/pprof/{profile,heap,...} for CPU/memory profiling
```

Real measured numbers (Apple M2, local machine): building a snapshot for
100 services / 1000 endpoints takes ~0.83ms and 14,590 allocations;
propagating a new snapshot to 10/25/50 concurrently-connected xDS clients
(real gRPC streams, not mocked) takes 185-203ms end-to-end, a number
dominated by the test's 200ms reconcile-tick interval rather than by
client fan-out cost (latency is flat, not increasing, from 10 to 50
clients); 100 services/1000 endpoints under continuous churn
(register+heartbeat+deregister) sustained 4.58M operations in 2 real
seconds with zero reconciliation failures.

Honestly scoped: this release simulates Envoy's xDS client behavior via
real gRPC ADS streams rather than running up to 100 full Envoy proxy
binaries, which was not feasible on the local development machine used for
this project. `test/benchmark/results/v0.9.0_scale.json` documents this
limitation explicitly rather than presenting simulated-client numbers as
full-Envoy numbers.

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
