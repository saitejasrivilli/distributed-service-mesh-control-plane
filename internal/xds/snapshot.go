// Package xds builds versioned Envoy xDS snapshots (CDS/EDS/LDS/RDS) from
// service registry state and serves them over gRPC.
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

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
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

// BuildSnapshot reads every service in Namespace from reg and produces a
// versioned CDS/EDS/LDS/RDS snapshot. version must be unique and increasing
// across calls (the caller owns versioning) so Envoy can detect staleness.
func BuildSnapshot(reg registry.Registry, version string) (*cachev3.Snapshot, error) {
	services := reg.ListServices(Namespace)
	sort.Strings(services)

	var clusters, endpoints, listeners, routes []types.Resource

	for i, svc := range services {
		port := uint32(basePort + i)

		cluster := buildCluster(svc)
		clusters = append(clusters, cluster)

		instances := reg.HealthyInstances(Namespace, svc, staleAfter)
		endpoints = append(endpoints, buildClusterLoadAssignment(svc, instances))

		routeConfigName := svc + "-route"
		routes = append(routes, buildRouteConfiguration(routeConfigName, svc))

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
		resourcev3.RouteType:    routes,
	})
	if err != nil {
		return nil, fmt.Errorf("construct snapshot: %w", err)
	}
	if err := snap.Consistent(); err != nil {
		return nil, fmt.Errorf("inconsistent snapshot: %w", err)
	}
	return snap, nil
}

func buildCluster(serviceName string) *clusterv3.Cluster {
	return &clusterv3.Cluster{
		Name:                 serviceName,
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
}

func buildClusterLoadAssignment(serviceName string, instances []registry.Instance) *endpointv3.ClusterLoadAssignment {
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
		ClusterName: serviceName,
		Endpoints: []*endpointv3.LocalityLbEndpoints{
			{LbEndpoints: lbEndpoints},
		},
	}
}

func buildRouteConfiguration(routeConfigName, clusterName string) *routev3.RouteConfiguration {
	return &routev3.RouteConfiguration{
		Name: routeConfigName,
		VirtualHosts: []*routev3.VirtualHost{
			{
				Name:    clusterName,
				Domains: []string{"*"},
				Routes: []*routev3.Route{
					{
						Match: &routev3.RouteMatch{
							PathSpecifier: &routev3.RouteMatch_Prefix{Prefix: "/"},
						},
						Action: &routev3.Route_Route{
							Route: &routev3.RouteAction{
								ClusterSpecifier: &routev3.RouteAction_Cluster{Cluster: clusterName},
							},
						},
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
