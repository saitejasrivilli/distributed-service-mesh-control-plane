# ADR-011: Failure Handling

## Context

This project deliberately runs a **single** control-plane process — no
consensus, no replicas, no leader election. Every other ADR touches a piece
of failure handling; this one names the trade-off explicitly and
consolidates where each failure mode is handled.

## Decision: honest single point of failure

The control plane is a single point of failure by design, not by oversight.
See `docs/runbooks/control-plane-failure.md` for the full discussion:
Envoy keeps serving its last-known-good snapshot if the control plane
disappears, but no new registrations/deregistrations/heartbeats/route
changes take effect until it returns, and any in-flight registry state is
lost (in-memory, not persisted — ADR-002).

**Why not build multi-replica failover into this project:** doing so
correctly requires either a shared/persistent registry backend (a real
distributed-systems problem: consistency model, quorum, partition
handling) or a gossip protocol between control-plane instances — both are
substantial projects in their own right, and bolting on a half-measure
(e.g., active-passive with no real consistency story) would be worse than
naming the limitation clearly. This is the single most important trade-off
to be able to explain in an interview: **why is centralized xDS control
simpler to reason about than gossip-based discovery, and what do you give
up?** Answer: a centralized control plane gives one consistent view of
desired state and trivial-to-reason-about ordering (this project's whole
design leans on that — see ADR-003's full-rebuild-every-tick model); the
cost is exactly this single point of failure.

## Where each other failure mode is handled

| Failure | Handled by | Doc |
|---|---|---|
| Backend crash/hang | Heartbeat staleness -> `SweepStale` -> EDS exclusion | ADR-008, runbook: backend-failure |
| Envoy restart/disconnect | go-control-plane's ADS server re-serves last snapshot from cache on reconnect; `ConnectionTracker` reflects the gap | ADR-010 |
| Control-plane restart | See above — honest SPOF | This ADR, runbook: control-plane-failure |
| Invalid config (bad route weights, inconsistent snapshot) | Rejected before ever reaching the routing store or Envoy | ADR-007, runbook: config-rejection |
| Rapid endpoint churn | Reconciler rebuilds the full snapshot each tick; measured 4.58M churn ops/2s with 0 failures | v0.9.0 benchmarks |
| Reconcile failure (build/publish error) | Exponential backoff + jitter, previous snapshot stays authoritative | ADR-006 |

## Consequences

A reviewer asking "what happens if X fails" should be answerable by
pointing at one of: a registry health-state transition, a runbook, or this
table — not a hand-wave. That was the test applied while writing this ADR.
