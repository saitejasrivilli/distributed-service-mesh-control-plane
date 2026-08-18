# Observability

## Metrics (Prometheus)

Every collector is registered against a per-`ControlPlane`-instance
registry, never the process-global default (a real bug hit in v0.1.0's
tests otherwise — duplicate-registration panics when more than one
instance existed in a process). Coverage: HTTP request count/latency,
services/endpoints counts, Envoy connection count, xDS update/failure
counts, config version, reconciliation attempt/failure counts and
duration, stale-instance-transition count. All updated directly inside
`Reconciler.Reconcile` — metrics can't drift from the state they describe
because there's no separate update path.

## Structured logging

`slog`-based JSON logging throughout, with correlation IDs propagated via
context (`internal/logging`) and injected/echoed by HTTP middleware
(`X-Correlation-ID` header, auto-generated if absent).

## Debug endpoints

`GET /v1/debug/services/{name}` (registry + traffic spec),
`GET /v1/debug/envoys` (live connected streams + node IDs),
`GET /v1/debug/config/{service}` (reads the reconciler's last *published*
snapshot directly — deliberately not recomputed from current registry
state, since the two can legitimately differ for a few seconds around a
change, and that's exactly the gap this endpoint is for diagnosing).

## Grafana

Dashboard and Prometheus datasource are both file-provisioned
(`deployments/docker/grafana/provisioning/`) — `docker compose up` and the
dashboard is already there, no manual "add datasource" step. Verified
live: dashboard queries were checked against Envoy's actual
`/stats/prometheus` metric names (e.g.
`envoy_cluster_upstream_rq_xx{envoy_response_code_class="2"}`), not
assumed.

## Honest gaps

`reconciliation_duration_seconds` measures control-plane build+publish
time, not true Envoy-side xDS ACK round-trip latency (would require
Envoy-side instrumentation this project doesn't add). Debug endpoints have
no auth (see `docs/adr/ADR-012-security.md`).
