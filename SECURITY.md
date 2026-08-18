# Security

This is an independent portfolio/demonstration project. See
[`docs/adr/ADR-012-security.md`](docs/adr/ADR-012-security.md) for a full,
honest account of the current security posture.

**Summary: as of v1.0.0, there is no authentication or authorization on the
management API or the xDS gRPC server.** Do not expose this control plane
to an untrusted network. It is intended for local development, CI, and
demo Kubernetes clusters (e.g. `kind`), not for production traffic.

## Reporting a vulnerability

This is not a maintained production project. If you find an issue while
reviewing the code, please open a GitHub issue describing it — there is no
dedicated security contact or disclosure SLA.

## Scope

- No secrets are stored or used anywhere in this repository.
- Kubernetes RBAC for `cmd/k8s-watcher` is scoped to least privilege
  (`get/list/watch` on `endpoints` only) — see
  `deployments/kubernetes/k8s-watcher.yaml`.
- Dependency updates are not on an automated schedule; check `go.sum`
  against known CVEs before any non-demo use.
