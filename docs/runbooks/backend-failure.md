# Runbook: Backend Failure

## Symptom

Requests to a service through Envoy start returning errors (`503`, `no
healthy upstream`), or a subset of a service's instances stop receiving
traffic.

## What happens automatically

1. The backend stops sending heartbeats (crash, network partition, or
   process hang).
2. `internal/reconciliation.Reconciler.Reconcile` calls
   `registry.SweepStale` on every reconcile tick (default every
   `CP_RECONCILE_INTERVAL`, 2s). Once `now - LastHeartbeat >
   CP_STALE_AFTER` (default 15s), the instance's `Healthy` flag flips to
   `false` — this is a **persisted state transition**, not just a filter
   applied at read time.
3. The next snapshot build excludes the instance from that service's EDS
   `ClusterLoadAssignment` (see `internal/xds.BuildSnapshot` ->
   `HealthyInstances`).
4. Envoy receives the updated EDS resource over the existing ADS stream and
   stops routing to the dead instance — **no Envoy restart**.
5. If the backend recovers and resumes heartbeating (`POST
   /v1/services/{name}/instances/{id}/heartbeat`), `Healthy` flips back to
   `true` on the next heartbeat call, and the next reconcile tick adds it
   back to EDS.

## How to verify by hand

```
curl localhost:8080/v1/services/{name}                      # check Healthy field per instance
curl "localhost:8080/v1/services/{name}/instances?healthy=true"  # what Envoy currently sees
```

If a container-orchestrated deployment (Kubernetes) is in play, also check
`kubectl get pods` for `CrashLoopBackOff` / restart counts — the pod
scheduler's own liveness/readiness probes are a separate signal from the
mesh registry's heartbeat staleness.

## Manual remediation

- If the backend process is actually healthy but not heartbeating (bug in
  the heartbeat client), restart the client or send a manual heartbeat.
- If the backend is genuinely down, no control-plane action is needed —
  traffic is already routed away. Fix/restart the backend; it will
  re-register or resume heartbeating and rejoin automatically.
- If **all** instances of a service go unhealthy simultaneously, Envoy's
  cluster has zero healthy endpoints and every request to that service
  fails — this is expected: the mesh cannot route to instances that don't
  exist. Escalate as a service outage, not a mesh bug.

## Automated proof

`scripts/health_reconciliation_smoke_test.sh` reproduces this scenario
end-to-end against real Docker containers: registers a backend, lets it go
stale with no heartbeats, confirms Envoy stops routing to it, then sends a
heartbeat and confirms recovery — all without restarting Envoy or the
control plane.
