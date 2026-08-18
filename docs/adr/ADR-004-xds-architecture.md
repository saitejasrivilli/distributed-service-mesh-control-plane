# ADR-004: xDS Architecture

## Context

v0.4.0 is the core release: the control plane must generate CDS/EDS/LDS/RDS
resources from registry state and push them to Envoy dynamically — no Envoy
restarts, ever.

## Problem

How should registry state become versioned Envoy configuration, served over
gRPC, without hand-rolling the xDS wire protocol?

## Options

1. **`envoyproxy/go-control-plane`'s `SnapshotCache` + `server/v3`** — the
   reference Go xDS server implementation; handles ADS stream multiplexing,
   resource-type-aware diffing, and node/version bookkeeping.
2. **Hand-roll the gRPC discovery services** — full control, but reimplements
   a well-tested protocol (delta vs. state-of-the-world, nonce/version
   tracking) for no benefit at this project's scale.
3. **REST-based xDS (ADS over HTTP)** — Envoy supports it, but gRPC ADS is
   the production-standard transport and is what the target role's JD
   actually asks about.

## Decision

Option 1. `internal/xds.BuildSnapshot(reg, version)` is a pure function:
registry state in, a `*cachev3.Snapshot` out, covering CDS/EDS/LDS/RDS
together so `snap.Consistent()` can be checked before publish — an
inconsistent snapshot (e.g., a route referencing a nonexistent cluster) is
never sent to Envoy.

`internal/reconciliation.Reconciler` owns versioning: every reconcile
attempt increments a single atomic counter, guaranteeing versions are
strictly increasing even across concurrent triggers (registry mutation
webhook in a later release, or the periodic ticker in this one).

One Envoy node ID (`demo-envoy`) is used in this release — multi-node
snapshot fan-out (different Envoys seeing different node-scoped resources)
is real go-control-plane capability but not yet wired to per-service
audiences; that's future work once node/cluster identity has an actual use
(e.g., per-service sidecar vs. per-team fleet).

## Configuration generation is deterministic

Listener ports are assigned by sorted service name (`basePort + index`), not
insertion order — the same registry contents always produce the same
snapshot regardless of registration order. This was verified directly
(`TestBuildSnapshotDeterministicPortAssignment`).

## Trade-offs

- No incremental (delta) xDS — every reconcile ships a full snapshot.
  Acceptable at this scale; delta xDS would matter at hundreds of services,
  which is v0.9.0's concern, not v0.4.0's.
- The reconciler polls the registry every `ReconcileInterval` (2s default)
  rather than reacting to registry mutation callbacks. Real distributed
  control planes do both; the periodic loop keeps this release simple while
  still delivering sub-2s propagation, measured directly in the v0.4.0
  smoke test (`scripts/xds_smoke_test.sh`).

## Consequences

Verified live (not simulated) via Docker: registering a backend causes
Envoy to add a cluster, endpoint, listener, and route with **zero restart**
(`cds: add 1 cluster(s)`, `lds: add/update listener`); deregistering it
removes the listener entirely once no endpoints remain. See
`scripts/xds_smoke_test.sh` for the automated, repeatable version of this
proof.
