# Control Plane

## Structure

`internal/controlplane.ControlPlane` is a thin orchestrator: it constructs
the registry, routing store, snapshot cache, reconciler, xDS server, and
HTTP API server via plain constructor injection (no DI framework — see
ADR-001), then runs all three concurrently (`Run`) and shuts them down
together on context cancellation.

## The reconciler is the heart of the control plane

`internal/reconciliation.Reconciler.Reconcile` does, in order: sweep stale
instances (health state transition), rebuild the full snapshot, check
consistency, publish. This ordering means a health transition from the
current tick is guaranteed visible in the snapshot published in that same
tick — no lag between "instance went stale" and "Envoy stopped routing to
it" beyond the reconcile interval itself.

## Failure isolation within the reconciler

If `BuildSnapshot` or `SetSnapshot` fails, the function returns an error
without ever calling `SetSnapshot` with bad data — the cache's previous
snapshot (still valid) keeps being served. `Run` applies exponential
backoff with jitter after consecutive failures (capped at 30s), resetting
to normal cadence on the next success (ADR-006).

## Why not more indirection

`ControlPlane.New` wires concrete types together directly rather than via
a plugin/factory system. Every dependency has exactly one implementation
in this project (`registry.InMemory`, `cachev3.SnapshotCache`, etc.) — an
abstraction layer with only one implementation is speculative complexity,
not flexibility. Where a real seam exists (the registry storage backend),
it's already an interface (`registry.Registry`) precisely because ADR-002
anticipated a persistent implementation being genuinely likely future
work.
