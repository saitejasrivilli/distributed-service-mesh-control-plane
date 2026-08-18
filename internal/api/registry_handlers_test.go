package api

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func doJSON(t *testing.T, s *Server, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rr := httptest.NewRecorder()
	s.httpServer.Handler.ServeHTTP(rr, r)
	return rr
}

func TestRegisterServiceEndpoint(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"svc-a","instance_id":"i1","address":"10.0.0.1","port":8080}`)
	if rr.Code != http.StatusCreated {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestRegisterServiceRejectsMissingFields(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodPost, "/v1/services", `{"address":"10.0.0.1"}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400", rr.Code)
	}
}

func TestGetServiceAfterRegister(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"svc-a","instance_id":"i1"}`)

	rr := doJSON(t, s, http.MethodGet, "/v1/services/svc-a", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Instances []map[string]any `json:"instances"`
	}
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(resp.Instances) != 1 {
		t.Fatalf("got %d instances, want 1", len(resp.Instances))
	}
}

func TestGetUnknownServiceReturns404(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodGet, "/v1/services/nope", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDeregisterInstance(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"svc-a","instance_id":"i1"}`)

	rr := doJSON(t, s, http.MethodDelete, "/v1/services/svc-a/instances/i1", "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}

	rr = doJSON(t, s, http.MethodGet, "/v1/services/svc-a", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after deregister", rr.Code)
	}
}

func TestHeartbeatEndpoint(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"svc-a","instance_id":"i1"}`)

	rr := doJSON(t, s, http.MethodPost, "/v1/services/svc-a/instances/i1/heartbeat", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestHeartbeatUnknownInstanceReturns404(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodPost, "/v1/services/svc-a/instances/nope/heartbeat", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestListInstancesHealthyFilter(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPost, "/v1/services", `{"service_name":"svc-a","instance_id":"i1"}`)

	rr := doJSON(t, s, http.MethodGet, "/v1/services/svc-a/instances?healthy=true", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
	var resp struct {
		Instances []map[string]any `json:"instances"`
	}
	_ = json.Unmarshal(rr.Body.Bytes(), &resp)
	if len(resp.Instances) != 1 {
		t.Fatalf("got %d healthy instances, want 1", len(resp.Instances))
	}
}
