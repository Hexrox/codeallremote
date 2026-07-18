# Development Guide

## Quick Start

```bash
# Build the server
go build -o car ./cmd/server

# Run with configuration
./car -config testdata/config.json

# Run tests
go test ./...

# Run tests with coverage
go test -cover ./...

# Run specific package tests
go test ./internal/config/...
go test ./internal/server/...
```

## Project Structure

```
.
├── cmd/
│   └── server/          # Server entry point
│       └── carctl/       # carctl backup/restore CLI
├── internal/
│   ├── adapter/         # Adapter contract + fake + claude + codex
│   ├── api/              # REST + pairing handlers
│   ├── app/              # Service composition (sessions, approvals, etc.)
│   ├── approval/        # Approval bridge + stores
│   ├── audit/           # Audit writer with recursive redaction
│   ├── auth/            # Constant-time token compare / Bearer parsing
│   ├── config/          # Configuration loading and validation
│   ├── domain/          # Domain models
│   ├── errcat/          # Stable API error catalog
│   ├── identity/        # Device pairing, tokens, revocation
│   ├── lifecycle/       # Graceful shutdown + startup reconciliation
│   ├── obs/             # Metrics, correlation IDs, liveness/readiness
│   ├── plugin/          # Plugin manifest + registry
│   ├── projection/      # Session snapshot read model
│   ├── protocol/        # Command dispatcher + negotiation + sync harness
│   ├── server/          # HTTP server, /health, /ready, route registration
│   ├── session/         # State machine + idempotency store
│   ├── storage/         # SQLite + migrations + repositories
│   ├── workspace/       # Workspace registration + path policy
│   ├── wrapper/         # Process wrapper (process group, signals)
│   └── ws/              # WebSocket gateway (replay, backpressure, resync)
├── sdk/                 # Public adapter plugin SDK contract
├── android/             # Android client (Kotlin/Compose + REST/WS SDK)
├── deploy/              # Dockerfile, systemd, WireGuard/Caddy, backup runbook
├── schemas/             # JSON schemas (event/session/approval/error/config)
├── scripts/             # check-schema, check-docs, ci.sh
├── testdata/            # Test fixtures
└── docs/                # Documentation
```

Android is a Gradle module under `android/`; see `android/README.md` (the
app module's structure is under `android/app/src/main/java/io/codeallremote/car/android/`).


## Configuration

See `testdata/config.json` for an example configuration file.

Required fields:
- `server.host` and `server.port`
- `storage.type` ("sqlite" or "postgres") and `storage.data_source`
- `security.api_token` (minimum 16 characters)

## Testing

Tests use the standard Go testing package. Run all tests:

```bash
go test ./...
```

For verbose output:

```bash
go test -v ./...
```

## Code Guidelines

- Follow Go idioms and `go fmt` formatting
- Validate configuration at startup
- Use structured logging with `slog`
- Handle errors explicitly
- Write tests for all acceptance criteria

## Milestone status (in_review)

Implementation complete through M5. Status is `in_review`: acceptance
evidence in `tasks/25-post-review-closure.md`; `accepted` only after review
sign-off and a green CI run (including Android instrumented tests).

- M1 (single-user core): bootstrap, storage/event journal, session manager,
  adapter boundary + fake, wrapper, output normalization, approval bridge,
  workspace policy, snapshot projection, cursor repository, audit,
  graceful shutdown/reconciliation — implemented & tested.
- M2 (remote API + Android): REST API, WebSocket gateway, device pairing &
  revocation, error catalog, WS hardening; Android SDK + screens + ViewModels
  + notifications — implemented & tested.
- M3 (deployment): Dockerfile/systemd/health+ready, WireGuard/VPS reverse
  proxy, `carctl` backup/restore — implemented; WireGuard/VPS are config files.
- M4 (platform): plugin registry, observability, failure test suite, CI/CD,
  command dispatcher, protocol negotiation, synchronization harness — implemented.
- M5 (extensibility): adapter SDK contract, Codex skeleton (no real process),
  real Claude Code adapter (ProcessWrapper + OutputParser + stdin approvals).

Known limitations and follow-ups are enumerated in
`tasks/25-post-review-closure.md` §1 and §5.
