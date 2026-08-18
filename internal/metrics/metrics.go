// Package metrics defines Prometheus instrumentation for the control plane.
package metrics

import (
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

// Registry bundles the control-plane's Prometheus collectors along with the
// Gatherer they were registered against, so /metrics can serve exactly these
// collectors rather than the global default registry.
type Registry struct {
	Gatherer            prometheus.Gatherer
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec
}

// New creates its own registry, registers the control-plane's collectors
// against it, and returns both.
func New() *Registry {
	reg := prometheus.NewRegistry()
	factory := promauto.With(reg)
	return &Registry{
		Gatherer: reg,
		HTTPRequestsTotal: factory.NewCounterVec(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "http_requests_total",
			Help:      "Total HTTP requests handled by the management API.",
		}, []string{"path", "method", "status"}),
		HTTPRequestDuration: factory.NewHistogramVec(prometheus.HistogramOpts{
			Namespace: "controlplane",
			Name:      "http_request_duration_seconds",
			Help:      "HTTP request duration in seconds.",
			Buckets:   prometheus.DefBuckets,
		}, []string{"path", "method"}),
	}
}
