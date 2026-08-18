package api

import (
	"encoding/json"
	"net/http"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/routing"
)

func (s *Server) handlePutRoute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("service")
	var spec routing.Spec
	if err := json.NewDecoder(r.Body).Decode(&spec); err != nil {
		writeJSONError(w, http.StatusBadRequest, "invalid request body")
		return
	}
	spec.Service = name
	if err := s.routes.Set(spec); err != nil {
		writeJSONError(w, http.StatusBadRequest, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(map[string]string{"status": "route updated"})
}

func (s *Server) handleGetRoute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("service")
	spec, ok := s.routes.Get(name)
	if !ok {
		writeJSONError(w, http.StatusNotFound, "no route configured for service")
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(spec)
}

func (s *Server) handleDeleteRoute(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("service")
	s.routes.Delete(name)
	w.WriteHeader(http.StatusNoContent)
}
