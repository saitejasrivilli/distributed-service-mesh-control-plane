// Package registry implements an in-memory, thread-safe service registry:
// instance registration, deregistration, heartbeats, and health-aware
// endpoint listing. A persistent backend can later satisfy the same
// Registry interface without touching callers.
package registry

import (
	"errors"
	"sort"
	"sync"
	"time"
)

// Instance is one registered endpoint of a service.
type Instance struct {
	ServiceName   string
	Namespace     string
	InstanceID    string
	Address       string
	Port          int
	Protocol      string
	Version       string
	Metadata      map[string]string
	Healthy       bool
	RegisteredAt  time.Time
	LastHeartbeat time.Time
}

func key(namespace, name string) string { return namespace + "/" + name }

// ErrNotFound is returned when a service or instance lookup misses.
var ErrNotFound = errors.New("registry: not found")

// Registry is the interface future persistent implementations must satisfy.
type Registry interface {
	Register(inst Instance) error
	Deregister(namespace, serviceName, instanceID string) error
	Heartbeat(namespace, serviceName, instanceID string) error
	ListServices(namespace string) []string
	GetService(namespace, serviceName string) ([]Instance, error)
	HealthyInstances(namespace, serviceName string, staleAfter time.Duration) []Instance
	SweepStale(staleAfter time.Duration) []Instance
}

// InMemory is a Registry backed by an in-process map, safe for concurrent use.
type InMemory struct {
	mu   sync.RWMutex
	svcs map[string]map[string]Instance // key(ns,svc) -> instanceID -> Instance
	now  func() time.Time
}

// New constructs an empty in-memory registry.
func New() *InMemory {
	return &InMemory{
		svcs: make(map[string]map[string]Instance),
		now:  time.Now,
	}
}

// Register is idempotent: re-registering the same (namespace, service,
// instance ID) updates the existing entry rather than creating a duplicate.
func (r *InMemory) Register(inst Instance) error {
	if inst.ServiceName == "" || inst.InstanceID == "" {
		return errors.New("registry: service name and instance ID are required")
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	k := key(inst.Namespace, inst.ServiceName)
	if r.svcs[k] == nil {
		r.svcs[k] = make(map[string]Instance)
	}
	inst.RegisteredAt = r.now()
	inst.LastHeartbeat = inst.RegisteredAt
	if !inst.Healthy {
		inst.Healthy = true // instances register as healthy by default
	}
	r.svcs[k][inst.InstanceID] = inst
	return nil
}

// Deregister removes an instance. Removing an instance that does not exist
// is not an error (idempotent).
func (r *InMemory) Deregister(namespace, serviceName, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(namespace, serviceName)
	if r.svcs[k] == nil {
		return nil
	}
	delete(r.svcs[k], instanceID)
	return nil
}

// Heartbeat refreshes an instance's LastHeartbeat and marks it healthy.
func (r *InMemory) Heartbeat(namespace, serviceName, instanceID string) error {
	r.mu.Lock()
	defer r.mu.Unlock()
	k := key(namespace, serviceName)
	instances, ok := r.svcs[k]
	if !ok {
		return ErrNotFound
	}
	inst, ok := instances[instanceID]
	if !ok {
		return ErrNotFound
	}
	inst.LastHeartbeat = r.now()
	inst.Healthy = true
	instances[instanceID] = inst
	return nil
}

// ListServices returns the sorted names of every service with at least one
// instance registered in namespace.
func (r *InMemory) ListServices(namespace string) []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	prefix := namespace + "/"
	var names []string
	for k, instances := range r.svcs {
		if len(instances) == 0 {
			continue
		}
		if len(k) > len(prefix) && k[:len(prefix)] == prefix {
			names = append(names, k[len(prefix):])
		}
	}
	sort.Strings(names)
	return names
}

// GetService returns all instances (healthy or not) of a service, in
// deterministic (instance ID) order.
func (r *InMemory) GetService(namespace, serviceName string) ([]Instance, error) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k := key(namespace, serviceName)
	instances, ok := r.svcs[k]
	if !ok || len(instances) == 0 {
		return nil, ErrNotFound
	}
	return sortedInstances(instances), nil
}

// HealthyInstances returns instances marked healthy whose last heartbeat is
// within staleAfter of now, in deterministic order. An instance that has
// gone stale (no heartbeat within staleAfter) is excluded even if its
// Healthy flag hasn't been explicitly flipped yet.
func (r *InMemory) HealthyInstances(namespace, serviceName string, staleAfter time.Duration) []Instance {
	r.mu.RLock()
	defer r.mu.RUnlock()
	k := key(namespace, serviceName)
	instances := r.svcs[k]
	if len(instances) == 0 {
		return nil
	}
	now := r.now()
	var out []Instance
	for _, inst := range sortedInstances(instances) {
		if !inst.Healthy {
			continue
		}
		if staleAfter > 0 && now.Sub(inst.LastHeartbeat) > staleAfter {
			continue
		}
		out = append(out, inst)
	}
	return out
}

// SweepStale scans every service across all namespaces and transitions any
// instance whose last heartbeat exceeds staleAfter from Healthy=true to
// Healthy=false, persisting the state transition (not just filtering it out
// of reads, as HealthyInstances does). Returns the instances that just
// transitioned, for logging/metrics. A staleAfter of zero disables sweeping.
func (r *InMemory) SweepStale(staleAfter time.Duration) []Instance {
	if staleAfter <= 0 {
		return nil
	}
	r.mu.Lock()
	defer r.mu.Unlock()

	now := r.now()
	var transitioned []Instance
	for _, instances := range r.svcs {
		for id, inst := range instances {
			if inst.Healthy && now.Sub(inst.LastHeartbeat) > staleAfter {
				inst.Healthy = false
				instances[id] = inst
				transitioned = append(transitioned, inst)
			}
		}
	}
	sort.Slice(transitioned, func(i, j int) bool { return transitioned[i].InstanceID < transitioned[j].InstanceID })
	return transitioned
}

func sortedInstances(m map[string]Instance) []Instance {
	out := make([]Instance, 0, len(m))
	for _, v := range m {
		out = append(out, v)
	}
	sort.Slice(out, func(i, j int) bool { return out[i].InstanceID < out[j].InstanceID })
	return out
}
