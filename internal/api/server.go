// Package api implements the control plane's HTTP management API.
package api

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"log/slog"
	"net/http"
	"sync/atomic"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
)

// ReadinessChecker reports whether the control plane is ready to serve traffic.
type ReadinessChecker interface {
	Ready() bool
}

// AtomicReadiness is a ReadinessChecker backed by an atomic flag, safe for
// concurrent use. It defaults to not-ready until explicitly set.
type AtomicReadiness struct {
	ready atomic.Bool
}

// SetReady flips the readiness flag.
func (a *AtomicReadiness) SetReady(ready bool) { a.ready.Store(ready) }

// Ready implements ReadinessChecker.
func (a *AtomicReadiness) Ready() bool { return a.ready.Load() }

// Server is the control plane's HTTP management API.
type Server struct {
	httpServer *http.Server
	logger     *slog.Logger
	readiness  ReadinessChecker
}

// New constructs a Server wired with health, readiness, and metrics endpoints.
func New(cfg config.Config, logger *slog.Logger, metricsReg *metrics.Registry, readiness ReadinessChecker) *Server {
	mux := http.NewServeMux()
	s := &Server{logger: logger, readiness: readiness}

	mux.HandleFunc("GET /healthz", s.handleHealthz)
	mux.HandleFunc("GET /readyz", s.handleReadyz)
	mux.Handle("GET /metrics", promhttp.HandlerFor(metricsReg.Gatherer, promhttp.HandlerOpts{}))

	handler := withCorrelationID(withMetrics(mux, metricsReg))

	s.httpServer = &http.Server{
		Addr:         cfg.HTTPAddr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}
	return s
}

// Addr returns the address the server is configured to listen on.
func (s *Server) Addr() string { return s.httpServer.Addr }

// ListenAndServe starts the HTTP server. It blocks until the server stops and
// returns http.ErrServerClosed on a clean shutdown.
func (s *Server) ListenAndServe() error {
	return s.httpServer.ListenAndServe()
}

// Shutdown gracefully drains in-flight requests within ctx's deadline.
func (s *Server) Shutdown(ctx context.Context) error {
	return s.httpServer.Shutdown(ctx)
}

func (s *Server) handleHealthz(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context(), s.logger)
	log.Debug("healthz check")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ok"}`))
}

func (s *Server) handleReadyz(w http.ResponseWriter, r *http.Request) {
	log := logging.FromContext(r.Context(), s.logger)
	if !s.readiness.Ready() {
		log.Debug("readyz check: not ready")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"status":"not ready"}`))
		return
	}
	log.Debug("readyz check: ready")
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte(`{"status":"ready"}`))
}

func withCorrelationID(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		id := r.Header.Get("X-Correlation-ID")
		if id == "" {
			id = newCorrelationID()
		}
		ctx := logging.WithCorrelationID(r.Context(), id)
		w.Header().Set("X-Correlation-ID", id)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

func withMetrics(next http.Handler, reg *metrics.Registry) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		duration := time.Since(start).Seconds()
		path := r.URL.Path
		reg.HTTPRequestDuration.WithLabelValues(path, r.Method).Observe(duration)
		reg.HTTPRequestsTotal.WithLabelValues(path, r.Method, http.StatusText(rec.status)).Inc()
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (r *statusRecorder) WriteHeader(status int) {
	r.status = status
	r.ResponseWriter.WriteHeader(status)
}

func newCorrelationID() string {
	buf := make([]byte, 8)
	if _, err := rand.Read(buf); err != nil {
		return "unknown"
	}
	return hex.EncodeToString(buf)
}

// ErrNotReady is returned by callers awaiting readiness that never arrives.
var ErrNotReady = errors.New("control plane not ready")
