// Command control-plane runs the service mesh control-plane HTTP management API.
package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/controlplane"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("invalid configuration: %v", err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cp := controlplane.New(cfg)
	if err := cp.Run(ctx); err != nil {
		log.Fatalf("control plane exited with error: %v", err)
	}
}
