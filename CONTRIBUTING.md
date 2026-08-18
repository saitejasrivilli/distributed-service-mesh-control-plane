# Contributing

This is primarily a portfolio project, but it's structured like a real
codebase and welcomes issues/PRs pointing out bugs or gaps.

## Development

```
go build ./...
go test ./...
go test -race ./...
go vet ./...
gofmt -l .
golangci-lint run ./...
```

All five must pass before a commit. See `.github/workflows/` for the exact
CI gate.

## Commit style

Conventional commits: `feat(scope): ...`, `fix(scope): ...`,
`test(scope): ...`, `docs(scope): ...`. See `git log` for examples from
this project's own release history.

## Release process

Each release (`vX.Y.0`) is scoped to one capability, documented in
`CHANGELOG.md`, and — where the capability touches a real architectural
decision — recorded as an ADR under `docs/adr/`. See `docs/architecture/`
for how releases build on each other.

## What not to send

- PRs that fabricate benchmark numbers or claim untested functionality —
  every number in this repo traces to a file under `test/benchmark/results/`
  or a script under `scripts/`.
- New AWS-specific integrations before the core project (through v1.0.0) is
  stable — see the README's "Future work" section.
