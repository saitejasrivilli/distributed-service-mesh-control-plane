# Distributed Service Mesh Control Plane

Repo: https://github.com/saitejasrivilli/distributed-service-mesh-control-plane

**Status: v1.0.0 — production-quality release.** Service registry,
dynamic Envoy xDS (CDS/EDS/LDS/RDS), weighted/canary traffic management,
health-aware reconciliation, Kubernetes deployment, Prometheus/Grafana
observability, and measured scale/performance validation. See
`CHANGELOG.md` for the full per-release history.

> This is an independent service-mesh implementation inspired by
> production service-discovery and traffic-management architectures. It is
> not an implementation of AWS ECS Service Connect, AWS Service Gateway,
> or AWS Cloud Map.

## 1. Problem

Production infrastructure teams (this project targets an AWS SDE role
focused on container networking, service mesh, and Envoy/xDS) solve a
recurring set of problems: how does service A find service B, how does
traffic shift between versions safely, and how does the system react when
an instance dies — all without restarting the data plane. This project is
a real, working, from-scratch implementation of that control-plane/
data-plane pattern, built specifically to demonstrate the specialized
pieces (xDS, CDS/EDS/LDS/RDS, Envoy sidecar architecture, Kubernetes-native
discovery) rather than re-proving general distributed-systems experience
already held elsewhere.

## 2. Architecture

```
Management API (HTTP)
       |
       v
Control Plane
  Registry            in-memory, thread-safe, heartbeat-driven health
  Reconciler           registry+routing state -> versioned xDS snapshot
  Config Builder        internal/xds.BuildSnapshot (CDS/EDS/LDS/RDS)
  xDS Server            gRPC ADS, go-control-plane SnapshotCache
       |
   CDS / EDS / LDS / RDS
       |
   Envoy (data plane, sidecar per backend)
       |
   Backend services
```

Full diagram and component responsibilities:
`docs/architecture/system.md`. Fifteen-minute deep dive with data/control
flow, consistency/concurrency models, and platform paths:
`docs/interview/architecture.md`.

## 3. Control plane / data plane

The control plane (`internal/controlplane`, `internal/registry`,
`internal/reconciliation`, `internal/xds`) only ever configures Envoy — it
never proxies traffic itself. Envoy is the data plane: every request flows
client -> Envoy -> backend, with cluster selection, health-aware routing,
retries, timeouts, and circuit breaking all enforced by Envoy from
control-plane-generated config. See `docs/interview/control-plane.md`.

## 4. Service registry

`internal/registry`: register/deregister/heartbeat, namespace isolation,
deterministic listings, thread-safe under `-race`. See
`docs/adr/ADR-002-service-registry.md` and
`docs/interview/service-discovery.md`.

## 5. Service discovery

Two sources converge on the same registry HTTP API: manual registration
(Docker Compose demos) and `cmd/k8s-watcher` bridging Kubernetes
`Endpoints` (zero hardcoded pod IPs). See
`docs/adr/ADR-009-kubernetes-deployment.md`.

## 6. Envoy architecture

Envoy runs as a sidecar in front of each backend; backends implement zero
routing logic. v0.3.0 proves static connectivity (including Envoy-to-Envoy
sidecar chaining) before v0.4.0 introduces dynamic ADS config. See
`docs/interview/container-networking.md`.

## 7. xDS architecture

`internal/xds` builds CDS/EDS/LDS/RDS from registry + routing state using
`envoyproxy/go-control-plane`'s `SnapshotCache`, versioned and
consistency-checked before every publish. See `docs/adr/ADR-004-xds-architecture.md`
and `docs/interview/xds.md`.

## 8. CDS/EDS/LDS/RDS

- **CDS**: one cluster per service, or per `service::version` under a
  traffic split.
- **EDS**: healthy (heartbeat-fresh) instances, filtered by version when
  split.
- **LDS**: one listener per service, deterministic port assignment.
- **RDS**: single-cluster or `weighted_clusters` route, carrying retry/
  timeout policy.

## 9. Traffic management

Weighted/canary routing, retries, timeouts, circuit breaking — validated
before publish, applied dynamically with zero Envoy restarts. Measured: a
90/10 canary split landed at 86.5%/13.5% over 200 real requests; a live
shift to 50/50 measured 46%/54% over the next 200. See
`docs/adr/ADR-007-traffic-management.md` and
`docs/interview/traffic-management.md`.

## 10. Failure handling

Backend failure -> heartbeat staleness -> persisted health transition ->
EDS exclusion, zero restarts. Invalid config is never published (two
independent validation gates). Control-plane failure is an **honest single
point of failure** — documented, not hidden. See
`docs/adr/ADR-011-failure-handling.md`, `docs/runbooks/`, and
`docs/interview/failure-modes.md`.

## 11. Kubernetes deployment

Deployed into a real `kind` cluster: control-plane, 3 backend replicas,
Envoy, and `k8s-watcher`. Scaling 3->5->2 replicas is reflected in the
registry and Envoy's config within seconds, zero hardcoded IPs anywhere.
See `docs/adr/ADR-009-kubernetes-deployment.md`.

## 12. Observability

Prometheus metrics (services/endpoints/xDS updates/reconciliation/
connected Envoys), an auto-provisioned Grafana dashboard, structured JSON
logging with correlation IDs, and debug endpoints
(`/v1/debug/services/{name}`, `/v1/debug/envoys`, `/v1/debug/config/{service}`).
See `docs/adr/ADR-010-observability.md` and `docs/interview/observability.md`.

## 13. Performance

Measured (not invented), Apple M2 local machine: 100 services/1000
endpoints snapshot generation in ~0.83ms; propagation to 10-50 concurrent
real gRPC ADS clients in 185-203ms (flat across client counts); 4.58M
registry churn operations in 2s with zero reconciliation failures. Full
numbers and methodology: `test/benchmark/results/v0.9.0_scale.json` and
`docs/interview/scalability.md`.

## 14. Testing

Unit + integration tests across every package (85.7% coverage on core
control-plane logic, measured via `go test -coverpkg=./internal/...`), all
passing under `-race`. Five automated, scripted smoke tests exercise real
Docker/Kubernetes infrastructure end-to-end (not mocks): `scripts/*.sh`.
Go benchmarks and real-gRPC-client scale tests: `internal/xds/*_bench_test.go`,
`test/benchmark/`.

## 15. Limitations

- Single control-plane process, no HA/failover (ADR-011) — deliberate,
  documented trade-off.
- No authentication on the management API or xDS server (ADR-012) — this
  is a portfolio/demo project, not hardened for untrusted networks.
- In-memory registry only, no persistence (ADR-002).
- Full-rebuild-every-tick, not incremental/delta xDS (ADR-003, ADR-004).
- Scale validation used real gRPC ADS clients simulating Envoy's protocol,
  not up to 100 real Envoy binaries, due to local machine constraints
  (explicitly noted in `test/benchmark/results/v0.9.0_scale.json`).

See `docs/interview/tradeoffs.md` for the full list with reasoning.

## 16. Future work

- v1.1.0+ (optional, only after this release is stable): AWS Cloud Map
  integration, ECS deployment, ALB/NLB ingress controller — only if
  implemented and tested against real AWS resources, never faked.
- Delta xDS for higher service counts.
- Persistent/shared registry backend and multi-replica control-plane HA.
- mTLS between Envoy and the control plane; API-key/OIDC on management
  endpoints.

---

## Quickstart

```
git clone https://github.com/saitejasrivilli/distributed-service-mesh-control-plane.git
cd distributed-service-mesh-control-plane
go run ./cmd/control-plane
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/metrics
```

Full observability stack (control-plane + backend-a + Envoy + Prometheus +
Grafana):

```
docker compose up --build
open http://localhost:3000       # Grafana, dashboard auto-provisioned
open http://localhost:9090       # Prometheus
curl localhost:8080/v1/debug/envoys
curl localhost:8080/v1/debug/config/backend-a
```

## Example: service registration

```
curl -X POST localhost:8080/v1/services -d '{
  "service_name":"backend-a","instance_id":"i1",
  "address":"10.0.0.5","port":9000
}'
curl localhost:8080/v1/services/backend-a
curl -X POST localhost:8080/v1/services/backend-a/instances/i1/heartbeat
curl "localhost:8080/v1/services/backend-a/instances?healthy=true"
curl -X DELETE localhost:8080/v1/services/backend-a/instances/i1
```

## Example: dynamic xDS update (no Envoy restart)

```
docker compose -f deployments/docker/docker-compose-xds.yml up --build
curl -X POST localhost:8080/v1/services -d '{"service_name":"backend-a","instance_id":"i1","address":"<backend-a-ip>","port":9000}'
curl localhost:20000/echo   # Envoy now serving backend-a -- no restart happened
./scripts/xds_smoke_test.sh   # full dynamic add/remove test, automated
```

## Example: canary route change

```
docker compose -f deployments/docker/docker-compose-traffic.yml up --build
curl -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":90},{"version":"v2","weight":10}]}'
curl -X PUT localhost:8080/v1/routes/backend-a -d '{"splits":[{"version":"v1","weight":50},{"version":"v2","weight":50}]}'
./scripts/traffic_smoke_test.sh   # measures real canary distribution at both splits
```

## Failure demonstration

```
./scripts/health_reconciliation_smoke_test.sh   # stale -> unhealthy -> Envoy avoids -> heartbeat -> recovered
./scripts/envoy_smoke_test.sh                   # backend kill/restart via Envoy health checks
```

## Kubernetes deployment

```
kind create cluster --name mesh-demo
docker build -t mesh/control-plane:dev .
kind load docker-image mesh/control-plane:dev --name mesh-demo
kubectl apply -f deployments/kubernetes/
./scripts/k8s_smoke_test.sh   # full build+deploy+discovery+scaling test
```

## Benchmarks and scale validation

```
go test ./internal/xds/... -bench=BuildSnapshot -benchmem
go test ./test/benchmark/... -v
curl localhost:8080/debug/pprof/heap -o heap.pprof
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

## Test strategy

See section 14 above, `docs/interview/architecture.md`'s concurrency/
consistency notes, and each release's entry in `CHANGELOG.md` for the
specific tests added per capability.

## Release history

`CHANGELOG.md` (full detail) and `docs/releases/index.md` (quick index
with tag links). v0.1.0 through v1.0.0, one capability per release, tested
and tagged before the next began.

## Documentation index

- `docs/architecture/system.md` — design, updated per release
- `docs/adr/` — 12 Architecture Decision Records
- `docs/runbooks/` — operational playbooks for real failure scenarios
- `docs/interview/` — deep-dive docs + `project-walkthrough.md` (30s/2min/5min/15min explanations)
- `docs/resume-evidence.md` — every resume-worthy claim mapped to code/test/release
- `SECURITY.md`, `CONTRIBUTING.md`
