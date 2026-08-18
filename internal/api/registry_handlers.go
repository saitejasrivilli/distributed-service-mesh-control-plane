package api

import (
	"encoding/json"
	"errors"
	"net/http"
	"time"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/registry"
)

const defaultStaleAfter = 15 * time.Second

// registerServiceRequest is the POST /v1/services body.
type registerServiceRequest struct {
	ServiceName string            `json:"service_name"`
	Namespace   string            `json:"namespace"`
	InstanceID  string            `json:"instance_id"`
	Address     string            `json:"address"`
	Port        int               `json:"port"`
	Protocol    string            `json:"protocol"`
	Version     string            `json:"version"`
	Metadata    map[string]string `json:"metadata,omitempty"`
}

func namespaceOrDefault(ns string) string {
	if ns == "" {
		return "default"
	}
	return ns
}

func (s *Server) handleRegisterService(w http.ResponseWriter, r *http.Request) {
	var req registerServiceRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	if req.ServiceName == "" || req.InstanceID == "" {
		writeJSONError(w, http.StatusBadRequest, "service_name and instance_id are required")
		return
	}
	inst := registry.Instance{
		ServiceName: req.ServiceName,
		Namespace:   namespaceOrDefault(req.Namespace),
		InstanceID:  req.InstanceID,
		Address:     req.Address,
		Port:        req.Port,
		Protocol:    req.Protocol,
		Version:     req.Version,
		Metadata:    req.Metadata,
	}
	if err := s.registry.Register(inst); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.WriteHeader(http.StatusCreated)
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "registered"})
}

func (s *Server) handleDeregisterInstance(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	ns := namespaceOrDefault(r.URL.Query().Get("namespace"))
	if err := s.registry.Deregister(ns, name, id); err != nil {
		writeJSONError(w, http.StatusInternalServerError, err.Error())
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleHeartbeat(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	id := r.PathValue("id")
	ns := namespaceOrDefault(r.URL.Query().Get("namespace"))
	err := s.registry.Heartbeat(ns, name, id)
	switch {
	case err == nil:
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(map[string]string{"status": "ok"})
	case errors.Is(err, registry.ErrNotFound):
		writeJSONError(w, http.StatusNotFound, "instance not found")
	default:
		writeJSONError(w, http.StatusInternalServerError, err.Error())
	}
}

func (s *Server) handleGetService(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns := namespaceOrDefault(r.URL.Query().Get("namespace"))
	instances, err := s.registry.GetService(ns, name)
	if errors.Is(err, registry.ErrNotFound) {
		writeJSONError(w, http.StatusNotFound, "service not found")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"service": name, "instances": instances})
}

func (s *Server) handleListInstances(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	ns := namespaceOrDefault(r.URL.Query().Get("namespace"))
	healthyOnly := r.URL.Query().Get("healthy") == "true"

	var instances []registry.Instance
	var err error
	if healthyOnly {
		instances = s.registry.HealthyInstances(ns, name, defaultStaleAfter)
	} else {
		instances, err = s.registry.GetService(ns, name)
		if errors.Is(err, registry.ErrNotFound) {
			writeJSONError(w, http.StatusNotFound, "service not found")
			return
		}
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]any{"service": name, "instances": instances})
}

func writeJSONError(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": msg})
}
