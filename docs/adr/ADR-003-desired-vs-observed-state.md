# ADR-003: Desired vs. Observed State

## Context

The control plane's core loop (registry -> reconciler -> xDS -> Envoy) only
makes sense if "desired state" and "observed state" are clearly distinct
concepts with a defined convergence mechanism between them.

## Decision

**Desired state** = what the registry says should be routable right now:
every instance with `Healthy == true` and a heartbeat within `staleAfter`
(`registry.HealthyInstances`), plus whatever traffic-management `Spec` is
configured in `routing.Store` for a service.

**Observed state** = what Envoy is actually doing, which this project
observes indirectly: via the xDS ACK/NACK protocol (handled by
go-control-plane's server, not custom code), via `internal/xds.tracker.go`
tracking which streams are connected, and via `GET
/v1/debug/config/{service}` reading the reconciler's last **published**
snapshot rather than recomputing desired state fresh.

**Convergence** happens exactly once per reconcile pass
(`Reconciler.Reconcile`, see ADR-006): sweep stale instances (observed
heartbeat gaps become desired-state health changes), then build+publish a
new snapshot. There is no separate "diffing" step that computes a
minimal delta between old and new desired state — `xds.BuildSnapshot`
always rebuilds the full snapshot from current registry state, and
go-control-plane's cache handles the actual wire-level diffing against
what each connected Envoy last acknowledged.

## Trade-offs

Rebuilding the full snapshot every reconcile (rather than hand-rolling an
incremental diff) is simpler and correct by construction — there is no way
for computed-desired-state and published state to drift out of sync from a
partial-update bug, since there are no partial updates. The cost is
recomputing snapshot data structures every tick even when nothing changed;
measured at ~0.83ms for 100 services/1000 endpoints (v0.9.0 benchmarks),
this cost is negligible at this project's scale.

## Consequences

Every other ADR in this project (004 xDS, 006 reconciliation, 007 traffic
management) builds on this same desired-state -> full-rebuild -> publish
model without exception, which is what let v0.5.0's weighted routing and
v0.6.0's health sweeping slot into `BuildSnapshot`/`Reconcile` without
restructuring either.
