# Project Walkthrough

## 30-second explanation

I built an independent Envoy xDS service-mesh control plane in Go: a
service registry, a reconciler that turns registry state into versioned
CDS/EDS/LDS/RDS snapshots, and a gRPC ADS server that pushes them to Envoy
— all dynamically, with zero Envoy restarts. It runs standalone via Docker
Compose or in Kubernetes with a bridge component that discovers pod IPs
automatically. Every claim in it is backed by a passing test or a live,
scripted demo against real containers.

## 2-minute explanation

The architecture is: Service Registry -> Reconciler -> versioned xDS
Snapshot -> gRPC ADS server -> Envoy -> backend traffic. The registry
(v0.2.0) is a thread-safe in-memory store with heartbeat-based health —
missing a heartbeat past a threshold flips an instance unhealthy, which
propagates automatically. The reconciler (v0.4.0/v0.6.0) rebuilds a full
snapshot every tick from current registry + traffic-management state, and
never publishes an inconsistent one (checked via go-control-plane's
`Consistent()`). v0.5.0 adds weighted/canary routing, retries, timeouts,
and circuit breaking, all as Envoy `RouteAction`/`Cluster` config generated
from a validated spec. v0.7.0 deploys the whole thing into Kubernetes with
a small bridge process that watches Service endpoints and calls the same
registry HTTP API a manual integration would — no pod IP is ever
hardcoded. v0.8.0 adds Prometheus metrics, a Grafana dashboard, and debug
endpoints that answer "what does Envoy actually have right now." Every
release has a scripted, automated smoke test against real Docker/
Kubernetes, not mocks — I have logs and measured numbers for every claim,
not estimates.

## 5-minute explanation

Start from the problem: production service meshes (the kind AWS/Envoy/
Istio-style infra teams operate) solve the same handful of problems —
service discovery, dynamic traffic control, and failure isolation — via a
control-plane/data-plane split. I built a minimal but real version of that
split to close a specific gap in my background: I'd done load balancing,
replication, and fault tolerance at the application layer before, but not
the service-mesh-specific stack (xDS, CDS/EDS/LDS/RDS, Envoy sidecar
architecture). So this project is deliberately scoped to *not* re-prove
generic distributed-systems concepts I already had evidence for, and to
concentrate on the specialized pieces.

Walking the releases: v0.1.0 is the control-plane skeleton (config,
structured logging with correlation IDs, Prometheus metrics, graceful
shutdown) — unglamorous but every later release depends on it being
correct. v0.2.0 is the service registry: register/deregister/heartbeat,
namespace isolation, health-aware endpoint filtering, all thread-safe and
tested under `-race`. v0.3.0 introduces the actual data plane — Envoy as a
sidecar, static config first, to prove connectivity (client -> Envoy ->
backend, and Envoy-to-Envoy chaining) before anything dynamic. v0.4.0 is
the core release: the xDS control plane itself, using go-control-plane's
`SnapshotCache` rather than hand-rolling the wire protocol, verified live
by registering/deregistering a backend and watching Envoy's admin
interface add/remove the cluster with zero restarts. v0.5.0 layers
traffic management on top — I measured an actual 90/10 canary split
landing at 86.5%/13.5% over 200 real requests, then shifted it to 50/50
live and re-measured. v0.6.0 makes health state a first-class, persisted
concept (not just a read-time filter) and documents the invariant that
invalid config is never published — with runbooks for the failure modes
that actually happen. v0.7.0 deploys into a real `kind` cluster and proves
3->5->2 replica scaling flows through automatically. v0.8.0 adds
observability people would actually use to debug this in production.
v0.9.0 is honest performance validation — real numbers, including an
explicit note about what I *didn't* measure (100 real Envoy binaries,
which wasn't feasible on my laptop) rather than dressing up simulated
numbers as something they're not.

## 15-minute deep dive

See the individual docs in this directory:
- `architecture.md` — full component map and data/control flow
- `control-plane.md` — internal structure of the reconciler and registry
- `xds.md` — CDS/EDS/LDS/RDS generation and versioning in detail
- `service-discovery.md` — registry design and Kubernetes bridging
- `traffic-management.md` — weighted routing, retries, circuit breaking
- `container-networking.md` — the Envoy sidecar model and Kubernetes deployment
- `failure-modes.md` — what happens when each component fails
- `scalability.md` — v0.9.0's measured numbers and their honest limits
- `observability.md` — metrics, debug endpoints, Grafana
- `tradeoffs.md` — the deliberate simplifications and what they cost

And `docs/resume-evidence.md`, which maps every claim I'd put on a resume
back to the specific code, test, and release that backs it.
