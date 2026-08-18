# ADR-010: Observability

## Context

v0.8.0 targets the operational-excellence portion of the target role: an
operator should be able to answer "what services exist, which endpoints
are healthy, which Envoy has which config version, and why was an endpoint
removed" without reading source code or attaching a debugger.

## Decision

**Metrics** (`internal/metrics`): every control-plane collector uses a
per-instance `prometheus.NewRegistry()` (not the process-global default
registry — this bit us in v0.1.0's tests, see `docs/architecture/system.md`)
covering: `services_total`, `endpoints_total`, `envoy_connections_total`,
`xds_updates_total` / `xds_update_failures_total`, `config_version`,
`reconciliation_attempts_total` / `reconciliation_failures_total`,
`reconciliation_duration_seconds` (a histogram, doubling as the propagation-
latency proxy since a reconcile pass is build-then-publish), and
`stale_instances_transitioned_total`. All are updated directly inside
`Reconciler.Reconcile` (see ADR-006), so metrics never drift from the state
they describe.

**Envoy connection tracking** (`internal/xds/tracker.go`):
`ConnectionTracker` implements `serverv3.Callbacks` (the same interface
go-control-plane's xDS server already calls on stream open/request/close)
to maintain a live map of connected streams and their negotiated node IDs,
without touching the snapshot-serving path itself.

**Debug endpoints** (`internal/api/debug_handlers.go`):

- `GET /v1/debug/services/{name}` — every instance's health/heartbeat state
  plus any configured traffic-management spec, straight from the registry
  and routing store.
- `GET /v1/debug/envoys` — every currently-connected stream, its node ID,
  and connection time, straight from `ConnectionTracker`.
- `GET /v1/debug/config/{service}` — reads the reconciler's
  `LastSnapshot()` directly (not the registry) specifically so an operator
  can see **what Envoy was actually told**, which can legitimately differ
  from current registry state for the few seconds around a change — that
  gap is exactly what this endpoint exists to diagnose.

**Grafana** (`deployments/docker/grafana/`): dashboard and datasource
auto-provisioned via file-based provisioning (no manual "add datasource"
click-through needed) — verified live: `docker compose up` and the
dashboard `Service Mesh Control Plane` appears already configured.

## Trade-offs

- Debug endpoints expose no authentication — acceptable for a project
  demonstrating the pattern; a production deployment would put these behind
  the same authn/authz boundary as the rest of the management API (none
  exists yet in this project — see `SECURITY.md`, planned).
- `reconciliation_duration_seconds` measures the control plane's own
  build+publish time, not true end-to-end propagation latency to Envoy
  (that would require Envoy-side instrumentation of xDS ACK round-trip
  time). Documented here rather than silently mislabeled.

## Consequences

Verified live via `docker compose up` (this project's root
`docker-compose.yml`, added in this release): Prometheus scrapes both
`control-plane:8080/metrics` and `envoy-dynamic:9901/stats/prometheus`
successfully (`up` health for both targets); after registering a backend,
`controlplane_services_total`, `endpoints_total`, `config_version`, and
`reconciliation_attempts_total` all reflect real, non-zero measured values;
all three debug endpoints return real data including a live Envoy
connection (`node_id: demo-envoy`); the Grafana dashboard is auto-
provisioned and queryable against real Envoy metric names
(`envoy_cluster_upstream_rq_xx{envoy_response_code_class="2"}`, confirmed
directly against Envoy's own `/stats/prometheus` output).
