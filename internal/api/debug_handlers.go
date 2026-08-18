package api

import (
	"encoding/json"
	"net/http"
	"time"

	resourcev3 "github.com/envoyproxy/go-control-plane/pkg/resource/v3"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

// handleDebugService answers "what does the mesh currently know about this
// service?" — every registered instance, its health, and its last
// heartbeat, plus any configured traffic-management spec.
func (s *Server) handleDebugService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns := namespaceOrDefault(r.URL.Query().Get("namespace"))
	instances, err := s.registry.GetService(ns, name)
	if err != nil {
		writeJSONError(w, http.StatusNotFound, "service not found")
		return
	}
	route, hasRoute := s.routes.Get(name)
	resp := map[string]any{
		"service":   name,
		"namespace": ns,
		"instances": instances,
	}
	if hasRoute {
		resp["route"] = route
	} else {
		resp["route"] = nil
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(resp)
}

// handleDebugEnvoys answers "which Envoys are connected, and since when?"
func (s *Server) handleDebugEnvoys(w http.ResponseWriter, r *http.Request) {
	connected := s.envoys.Connected()
	type envoyView struct {
		StreamID       int64     `json:"stream_id"`
		NodeID         string    `json:"node_id"`
		ConnectedSince time.Time `json:"connected_since"`
	}
	views := make([]envoyView, 0, len(connected))
	for _, c := range connected {
		views = append(views, envoyView{StreamID: c.StreamID, NodeID: c.NodeID, ConnectedSince: c.ConnectedAt})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"count":  s.envoys.Count(),
		"envoys": views,
	})
}

// handleDebugConfig answers "what xDS config is currently published for this
// service, and at what version?" — reads directly from the reconciler's last
// published snapshot, so it reflects exactly what Envoy was told, not what
// the registry currently looks like (those can differ for a few seconds
// around a change, and that gap is exactly what this endpoint is for
// diagnosing).
func (s *Server) handleDebugConfig(w http.ResponseWriter, r *http.Request) {
	service := r.PathValue("service")
	snap := s.reconciler.LastSnapshot()
	if snap == nil {
		writeJSONError(w, http.StatusNotFound, "no snapshot has been published yet")
		return
	}

	clusters := snap.GetResources(resourcev3.ClusterType)
	endpoints := snap.GetResources(resourcev3.EndpointType)
	routes := snap.GetResources(resourcev3.RouteType)
	listeners := snap.GetResources(resourcev3.ListenerType)

	var clusterNames, endpointNames []string
	for name := range clusters {
		if name == service || hasPrefix(name, service+"::") {
			clusterNames = append(clusterNames, name)
		}
	}
	for name := range endpoints {
		if name == service || hasPrefix(name, service+"::") {
			endpointNames = append(endpointNames, name)
		}
	}
	_, hasRoute := routes[service+"-route"]
	_, hasListener := listeners[service+"-listener"]

	spec, hasSpec := s.routes.Get(service)
	var routeSpec *routing.Spec
	if hasSpec {
		routeSpec = &spec
	}

	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{
		"service":        service,
		"config_version": snap.GetVersion(resourcev3.ClusterType),
		"clusters":       clusterNames,
		"endpoints":      endpointNames,
		"has_route":      hasRoute,
		"has_listener":   hasListener,
		"traffic_spec":   routeSpec,
	})
}

func hasPrefix(s, prefix string) bool {
	return len(s) >= len(prefix) && s[:len(prefix)] == prefix
}
