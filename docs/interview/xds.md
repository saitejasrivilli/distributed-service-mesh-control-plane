# xDS (CDS/EDS/LDS/RDS)

## What each resource type does here

- **CDS** (`buildCluster`): one Envoy `Cluster` per service, or per
  `service::version` when a traffic split is configured. Discovery type is
  always `EDS` (never `STATIC` or `STRICT_DNS` for backends) so endpoint
  membership is fully dynamic.
- **EDS** (`buildClusterLoadAssignment`): the healthy (heartbeat-fresh)
  instances for a cluster, filtered by `Instance.Version` when a split is
  active.
- **LDS** (`buildListener`): one listener per service, on a deterministically
  assigned port (`basePort + sorted-index`), using RDS (not inline routes)
  for its route config so route changes don't require a listener update.
- **RDS** (`buildRouteConfiguration`): one route config per service,
  either a single-cluster route (no split configured) or a
  `weighted_clusters` route (split configured), carrying retry policy and
  timeout from the traffic-management spec.

## Why go-control-plane's SnapshotCache

Implementing the xDS wire protocol by hand (SotW vs. delta, version/nonce
bookkeeping, ADS multiplexing across resource types on one stream) is a
solved, well-tested problem in `envoyproxy/go-control-plane`. Using it
means this project's code is entirely about *what* config to generate, not
*how* to serve it — see ADR-004.

## Versioning

One version string per snapshot, shared across all four resource types
(ADR-005) — a monotonic counter, not a change-hash. Advances on every
reconcile tick regardless of whether anything changed.

## Consistency guarantee

`cachev3.Snapshot.Consistent()` is checked before every publish. This
project has never (as of v1.0.0) produced an inconsistent snapshot in
testing, but the check exists specifically so a *future* bug in
`BuildSnapshot` fails loud (falls back to the previous good snapshot) 
rather than silently pushing broken config to Envoy.

## Scale characteristics (measured, v0.9.0)

Building a snapshot for 100 services / 1000 endpoints: ~0.83ms, 14,590
allocations. Propagating a snapshot update to 10-50 concurrently connected
ADS clients: 185-203ms end-to-end in testing, a number dominated by the
test's reconcile-tick interval rather than client count — fan-out itself
is cheap at this scale.
