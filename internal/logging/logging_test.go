package logging

import (
	"context"
	"testing"
)

func TestCorrelationIDRoundTrip(t *testing.T) {
	ctx := WithCorrelationID(context.Background(), "req-123")
	if got := CorrelationID(ctx); got != "req-123" {
		t.Fatalf("CorrelationID = %q, want req-123", got)
	}
}

func TestCorrelationIDEmptyWhenUnset(t *testing.T) {
	if got := CorrelationID(context.Background()); got != "" {
		t.Fatalf("CorrelationID = %q, want empty", got)
	}
}

func TestNewDoesNotPanicOnUnknownLevel(t *testing.T) {
	l := New("nonsense")
	if l == nil {
		t.Fatal("expected non-nil logger")
	}
}
