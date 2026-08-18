// Package routing holds desired traffic-management configuration (weighted
// version splits, retries, timeouts, circuit breaking) per service, validated
// before it is ever handed to the xDS snapshot builder.
package routing

import (
	"errors"
	"fmt"
	"sync"
)

// VersionWeight is one version's share of traffic for a service, as a
// percentage. All weights for a service must sum to 100.
type VersionWeight struct {
	Version string `json:"version"`
	Weight  uint32 `json:"weight"`
}

// CircuitBreaker bounds resource usage per cluster. Zero fields fall back to
// Envoy's own defaults (1024 connections/pending/requests, 3 retries).
type CircuitBreaker struct {
	MaxConnections     uint32 `json:"max_connections,omitempty"`
	MaxPendingRequests uint32 `json:"max_pending_requests,omitempty"`
	MaxRequests        uint32 `json:"max_requests,omitempty"`
	MaxRetries         uint32 `json:"max_retries,omitempty"`
}

// Spec is the desired traffic-management configuration for one service.
type Spec struct {
	Service        string          `json:"service"`
	Splits         []VersionWeight `json:"splits"`
	RetryOn        string          `json:"retry_on,omitempty"` // e.g. "5xx", "connect-failure"; empty disables retries
	NumRetries     uint32          `json:"num_retries,omitempty"`
	TimeoutMs      uint32          `json:"timeout_ms,omitempty"` // 0 uses Envoy's default
	CircuitBreaker CircuitBreaker  `json:"circuit_breaker,omitempty"`
}

// Validate enforces the invariant that must hold before a Spec is ever
// published to Envoy: weights must be non-empty and sum to exactly 100.
func (s Spec) Validate() error {
	if s.Service == "" {
		return errors.New("routing: service is required")
	}
	if len(s.Splits) == 0 {
		return errors.New("routing: at least one version split is required")
	}
	seen := make(map[string]bool, len(s.Splits))
	var total uint32
	for _, sp := range s.Splits {
		// An empty Version is a legitimate label meaning "unversioned" —
		// registry instances default to Version == "" when not tagged, so a
		// route with a single unversioned split must be representable.
		if seen[sp.Version] {
			return fmt.Errorf("routing: duplicate version %q in splits", sp.Version)
		}
		seen[sp.Version] = true
		total += sp.Weight
	}
	if total != 100 {
		return fmt.Errorf("routing: split weights must sum to 100, got %d", total)
	}
	if s.RetryOn != "" && s.NumRetries == 0 {
		return errors.New("routing: num_retries must be > 0 when retry_on is set")
	}
	return nil
}

// Store is a thread-safe, in-memory holder of the current desired Spec per
// service. Only validated specs are ever stored.
type Store struct {
	mu    sync.RWMutex
	specs map[string]Spec
}

// NewStore constructs an empty route Store.
func NewStore() *Store {
	return &Store{specs: make(map[string]Spec)}
}

// Set validates spec and stores it, replacing any prior spec for the same
// service. Returns the validation error, if any, without mutating state.
func (s *Store) Set(spec Spec) error {
	if err := spec.Validate(); err != nil {
		return err
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	s.specs[spec.Service] = spec
	return nil
}

// Get returns the Spec for service and whether one is configured.
func (s *Store) Get(service string) (Spec, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	spec, ok := s.specs[service]
	return spec, ok
}

// Delete removes any configured Spec for service.
func (s *Store) Delete(service string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.specs, service)
}
