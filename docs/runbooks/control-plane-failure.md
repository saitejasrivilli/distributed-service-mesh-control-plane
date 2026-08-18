# Runbook: Control-Plane Failure

## Symptom

The control plane process is unreachable (`GET /healthz` fails), xDS
connections from Envoy are refused, or `docker ps` / `kubectl get pods`
shows the control-plane container/pod is down or crash-looping.

## What Envoy does when the control plane is unreachable

Envoy's ADS client keeps the **last known-good snapshot** and continues
serving traffic with it — Envoy does not blank its config just because the
xDS stream drops. This is inherent to Envoy's xDS client design, not
something this project implements. Confirm by checking Envoy's admin
`/clusters` and `/listeners` endpoints (`localhost:9901` in the demo
compose files) — they should still show the last-published config while
the control plane is down.

## Honest limitation: single point of failure

This project runs **one** control-plane process (single-node xDS). If it crashes:

- Existing Envoys keep working off their last snapshot (see above).
- **No new** service registrations, deregistrations, heartbeats, or route
  changes take effect until the control plane comes back — the management
  API is unreachable and there is no standby to fail over to.
- Any registration/heartbeat calls made while the control plane is down are
  simply lost (in-memory registry, not persisted — see ADR-002).

This is a deliberate, documented trade-off for this project's scope, not an
oversight: production service meshes run multiple control-plane replicas
behind a load balancer specifically to avoid this. Doing so here would
require either a shared/persistent registry backend or a gossip/replication
protocol between control-plane instances — out of scope until a later
release explicitly tackles it (a dedicated ADR for failure handling is planned as this project matures).

## Recovery

1. Restart the control-plane process/container
   (`docker compose restart control-plane` or the Kubernetes equivalent).
2. On restart, the in-memory registry starts empty — every backend must
   re-register. If backends already have a heartbeat/registration retry
   loop (recommended for production use of this mesh), they will
   self-heal without operator intervention.
3. Once services re-register, the reconciler's first `Reconcile` call (run
   immediately on `Reconciler.Run`) rebuilds and republishes a full
   snapshot, and Envoy's ADS stream reconnects automatically (Envoy retries
   its xDS connection on its own).

## How to verify recovery

```
curl localhost:8080/healthz
curl localhost:8080/readyz
curl localhost:8080/v1/services/{name}    # confirm backends re-registered
```
