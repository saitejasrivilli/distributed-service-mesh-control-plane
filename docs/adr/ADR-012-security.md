# ADR-012: Security

## Context

This project's management API (`internal/api`) and xDS gRPC server
(`internal/xds`) have **no authentication or authorization** as of v1.0.0.
This ADR names that honestly rather than implying a security posture that
doesn't exist.

## Current state

- `POST /v1/services`, `PUT /v1/routes/{service}`, `GET
  /v1/debug/*`, and every other management endpoint are open to any client
  that can reach the control plane's HTTP port. There is no API key, mTLS,
  or RBAC layer at the HTTP level.
- The xDS gRPC server accepts any Envoy connection with no node
  authentication beyond the node ID string Envoy self-reports (which is
  not verified against anything).
- Kubernetes RBAC (`deployments/kubernetes/k8s-watcher.yaml`) is the one
  place least-privilege actually exists in this project: the watcher's
  ServiceAccount is scoped to `get/list/watch` on `endpoints` only, nothing
  else.
- No secrets are used or stored anywhere in this project — no credentials
  in configs, no API keys, nothing to leak.

## Why this is acceptable for this project's scope

This is a portfolio/demonstration project run locally or in a demo
Kubernetes cluster, not a production deployment accepting untrusted
traffic. Adding real authn/authz (mTLS between Envoy and the control plane,
API-key or OIDC-gated management endpoints) is legitimate, well-understood
future work — but doing it superficially (e.g., a hardcoded shared secret)
would create a false sense of security worse than the current honest gap.

## What a production hardening pass would add

1. mTLS between Envoy and the control plane's xDS server (Envoy already
   supports this natively via `transport_socket` config).
2. An API-key or OIDC middleware in front of `internal/api`'s mutating
   endpoints (`POST`/`PUT`/`DELETE`), leaving `GET /healthz`, `/readyz`,
   `/metrics` open for infra tooling as today.
3. Network-level isolation (the xDS and management ports should never be
   internet-reachable regardless of app-level auth) — this is already true
   in the Kubernetes manifests (`ClusterIP` services, no `Ingress`/
   `LoadBalancer` exposing them) but worth stating as a requirement, not an
   accident.

## Consequences

Do not deploy this control plane to accept traffic from untrusted networks
without implementing the above. `SECURITY.md` at the repo root points here
for anyone evaluating this project for anything beyond local
demonstration/portfolio use.
