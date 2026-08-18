package registry

import (
	"sync"
	"testing"
	"time"
)

func inst(id string) Instance {
	return Instance{ServiceName: "svc-a", Namespace: "default", InstanceID: id, Address: "10.0.0.1", Port: 8080}
}

func TestRegisterAndGetService(t *testing.T) {
	r := New()
	if err := r.Register(inst("i1")); err != nil {
		t.Fatalf("Register: %v", err)
	}
	got, err := r.GetService("default", "svc-a")
	if err != nil {
		t.Fatalf("GetService: %v", err)
	}
	if len(got) != 1 || got[0].InstanceID != "i1" {
		t.Fatalf("got %+v", got)
	}
}

func TestRegisterRequiresServiceAndInstanceID(t *testing.T) {
	r := New()
	if err := r.Register(Instance{}); err == nil {
		t.Fatal("expected error for empty service/instance ID")
	}
}

func TestDuplicateRegistrationIsIdempotent(t *testing.T) {
	r := New()
	_ = r.Register(inst("i1"))
	_ = r.Register(inst("i1"))
	got, _ := r.GetService("default", "svc-a")
	if len(got) != 1 {
		t.Fatalf("expected 1 instance after duplicate register, got %d", len(got))
	}
}

func TestDeregister(t *testing.T) {
	r := New()
	_ = r.Register(inst("i1"))
	if err := r.Deregister("default", "svc-a", "i1"); err != nil {
		t.Fatalf("Deregister: %v", err)
	}
	if _, err := r.GetService("default", "svc-a"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound after deregister, got %v", err)
	}
}

func TestDeregisterNonexistentIsNotError(t *testing.T) {
	r := New()
	if err := r.Deregister("default", "svc-a", "nope"); err != nil {
		t.Fatalf("expected nil error, got %v", err)
	}
}

func TestHeartbeatUpdatesLastHeartbeat(t *testing.T) {
	r := New()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return fixed }
	_ = r.Register(inst("i1"))

	later := fixed.Add(5 * time.Second)
	r.now = func() time.Time { return later }
	if err := r.Heartbeat("default", "svc-a", "i1"); err != nil {
		t.Fatalf("Heartbeat: %v", err)
	}
	got, _ := r.GetService("default", "svc-a")
	if !got[0].LastHeartbeat.Equal(later) {
		t.Fatalf("LastHeartbeat = %v, want %v", got[0].LastHeartbeat, later)
	}
}

func TestHeartbeatUnknownInstanceReturnsNotFound(t *testing.T) {
	r := New()
	_ = r.Register(inst("i1"))
	if err := r.Heartbeat("default", "svc-a", "unknown"); err != ErrNotFound {
		t.Fatalf("expected ErrNotFound, got %v", err)
	}
}

func TestStaleInstanceExcludedFromHealthy(t *testing.T) {
	r := New()
	fixed := time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)
	r.now = func() time.Time { return fixed }
	_ = r.Register(inst("i1"))

	// Advance time beyond the staleness threshold without a heartbeat.
	r.now = func() time.Time { return fixed.Add(1 * time.Hour) }
	healthy := r.HealthyInstances("default", "svc-a", 10*time.Second)
	if len(healthy) != 0 {
		t.Fatalf("expected 0 healthy instances after going stale, got %d", len(healthy))
	}
}

func TestHealthyInstancesExcludesUnhealthy(t *testing.T) {
	r := New()
	_ = r.Register(inst("i1"))
	_ = r.Register(inst("i2"))
	// Directly mark i2 unhealthy via re-register through internal map for test purposes.
	r.mu.Lock()
	m := r.svcs[key("default", "svc-a")]
	e := m["i2"]
	e.Healthy = false
	m["i2"] = e
	r.mu.Unlock()

	healthy := r.HealthyInstances("default", "svc-a", 0)
	if len(healthy) != 1 || healthy[0].InstanceID != "i1" {
		t.Fatalf("got %+v", healthy)
	}
}

func TestNamespaceIsolation(t *testing.T) {
	r := New()
	_ = r.Register(Instance{ServiceName: "svc-a", Namespace: "ns1", InstanceID: "i1"})
	_ = r.Register(Instance{ServiceName: "svc-a", Namespace: "ns2", InstanceID: "i2"})

	got1, err := r.GetService("ns1", "svc-a")
	if err != nil || len(got1) != 1 || got1[0].InstanceID != "i1" {
		t.Fatalf("ns1 got %+v, err %v", got1, err)
	}
	got2, err := r.GetService("ns2", "svc-a")
	if err != nil || len(got2) != 1 || got2[0].InstanceID != "i2" {
		t.Fatalf("ns2 got %+v, err %v", got2, err)
	}
}

func TestListServicesDeterministicOrder(t *testing.T) {
	r := New()
	_ = r.Register(Instance{ServiceName: "svc-b", Namespace: "default", InstanceID: "i1"})
	_ = r.Register(Instance{ServiceName: "svc-a", Namespace: "default", InstanceID: "i1"})
	got := r.ListServices("default")
	if len(got) != 2 || got[0] != "svc-a" || got[1] != "svc-b" {
		t.Fatalf("got %v, want [svc-a svc-b]", got)
	}
}

func TestConcurrentRegistrationAndDeregistration(t *testing.T) {
	r := New()
	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(2)
		id := string(rune('a' + i%26))
		go func(id string) {
			defer wg.Done()
			_ = r.Register(Instance{ServiceName: "svc-a", Namespace: "default", InstanceID: id})
		}(id)
		go func(id string) {
			defer wg.Done()
			_ = r.Deregister("default", "svc-a", id)
		}(id)
	}
	wg.Wait() // must not panic or deadlock under -race
}

func TestRegistryRestartBehaviorStartsEmpty(t *testing.T) {
	r := New()
	_ = r.Register(inst("i1"))
	// Simulate restart: a fresh in-memory registry has no prior state.
	fresh := New()
	if _, err := fresh.GetService("default", "svc-a"); err != ErrNotFound {
		t.Fatalf("expected fresh registry to be empty, got err %v", err)
	}
}
