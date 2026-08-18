package xds

import (
	"context"
	"testing"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
)

func TestConnectionTrackerTracksOpenAndClose(t *testing.T) {
	var opens, closes int
	tr := NewConnectionTracker(func() { opens++ }, func() { closes++ })
	cb := tr.Callbacks()

	if err := cb.OnStreamOpen(context.Background(), 1, ""); err != nil {
		t.Fatalf("OnStreamOpen: %v", err)
	}
	if tr.Count() != 1 {
		t.Fatalf("Count() = %d, want 1", tr.Count())
	}
	if opens != 1 {
		t.Fatalf("opens = %d, want 1", opens)
	}

	cb.OnStreamClosed(1, nil)
	if tr.Count() != 0 {
		t.Fatalf("Count() = %d, want 0 after close", tr.Count())
	}
	if closes != 1 {
		t.Fatalf("closes = %d, want 1", closes)
	}
}

func TestConnectionTrackerRecordsNodeID(t *testing.T) {
	tr := NewConnectionTracker(nil, nil)
	cb := tr.Callbacks()
	_ = cb.OnStreamOpen(context.Background(), 1, "")
	_ = cb.OnStreamRequest(1, &discoveryv3.DiscoveryRequest{Node: &corev3.Node{Id: "demo-envoy"}})

	connected := tr.Connected()
	if len(connected) != 1 {
		t.Fatalf("Connected() len = %d, want 1", len(connected))
	}
	if connected[0].NodeID != "demo-envoy" {
		t.Errorf("NodeID = %q, want demo-envoy", connected[0].NodeID)
	}
}
