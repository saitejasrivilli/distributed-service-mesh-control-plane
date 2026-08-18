// Package controlplane wires together config, logging, metrics, registry,
// xDS, and the API server into a single runnable control-plane process.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/api"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/reconciliation"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/xds"
)

// ControlPlane owns the lifecycle of the control-plane's HTTP and xDS surfaces.
type ControlPlane struct {
	cfg        config.Config
	logger     *slog.Logger
	server     *api.Server
	readiness  *api.AtomicReadiness
	xdsServer  *xds.Server
	reconciler *reconciliation.Reconciler
}

// New constructs a ControlPlane from cfg with its own dependencies injected.
func New(cfg config.Config) *ControlPlane {
	logger := logging.New(cfg.LogLevel)
	metricsReg := metrics.New()
	readiness := &api.AtomicReadiness{}
	svcRegistry := registry.New()
	routeStore := routing.NewStore()
	server := api.New(cfg, logger, metricsReg, readiness, svcRegistry, routeStore)

	snapshotCache := cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil)
	xdsServer := xds.NewServer(context.Background(), snapshotCache, nil, logger)
	reconciler := reconciliation.New(svcRegistry, routeStore, snapshotCache, logger, cfg.StaleAfter)

	return &ControlPlane{
		cfg:        cfg,
		logger:     logger,
		server:     server,
		readiness:  readiness,
		xdsServer:  xdsServer,
		reconciler: reconciler,
	}
}

// Run starts the control plane (HTTP API, xDS server, reconciliation loop)
// and blocks until ctx is canceled, then performs a graceful shutdown bounded
// by cfg.ShutdownTimeout.
func (c *ControlPlane) Run(ctx context.Context) error {
	reconcileCtx, stopReconcile := context.WithCancel(ctx)
	defer stopReconcile()
	go c.reconciler.Run(reconcileCtx, c.cfg.ReconcileInterval)

	httpErrCh := make(chan error, 1)
	go func() {
		c.logger.Info("control plane listening", "addr", c.server.Addr())
		c.readiness.SetReady(true)
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			httpErrCh <- err
			return
		}
		httpErrCh <- nil
	}()

	xdsErrCh := make(chan error, 1)
	go func() {
		if err := c.xdsServer.Serve(c.cfg.XDSAddr); err != nil {
			xdsErrCh <- err
			return
		}
		xdsErrCh <- nil
	}()

	select {
	case <-ctx.Done():
		c.logger.Info("shutdown signal received, draining connections")
		c.readiness.SetReady(false)
		stopReconcile()
		c.xdsServer.GracefulStop()

		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.cfg.ShutdownTimeout)
		defer cancel()
		if err := c.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		<-httpErrCh
		<-xdsErrCh
		return nil
	case err := <-httpErrCh:
		if err != nil {
			return fmt.Errorf("http server exited unexpectedly: %w", err)
		}
		return nil
	case err := <-xdsErrCh:
		if err != nil {
			return fmt.Errorf("xds server exited unexpectedly: %w", err)
		}
		return nil
	}
}
