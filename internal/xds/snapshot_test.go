package xds

import (
	"testing"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
)

func regWithServices(t *testing.T, services ...string) *registry.InMemory {
	t.Helper()
	r := registry.New()
	for _, svc := range services {
		if err := r.Register(registry.Instance{
			ServiceName: svc, Namespace: Namespace, InstanceID: svc + "-i1",
			Address: "10.0.0.1", Port: 9000,
		}); err != nil {
			t.Fatalf("register %s: %v", svc, err)
		}
	}
	return r
}

func TestBuildSnapshotProducesAllFourResourceTypes(t *testing.T) {
	r := regWithServices(t, "backend-a", "backend-b")
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if got := len(snap.GetResources(resourcev3.ClusterType)); got != 2 {
		t.Errorf("clusters = %d, want 2", got)
	}
	if got := len(snap.GetResources(resourcev3.EndpointType)); got != 2 {
		t.Errorf("endpoints = %d, want 2", got)
	}
	if got := len(snap.GetResources(resourcev3.ListenerType)); got != 2 {
		t.Errorf("listeners = %d, want 2", got)
	}
	if got := len(snap.GetResources(resourcev3.RouteType)); got != 2 {
		t.Errorf("routes = %d, want 2", got)
	}
}

func TestBuildSnapshotIsConsistent(t *testing.T) {
	r := regWithServices(t, "backend-a")
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if err := snap.Consistent(); err != nil {
		t.Fatalf("snapshot not consistent: %v", err)
	}
}

func TestBuildSnapshotVersioning(t *testing.T) {
	r := regWithServices(t, "backend-a")
	snap1, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot v1: %v", err)
	}
	snap2, err := BuildSnapshot(r, nil, "v2")
	if err != nil {
		t.Fatalf("BuildSnapshot v2: %v", err)
	}
	if got := snap1.GetVersion(resourcev3.ClusterType); got != "v1" {
		t.Errorf("snap1 version = %q, want v1", got)
	}
	if got := snap2.GetVersion(resourcev3.ClusterType); got != "v2" {
		t.Errorf("snap2 version = %q, want v2", got)
	}
}

func TestBuildSnapshotDeterministicPortAssignment(t *testing.T) {
	r := regWithServices(t, "backend-b", "backend-a")
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	listeners := snap.GetResources(resourcev3.ListenerType)
	la := listeners["backend-a-listener"].(*listenerv3.Listener)
	lb := listeners["backend-b-listener"].(*listenerv3.Listener)
	portA := la.Address.GetSocketAddress().GetPortValue()
	portB := lb.Address.GetSocketAddress().GetPortValue()
	// backend-a sorts before backend-b, so it must get the lower port,
	// regardless of registration order.
	if portA >= portB {
		t.Errorf("portA=%d portB=%d, want portA < portB", portA, portB)
	}
}

func TestBuildSnapshotOnlyHealthyEndpointsInEDS(t *testing.T) {
	r := registry.New()
	_ = r.Register(registry.Instance{ServiceName: "backend-a", Namespace: Namespace, InstanceID: "i1", Address: "10.0.0.1", Port: 9000})

	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	cla := snap.GetResources(resourcev3.EndpointType)["backend-a"].(*endpointv3.ClusterLoadAssignment)
	if got := len(cla.Endpoints[0].LbEndpoints); got != 1 {
		t.Fatalf("endpoints = %d, want 1", got)
	}
}

func TestBuildSnapshotEmptyRegistryProducesEmptySnapshot(t *testing.T) {
	r := registry.New()
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	if got := len(snap.GetResources(resourcev3.ClusterType)); got != 0 {
		t.Errorf("clusters = %d, want 0", got)
	}
}

func TestClusterUsesEDSDiscoveryType(t *testing.T) {
	r := regWithServices(t, "backend-a")
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	c := snap.GetResources(resourcev3.ClusterType)["backend-a"].(*clusterv3.Cluster)
	if c.GetType() != clusterv3.Cluster_EDS {
		t.Errorf("cluster discovery type = %v, want EDS", c.GetType())
	}
}

func TestRouteConfigurationRoutesToMatchingCluster(t *testing.T) {
	r := regWithServices(t, "backend-a")
	snap, err := BuildSnapshot(r, nil, "v1")
	if err != nil {
		t.Fatalf("BuildSnapshot: %v", err)
	}
	rc := snap.GetResources(resourcev3.RouteType)["backend-a-route"].(*routev3.RouteConfiguration)
	cluster := rc.VirtualHosts[0].Routes[0].GetRoute().GetCluster()
	if cluster != "backend-a" {
		t.Errorf("route cluster = %q, want backend-a", cluster)
	}
}
