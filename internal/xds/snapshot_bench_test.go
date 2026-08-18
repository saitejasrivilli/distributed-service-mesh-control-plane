package xds

import (
	"fmt"
	"testing"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

// populate registers numServices services with instancesPerService instances
// each, returning the total endpoint count.
func populate(b *testing.B, numServices, instancesPerService int) *registry.InMemory {
	b.Helper()
	r := registry.New()
	for s := 0; s < numServices; s++ {
		svc := fmt.Sprintf("svc-%04d", s)
		for i := 0; i < instancesPerService; i++ {
			if err := r.Register(registry.Instance{
				ServiceName: svc,
				Namespace:   Namespace,
				InstanceID:  fmt.Sprintf("i-%04d", i),
				Address:     "10.0.0.1",
				Port:        9000,
			}); err != nil {
				b.Fatalf("register: %v", err)
			}
		}
	}
	return r
}

func benchmarkBuildSnapshot(b *testing.B, numServices, instancesPerService int) {
	r := populate(b, numServices, instancesPerService)
	routes := routing.NewStore()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		if _, err := BuildSnapshot(r, routes, fmt.Sprintf("v%d", i)); err != nil {
			b.Fatalf("BuildSnapshot: %v", err)
		}
	}
}

func BenchmarkBuildSnapshot_10Services_10Endpoints(b *testing.B) {
	benchmarkBuildSnapshot(b, 10, 10)
}

func BenchmarkBuildSnapshot_100Services_1000Endpoints(b *testing.B) {
	benchmarkBuildSnapshot(b, 100, 10)
}

func BenchmarkBuildSnapshot_100Services_1Endpoint(b *testing.B) {
	benchmarkBuildSnapshot(b, 100, 1)
}
