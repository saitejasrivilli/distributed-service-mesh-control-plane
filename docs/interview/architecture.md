# Architecture

## Components

```
Management API (HTTP)          internal/api
       |
Control Plane                  internal/controlplane (orchestrator)
  Registry                     internal/registry
  Reconciler                   internal/reconciliation
  Config Builder                internal/xds (BuildSnapshot)
  xDS Server (gRPC ADS)         internal/xds (Server, ConnectionTracker)
       |
   CDS / EDS / LDS / RDS
       |
   Envoy (data plane)
       |
   Backend services (cmd/demo-service, or any registered instance)
```

## Data flow

Register/heartbeat/deregister (HTTP) -> registry state change -> next
reconcile tick sweeps stale instances -> `BuildSnapshot` reads registry +
routing store -> versioned snapshot -> `SnapshotCache.SetSnapshot` -> gRPC
ADS pushes to every connected Envoy -> Envoy applies CDS/EDS/LDS/RDS with
zero restart -> client traffic routes accordingly.

## Control flow

The reconciler (`internal/reconciliation.Reconciler.Run`) is the only
writer to the xDS snapshot cache. Nothing else calls `SetSnapshot`
directly — the HTTP API only ever mutates the registry/routing store, never
the snapshot. This single-writer design is why "invalid config never
reaches Envoy" is enforceable in one place (see ADR-004, ADR-007).

## Consistency model

Full-rebuild-every-tick, not incremental diffing (ADR-003). Registry reads
are strongly consistent within one mutex-guarded process (ADR-002); there
is no cross-process consistency story because there is exactly one
control-plane process (ADR-011).

## Concurrency model

Registry: `sync.RWMutex` guarding a plain map. Reconciler: single
goroutine running `Reconcile` on a ticker, with `Reconcile` itself callable
concurrently from tests without corrupting shared state (each call does
its own read-then-write under the registry's own locking). xDS server:
go-control-plane's own goroutine-per-stream model, unmodified.

## Platform paths

Two identical deployment paths converge on the same registry HTTP API: a
human/script calling `curl -X POST /v1/services` (Docker Compose demos,
v0.1.0-v0.6.0), or `cmd/k8s-watcher` calling the same API on behalf of
Kubernetes Endpoints (v0.7.0). Neither path is privileged over the other.
