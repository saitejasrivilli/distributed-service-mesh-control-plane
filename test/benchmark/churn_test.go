package benchmark

import (
	"context"
	"fmt"
	"runtime"
	"testing"
	"time"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
)

// TestScale_100Services1000Endpoints runs the real reconciliation loop
// against a registry populated with 100 services / 1000 endpoints while
// concurrently churning registrations/heartbeats/deregistrations, and
// measures actual reconcile-loop throughput and process memory -- not
// invented numbers.
func TestScale_100Services1000Endpoints(t *testing.T) {
	_, reg, recon, stop := startRealControlPlane(t)
	defer stop()

	const numServices = 100
	const instancesPerService = 10 // 1000 endpoints total

	for s := 0; s < numServices; s++ {
		svc := fmt.Sprintf("svc-%04d", s)
		for i := 0; i < instancesPerService; i++ {
			if err := reg.Register(registry.Instance{
				ServiceName: svc, Namespace: "default", InstanceID: fmt.Sprintf("i-%04d", i),
				Address: "10.0.0.1", Port: 9000,
			}); err != nil {
				t.Fatalf("register: %v", err)
			}
		}
	}

	var memBefore, memAfter runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&memBefore)

	// Churn: concurrently register/deregister/heartbeat a rotating set of
	// instances for 2 real seconds while the reconciler keeps running.
	churnCtx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	churnCount := 0
	for churnCtx.Err() == nil {
		svc := fmt.Sprintf("svc-%04d", churnCount%numServices)
		id := fmt.Sprintf("churn-%d", churnCount)
		_ = reg.Register(registry.Instance{ServiceName: svc, Namespace: "default", InstanceID: id, Address: "10.0.0.2", Port: 9001})
		_ = reg.Heartbeat("default", svc, id)
		_ = reg.Deregister("default", svc, id)
		churnCount++
	}

	runtime.ReadMemStats(&memAfter)

	attemptsBefore := recon.Attempts()
	time.Sleep(1 * time.Second) // let a few more reconcile ticks run
	attemptsAfter := recon.Attempts()

	t.Logf("scale test: %d services, %d endpoints, %d churn operations in 2s", numServices, numServices*instancesPerService, churnCount)
	t.Logf("reconcile attempts during 1s window after churn: %d", attemptsAfter-attemptsBefore)
	t.Logf("heap alloc before churn: %d KB, after: %d KB (delta: %d KB)",
		memBefore.HeapAlloc/1024, memAfter.HeapAlloc/1024, (memAfter.HeapAlloc-memBefore.HeapAlloc)/1024)
	t.Logf("failures: %d (want 0)", recon.Failures())

	if recon.Failures() != 0 {
		t.Errorf("expected 0 reconciliation failures during churn, got %d", recon.Failures())
	}
	if churnCount == 0 {
		t.Fatal("churn loop did not execute any operations")
	}
}
