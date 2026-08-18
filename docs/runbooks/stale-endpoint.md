# Runbook: Stale Endpoint

## Symptom

An instance is still listed by `GET /v1/services/{name}` but traffic is no
longer reaching it, or conversely, `GET
/v1/services/{name}/instances?healthy=true` shows fewer instances than
expected.

## Definition of "stale" in this system

An instance is stale when `now - LastHeartbeat > CP_STALE_AFTER` (default
15s). Staleness is evaluated in two places, deliberately:

1. **`registry.HealthyInstances`** — a read-time filter used anywhere the
   current healthy set is needed without mutating state.
2. **`registry.SweepStale`**, called every reconcile tick — a **write**
   that flips `Healthy` from `true` to `false` once an instance crosses the
   threshold, so the transition is visible via `GET /v1/services/{name}`
   (not just implied by absence from the `?healthy=true` filter).

Both use the same `CP_STALE_AFTER` value, so they never disagree about
whether an instance is stale at a given point in time.

## Why heartbeating matters

This registry does **not** infer liveness on its own — it only reacts to
the absence of `POST /v1/services/{name}/instances/{id}/heartbeat` calls.
If a backend's heartbeat client stops running (but the backend process
itself is still up and serving traffic on the wire), the mesh will still
mark it unhealthy and stop routing to it after `CP_STALE_AFTER`. This is
intentional — the registry trusts the heartbeat signal, not passive
assumptions about process liveness. If a deployment integrates this mesh,
the backend's heartbeat loop is as load-bearing as the backend itself.

## How to diagnose

```
curl localhost:8080/v1/services/{name}
# check each instance's LastHeartbeat vs. current time, and Healthy
```

- If `Healthy: false` and `LastHeartbeat` is old: the heartbeat client
  stopped. Check the backend's heartbeat loop/process.
- If `Healthy: true` but traffic still isn't reaching the instance: check
  Envoy's own health checking (if configured) and `/clusters` admin
  endpoint — Envoy-level active health checks are a separate mechanism from
  registry heartbeat staleness (see `deployments/envoy/*.yaml` for any
  `health_checks` blocks).

## Remediation

- Resume/restart the backend's heartbeat client — `Healthy` flips back to
  `true` on the next successful heartbeat call, and the instance rejoins
  EDS on the next reconcile tick (sub-`CP_RECONCILE_INTERVAL` latency).
- If an instance should be permanently removed (not just recovering), call
  `DELETE /v1/services/{name}/instances/{id}` explicitly rather than
  waiting for staleness — deregistration is immediate, staleness is a
  fallback safety net for the case where a clean deregister call never
  happens (crash, network partition).
