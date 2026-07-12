# M1 tasks — local CAR core

These are implementation tasks for an agent. They intentionally exclude Android, VPS exposure and additional coding-agent integrations.

## M1-01 — Bootstrap CAR server

**Goal:** Create the server application skeleton and configuration loader.

**Inputs:** `docs/03-architecture.md`, `docs/05-design-principles.md`.

**Scope:** HTTP service, structured configuration, health endpoint, graceful startup/shutdown, structured logs. No Claude Code execution.

**Acceptance criteria:**

- Starts with an explicit local data directory and configuration file.
- `GET /api/v1/health` returns liveness without exposing secrets.
- Invalid configuration fails before opening a public listener.
- Shutdown stops accepting requests before closing storage.

**Out of scope:** Authentication, WebSocket, database schema beyond bootstrap migration support.

## M1-02 — Implement domain storage and event journal

**Goal:** Persist workspaces, sessions, runs and ordered events.

**Inputs:** `docs/04-domain-model.md`, `docs/10-session-lifecycle.md`, `docs/16-storage-and-retention.md`.

**Scope:** Migrations; repositories; transactional event append; per-session sequence allocation.

**Acceptance criteria:**

- A session and its initial event are committed atomically.
- Concurrent event writes never duplicate or reorder per-session sequences.
- Restarting the service reconstructs a session snapshot from persisted state.
- Repository tests use a temporary database and cover rollback on write failure.

**Out of scope:** Artifact compaction, PostgreSQL support, cloud backup.

## M1-03 — Implement session manager

**Goal:** Enforce CAR session/run lifecycle independently of a real adapter.

**Inputs:** `docs/10-session-lifecycle.md`, ADR-003.

**Scope:** State machine, idempotency-key storage, transition validation, domain event publication.

**Acceptance criteria:**

- Invalid transitions return a typed conflict error.
- Retries using the same idempotency key do not create a second session or run.
- Interrupt-versus-exit race yields one terminal state and an audit event.
- Every transition is persisted before being published to subscribers.

**Out of scope:** Claude parsing and process management.

## M1-04 — Define and implement adapter boundary

**Goal:** Provide a versioned agent adapter interface plus a fake adapter for tests.

**Inputs:** ADR-005, `docs/11-claude-code-adapter.md`.

**Scope:** Capabilities, start/input/interrupt/observe/recover contract, adapter registry and fake test implementation.

**Acceptance criteria:**

- Core has no Claude Code-specific imports or branches.
- Unsupported capability produces an explicit typed result.
- Fake adapter can emit output, approval and completion signals deterministically.
- Contract tests verify each adapter signal maps to a documented domain event.

**Out of scope:** Production PTY wrapper.

