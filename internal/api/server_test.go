package api

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
)

func testServer(t *testing.T, addr string) (*Server, *AtomicReadiness) {
	t.Helper()
	cfg := config.Default()
	cfg.HTTPAddr = addr
	metricsReg := metrics.New()
	readiness := &AtomicReadiness{}
	logger := logging.New("error")
	reg := registry.New()
	return New(cfg, logger, metricsReg, readiness, reg), readiness
}

func TestHealthzAlwaysOK(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
}

func TestReadyzReflectsReadinessState(t *testing.T) {
	s, readiness := testServer(t, ":0")

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusServiceUnavailable {
		t.Fatalf("status = %d, want 503 before ready", rr.Code)
	}

	readiness.SetReady(true)
	rr = httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200 after ready", rr.Code)
	}
}

func TestMetricsEndpointServesPrometheusFormat(t *testing.T) {
	s, _ := testServer(t, ":0")

	// Generate at least one recorded metric before scraping.
	warmup := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(warmup, httptest.NewRequest(http.MethodGet, "/healthz", nil))

	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", rr.Code)
	}
	if rr.Body.Len() == 0 {
		t.Fatal("expected non-empty metrics body")
	}
}

func TestCorrelationIDPropagatesToResponse(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	req.Header.Set("X-Correlation-ID", "test-corr-id")
	s.httpServer.Handler.ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Correlation-ID"); got != "test-corr-id" {
		t.Fatalf("X-Correlation-ID = %q, want test-corr-id", got)
	}
}

func TestCorrelationIDGeneratedWhenAbsent(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	s.httpServer.Handler.ServeHTTP(rr, req)
	if got := rr.Header().Get("X-Correlation-ID"); got == "" {
		t.Fatal("expected auto-generated correlation ID")
	}
}

func TestConcurrentRequests(t *testing.T) {
	s, readiness := testServer(t, ":0")
	readiness.SetReady(true)

	var wg sync.WaitGroup
	for i := 0; i < 50; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			rr := httptest.NewRecorder()
			req := httptest.NewRequest(http.MethodGet, "/readyz", nil)
			s.httpServer.Handler.ServeHTTP(rr, req)
			if rr.Code != http.StatusOK {
				t.Errorf("status = %d, want 200", rr.Code)
			}
		}()
	}
	wg.Wait()
}

func TestGracefulShutdown(t *testing.T) {
	s, _ := testServer(t, "127.0.0.1:0")

	errCh := make(chan error, 1)
	go func() { errCh <- s.ListenAndServe() }()

	// Give the listener a moment to bind.
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		t.Fatalf("Shutdown returned error: %v", err)
	}

	select {
	case err := <-errCh:
		if err != nil && err != http.ErrServerClosed {
			t.Fatalf("ListenAndServe returned unexpected error: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("server did not stop after Shutdown")
	}
}
