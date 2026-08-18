// Package benchmark contains scale/performance validation tests (v0.9.0)
// that exercise the real control plane over a real network connection —
// no mocks. Run with: go test ./test/benchmark/... -run TestXDSScale -v
package benchmark

import (
	"context"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/logging"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/reconciliation"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/xds"
)

// startRealControlPlane spins up a real xDS gRPC server, a real registry,
// and a real reconciler wired together exactly as internal/controlplane
// does, listening on an ephemeral localhost port. Returns a dial address
// and a stop function.
func startRealControlPlane(t *testing.T) (addr string, reg *registry.InMemory, recon *reconciliation.Reconciler, stop func()) {
	t.Helper()
	logger := logging.New("error")
	reg = registry.New()
	routes := routing.NewStore()
	cache := cachev3.NewSnapshotCache(true, cachev3.IDHash{}, nil)
	recon = reconciliation.New(reg, routes, cache, logger, 30*time.Second, nil)

	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	xdsSrv := xds.NewServer(context.Background(), cache, nil, logger)
	go func() { _ = xdsSrv.ServeListener(lis) }()

	ctx, cancel := context.WithCancel(context.Background())
	go recon.Run(ctx, 200*time.Millisecond)

	return lis.Addr().String(), reg, recon, func() {
		cancel()
		xdsSrv.GracefulStop()
	}
}

// connectADSClient dials the control plane and opens an ADS stream,
// subscribing to CDS. Returns a channel that receives the resource-name
// count of every CDS response (used to detect when a new service appears).
func connectADSClient(t *testing.T, addr string, nodeID string) (updates chan int, closeFn func()) {
	t.Helper()
	conn, err := grpc.NewClient(addr, grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	client := discoveryv3.NewAggregatedDiscoveryServiceClient(conn)
	ctx, cancel := context.WithCancel(context.Background())
	stream, err := client.StreamAggregatedResources(ctx)
	if err != nil {
		t.Fatalf("open stream: %v", err)
	}

	updates = make(chan int, 100)
	go func() {
		for {
			resp, err := stream.Recv()
			if err != nil {
				close(updates)
				return
			}
			updates <- len(resp.Resources)
			_ = stream.Send(&discoveryv3.DiscoveryRequest{
				VersionInfo:   resp.VersionInfo,
				ResponseNonce: resp.Nonce,
				TypeUrl:       resp.TypeUrl,
				Node:          nil,
			})
		}
	}()

	req := &discoveryv3.DiscoveryRequest{
		TypeUrl: resourcev3.ClusterType,
		Node:    &corev3.Node{Id: nodeID},
	}
	if err := stream.Send(req); err != nil {
		t.Fatalf("send initial request: %v", err)
	}

	return updates, func() { cancel(); _ = conn.Close() }
}

// TestXDSScale_ConcurrentClients connects N Envoy-equivalent ADS clients to
// a real, running control plane, registers a new service, and measures how
// long it takes for every connected client to observe the new cluster --
// a real, measured propagation-latency number, not an estimate.
func TestXDSScale_ConcurrentClients(t *testing.T) {
	for _, n := range []int{10, 25, 50} {
		n := n
		t.Run(fmt.Sprintf("clients=%d", n), func(t *testing.T) {
			addr, reg, _, stop := startRealControlPlane(t)
			defer stop()

			var wg sync.WaitGroup
			clientUpdates := make([]chan int, n)
			var closers []func()
			for i := 0; i < n; i++ {
				// All simulated Envoys use the same node ID: this release's
				// reconciler publishes one snapshot per (fixed) NodeID (see
				// ADR-004) -- multiple Envoy replicas sharing identical
				// config is exactly the topology that models today.
				updates, closeFn := connectADSClient(t, addr, reconciliation.NodeID)
				clientUpdates[i] = updates
				closers = append(closers, closeFn)
			}
			defer func() {
				for _, c := range closers {
					c()
				}
			}()

			// Drain the initial (empty) CDS response on every client before
			// measuring, so the timer only captures the update we trigger.
			for i := 0; i < n; i++ {
				select {
				case <-clientUpdates[i]:
				case <-time.After(3 * time.Second):
					t.Fatalf("client %d: timed out waiting for initial snapshot", i)
				}
			}

			start := time.Now()
			if err := reg.Register(registry.Instance{
				ServiceName: "scale-test-svc", Namespace: xds.Namespace, InstanceID: "i1",
				Address: "10.0.0.1", Port: 9000,
			}); err != nil {
				t.Fatalf("register: %v", err)
			}

			wg.Add(n)
			latencies := make([]time.Duration, n)
			for i := 0; i < n; i++ {
				i := i
				go func() {
					defer wg.Done()
					for {
						select {
						case count, ok := <-clientUpdates[i]:
							if !ok {
								return
							}
							if count > 0 {
								latencies[i] = time.Since(start)
								return
							}
						case <-time.After(5 * time.Second):
							t.Errorf("client %d: timed out waiting for propagated update", i)
							return
						}
					}
				}()
			}
			wg.Wait()

			var maxLatency time.Duration
			var total time.Duration
			for _, l := range latencies {
				if l > maxLatency {
					maxLatency = l
				}
				total += l
			}
			avg := total / time.Duration(n)
			t.Logf("clients=%d: max_propagation_latency=%v avg_propagation_latency=%v", n, maxLatency, avg)
		})
	}
}
