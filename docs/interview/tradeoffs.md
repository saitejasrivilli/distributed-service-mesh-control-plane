# Trade-offs

A consolidated list of the deliberate simplifications in this project, why
each was made, and what a production system would need to add. Each links
to the ADR with full reasoning.

| Trade-off | Why | What production needs | ADR |
|---|---|---|---|
| In-memory registry, no persistence | Simplicity; a persistent backend is real future work behind the same interface | Persistent/shared store (etcd, Consul-style) | ADR-002 |
| Single control-plane process, no HA | Multi-replica failover needs a consistency story (shared store or gossip) that's a project in itself | Multi-replica + shared registry, or gossip | ADR-011 |
| Full-rebuild-every-tick, not delta xDS | Correctness by construction — no partial-update drift bugs possible | Incremental/delta xDS at higher service counts | ADR-003, ADR-004 |
| Polling-based Kubernetes discovery (2s) | Simple to reason about/test; indistinguishable from watch-based at this scale | client-go informer/watch for lower latency at scale | ADR-009 |
| One shared version per snapshot (not per-resource-type) | Avoids a "which resource type is stale" debugging dimension not needed yet | Per-type versioning if CDS/EDS/LDS/RDS ever update independently | ADR-005 |
| No auth on management/xDS APIs | Portfolio/demo scope; a superficial auth layer would be worse than an honest gap | mTLS (Envoy<->control-plane), API-key/OIDC on mutating endpoints | ADR-012 |
| Heartbeat-based health only (no active health checks from control plane) | Envoy's own `health_checks` (data-plane layer) is complementary, not replaced | Keep both layers; this project demonstrates the control-plane half | ADR-008 |
| Weighted round-robin only (no session affinity, no header routing) | Nothing in this project's scope required them | Extend `routing.Spec` with matchers/affinity if a use case demands it | ADR-007 |

## The one trade-off worth leading with in an interview

**Centralized control plane vs. gossip-based discovery.** A centralized
xDS control plane (this project's model, and Istio/Linkerd/AWS App Mesh's
model) gives one consistent view of desired state and lets every other
design decision (full-rebuild-every-tick, single-writer snapshot
publishing, straightforward debug endpoints reading "the" last snapshot)
be simple *because* there's one source of truth. The cost is exactly the
single point of failure documented in ADR-011. Naming this trade-off
explicitly — rather than either ignoring it or over-engineering a
half-measure HA story — is itself the signal this project is meant to
send.
