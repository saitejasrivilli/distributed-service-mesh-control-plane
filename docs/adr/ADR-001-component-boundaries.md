# ADR-001: Component Boundaries

## Context

The control plane will grow to include a service registry, reconciliation
loop, xDS servers (CDS/EDS/LDS/RDS), and traffic-management logic across nine
planned releases. Getting the v0.1.0 package boundaries wrong makes every
later release harder to slot in cleanly.

## Problem

How should the initial skeleton be decomposed so that registry, xDS, and
routing logic can be added later without restructuring the HTTP/lifecycle
layer?

## Options

1. **Single `main.go`, no internal packages** — fastest to write, but
   entangles config/logging/metrics/HTTP concerns and makes later components
   hard to test in isolation.
2. **Package-per-concern under `internal/`, wired by a top-level orchestrator
   (`internal/controlplane`)** — each concern (config, logging, metrics, api)
   is independently testable; the orchestrator is the only place that knows
   how they fit together.
3. **Framework-style DI container** — over-engineered for a project at this
   stage; adds indirection with no current benefit.

## Decision

Option 2. `internal/controlplane.New(cfg)` constructs and injects logger,
metrics registry, and readiness checker into `internal/api.Server` via plain
constructor injection (no DI framework). `internal/api` depends only on
`internal/config`, `internal/logging`, `internal/metrics` — it does not know
about the registry or xDS packages that will be added in v0.2.0+.

## Trade-offs

- More files/packages than a single-file skeleton, but each is small and has
  a single reason to change.
- Constructor injection is a bit more verbose to wire up than a global
  singleton, but avoids the duplicate-registration bug encountered with a
  shared global Prometheus registry, and keeps every component testable
  without a running process.

## Consequences

Future releases add new `internal/<concern>` packages (registry,
reconciliation, xds/*) and wire them into `internal/controlplane` the same
way — the HTTP/lifecycle layer in `internal/api` should need minimal changes.
