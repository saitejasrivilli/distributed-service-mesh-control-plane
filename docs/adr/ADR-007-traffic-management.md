# ADR-007: Traffic Management

## Context

v0.5.0 adds the service-to-service traffic controls the target role cares
about: weighted/canary routing, retries, timeouts, circuit breaking —
applied dynamically, without Envoy restarts, building on v0.4.0's xDS
pipeline.

## Problem

How should a desired traffic split (e.g., 90% v1 / 10% v2) become Envoy
`weighted_clusters` routing, alongside per-route retry/timeout policy and
per-cluster circuit breaker thresholds, while keeping "invalid config is
never published" true?

## Decision

`internal/routing.Spec` is the desired-state type: a service name, a list of
`VersionWeight{Version, Weight}` splits, an optional retry policy, an
optional timeout, and an optional circuit breaker. `Spec.Validate()` is the
single gate: weights must sum to exactly 100, versions must be unique, and
`retry_on` requires `num_retries > 0`. `routing.Store.Set` only ever stores
a spec that passed `Validate()` — an invalid spec is rejected at the HTTP
layer (`PUT /v1/routes/{service}` returns 400) and never reaches the
reconciler.

`xds.BuildSnapshot` was extended (not replaced) to consume the store: for
each service, each version split becomes its own EDS-backed cluster named
`service::version`, filtered from the registry by `Instance.Version`. When
no `Spec` is configured for a service, it falls back to the pre-v0.5.0
behavior — one cluster named after the service, with all healthy instances
regardless of version. This kept every v0.2.0-v0.4.0 test passing unchanged
and means traffic management is opt-in per service.

An empty `Version` (`""`) is a legitimate split label meaning "unversioned"
— registry instances default to `Version == ""` when callers don't tag one.
An earlier draft of `Validate()` rejected empty versions outright, which
made the single-cluster fallback case (`Splits: [{Version: "", Weight:
100}]`) impossible to express explicitly; this was caught by
`TestRetryPolicyAppliedToRoute` failing and fixed by allowing `""` as an
ordinary (if unusual) version label.

## Trade-offs

- Retries/timeouts/circuit-breakers are set per-service, not per-route or
  per-path — sufficient for this project's scope (one route per service),
  revisit if RDS grows multiple routes per virtual host.
- No canary-by-header or percentage-of-users stickiness — weighted
  round-robin only. Session affinity is future work if a use case demands
  it.

## Consequences

Verified live (not simulated): a 90/10 canary split measured 173/27 (86.5%
/13.5%) over 200 real requests; shifting to 50/50 via `PUT
/v1/routes/backend-a` — with **zero Envoy restart** — measured 92/108
(46%/54%) over the next 200 requests. See `scripts/traffic_smoke_test.sh`
and `test/benchmark/results/v0.5.0_latency.json` for the automated,
repeatable version and the measured latency numbers (p50/p95/p99, 0 errors
over 300 requests).
