package api

import (
	"net/http"
	"testing"
)

func TestPutRouteValid(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":90},{"version":"v2","weight":10}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestPutRouteInvalidWeightsRejected(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":50}]}`)
	if rr.Code != http.StatusBadRequest {
		t.Fatalf("status = %d, want 400, body = %s", rr.Code, rr.Body.String())
	}
}

func TestGetRouteAfterPut(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":100}]}`)
	rr := doJSON(t, s, http.MethodGet, "/v1/routes/backend-a", "")
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}

func TestGetRouteUnknownReturns404(t *testing.T) {
	s, _ := testServer(t, ":0")
	rr := doJSON(t, s, http.MethodGet, "/v1/routes/nope", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404", rr.Code)
	}
}

func TestDeleteRoute(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":100}]}`)
	rr := doJSON(t, s, http.MethodDelete, "/v1/routes/backend-a", "")
	if rr.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want 204", rr.Code)
	}
	rr = doJSON(t, s, http.MethodGet, "/v1/routes/backend-a", "")
	if rr.Code != http.StatusNotFound {
		t.Fatalf("status = %d, want 404 after delete", rr.Code)
	}
}

func TestPutRouteCanaryShiftOverwrites(t *testing.T) {
	s, _ := testServer(t, ":0")
	doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":90},{"version":"v2","weight":10}]}`)
	rr := doJSON(t, s, http.MethodPut, "/v1/routes/backend-a", `{"splits":[{"version":"v1","weight":50},{"version":"v2","weight":50}]}`)
	if rr.Code != http.StatusOK {
		t.Fatalf("status = %d, body = %s", rr.Code, rr.Body.String())
	}
}
