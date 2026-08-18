package xds

import (
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

func regWithVersionedInstances(t *testing.T) *registry.InMemory {
	t.Helper()
	r := registry.New()
	if err := r.Register(registry.Instance{ServiceName: "backend-a", Namespace: Namespace, InstanceID: "v1-i1", Address: "10.0.0.1", Port: 9000, Version: "v1"}); err != nil {
		t.Fatal(err)
	}
	if err := r.Register(registry.Instance{ServiceName: "backend-a", Namespace: Namespace, InstanceID: "v2-i1", Address: "10.0.0.2", Port: 9000, Version: "v2"}); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestWeightedRoutingProducesPerVersionClusters(t *testing.T) {
	r := regWithVersionedInstances(t)
	routes := routing.NewStore()
	if err := routes.Set(routing.Spec{
		Service: "backend-a",
		Splits:  []routing.VersionWeight{{Version: "v1", Weight: 90}, {Version: "v2", Weight: 10}},
	}); err != nil {
		t.Fatalf("Set: %v", err)
	}

	snap, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	clusters := snap.GetResources(resourcev3.ClusterType)
	if _, ok := clusters["backend-a::v1"]; !ok {
		t.Error("expected cluster backend-a::v1")
	}
	if _, ok := clusters["backend-a::v2"]; !ok {
		t.Error("expected cluster backend-a::v2")
	}

	rc := snap.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	wc := rc.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters()
	if wc == nil {
		t.Fatal("expected weighted_clusters route action")
	}
	weights := map[string]uint32{}
	for _, c := range wc.Clusters {
		weights[c.Name] = c.Weight.GetValue()
	}
	if weights["backend-a::v1"] != 90 || weights["backend-a::v2"] != 10 {
		t.Errorf("weights = %+v, want v1=90 v2=10", weights)
	}
}

func TestWeightedRoutingEndpointsFilteredByVersion(t *testing.T) {
	r := regWithVersionedInstances(t)
	routes := routing.NewStore()
	_ = routes.Set(routing.Spec{
		Service: "backend-a",
		Splits:  []routing.VersionWeight{{Version: "v1", Weight: 50}, {Version: "v2", Weight: 50}},
	})

	snap, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	endpoints := snap.GetResources(resourcev3.EndpointType)
	v1cla := endpoints["backend-a::v1"]
	if v1cla == nil {
		t.Fatal("expected EDS for backend-a::v1")
	}
}

func TestCanaryWeightShiftChangesSnapshot(t *testing.T) {
	r := regWithVersionedInstances(t)
	routes := routing.NewStore()
	_ = routes.Set(routing.Spec{Service: "backend-a", Splits: []routing.VersionWeight{{Version: "v1", Weight: 90}, {Version: "v2", Weight: 10}}})
	snap1, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot v1: %v", err)
	}

	_ = routes.Set(routing.Spec{Service: "backend-a", Splits: []routing.VersionWeight{{Version: "v1", Weight: 50}, {Version: "v2", Weight: 50}}})
	snap2, err := BuildSnapshot(r, routes, "v2")
	if err != nil {
		t.Fatalf("BuildSnapshot v2: %v", err)
	}

	rc1 := snap1.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	rc2 := snap2.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	w1 := rc1.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters
	w2 := rc2.VirtualHosts[0].Routes[0].GetRoute().GetWeightedClusters().Clusters

	get := func(cs []*routev3.WeightedCluster_ClusterWeight, name string) uint32 {
		for _, c := range cs {
			if c.Name == name {
				return c.Weight.GetValue()
			}
		}
		return 0
	}
	if get(w1, "backend-a::v1") != 90 {
		t.Errorf("snap1 v1 weight = %d, want 90", get(w1, "backend-a::v1"))
	}
	if get(w2, "backend-a::v1") != 50 {
		t.Errorf("snap2 v1 weight = %d, want 50 (canary shift not reflected)", get(w2, "backend-a::v1"))
	}
}

func TestRetryPolicyAppliedToRoute(t *testing.T) {
	r := regWithServices(t, "backend-a")
	routes := routing.NewStore()
	_ = routes.Set(routing.Spec{
		Service:    "backend-a",
		Splits:     []routing.VersionWeight{{Version: "", Weight: 100}},
		RetryOn:    "5xx",
		NumRetries: 3,
	})
	snap, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	rc := snap.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	rp := rc.VirtualHosts[0].Routes[0].GetRoute().GetRetryPolicy()
	if rp == nil {
		t.Fatal("expected retry policy")
	}
	if rp.RetryOn != "5xx" || rp.NumRetries.GetValue() != 3 {
		t.Errorf("retry policy = %+v, want retry_on=5xx num_retries=3", rp)
	}
}

func TestTimeoutAppliedToRoute(t *testing.T) {
	r := regWithServices(t, "backend-a")
	routes := routing.NewStore()
	_ = routes.Set(routing.Spec{
		Service:   "backend-a",
		Splits:    []routing.VersionWeight{{Version: "", Weight: 100}},
		TimeoutMs: 2500,
	})
	snap, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	rc := snap.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	timeout := rc.VirtualHosts[0].Routes[0].GetRoute().GetTimeout()
	if timeout.AsDuration().Milliseconds() != 2500 {
		t.Errorf("timeout = %v, want 2500ms", timeout.AsDuration())
	}
}

func TestCircuitBreakerAppliedToCluster(t *testing.T) {
	r := regWithServices(t, "backend-a")
	routes := routing.NewStore()
	_ = routes.Set(routing.Spec{
		Service: "backend-a",
		Splits:  []routing.VersionWeight{{Version: "", Weight: 100}},
		CircuitBreaker: routing.CircuitBreaker{
			MaxConnections: 50,
			MaxRequests:    100,
		},
	})
	snap, err := BuildSnapshot(r, routes, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	c := snap.GetResources(resourcev3.ClusterType)["backend-a"].(*clusterv3.Cluster)
	if c.CircuitBreakers == nil || len(c.CircuitBreakers.Thresholds) != 1 {
		t.Fatal("expected circuit breaker thresholds")
	}
	th := c.CircuitBreakers.Thresholds[0]
	if th.MaxConnections.GetValue() != 50 || th.MaxRequests.GetValue() != 100 {
		t.Errorf("thresholds = %+v, want max_connections=50 max_requests=100", th)
	}
}

func TestNoRouteSpecFallsBackToSingleCluster(t *testing.T) {
	r := regWithServices(t, "backend-a")
	snap, err := BuildSnapshot(r, routing.NewStore(), "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	rc := snap.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	action := rc.VirtualHosts[0].Routes[0].GetRoute()
	if action.GetCluster() != "backend-a" {
		t.Errorf("cluster = %q, want backend-a (single-cluster fallback)", action.GetCluster())
	}
	if action.GetWeightedClusters() != nil {
		t.Error("expected no weighted_clusters when no route spec configured")
	}
}
