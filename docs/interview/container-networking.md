# Container Networking

## Sidecar model

Envoy runs as a sidecar in front of each backend (v0.3.0): the application
container implements zero routing logic — Envoy owns cluster selection,
health checking, retries, and traffic splitting. Verified directly:
`cmd/demo-service` (the backend) is a ~40-line HTTP server with no
knowledge of the mesh at all.

## Static -> dynamic progression

v0.3.0 deliberately starts with **static** Envoy config (hardcoded
clusters/listeners in `deployments/envoy/envoy-a.yaml`) to prove basic
connectivity (client -> Envoy -> backend, and Envoy-to-Envoy chaining)
before any control-plane involvement. Only once that's verified does
v0.4.0 introduce dynamic ADS-based config. This ordering matters: it means
any bug found in later releases is attributable to the xDS/control-plane
layer, not to basic Envoy/Docker networking.

## Kubernetes networking (v0.7.0)

Pods communicate over the cluster's pod network (`kind`'s default CNI in
this project's testing); Services provide stable DNS names
(`control-plane`, `backend-a`) that both the Envoy ADS bootstrap and
`k8s-watcher` depend on instead of any hardcoded IP. `cmd/k8s-watcher`
converts Kubernetes' own endpoint tracking into the same registry API the
Docker Compose demos use, so pod IP churn from scaling/rescheduling flows
through to Envoy automatically — proven live via 3->5->2 replica scaling
in `scripts/k8s_smoke_test.sh`.

## Container-network traffic flow, end to end

```
client -> Envoy (data plane, dynamic CDS/EDS/LDS/RDS)
       -> backend pod (selected by Envoy's load balancer, from EDS)
```

with the control plane out of the request path entirely — it only ever
configures Envoy, never proxies traffic itself. This is the same
control-plane/data-plane separation production service meshes rely on for
locality: adding a control-plane replica or restarting it never blocks
in-flight request traffic (Envoy keeps its last snapshot — see
`docs/runbooks/control-plane-failure.md`).
