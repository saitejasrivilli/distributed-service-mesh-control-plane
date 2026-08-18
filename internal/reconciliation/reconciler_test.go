package reconciliation

import (
	"context"
	"testing"
	"time"

	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/metrics"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

func newTestReconciler() (*Reconciler, *registry.InMemory, cachev3.SnapshotCache) {
	reg := registry.New()
	routes := routing.NewStore()
	cache := cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil)
	r := New(reg, routes, cache, logging.New("error"), 15*time.Second, metrics.New())
	return r, reg, cache
}

func TestReconcilePublishesSnapshot(t *testing.T) {
	r, reg, cache := newTestReconciler()
	_ = reg.Register(registry.Instance{ServiceName: "backend-a", Namespace: "default", InstanceID: "i1", Address: "10.0.0.1", Port: 9000})

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	snap, err := cache.GetSnapshot(NodeID)
	if err != nil {
		t.Fatalf("GetSnapshot: %v", err)
	}
	if snap == nil {
		t.Fatal("expected a published snapshot")
	}
}

func TestReconcileVersionIncreasesEachCall(t *testing.T) {
	r, _, _ := newTestReconciler()
	_ = r.Reconcile(context.Background())
	first := r.version.Load()
	_ = r.Reconcile(context.Background())
	second := r.version.Load()
	if second <= first {
		t.Fatalf("version did not increase: first=%d second=%d", first, second)
	}
}

func TestReconcileTracksAttemptsAndFailures(t *testing.T) {
	r, _, _ := newTestReconciler()
	_ = r.Reconcile(context.Background())
	_ = r.Reconcile(context.Background())
	if r.Attempts() != 2 {
		t.Fatalf("Attempts() = %d, want 2", r.Attempts())
	}
	if r.Failures() != 0 {
		t.Fatalf("Failures() = %d, want 0", r.Failures())
	}
}

func TestRunReconcilesPeriodicallyAndStopsOnCancel(t *testing.T) {
	r, _, _ := newTestReconciler()
	ctx, cancel := context.WithCancel(context.Background())

	done := make(chan struct{})
	go func() {
		r.Run(ctx, 20*time.Millisecond)
		close(done)
	}()

	time.Sleep(100 * time.Millisecond)
	cancel()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Run did not stop after context cancel")
	}

	if r.Attempts() < 2 {
		t.Fatalf("Attempts() = %d, want at least 2 periodic reconciles", r.Attempts())
	}
}

func TestReconcileSweepsStaleInstances(t *testing.T) {
	reg := registry.New()
	routes := routing.NewStore()
	cache := cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil)
	r := New(reg, routes, cache, logging.New("error"), 10*time.Millisecond, metrics.New())

	_ = reg.Register(registry.Instance{ServiceName: "backend-a", Namespace: "default", InstanceID: "i1", Address: "10.0.0.1", Port: 9000})
	time.Sleep(20 * time.Millisecond)

	if err := r.Reconcile(context.Background()); err != nil {
		t.Fatalf("Reconcile: %v", err)
	}
	instances, err := reg.GetService("default", "backend-a")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if instances[0].Healthy {
		t.Fatal("expected instance to be marked unhealthy after reconcile sweeps stale instances")
	}
}

func TestBackoffIncreasesWithAttemptsAndCapsAtMax(t *testing.T) {
	prev := time.Duration(0)
	for attempt := 1; attempt <= 3; attempt++ {
		d := backoff(attempt)
		if d <= prev {
			t.Errorf("backoff(%d) = %v, want > backoff(%d) = %v", attempt, d, attempt-1, prev)
		}
		prev = d
	}
	capped := backoff(10)
	if capped > maxBackoff+maxBackoff/5 {
		t.Errorf("backoff(10) = %v, want capped near maxBackoff=%v", capped, maxBackoff)
	}
}
