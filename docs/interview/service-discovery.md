# Service Discovery

## Registry model

`internal/registry.InMemory`: a mutex-guarded map keyed by
`namespace + "/" + serviceName -> instanceID -> Instance`. Idempotent
registration (re-registering the same instance ID updates, doesn't
duplicate), namespace isolation, deterministic (instance-ID-sorted)
listings.

## Health model

Heartbeat-driven, not passive-liveness-driven: an instance is discoverable
only if `Healthy == true` AND its last heartbeat is within `staleAfter`.
`SweepStale` (v0.6.0) persists the `Healthy` transition rather than only
filtering reads, so `GET /v1/services/{name}` shows the true state, not
just an implied one. See ADR-008.

## Two discovery sources, one API

Manual registration (`curl POST /v1/services`, used by the Docker Compose
demos in v0.1.0-v0.6.0) and Kubernetes-bridged discovery
(`cmd/k8s-watcher`, v0.7.0) both go through the exact same HTTP API and
health model — Kubernetes is "just another registrant," not a special
case. See ADR-009.

## Why polling for Kubernetes, not the watch API

`k8s-watcher` polls `Endpoints` every 2s rather than using client-go's
informer/watch machinery. At single-digit-to-low-hundreds replica counts, a
2s poll is indistinguishable in practice from a push-based watch, and a
poll loop is much simpler to reason about and test. Moving to watch-based
discovery is a clean, well-understood upgrade path if this needed to track
much larger fleets with lower latency — see ADR-009's trade-offs section.

## Why not distributed consensus for the registry itself

A single in-memory registry is correct for a single control-plane process.
Multi-control-plane HA would need either a shared persistent store (etcd/
Consul-style) or a replication protocol between registries — deliberately
out of scope; see ADR-011's failure-handling table for the honest
single-point-of-failure trade-off this implies.
