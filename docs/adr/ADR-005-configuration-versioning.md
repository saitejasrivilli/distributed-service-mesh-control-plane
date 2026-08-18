# ADR-005: Configuration Versioning

## Context

Envoy's xDS protocol requires every snapshot to carry a version string per
resource type, used for ACK/NACK bookkeeping and to detect stale config.
The control plane needs a versioning scheme that's simple, monotonic, and
never reused.

## Decision

`internal/reconciliation.Reconciler` owns a single `atomic.Uint64` counter
(`r.version`), incremented once per `Reconcile` call regardless of whether
registry state actually changed. The version string handed to
`xds.BuildSnapshot` is `fmt.Sprintf("v%d", version)` — human-readable in
logs (`published xds snapshot version=42`) and in the
`controlplane_config_version` Prometheus gauge (v0.8.0).

All four resource types (CDS/EDS/LDS/RDS) in a snapshot share the same
version string — `cachev3.NewSnapshot` takes one version per call, applied
uniformly. This was a deliberate simplification: independently versioning
each resource type would let a client ACK CDS at v41 while still running
LDS at v39, which is valid xDS but adds a debugging dimension ("which
resource type is stale?") this project's scale doesn't need.

## Trade-offs

- Incrementing on every tick (not just on actual change) means the version
  number climbs even when nothing changed — intentional, since "did
  anything change" would require a diff this project doesn't compute (see
  ADR-003). The version number is a monotonic clock, not a change counter.
- Shared versioning across CDS/EDS/LDS/RDS trades fine-grained visibility
  for simplicity — acceptable while all four are always rebuilt together
  in one `BuildSnapshot` call.

## Consequences

`snap.Consistent()` (see ADR-004) is checked before every publish, so an
inconsistent snapshot never gets a version number handed to Envoy in the
first place — invalid config and version advancement are mutually
exclusive by construction, not by convention.
