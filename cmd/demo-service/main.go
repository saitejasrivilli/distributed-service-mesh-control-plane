// Command demo-service is a minimal backend used to exercise mesh routing in
// later releases. v0.1.0 only needs it to answer plain HTTP health/echo
// requests behind an Envoy sidecar.
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"
	"os"
)

func main() {
	addr := flag.String("addr", ":9000", "address to listen on")
	name := flag.String("name", envOr("DEMO_SERVICE_NAME", "demo-service"), "service instance name")
	flag.Parse()

	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	mux.HandleFunc("GET /echo", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"service": *name})
	})

	log.Printf("demo-service %q listening on %s", *name, *addr)
	if err := http.ListenAndServe(*addr, mux); err != nil {
		log.Fatalf("demo-service exited: %v", err)
	}
}

func envOr(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}
