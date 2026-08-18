# Changelog

## v0.9.0 — Failure, scale, and performance validation

- `internal/xds/snapshot_bench_test.go`: Go benchmarks for xDS snapshot
  generation at 10/100 services and up to 1000 endpoints. Measured: 100
  services / 1000 endpoints builds in ~0.83ms/op, 14,590 allocs/op.
- `test/benchmark/xds_scale_test.go`: real gRPC ADS clients (not mocked)
  connect to a real running control plane; measures actual snapshot
  propagation latency at 10/25/50 concurrently-connected clients. Measured:
  185-203ms end-to-end, flat across client counts (dominated by the test's
  200ms reconcile interval, not by fan-out cost).
- `test/benchmark/churn_test.go`: 100 services/1000 endpoints
  pre-populated, then 2 seconds of continuous register/heartbeat/
  deregister churn against the real registry + reconciler. Measured: 4.58M
  churn operations in 2s, 0 reconciliation failures, ~4.4MB heap growth.
- `internal/xds`: added `Server.ServeListener` (serve on a pre-bound
  listener) to support tests dialing a real ephemeral port.
- `internal/api`: pprof endpoints (`GET /debug/pprof/*`) for CPU/memory
  profiling, verified live (heap profile fetched successfully).
- `test/benchmark/results/v0.9.0_scale.json`: every number above, with an
  explicit, honest note on what wasn't measured (100 *real* Envoy proxy
  binaries were not run locally; this release used real gRPC ADS clients
  simulating Envoy's xDS behavior instead) rather than presenting simulated
  numbers as something they're not.
- A real bug was caught and fixed while building the scale test: the
  initial test connected simulated Envoy clients using distinct node IDs,
  which silently hung forever because the reconciler always publishes
  under one fixed node ID (`demo-envoy`, per ADR-004's single-node design)
  — fixed by connecting all simulated clients under that same node ID,
  which is also the architecturally correct topology for this release.

## v0.8.0 — Observability and operational tooling

- `internal/metrics`: new control-plane collectors — `services_total`,
  `endpoints_total`, `envoy_connections_total`, `xds_updates_total` /
  `xds_update_failures_total`, `config_version`,
  `reconciliation_attempts_total` / `reconciliation_failures_total`,
  `reconciliation_duration_seconds`, `stale_instances_transitioned_total`
  — all updated directly inside `Reconciler.Reconcile`.
- `internal/xds/tracker.go`: `ConnectionTracker` tracks currently-open xDS
  streams and their negotiated node IDs via `serverv3.Callbacks`.
- `internal/api/debug_handlers.go`: `GET /v1/debug/services/{name}`, `GET
  /v1/debug/envoys`, `GET /v1/debug/config/{service}` — the latter reads
  the reconciler's last *published* snapshot directly, so it reflects
  exactly what Envoy was told (which can briefly differ from current
  registry state around a change).
- Root `docker-compose.yml` + `prometheus.yml` +
  `deployments/docker/grafana/`: full observability stack (control-plane +
  backend-a + Envoy + Prometheus + Grafana), with datasource and dashboard
  auto-provisioned — no manual Grafana setup required.
- Verified live: both Prometheus scrape targets (`control-plane`, `envoy`)
  report `up`; after registering a backend, `controlplane_services_total`,
  `endpoints_total`, `config_version`, and `reconciliation_attempts_total`
  show real non-zero values; all three debug endpoints return real data
  including a live Envoy connection (`node_id: demo-envoy`); the Grafana
  dashboard auto-provisions and its queries were checked against Envoy's
  actual `/stats/prometheus` metric names.
- Unit tests: `ConnectionTracker` open/close/node-ID tracking (2 tests in
  `internal/xds`), debug endpoint behavior including 404s before any
  snapshot and correct reflection of a real built snapshot (5 tests in
  `internal/api`). All pass under `-race`.
- ADR-010 documents the design, including the honest trade-off that
  `reconciliation_duration_seconds` measures control-plane build+publish
  time, not true Envoy-side xDS ACK round-trip latency.

## v0.7.0 — Kubernetes container networking

- `cmd/k8s-watcher`: bridges Kubernetes `Endpoints` to the existing registry
  HTTP API — polls every 2s, registers new pod IPs, heartbeats known ones,
  deregisters pods no longer present. No pod IP is ever hardcoded anywhere
  in the deployment.
- `deployments/kubernetes/`: `control-plane.yaml`, `backend-a.yaml` (3
  replicas), `k8s-watcher.yaml` (ServiceAccount/Role/RoleBinding scoped to
  `get/list/watch` on `endpoints` only), `envoy-dynamic.yaml` (ConfigMap +
  Deployment using the same ADS bootstrap as v0.4.0's Docker demo).
  Deployments/Services use readiness/liveness probes and resource
  requests/limits throughout.
- `scripts/k8s_smoke_test.sh`: builds the image, loads it into a `kind`
  cluster, deploys the full stack, and proves live: 3 replicas discovered
  with zero hardcoded IPs; traffic flows client -> Envoy -> a
  k8s-discovered backend; scaling to 5 then to 2 replicas is reflected in
  the registry within seconds at every step, with traffic continuing
  throughout and zero restarts anywhere in the chain.
- ADR-009 documents why Kubernetes awareness lives in a separate bridge
  process rather than inside `internal/registry` (keeps the registry
  platform-agnostic, serving both the Kubernetes and plain-Docker-Compose
  deployment paths through the same HTTP API), and why polling was chosen
  over the Kubernetes watch API for this release's scale.

## v0.6.0 — Health-aware discovery and reconciliation

- `internal/registry.SweepStale(staleAfter)`: transitions instances from
  `Healthy=true` to `Healthy=false` once `now - LastHeartbeat` exceeds
  `staleAfter` — a persisted state transition (visible via `GET
  /v1/services/{name}`), not just a read-time filter. Recovers to
  `Healthy=true` on the next heartbeat.
- `internal/reconciliation.Reconciler`: sweeps stale instances and
  rebuilds/publishes the snapshot in one reconcile pass; `Run` now applies
  exponential backoff with jitter (capped at 30s) after consecutive
  reconcile failures, resetting on success.
- `internal/config`: new `CP_STALE_AFTER` setting (default `15s`),
  validated.
- Core invariant made explicit and documented:
  **invalid configuration is never published to Envoy** — enforced at two
  layers (route spec validation before the routing store, snapshot
  consistency check before publish).
- `docs/runbooks/`: `backend-failure.md`, `control-plane-failure.md`
  (including an honest single-point-of-failure discussion for the
  single-node xDS design), `stale-endpoint.md`, `config-rejection.md`.
- `scripts/health_reconciliation_smoke_test.sh`: live proof against real
  Docker containers — a backend with no heartbeats goes stale, is excluded
  from Envoy's EDS with zero restart, then recovers on the next heartbeat,
  with Envoy resuming traffic with zero restart.
- Unit tests: `SweepStale` transition/no-op/recovery/disabled-at-zero (4
  tests in `internal/registry`), reconciler sweep integration and backoff
  monotonicity/cap (2 new tests in `internal/reconciliation`). All pass
  under `-race`.
- ADR-006 documents why the health sweep and snapshot rebuild happen in a
  single reconcile pass.

## v0.5.0 — Service-to-service traffic management

- `internal/routing`: `Spec` (version-weighted splits, retry policy,
  timeout, circuit breaker) with `Validate()` (weights must sum to 100,
  no duplicate versions, `retry_on` requires `num_retries > 0`) and a
  thread-safe `Store` that only ever holds validated specs.
- `internal/api`: `PUT/GET/DELETE /v1/routes/{service}` management
  endpoints; invalid specs rejected with 400 before ever reaching the
  reconciler.
- `internal/xds`: `BuildSnapshot` extended to consume `routing.Store` —
  weighted `RouteAction_WeightedClusters` routing across per-version EDS
  clusters, retry policy and timeout on the route, circuit breaker
  thresholds on the cluster. No configured route falls back to the
  pre-v0.5.0 single-cluster behavior (fully backward compatible with
  v0.2.0-v0.4.0 tests).
- `cmd/demo-service`: `-version` flag so canary/traffic-split demos can
  distinguish which version instance answered a request.
- `deployments/docker/docker-compose-traffic.yml` +
  `scripts/traffic_smoke_test.sh`: live proof, against real Docker
  containers, that a 90/10 canary split measured 173/27 (86.5%/13.5%) over
  200 real requests, and that shifting to 50/50 — with **zero Envoy
  restart** — measured 92/108 (46%/54%) over the next 200 requests.
- `test/benchmark/results/v0.5.0_latency.json`: measured (not invented)
  latency — 300 sequential requests, 0 errors, p50=2.01ms, p95=2.81ms,
  p99=3.38ms.
- Unit tests: weight-sum/duplicate-version/retry validation (11 tests in
  `internal/routing`), weighted-cluster generation, per-version EDS
  filtering, canary weight shift reflected in snapshot, retry policy,
  timeout, circuit breaker, single-cluster fallback (9 tests in
  `internal/xds`), plus HTTP-layer route endpoint tests. All pass under
  `-race`; a real bug (empty-`Version` splits incorrectly rejected by
  `Validate`, which made the single-cluster fallback unexpressable) was
  caught by these tests and fixed before this release.

## v0.4.0 — xDS control plane

- `internal/xds`: pure `BuildSnapshot(registry, version)` generating
  CDS/EDS/LDS/RDS resources from registry state, using
  `envoyproxy/go-control-plane`'s `SnapshotCache`. Deterministic port
  assignment (sorted service name order), snapshot consistency checked
  before every publish (an inconsistent snapshot is never sent to Envoy).
- `internal/xds/server.go`: gRPC server exposing ADS plus the individual
  CDS/EDS/LDS/RDS discovery services, with a gRPC health check endpoint.
- `internal/reconciliation`: periodic (configurable interval, default 2s)
  and on-demand reconcile loop with monotonically increasing snapshot
  versions and attempt/failure counters.
- `internal/config`: new `CP_XDS_ADDR` (default `:18000`) and
  `CP_RECONCILE_INTERVAL` (default `2s`) settings, validated.
- `internal/controlplane`: control plane now runs HTTP API, xDS gRPC server,
  and the reconciliation loop concurrently, all shut down gracefully together.
- `deployments/envoy/envoy-dynamic.yaml` + `deployments/docker/docker-compose-xds.yml`:
  a real Envoy connected via ADS to the control plane, no static config.
- `scripts/xds_smoke_test.sh`: automated, repeatable proof (against live
  Docker containers) that registering a backend causes Envoy to add a
  cluster/endpoint/listener/route and start serving traffic **without a
  restart**, and that deregistering the last instance removes the listener.
  Both scenarios pass as of this release.
- Unit tests: snapshot resource counts, consistency, versioning,
  deterministic port assignment, healthy-only EDS filtering, empty-registry
  behavior, EDS discovery type, route-to-cluster correctness (8 tests);
  reconciler publish/version/attempt-tracking/periodic-run-and-cancel
  (4 tests). All pass under `-race`.

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
