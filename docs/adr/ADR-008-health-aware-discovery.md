# ADR-008: Health-Aware Discovery

## Context

Service discovery is only useful if unhealthy instances are excluded from
the set Envoy actually routes to. This project defines "healthy" purely in
terms of heartbeat recency, not passive process/TCP liveness assumptions.

## Decision

An instance is discoverable (returned by `HealthyInstances`, and therefore
present in EDS) if and only if:

1. `Instance.Healthy == true` (flipped `true` on register/heartbeat,
   `false` by `SweepStale` — see ADR-006), **and**
2. `now - LastHeartbeat <= staleAfter` (defense in depth: even if `Healthy`
   hasn't been swept yet, a stale-but-not-yet-swept instance is still
   excluded from reads).

This double condition exists because sweeping happens on the reconciler's
cadence (every `CP_RECONCILE_INTERVAL`), while `HealthyInstances` can be
called at any time (e.g., mid-tick, from a debug endpoint) — the read-time
staleness check guarantees correctness doesn't depend on sweep timing.

Health-aware discovery in Kubernetes (v0.7.0) layers on top of this
unchanged: `cmd/k8s-watcher` calls the same `Heartbeat` API on the same
cadence a manually-integrated backend would, so Kubernetes-discovered
instances go through the identical health model as Docker Compose demo
instances — there is no separate "k8s health" concept.

## Trade-offs

- This is heartbeat-based, not passive-check-based (no TCP/HTTP active
  health checking from the control plane itself toward instances — that's
  Envoy's `health_checks` cluster config, used in the v0.3.0 static Envoy
  demo, and is a separate, complementary mechanism at the data-plane
  layer). The registry only trusts what backends actively report.
- Single staleness window (`CP_STALE_AFTER`) for every service — no
  per-service tuning yet. Straightforward future work if a service needs a
  tighter or looser SLA.

## Consequences

See `docs/runbooks/stale-endpoint.md` and `docs/runbooks/backend-failure.md`
for the operational playbooks, and
`scripts/health_reconciliation_smoke_test.sh` for the live, automated proof
that a stale instance is excluded from EDS and recovers on heartbeat, with
zero Envoy restarts.
