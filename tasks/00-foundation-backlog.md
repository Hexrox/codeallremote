# Foundation backlog

## T-001: Define adapter interface

**Goal:** Produce a versioned interface and capability document for the Claude Code adapter.

**Acceptance criteria:**

- Covers start, input, interrupt, output, approval, exit and restore behavior.
- States how unsupported capabilities are represented.
- Does not expose client or storage concerns.

## T-002: Specify session lifecycle

**Goal:** Define session and run state machines, permitted transitions and restart recovery.

**Acceptance criteria:**

- Includes terminal states and orphan-process handling.
- Defines event emission for each transition.
- Includes reconnect and server-restart scenarios.

## T-003: Specify API and event protocol

**Goal:** Define HTTP resources, WebSocket envelope, authentication and replay cursor semantics.

**Acceptance criteria:**

- Android can recover from a dropped connection without duplicate commands.
- All mutating commands are idempotency-key aware.
- Approval decisions are authorized and auditable.

## T-004: Design Android MVP

**Goal:** Specify navigation, offline state and approval notification flows.

**Acceptance criteria:**

- Covers dashboard, session detail, prompt entry, live output and approval decision.
- Defines empty, loading, disconnected and error states.
- Maps each screen to server contracts.

