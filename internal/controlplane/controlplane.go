// Package controlplane wires together config, logging, metrics, and the API
// server into a single runnable control-plane process.
package controlplane

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/api"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
)

// ControlPlane owns the lifecycle of the control-plane's HTTP surface.
type ControlPlane struct {
	cfg       config.Config
	logger    *slog.Logger
	server    *api.Server
	readiness *api.AtomicReadiness
}

// New constructs a ControlPlane from cfg with its own dependencies injected.
func New(cfg config.Config) *ControlPlane {
	logger := logging.New(cfg.LogLevel)
	reg := metrics.New()
	readiness := &api.AtomicReadiness{}
	server := api.New(cfg, logger, reg, readiness)
	return &ControlPlane{cfg: cfg, logger: logger, server: server, readiness: readiness}
}

// Run starts the control plane and blocks until ctx is canceled, then
// performs a graceful shutdown bounded by cfg.ShutdownTimeout.
func (c *ControlPlane) Run(ctx context.Context) error {
	errCh := make(chan error, 1)
	go func() {
		c.logger.Info("control plane listening", "addr", c.server.Addr())
		c.readiness.SetReady(true)
		if err := c.server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			errCh <- err
			return
		}
		errCh <- nil
	}()

	select {
	case <-ctx.Done():
		c.logger.Info("shutdown signal received, draining connections")
		c.readiness.SetReady(false)
		shutdownCtx, cancel := context.WithTimeout(context.Background(), c.cfg.ShutdownTimeout)
		defer cancel()
		if err := c.server.Shutdown(shutdownCtx); err != nil {
			return fmt.Errorf("graceful shutdown failed: %w", err)
		}
		<-errCh
		return nil
	case err := <-errCh:
		if err != nil {
			return fmt.Errorf("server exited unexpectedly: %w", err)
		}
		return nil
	}
}
