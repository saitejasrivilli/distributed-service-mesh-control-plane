# Traffic Management

## Model

`internal/routing.Spec`: a service name, a list of `{Version, Weight}`
splits, optional retry policy (`RetryOn`/`NumRetries`), optional timeout
(`TimeoutMs`), optional circuit breaker thresholds. `Spec.Validate()`
enforces weights summing to exactly 100, no duplicate versions, and
`retry_on` requiring `num_retries > 0` — checked before the spec ever
enters `routing.Store`, so an invalid `PUT /v1/routes/{service}` is a
client-side no-op, never a partial or corrupting write.

## How it becomes Envoy config

Each version split becomes its own `service::version` EDS cluster (see
`docs/interview/xds.md`). The route action is a single-cluster route when
only one split exists at weight 100, or a `weighted_clusters` action
otherwise. Retry policy and timeout attach to the `RouteAction`; circuit
breaker thresholds attach to each version's `Cluster`.

## Measured behavior (not simulated)

A 90/10 configured split measured 173/27 (86.5%/13.5%) over 200 real HTTP
requests through a live Envoy; shifting the same route to 50/50 — with
zero Envoy restart — measured 92/108 (46%/54%) over the next 200 requests.
Reproducible via `scripts/traffic_smoke_test.sh`, which prints the split
counts directly. The separate latency numbers (p50=2.01ms, p95=2.81ms,
p99=3.38ms, 0 errors over 300 requests) live in
`test/benchmark/results/v0.5.0_latency.json`.

## Why per-service, not per-route granularity

Retry/timeout/circuit-breaker settings are scoped to a service (one route
config per service, ADR-007), not to individual paths within a service.
This matches the project's one-route-per-service RDS model; if a future
release needed path-level policy, it would extend `routing.Spec` with a
path matcher rather than changing this document's model.

## What's not implemented

No session affinity / sticky canary (weighted round-robin only), no
header-based routing rules, no traffic mirroring/shadowing. All
straightforward extensions of the same `RouteAction` machinery, not
attempted here because nothing in this project's scope required them.
