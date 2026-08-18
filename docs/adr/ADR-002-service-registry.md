# ADR-002: Service Registry

## Context

Service discovery is one of the specialized gaps this project targets. The
control plane needs to know which instances of a service exist, where they
live, and whether they're healthy — the data every later xDS/EDS release
builds on.

## Problem

How should service instances be tracked so that (a) registration is safe
under concurrent access, (b) stale/dead instances don't silently keep
receiving traffic, and (c) a future persistent backend (etcd, Consul-style
storage) can replace the storage layer without changing callers?

## Options

1. **In-memory map guarded by a mutex, behind a `Registry` interface** —
   simplest correct option for this stage; trivially swappable later.
2. **Immediately reach for a distributed consensus store (etcd/Raft)** —
   solves a scaling problem this project doesn't have yet; adds an external
   dependency and operational surface with no current benefit.
3. **Passive discovery only (e.g., Kubernetes endpoint watch)** — arrives
   naturally in v0.7.0 (Kubernetes), but an explicit register/deregister/
   heartbeat API is still needed for the reconciliation model in v0.4.0+ and
   for demoing discovery independent of a specific platform.

## Decision

Option 1. `internal/registry.Registry` is an interface; `InMemory` is the
only implementation for now. Health is heartbeat-driven: an instance is
`HealthyInstances`-eligible only if its `Healthy` flag is set AND its last
heartbeat is within a caller-supplied `staleAfter` window — this prevents a
registered-but-abandoned instance from being served indefinitely even if
nothing ever explicitly deregisters it.

Namespace isolation is enforced by keying storage on `namespace + "/" +
serviceName`, not by separate registry instances — keeps the implementation
single-map-simple while still preventing cross-namespace leakage.

## Trade-offs

- No persistence: a control-plane restart loses all registrations. Acceptable
  now — every registered backend is expected to re-register/heartbeat shortly
  after a restart, and later releases (Kubernetes-native discovery) reduce
  reliance on manual registration entirely.
- No distributed consensus: this registry is correct for a single
  control-plane process. Multi-control-plane HA is out of scope until the
  project's failure-mode work explicitly calls for it (see ADR-011,
  planned).

## Consequences

`internal/api` depends on `registry.Registry` (the interface), not
`*registry.InMemory` directly, so v0.4.0's EDS snapshot builder can read from
the same interface without caring whether the backing store is in-memory or
persistent.
