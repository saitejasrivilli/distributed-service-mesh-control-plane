package xds

import (
	"context"
	"fmt"
	"log/slog"
	"net"

	clusterservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/cluster/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	endpointservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/endpoint/v3"
	listenerservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/listener/v3"
	routeservicev3 "github.com/envoyproxy/go-control-plane/envoy/service/route/v3"
	cachev3 "github.com/envoyproxy/go-control-plane/pkg/cache/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
	"google.golang.org/grpc"
	"google.golang.org/grpc/health"
	healthpb "google.golang.org/grpc/health/grpc_health_v1"
)

// Server is the control plane's gRPC xDS server (CDS/EDS/LDS/RDS over ADS).
type Server struct {
	grpcServer *grpc.Server
	logger     *slog.Logger
}

// NewServer wires an xDS gRPC server against cache. callbacks may be nil.
func NewServer(ctx context.Context, cache cachev3.SnapshotCache, callbacks serverv3.Callbacks, logger *slog.Logger) *Server {
	xdsSrv := serverv3.NewServer(ctx, cache, callbacks)
	grpcServer := grpc.NewServer()

	discoveryv3.RegisterAggregatedDiscoveryServiceServer(grpcServer, xdsSrv)
	clusterservicev3.RegisterClusterDiscoveryServiceServer(grpcServer, xdsSrv)
	endpointservicev3.RegisterEndpointDiscoveryServiceServer(grpcServer, xdsSrv)
	listenerservicev3.RegisterListenerDiscoveryServiceServer(grpcServer, xdsSrv)
	routeservicev3.RegisterRouteDiscoveryServiceServer(grpcServer, xdsSrv)

	healthSrv := health.NewServer()
	healthpb.RegisterHealthServer(grpcServer, healthSrv)
	healthSrv.SetServingStatus("", healthpb.HealthCheckResponse_SERVING)

	return &Server{grpcServer: grpcServer, logger: logger}
}

// Serve blocks accepting xDS connections on addr until the listener errors
// or the server is stopped.
func (s *Server) Serve(addr string) error {
	lis, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("xds: listen on %s: %w", addr, err)
	}
	s.logger.Info("xds server listening", "addr", addr)
	return s.ServeListener(lis)
}

// ServeListener blocks accepting xDS connections on an already-created
// listener. Exposed separately from Serve so tests can bind an ephemeral
// port and learn its address before the server starts accepting.
func (s *Server) ServeListener(lis net.Listener) error {
	return s.grpcServer.Serve(lis)
}

// GracefulStop drains in-flight xDS streams and stops accepting new ones.
func (s *Server) GracefulStop() {
	s.grpcServer.GracefulStop()
}
