# Failure Modes

See `docs/adr/ADR-011-failure-handling.md` for the consolidated table; this
doc is the narrative version for talking through in an interview.

## Backend crash or hang

Detected via missed heartbeats, not passive TCP checks. `SweepStale`
persists a `Healthy=false` transition once `CP_STALE_AFTER` (default 15s)
elapses without a heartbeat; the next reconcile tick removes the instance
from EDS. Recovery is symmetric: one heartbeat call flips it back healthy,
next tick re-adds it. Verified live, zero Envoy restarts either direction
(`scripts/health_reconciliation_smoke_test.sh`).

## Envoy disconnect/restart

Handled entirely by go-control-plane's ADS server (unmodified third-party
code) — on reconnect, Envoy gets the current snapshot fresh.
`ConnectionTracker` (v0.8.0) reflects connection gaps for observability
(`GET /v1/debug/envoys`), but doesn't itself drive any recovery logic —
there's nothing to recover; the snapshot cache already has what's needed.

## Control-plane crash/restart

**Honest single point of failure** (ADR-011): existing Envoys keep serving
their last snapshot, but no new registry mutations take effect until the
control plane returns, and in-memory registry state is lost (not
persisted). Documented explicitly rather than hidden — see
`docs/runbooks/control-plane-failure.md` for what a production hardening
pass would need (persistent/shared registry, or multi-replica + failover).

## Invalid configuration

Two independent gates, at two different layers: `routing.Spec.Validate()`
before a spec enters the routing store, and `snap.Consistent()` before any
snapshot is published. Either failing means the *previous* config stays
authoritative — never a partial or broken push to Envoy. See
`docs/runbooks/config-rejection.md`.

## Rapid endpoint churn

The reconciler rebuilds the *entire* snapshot every tick rather than
tracking incremental deltas — under churn, this means no accumulated
drift between "what changed" and "what's published," at the cost of
recomputing everything every tick. Measured (v0.9.0): 4.58M
register/heartbeat/deregister operations in 2 seconds against 100
services/1000 endpoints, 0 reconciliation failures.

## What's explicitly NOT handled

- Network partitions between the control plane and a subset of Envoys —
  those Envoys simply keep their last snapshot until connectivity returns;
  there's no split-brain risk since there's only one control-plane replica
  to begin with.
- Byzantine/malicious Envoy or backend behavior — out of scope; see
  `docs/adr/ADR-012-security.md`.
