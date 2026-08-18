# ADR-006: Reconciliation

## Context

v0.4.0 introduced a reconciler that rebuilds and publishes xDS snapshots on
a timer. v0.6.0 needs it to also drive health-state transitions (not just
config generation) and to behave reasonably under repeated failure.

## Problem

Where should the desired-state -> observed-state reconciliation happen, and
how should the reconciler behave when reconciliation itself starts failing
(snapshot build errors, cache publish errors)?

## Decision

`Reconciler.Reconcile` does two things in one atomic step, in this order:

1. `registry.SweepStale(staleAfter)` — transitions any instance whose
   `LastHeartbeat` has exceeded `staleAfter` from `Healthy=true` to
   `Healthy=false`, persisting the transition (not just filtering reads).
2. `xds.BuildSnapshot` + `cache.SetSnapshot` — rebuild and publish,
   respecting the "never publish invalid config" invariant from ADR-004.

Doing the health sweep and snapshot build in the same reconcile pass (not
as two independently-scheduled loops) guarantees a stale-instance
transition is reflected in the very next snapshot, with no window where
they're out of sync.

`Reconciler.Run` applies exponential backoff with jitter
(`backoff(consecutiveFailures)`, capped at 30s) on top of the normal
interval after a failed reconcile, resetting to zero delay on the next
success. This prevents a persistently-failing control plane (e.g., a
transient cache error) from hammering retries in a tight loop, while
capping the backoff so recovery is still fast once the underlying issue
clears.

## Trade-offs

- Sweeping and snapshot-building in one pass means a slow snapshot build
  slightly delays how quickly the next sweep's *results* are visible in
  Envoy — acceptable at this project's scale (sub-millisecond builds
  observed for a handful of services).
- Backoff is per-process, not distributed — irrelevant today since there is
  one control-plane instance (see ADR-011 for the single-point-of-failure
  discussion), but would need coordination (e.g., jittered backoff seeded
  per-instance) if this control plane were ever run with multiple replicas
  reconciling independently.

## Consequences

Verified live: a backend with no heartbeats transitions to `Healthy=false`
within `CP_STALE_AFTER`, Envoy stops routing to it (confirmed via `no
healthy upstream`) with zero restarts, and a single heartbeat call recovers
it on the next reconcile tick — see
`scripts/health_reconciliation_smoke_test.sh` and
`docs/runbooks/backend-failure.md` / `docs/runbooks/stale-endpoint.md`.
