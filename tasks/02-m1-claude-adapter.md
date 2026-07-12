# M1 tasks — Claude Code adapter

## M1-05 — Process wrapper

**Goal:** Start and supervise Claude Code in a registered workspace.

**Inputs:** ADR-001, `docs/11-claude-code-adapter.md`.

**Scope:** Executable discovery, canonical workspace validation, process-group ownership, stdout/stderr capture, exit reporting and controlled interruption.

**Acceptance criteria:**

- Process starts only in a workspace registered by CAR.
- Process metadata excludes provider credentials and secret environment values.
- Interrupt terminates only the run's process group.
- Process exit produces one durable completion/failure event.
- Integration tests use a deterministic fake CLI; they do not require a real provider account.

## M1-06 — Normalize Claude Code output

**Goal:** Translate supported Claude Code output into CAR signals without leaking parser details into core.

**Inputs:** `docs/11-claude-code-adapter.md`, `docs/14-websocket-protocol.md`.

**Scope:** Version detection, output chunking, lifecycle detection, diagnostics and compatibility-degraded mode.

**Acceptance criteria:**

- Raw output ordering is preserved.
- Unsupported CLI version emits an explicit diagnostic.
- Parser failure does not claim completion or approval.
- Fixtures cover normal output, malformed output and abrupt process exit.

## M1-07 — Approval bridge

**Goal:** Convert supported agent approval prompts into CAR approval records and resume/deny actions.

**Inputs:** `docs/12-remote-approvals.md`.

**Acceptance criteria:**

- Pending approval has expiry and structured context.
- Duplicate or late decisions never write twice to the child process.
- Adapter exit cancels pending approval.
- Tests cover approve, deny, expiry and process exit races.

