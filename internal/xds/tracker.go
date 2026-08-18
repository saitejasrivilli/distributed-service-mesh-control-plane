package xds

import (
	"context"
	"sync"
	"time"

	corev3 "github.com/envoyproxy/go-control-plane/envoy/config/core/v3"
	discoveryv3 "github.com/envoyproxy/go-control-plane/envoy/service/discovery/v3"
	serverv3 "github.com/envoyproxy/go-control-plane/pkg/server/v3"
)

// ConnectedEnvoy describes one currently-open xDS stream, for debug/
// observability purposes.
type ConnectedEnvoy struct {
	StreamID    int64
	NodeID      string
	ConnectedAt time.Time
}

// ConnectionTracker implements serverv3.Callbacks to track currently-open
// xDS streams. Safe for concurrent use.
type ConnectionTracker struct {
	mu      sync.RWMutex
	streams map[int64]*ConnectedEnvoy
	onOpen  func()
	onClose func()
}

// NewConnectionTracker constructs a tracker. onOpen/onClose, if non-nil, are
// invoked on each stream open/close (used to drive the
// controlplane_envoy_connections_total gauge without this package importing
// the metrics package directly).
func NewConnectionTracker(onOpen, onClose func()) *ConnectionTracker {
	return &ConnectionTracker{streams: make(map[int64]*ConnectedEnvoy), onOpen: onOpen, onClose: onClose}
}

// Callbacks returns a serverv3.Callbacks backed by this tracker.
func (t *ConnectionTracker) Callbacks() serverv3.Callbacks {
	return serverv3.CallbackFuncs{
		StreamOpenFunc: func(_ context.Context, streamID int64, _ string) error {
			t.mu.Lock()
			t.streams[streamID] = &ConnectedEnvoy{StreamID: streamID, ConnectedAt: time.Now()}
			t.mu.Unlock()
			if t.onOpen != nil {
				t.onOpen()
			}
			return nil
		},
		StreamClosedFunc: func(streamID int64, _ *corev3.Node) {
			t.mu.Lock()
			delete(t.streams, streamID)
			t.mu.Unlock()
			if t.onClose != nil {
				t.onClose()
			}
		},
		StreamRequestFunc: func(streamID int64, req *discoveryv3.DiscoveryRequest) error {
			t.mu.Lock()
			if entry, ok := t.streams[streamID]; ok && req.GetNode().GetId() != "" {
				entry.NodeID = req.GetNode().GetId()
			}
			t.mu.Unlock()
			return nil
		},
	}
}

// Connected returns a snapshot of currently-open streams.
func (t *ConnectionTracker) Connected() []ConnectedEnvoy {
	t.mu.RLock()
	defer t.mu.RUnlock()
	out := make([]ConnectedEnvoy, 0, len(t.streams))
	for _, c := range t.streams {
		out = append(out, *c)
	}
	return out
}

// Count returns the number of currently-open streams.
func (t *ConnectionTracker) Count() int {
	t.mu.RLock()
	defer t.mu.RUnlock()
	return len(t.streams)
}
