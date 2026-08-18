package controlplane

import (
	"context"
	"testing"
	"time"

	"github.com/saitejasrivillibhutturu/distributed-service-mesh-control-plane/internal/config"
)

func TestRunStopsOnContextCancel(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	cp := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cp.Run(ctx) }()

	time.Sleep(50 * time.Millisecond)
	cancel()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Run returned error: %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return after context cancel")
	}
}

func TestRunTimesOutOnUnreachableShutdown(t *testing.T) {
	cfg := config.Default()
	cfg.HTTPAddr = "127.0.0.1:0"
	cfg.ShutdownTimeout = 1 * time.Millisecond
	cp := New(cfg)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() { done <- cp.Run(ctx) }()

	time.Sleep(20 * time.Millisecond)
	cancel()

	select {
	case <-done:
		// Either a clean shutdown or a timeout error is acceptable; the
		// important invariant is that Run always returns promptly.
	case <-time.After(3 * time.Second):
		t.Fatal("Run did not return within bounded time")
	}
}
