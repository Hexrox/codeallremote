# M4 tasks — CAR Protocol

## M4-05 — Define command dispatcher

**Inputs:** `docs/33-car-protocol.md`.

**Acceptance criteria:**

- Commands are authenticated, schema-validated and idempotency-aware.
- Accepted commands emit the documented lifecycle events.
- Unsupported commands return a stable error without reaching an adapter.

## M4-06 — Implement protocol negotiation

**Inputs:** `docs/34-protocol-versioning.md`.

**Acceptance criteria:**

- Incompatible clients receive a safe diagnostic and no session data.
- Additive fields remain readable by the previous supported client.
- Breaking changes have migration fixtures and rollback documentation.

## M4-07 — Implement synchronization test harness

**Inputs:** `docs/35-session-synchronization.md`, `docs/36-protocol-fixtures.md`.

**Acceptance criteria:**

- Replay produces no duplicate or missing session events.
- Expired cursors trigger snapshot resync.
- Timed-out commands are reconciled before retry.
- Unsynced Android drafts are never overwritten by server state.

