# ADR-009: Kubernetes Deployment

## Context

v0.7.0 targets the container-networking gap directly: deploy the mesh into
Kubernetes and prove that scaling a Deployment up/down flows through
service discovery and xDS to Envoy without hardcoded pod IPs or manual
intervention.

## Problem

`internal/registry` has no native Kubernetes awareness — it only knows
about instances explicitly registered/heartbeated/deregistered through its
HTTP API (see ADR-002). Kubernetes, meanwhile, already tracks which pods
back a Service via `Endpoints`/`EndpointSlice` objects. How should these two
be bridged without hardcoding pod IPs anywhere?

## Options

1. **Rewrite the registry to be Kubernetes-native** (watch EndpointSlices
   directly inside the control plane) — couples the control plane to one
   platform, contradicting the project's "independent service mesh, not a
   Kubernetes-only tool" framing (registry already supports non-k8s
   registration via its HTTP API for the Docker Compose demos in
   v0.1.0-v0.6.0).
2. **A separate bridge process (`cmd/k8s-watcher`) that watches Kubernetes
   Endpoints and drives the existing registry HTTP API** — keeps the
   registry platform-agnostic; Kubernetes becomes just another source of
   truth for instance discovery, on equal footing with the manual `curl
   POST /v1/services` used in earlier releases' demos.
3. **A Kubernetes operator/CRD-based approach** — far more machinery
   (CRDs, controller-runtime, webhook validation) than this project's scope
   justifies for what is fundamentally "list Endpoints, call an HTTP API."

## Decision

Option 2. `cmd/k8s-watcher` polls a target Service's `Endpoints` (via
`client-go`) every `poll-interval` (default 2s), diffs the current pod set
against what it has already registered, and converges: registers new pod
IPs, heartbeats known ones, deregisters pods no longer present. Instance
IDs are the pod name (`TargetRef.Name`), not a synthetic ID, so registry
state is directly traceable to `kubectl get pods`.

Polling (not the Kubernetes watch API) was chosen deliberately for this
release: a poll loop is trivial to reason about and test, and at the scale
of a demo/portfolio deployment (single-digit replica counts), a 2s poll
interval is indistinguishable from a push-based watch in practice. Moving
to `client-go`'s informer/watch machinery is straightforward future work if
this ever needs to track hundreds of endpoints with lower latency.

## Trade-offs

- Poll-based discovery adds up to `poll-interval` (2s) of latency between
  a pod becoming ready and it being registered, and between a pod
  terminating and being deregistered — acceptable given the reconciler's
  own `CP_RECONCILE_INTERVAL` (1-2s) is the same order of magnitude.
- `k8s-watcher` has one hardcoded Kubernetes Service to watch per instance
  (`-service=backend-a` flag) — a production version would need one
  watcher per mesh-enrolled service or a multi-service watch loop; this is
  explicitly out of scope for a project demonstrating the pattern.
- RBAC is scoped to `get/list/watch` on `endpoints` only, in the single
  namespace the watcher runs in (see `deployments/kubernetes/k8s-watcher.yaml`)
  — least-privilege for what the component actually does.

## Consequences

Verified live against a real `kind` cluster (`scripts/k8s_smoke_test.sh`):
3 replicas of `backend-a` are discovered with zero hardcoded IPs; scaling to
5 and then to 2 replicas is reflected in the registry (`GET
/v1/services/backend-a` instance count) within seconds, with traffic
through `envoy-dynamic` continuing throughout, no restarts anywhere in the
chain.
