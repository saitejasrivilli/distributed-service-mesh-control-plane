// Package reconciliation drives the desired-state (registry) -> versioned
// xDS snapshot -> Envoy control loop, including health-state transitions for
// stale instances.
package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"math/rand"
	"sync"
	"sync/atomic"
	"time"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/xds"
)

// NodeID is the Envoy node ID every snapshot is published under. Multi-node
// snapshot fan-out is future work; every connected Envoy uses this ID today.
const NodeID = "demo-envoy"

// maxBackoff caps the exponential backoff applied after consecutive
// reconcile failures, so a persistent failure never delays recovery by more
// than this long once the underlying issue is fixed.
const maxBackoff = 30 * time.Second

// Reconciler periodically (and on-demand) sweeps stale instances, rebuilds
// an xDS snapshot from registry state, and publishes it to the snapshot
// cache. Invalid snapshots are never published: on build or consistency
// error, the previous snapshot remains authoritative and the caller backs
// off exponentially (with jitter) before retrying.
type Reconciler struct {
	reg        registry.Registry
	routes     *routing.Store
	cache      cachev3.SnapshotCache
	logger     *slog.Logger
	metrics    *metrics.Registry
	staleAfter time.Duration
	version    atomic.Uint64

	attempts atomic.Uint64
	failures atomic.Uint64

	mu           sync.RWMutex
	lastSnapshot *cachev3.Snapshot
}

// New constructs a Reconciler. staleAfter bounds how long an instance may go
// without a heartbeat before SweepStale marks it unhealthy; zero disables
// sweeping. metricsReg may be nil to disable metrics recording. Call Run to
// start the periodic loop, and Reconcile to trigger an immediate rebuild
// (e.g. after a registry mutation).
func New(reg registry.Registry, routes *routing.Store, cache cachev3.SnapshotCache, logger *slog.Logger, staleAfter time.Duration, metricsReg *metrics.Registry) *Reconciler {
	return &Reconciler{reg: reg, routes: routes, cache: cache, logger: logger, staleAfter: staleAfter, metrics: metricsReg}
}

// Reconcile sweeps stale instances, rebuilds the snapshot from current
// registry state, and publishes it if valid.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	start := time.Now()
	r.attempts.Add(1)
	if r.metrics != nil {
		r.metrics.ReconciliationAttempts.Inc()
		defer func() { r.metrics.ReconciliationDuration.Observe(time.Since(start).Seconds()) }()
	}

	if transitioned := r.reg.SweepStale(r.staleAfter); len(transitioned) > 0 {
		if r.metrics != nil {
			r.metrics.StaleInstancesTransitioned.Add(float64(len(transitioned)))
		}
		for _, inst := range transitioned {
			r.logger.Warn("instance marked unhealthy: missed heartbeat deadline",
				"service", inst.ServiceName, "instance", inst.InstanceID, "last_heartbeat", inst.LastHeartbeat)
		}
	}

	version := r.version.Add(1)
	snap, err := xds.BuildSnapshot(r.reg, r.routes, fmt.Sprintf("v%d", version))
	if err != nil {
		r.failures.Add(1)
		if r.metrics != nil {
			r.metrics.ReconciliationFailures.Inc()
			r.metrics.XDSUpdateFailuresTotal.Inc()
		}
		r.logger.Error("snapshot build failed, keeping previous snapshot", "error", err)
		return err
	}

	if err := r.cache.SetSnapshot(ctx, NodeID, snap); err != nil {
		r.failures.Add(1)
		if r.metrics != nil {
			r.metrics.ReconciliationFailures.Inc()
			r.metrics.XDSUpdateFailuresTotal.Inc()
		}
		r.logger.Error("snapshot publish failed", "error", err)
		return err
	}

	r.mu.Lock()
	r.lastSnapshot = snap
	r.mu.Unlock()

	if r.metrics != nil {
		r.metrics.XDSUpdatesTotal.Inc()
		r.metrics.ConfigVersion.Set(float64(version))
		r.metrics.ServicesTotal.Set(float64(len(r.reg.ListServices(xds.Namespace))))
		r.metrics.EndpointsTotal.Set(float64(countEndpoints(r.reg)))
	}

	r.logger.Info("published xds snapshot", "version", version)
	return nil
}

func countEndpoints(reg registry.Registry) int {
	total := 0
	for _, svc := range reg.ListServices(xds.Namespace) {
		instances, err := reg.GetService(xds.Namespace, svc)
		if err != nil {
			continue
		}
		total += len(instances)
	}
	return total
}

// LastSnapshot returns the most recently published snapshot, or nil if none
// has been published yet. Used by debug/observability endpoints.
func (r *Reconciler) LastSnapshot() *cachev3.Snapshot {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.lastSnapshot
}

// Run reconciles once immediately, then every interval until ctx is done.
// Consecutive failures trigger exponential backoff with jitter (capped at
// maxBackoff) on top of the normal interval; a success resets the backoff.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	consecutiveFailures := 0
	for {
		if err := r.Reconcile(ctx); err != nil {
			consecutiveFailures++
		} else {
			consecutiveFailures = 0
		}

		wait := interval
		if consecutiveFailures > 0 {
			wait += backoff(consecutiveFailures)
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
	}
}

// backoff returns 2^attempt seconds (capped at maxBackoff) plus up to 20%
// jitter, so many failing reconcilers don't retry in lockstep.
func backoff(attempt int) time.Duration {
	base := time.Duration(1<<uint(attempt)) * time.Second
	if base > maxBackoff {
		base = maxBackoff
	}
	jitter := time.Duration(rand.Int63n(int64(base) / 5)) // #nosec G404 -- jitter only, not security-sensitive
	return base + jitter
}

// Attempts returns the total number of reconciliation attempts.
func (r *Reconciler) Attempts() uint64 { return r.attempts.Load() }

// Failures returns the total number of failed reconciliation attempts.
func (r *Reconciler) Failures() uint64 { return r.failures.Load() }
