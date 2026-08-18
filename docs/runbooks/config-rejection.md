# Runbook: Configuration Rejection

## Core invariant

**Invalid configuration must never be published to Envoy.** Two independent
gates enforce this, at two different layers:

1. **Route spec validation** (`internal/routing.Spec.Validate`): a `PUT
   /v1/routes/{service}` call with weights that don't sum to 100, duplicate
   version labels, or a `retry_on` without `num_retries` is rejected with
   `400 Bad Request` and never reaches the routing store — so a bad spec
   can never be picked up by the reconciler in the first place.
2. **Snapshot consistency check** (`xds.BuildSnapshot` calls
   `snap.Consistent()` before returning): if a generated snapshot somehow
   references a cluster that has no matching route, or vice versa, the
   error propagates up through `Reconciler.Reconcile`, which logs it and
   **returns without calling `cache.SetSnapshot`** — the previous
   (last-known-good) snapshot remains authoritative and continues being
   served to connected Envoys.

## Symptom: a route/config update doesn't seem to take effect

Check the control-plane logs for a snapshot build or publish failure:

```
docker logs <control-plane-container> | grep -i "snapshot build failed\|snapshot publish failed"
```

If present, the previous valid snapshot is still being served — this is
correct, not a bug. Compare `r.Attempts()` vs. `r.Failures()`
(exposed for future observability work in v0.8.0; today, visible via logs).

## Symptom: `PUT /v1/routes/{service}` returns 400

The response body's `error` field states which validation failed. Common
causes:

- Weights across all splits don't sum to exactly 100.
- Two splits share the same `version` label.
- `retry_on` is set but `num_retries` is 0 or omitted.

Fix the request body and resubmit — there is no partial application; a
rejected `PUT` has zero effect on the currently-published route.

## Why this matters operationally

Because rejected configuration never reaches Envoy, a bad `PUT
/v1/routes/{service}` call is a **client-side no-op**, not an outage. This
was a deliberate design choice (see ADR-007) so operators can safely
experiment with traffic-management changes via the API without risking
Envoy accepting malformed config — worst case is "my change didn't apply,"
never "Envoy is now broken."
