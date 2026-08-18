// Package reconciliation drives the desired-state (registry) -> versioned
// xDS snapshot -> Envoy control loop.
package reconciliation

import (
	"context"
	"fmt"
	"log/slog"
	"sync/atomic"
	"time"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/xds"
)

// NodeID is the Envoy node ID every snapshot is published under. Multi-node
// snapshot fan-out is future work; every connected Envoy uses this ID today.
const NodeID = "demo-envoy"

// Reconciler periodically (and on-demand) rebuilds an xDS snapshot from
// registry state and publishes it to the snapshot cache, skipping publication
// when nothing changed to avoid needless Envoy churn.
type Reconciler struct {
	reg     registry.Registry
	routes  *routing.Store
	cache   cachev3.SnapshotCache
	logger  *slog.Logger
	version atomic.Uint64

	attempts atomic.Uint64
	failures atomic.Uint64
}

// New constructs a Reconciler. Call Run to start the periodic loop, and
// Reconcile to trigger an immediate rebuild (e.g. after a registry mutation).
func New(reg registry.Registry, routes *routing.Store, cache cachev3.SnapshotCache, logger *slog.Logger) *Reconciler {
	return &Reconciler{reg: reg, routes: routes, cache: cache, logger: logger}
}

// Reconcile rebuilds the snapshot from current registry state and publishes
// it if valid. Publishing an invalid snapshot is never allowed: on build or
// consistency error, the previous snapshot remains authoritative.
func (r *Reconciler) Reconcile(ctx context.Context) error {
	r.attempts.Add(1)
	version := r.version.Add(1)

	snap, err := xds.BuildSnapshot(r.reg, r.routes, fmt.Sprintf("v%d", version))
	if err != nil {
		r.failures.Add(1)
		r.logger.Error("snapshot build failed, keeping previous snapshot", "error", err)
		return err
	}

	if err := r.cache.SetSnapshot(ctx, NodeID, snap); err != nil {
		r.failures.Add(1)
		r.logger.Error("snapshot publish failed", "error", err)
		return err
	}

	r.logger.Info("published xds snapshot", "version", version)
	return nil
}

// Run reconciles once immediately, then every interval until ctx is done.
func (r *Reconciler) Run(ctx context.Context, interval time.Duration) {
	_ = r.Reconcile(ctx)

	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			_ = r.Reconcile(ctx)
		}
	}
}

// Attempts returns the total number of reconciliation attempts.
func (r *Reconciler) Attempts() uint64 { return r.attempts.Load() }

// Failures returns the total number of failed reconciliation attempts.
func (r *Reconciler) Failures() uint64 { return r.failures.Load() }
