package api

import (
	"net/http"
	"testing"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/xds"
)

func TestDebugServiceReturnsInstancesAndRoute(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"backend-a","instance_id":"i1"}`)
	doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":100}]}`)

	rr := doJSON(t, s, http.MethodGet, "/v1/debug/services/backend-a", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !contains(rr.Body.String(), `"instances"`) || !contains(rr.Body.String(), `"route"`) {
		t.Fatalf("body missing expected fields: %s", rr.Body.String())
	}
}

func TestDebugServiceUnknownReturns404(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodGet, "/v1/debug/services/nope", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDebugEnvoysReturnsCount(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodGet, "/v1/debug/envoys", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	if !contains(rr.Body.String(), `"count":0`) {
		t.Fatalf("expected count:0 with no connections, got %s", rr.Body.String())
	}
}

func TestDebugConfigReturns404BeforeAnySnapshot(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodGet, "/v1/debug/config/backend-a", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 before any snapshot published", rr.Code)
	}
}

func TestDebugConfigReflectsLastSnapshot(t *testing.T) {
	reg := registry.New()
	_ = reg.Register(regInstance("backend-a", "i1"))
	snap, err := xds.BuildSnapshot(reg, routing.NewStore(), "v7")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}

	cfg := config.Default()
	cfg.HTTPAddr = ":0"
	metricsReg := metrics.New()
	logger := logging.New("error")
	s := New(cfg, logger, metricsReg, &AtomicReadiness{}, reg, routing.NewStore(), &fakeReconciler{snapshot: snap}, &fakeEnvoyTracker{})

	rr := doJSON(t, s, http.MethodGet, "/v1/debug/config/backend-a", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	body := rr.Body.String()
	if !contains(body, `"config_version":"v7"`) {
		t.Errorf("expected config_version v7, got %s", body)
	}
	if !contains(body, `"has_listener":true`) {
		t.Errorf("expected has_listener true, got %s", body)
	}
}

func regInstance(service, id string) registry.Instance {
	return registry.Instance{ServiceName: service, Namespace: xds.Namespace, InstanceID: id, Address: "10.0.0.1", Port: 9000}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (func() bool {
		for i := 0; i+len(substr) <= len(s); i++ {
			if s[i:i+len(substr)] == substr {
				return true
			}
		}
		return false
	})()
}
