package metrics

import "testing"

func TestNewRegistersAllCollectorsWithoutPanic(t *testing.T) {
	reg := New()
	if reg.Gatherer == nil {
		t.Fatal("expected non-nil Gatherer")
	}
	// Exercise every collector once so a duplicate-registration or nil
	// collector bug would surface immediately.
	reg.HTTPRequestsTotal.WithLabelValues("/x", "GET", "OK").Inc()
	reg.HTTPRequestDuration.WithLabelValues("/x", "GET").Observe(0.01)
	reg.ServicesTotal.Set(1)
	reg.EndpointsTotal.Set(1)
	reg.EnvoyConnectionsTotal.Set(1)
	reg.XDSUpdatesTotal.Inc()
	reg.XDSUpdateFailuresTotal.Inc()
	reg.ConfigVersion.Set(1)
	reg.ReconciliationAttempts.Inc()
	reg.ReconciliationFailures.Inc()
	reg.ReconciliationDuration.Observe(0.01)
	reg.StaleInstancesTransitioned.Inc()

	families, err := reg.Gatherer.Gather()
	if err != nil {
		t.Fatalf("Gather: %v", err)
	}
	if len(families) == 0 {
		t.Fatal("expected non-empty metric families after recording values")
	}
}

func TestNewCreatesIndependentRegistriesPerCall(t *testing.T) {
	// A prior bug used the global default registry, which panicked on the
	// second New() call in the same process (duplicate registration).
	_ = New()
	_ = New()
}
