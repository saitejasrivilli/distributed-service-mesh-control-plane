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
	Gatherer prometheus.Gatherer

	// HTTP management API.
	HTTPRequestsTotal   *prometheus.CounterVec
	HTTPRequestDuration *prometheus.HistogramVec

	// Control plane / xDS.
	ServicesTotal              prometheus.Gauge
	EndpointsTotal             prometheus.Gauge
	EnvoyConnectionsTotal      prometheus.Gauge
	XDSUpdatesTotal            prometheus.Counter
	XDSUpdateFailuresTotal     prometheus.Counter
	ConfigVersion              prometheus.Gauge
	ReconciliationAttempts     prometheus.Counter
	ReconciliationFailures     prometheus.Counter
	ReconciliationDuration     prometheus.Histogram
	StaleInstancesTransitioned prometheus.Counter
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
		ServicesTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "controlplane",
			Name:      "services_total",
			Help:      "Number of distinct services currently in the registry.",
		}),
		EndpointsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "controlplane",
			Name:      "endpoints_total",
			Help:      "Number of registered instances across all services.",
		}),
		EnvoyConnectionsTotal: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "controlplane",
			Name:      "envoy_connections_total",
			Help:      "Number of currently open xDS streams from Envoy.",
		}),
		XDSUpdatesTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "xds_updates_total",
			Help:      "Total number of xDS snapshots successfully published.",
		}),
		XDSUpdateFailuresTotal: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "xds_update_failures_total",
			Help:      "Total number of xDS snapshot build or publish failures.",
		}),
		ConfigVersion: factory.NewGauge(prometheus.GaugeOpts{
			Namespace: "controlplane",
			Name:      "config_version",
			Help:      "Monotonically increasing version of the last successfully published snapshot.",
		}),
		ReconciliationAttempts: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "reconciliation_attempts_total",
			Help:      "Total number of reconciliation attempts.",
		}),
		ReconciliationFailures: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "reconciliation_failures_total",
			Help:      "Total number of failed reconciliation attempts.",
		}),
		ReconciliationDuration: factory.NewHistogram(prometheus.HistogramOpts{
			Namespace: "controlplane",
			Name:      "reconciliation_duration_seconds",
			Help:      "Wall-clock time to sweep stale instances, build, and publish one xDS snapshot.",
			Buckets:   prometheus.DefBuckets,
		}),
		StaleInstancesTransitioned: factory.NewCounter(prometheus.CounterOpts{
			Namespace: "controlplane",
			Name:      "stale_instances_transitioned_total",
			Help:      "Total number of instances transitioned from healthy to unhealthy due to missed heartbeats.",
		}),
	}
}
