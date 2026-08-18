// Package xds builds versioned Envoy xDS snapshots (CDS/EDS/LDS/RDS) from
// service registry and traffic-management state, and serves them over gRPC.
package xds

import (
	"fmt"
	"sort"
	"time"

	clusterv3 "github.com/envoyproxy/go-control-plane/envoy/config/cluster/v3"
	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	endpointv3 "github.com/envoyproxy/go-control-plane/envoy/config/endpoint/v3"
	listenerv3 "github.com/envoyproxy/go-control-plane/envoy/config/listener/v3"
	routev3 "github.com/envoyproxy/go-control-plane/envoy/config/route/v3"
	routerv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/http/router/v3"
	hcmv3 "github.com/envoyproxy/go-control-plane/envoy/extensions/filters/network/http_connection_manager/v3"
	"github.com/envoyproxy/go-control-plane/pkg/cache/types"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"
	"google.golang.org/protobuf/types/known/anypb"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

// basePort is the first listener port allocated to services, in sorted-name
// order, so port assignment is deterministic given the same set of services.
const basePort = 20000

// staleAfter bounds how long an instance may go without a heartbeat before
// EDS excludes it, matching the registry's own staleness definition.
const staleAfter = 15 * time.Second

// Namespace is the single namespace snapshots are built from in this
// release. Multi-namespace snapshot generation is future work.
const Namespace = "default"

// noVersionSplit is the fallback used when no routing.Spec is configured for
// a service: every healthy instance, regardless of Version, backs a single
// cluster named after the service itself.
var noVersionSplit = []routing.VersionWeight{{Version: "", Weight: 100}}

// BuildSnapshot reads every service in Namespace from reg, applies any
// configured routing.Spec from routes, and produces a versioned
// CDS/EDS/LDS/RDS snapshot. version must be unique and increasing across
// calls (the caller owns versioning) so Envoy can detect staleness.
func BuildSnapshot(reg registry.Registry, routes *routing.Store, version string) (*cachev3.Snapshot, error) {
	services := reg.ListServices(Namespace)
	sort.Strings(services)

	var clusters, endpoints, listeners, routeConfigs []types.Resource

	for i, svc := range services {
		port := uint32(basePort + i)

		spec, hasSpec := routing.Spec{}, false
		if routes != nil {
			spec, hasSpec = routes.Get(svc)
		}
		splits := noVersionSplit
		if hasSpec {
			splits = spec.Splits
		}

		weightedClusters := make([]*routev3.WeightedCluster_ClusterWeight, 0, len(splits))
		for _, split := range splits {
			clusterName := clusterName(svc, split.Version)
			clusters = append(clusters, buildCluster(clusterName, spec.CircuitBreaker))

			instances := reg.HealthyInstances(Namespace, svc, staleAfter)
			if split.Version != "" {
				instances = filterByVersion(instances, split.Version)
			}
			endpoints = append(endpoints, buildClusterLoadAssignment(clusterName, instances))

			weightedClusters = append(weightedClusters, &routev3.WeightedCluster_ClusterWeight{
				Name:   clusterName,
				Weight: wrapperspb.UInt32(split.Weight),
			})
		}

		routeConfigName := svc + "-route"
		routeConfigs = append(routeConfigs, buildRouteConfiguration(routeConfigName, svc, weightedClusters, spec))

		listener, err := buildListener(svc, port, routeConfigName)
		if err != nil {
			return nil, fmt.Errorf("build listener for %q: %w", svc, err)
		}
		listeners = append(listeners, listener)
	}

	snap, err := cachev3.NewSnapshot(version, map[resourcev3.Type][]types.Resource{
		resourcev3.ClusterType:  clusters,
		resourcev3.EndpointType: endpoints,
		resourcev3.ListenerType: listeners,
		resourcev3.RouteType:    routeConfigs,
	})
	if err != nil {
		return nil, fmt.Errorf("construct snapshot: %w", err)
	}
	if err := snap.Consistent(); err != nil {
		return nil, fmt.Errorf("inconsistent snapshot: %w", err)
	}
	return snap, nil
}

func clusterName(serviceName, version string) string {
	if version == "" {
		return serviceName
	}
	return serviceName + "::" + version
}

func filterByVersion(instances []registry.Instance, version string) []registry.Instance {
	out := instances[:0:0]
	for _, inst := range instances {
		if inst.Version == version {
			out = append(out, inst)
		}
	}
	return out
}

func buildCluster(name string, cb routing.CircuitBreaker) *clusterv3.Cluster {
	c := &clusterv3.Cluster{
		Name:                 name,
		ConnectTimeout:       durationpb.New(2 * time.Second),
		ClusterDiscoveryType: &clusterv3.Cluster_Type{Type: clusterv3.Cluster_EDS},
		EdsClusterConfig: &clusterv3.Cluster_EdsClusterConfig{
			EdsConfig: &corev3.ConfigSource{
				ResourceApiVersion:    corev3.ApiVersion_V3,
				ConfigSourceSpecifier: &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}},
			},
		},
		LbPolicy: clusterv3.Cluster_ROUND_ROBIN,
	}
	if cb != (routing.CircuitBreaker{}) {
		threshold := &clusterv3.CircuitBreakers_Thresholds{}
		if cb.MaxConnections > 0 {
			threshold.MaxConnections = wrapperspb.UInt32(cb.MaxConnections)
		}
		if cb.MaxPendingRequests > 0 {
			threshold.MaxPendingRequests = wrapperspb.UInt32(cb.MaxPendingRequests)
		}
		if cb.MaxRequests > 0 {
			threshold.MaxRequests = wrapperspb.UInt32(cb.MaxRequests)
		}
		if cb.MaxRetries > 0 {
			threshold.MaxRetries = wrapperspb.UInt32(cb.MaxRetries)
		}
		c.CircuitBreakers = &clusterv3.CircuitBreakers{
			Thresholds: []*clusterv3.CircuitBreakers_Thresholds{threshold},
		}
	}
	return c
}

func buildClusterLoadAssignment(clusterName string, instances []registry.Instance) *endpointv3.ClusterLoadAssignment {
	lbEndpoints := make([]*endpointv3.LbEndpoint, 0, len(instances))
	for _, inst := range instances {
		lbEndpoints = append(lbEndpoints, &endpointv3.LbEndpoint{
			HostIdentifier: &endpointv3.LbEndpoint_Endpoint{
				Endpoint: &endpointv3.Endpoint{
					Address: &corev3.Address{
						Address: &corev3.Address_SocketAddress{
							SocketAddress: &corev3.SocketAddress{
								Address: inst.Address,
								PortSpecifier: &corev3.SocketAddress_PortValue{
									PortValue: uint32(inst.Port),
								},
							},
						},
					},
				},
			},
		})
	}
	return &endpointv3.ClusterLoadAssignment{
		ClusterName: clusterName,
		Endpoints: []*endpointv3.LocalityLbEndpoints{
			{LbEndpoints: lbEndpoints},
		},
	}
}

func buildRouteConfiguration(routeConfigName, virtualHostName string, weightedClusters []*routev3.WeightedCluster_ClusterWeight, spec routing.Spec) *routev3.RouteConfiguration {
	action := &routev3.RouteAction{}
	if len(weightedClusters) == 1 && weightedClusters[0].Weight.GetValue() == 100 {
		action.ClusterSpecifier = &routev3.RouteAction_Cluster{Cluster: weightedClusters[0].Name}
	} else {
		action.ClusterSpecifier = &routev3.RouteAction_WeightedClusters{
			WeightedClusters: &routev3.WeightedCluster{Clusters: weightedClusters},
		}
	}
	if spec.TimeoutMs > 0 {
		action.Timeout = durationpb.New(time.Duration(spec.TimeoutMs) * time.Millisecond)
	}
	if spec.RetryOn != "" {
		action.RetryPolicy = &routev3.RetryPolicy{
			RetryOn:    spec.RetryOn,
			NumRetries: wrapperspb.UInt32(spec.NumRetries),
		}
	}
	return &routev3.RouteConfiguration{
		Name: routeConfigName,
		VirtualHosts: []*routev3.VirtualHost{
			{
				Name:    virtualHostName,
				Domains: []string{"*"},
				Routes: []*routev3.Route{
					{
						Match: &routev3.RouteMatch{
							PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &routev3.Route_Route{Route: action},
					},
				},
			},
		},
	}
}

func buildListener(serviceName string, port uint32, routeConfigName string) (*listenerv3.Listener, error) {
	router, err := anypb.New(&routerv3.Router{})
	if err != nil {
		return nil, err
	}
	hcm := &hcmv3.HttpConnectionManager{
		StatPrefix: serviceName,
		RouteSpecifier: &hcmv3.HttpConnectionManager_Rds{
			Rds: &hcmv3.Rds{
				RouteConfigName: routeConfigName,
				ConfigSource: &corev3.ConfigSource{
					ResourceApiVersion:    corev3.ApiVersion_V3,
					ConfigSourceSpecifier: &corev3.ConfigSource_Ads{Ads: &corev3.AggregatedConfigSource{}},
				},
			},
		},
		HttpFilters: []*hcmv3.HttpFilter{
			{
				Name:       "envoy.filters.http.router",
				ConfigType: &hcmv3.HttpFilter_TypedConfig{TypedConfig: router},
			},
		},
	}
	hcmAny, err := anypb.New(hcm)
	if err != nil {
		return nil, err
	}
	return &listenerv3.Listener{
		Name: serviceName + "-listener",
		Address: &corev3.Address{
			Address: &corev3.Address_SocketAddress{
				SocketAddress: &corev3.SocketAddress{
					Address:       "0.0.0.0",
					PortSpecifier: &corev3.SocketAddress_PortValue{PortValue: port},
				},
			},
		},
		FilterChains: []*listenerv3.FilterChain{
			{
				Filters: []*listenerv3.Filter{
					{
						Name:       "envoy.filters.network.http_connection_manager",
						ConfigType: &listenerv3.Filter_TypedConfig{TypedConfig: hcmAny},
					},
				},
			},
		},
	}, nil
}
