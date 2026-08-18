# Resume Evidence

Every claim below is traceable to specific code, a test, and a release.
Do not use a claim from this file on a resume without keeping this mapping
handy for follow-up questions.

---

**Claim:** "Built an Envoy xDS control plane implementing CDS, EDS, LDS,
and RDS, dynamically updating Envoy with zero restarts."

Evidence: `internal/xds/snapshot.go`, `internal/xds/server.go`,
`internal/xds/snapshot_test.go` (8 unit tests),
`scripts/xds_smoke_test.sh` (live proof against real Docker containers),
release v0.4.0.

---

**Claim:** "Implemented health-aware service discovery with automatic
failure detection and recovery."

Evidence: `internal/registry/registry.go` (`SweepStale`),
`internal/reconciliation/reconciler.go`, `internal/registry/registry_test.go`
(17 tests including 4 for `SweepStale`),
`scripts/health_reconciliation_smoke_test.sh` (live: stale -> unhealthy ->
excluded from EDS -> heartbeat -> recovered, zero restarts), release
v0.2.0 (registry) and v0.6.0 (health-aware sweeping).

---

**Claim:** "Implemented weighted/canary traffic routing with measured
(not simulated) traffic distribution."

Evidence: `internal/routing/route.go`, `internal/xds/traffic_test.go` (7
tests), `scripts/traffic_smoke_test.sh` — measured 90/10 split landing at
86.5%/13.5% over 200 real requests, then a live shift to 50/50 measuring
46%/54%, zero Envoy restarts. Release v0.5.0.

---

**Claim:** "Deployed the service mesh into Kubernetes with dynamic, non-
hardcoded service discovery, verified across replica scaling events."

Evidence: `cmd/k8s-watcher/main.go`, `deployments/kubernetes/*.yaml`,
`scripts/k8s_smoke_test.sh` — live against a real `kind` cluster: 3
replicas discovered with zero hardcoded IPs, scaled to 5 then to 2, traffic
continued throughout. Release v0.7.0.

---

**Claim:** "Built Prometheus/Grafana observability and operator-facing
debug APIs for a distributed control plane."

Evidence: `internal/metrics/metrics.go`, `internal/api/debug_handlers.go`,
`internal/xds/tracker.go`, `docker-compose.yml` +
`deployments/docker/grafana/`, `internal/api/debug_handlers_test.go` (5
tests) + `internal/xds/tracker_test.go` (2 tests). Verified live:
Prometheus scrape targets `up`, real metric values, Grafana dashboard
auto-provisioned and its queries checked against Envoy's actual metric
names. Release v0.8.0.

---

**Claim:** "Measured control-plane performance at scale: sub-millisecond
config generation for 1000 endpoints, and validated propagation latency
across concurrently-connected xDS clients."

Evidence: `internal/xds/snapshot_bench_test.go` (Go benchmarks),
`test/benchmark/xds_scale_test.go` (real gRPC ADS clients, not mocked),
`test/benchmark/churn_test.go`,
`test/benchmark/results/v0.9.0_scale.json` — 0.83ms/op at 100
services/1000 endpoints; 185-203ms propagation to 10-50 concurrent
clients; 4.58M churn operations in 2s with 0 failures. Release v0.9.0.
Explicitly documented what was *not* measured (real Envoy binaries at
scale) rather than misrepresenting simulated numbers.

---

**Claim:** "Enforced a zero-invalid-config invariant end-to-end via
validated route specs and pre-publish consistency checks."

Evidence: `internal/routing/route.go` (`Spec.Validate`),
`internal/xds/snapshot.go` (`snap.Consistent()` before every publish),
`docs/runbooks/config-rejection.md`, `docs/adr/ADR-007-traffic-management.md`.
Releases v0.4.0 (consistency check) and v0.5.0 (spec validation).

---

**Claim:** "Documented and honestly assessed the single-point-of-failure
trade-off of a centralized control-plane architecture."

Evidence: `docs/adr/ADR-011-failure-handling.md`,
`docs/runbooks/control-plane-failure.md`. No release-specific test (this
is an architectural property, not a fixable bug) — the honesty of this
documentation is itself part of the evidence.
