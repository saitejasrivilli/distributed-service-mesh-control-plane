# Scalability

## What was actually measured (v0.9.0)

All numbers below are in `test/benchmark/results/v0.9.0_scale.json` and
reproducible via the commands listed there. Environment: Apple M2, macOS,
local machine — not cloud infrastructure, and explicitly labeled as such.

| Scenario | Result |
|---|---|
| Snapshot build, 10 services / 100 endpoints | 84.5µs/op, 1,510 allocs/op |
| Snapshot build, 100 services / 1000 endpoints | 834.7µs/op, 14,590 allocs/op |
| Propagation to 10 concurrent real gRPC ADS clients | 202.5ms max, 202.4ms avg |
| Propagation to 25 concurrent real gRPC ADS clients | 192.9ms max, 192.4ms avg |
| Propagation to 50 concurrent real gRPC ADS clients | 185.6ms max, 184.3ms avg |
| Churn: 100 services/1000 endpoints, 2s continuous register+heartbeat+deregister | 4,577,885 operations, 0 reconciliation failures |

## What the propagation-latency number actually shows

Latency is **flat, not increasing**, from 10 to 50 concurrently-connected
clients (185-203ms range) — meaning it's dominated by the test's 200ms
reconcile-tick interval, not by fan-out cost. This is the correct way to
read that number: it demonstrates that *this control plane's* client
fan-out is cheap at this scale, not that end-to-end latency is bounded at
~200ms in general (that bound comes from `CP_RECONCILE_INTERVAL`, which is
operator-configurable).

## What was honestly not measured

- 100 real Envoy proxy binaries (as opposed to real gRPC ADS clients
  simulating Envoy's protocol behavior) — not run due to local machine
  resource constraints. The gRPC clients exercise the identical wire
  protocol Envoy uses, so the control-plane-side measurement is real, but
  Envoy's own resource usage at that scale is not represented here.
- Sustained CPU utilization under prolonged load (the churn test ran for
  2 seconds; a longer-duration CPU profile via `pprof` was not captured
  for this release).

## Where the ceiling likely is

Given `BuildSnapshot`'s near-linear scaling with endpoint count (observed:
~10x endpoints -> ~10x time, comparing the 100/1000 vs 100/100 benchmark
cases) and full-rebuild-every-tick design (ADR-003), the practical ceiling
for this architecture is somewhere in the low-thousands of services before
either the reconcile interval needs lengthening or the full-rebuild model
needs replacing with true incremental (delta xDS) updates — a documented,
anticipated future direction, not encountered as an actual bottleneck at
this project's tested scale.
