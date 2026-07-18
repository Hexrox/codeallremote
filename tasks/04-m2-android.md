# M2 tasks — Android client

## M2-04 — Build Android navigation shell

**Inputs:** `docs/17-android-product.md`, `docs/18-android-navigation.md`.

**Acceptance criteria:**

- Deep links validate server and resource authorization.
- Home, workspace and session destinations have loading, empty, error and disconnected states.
- Screen state is derived from server snapshots/events, not duplicated business logic.

## M2-05 — Implement session supervision view

**Acceptance criteria:**

- Shows lifecycle state, transcript/output, timeline and pending approval indicator.
- Prompt drafts survive rotation and remain visibly unsent while offline.
- Output remains available when navigating between session tabs.

## M2-06 — Implement approval notification flow

**Inputs:** `docs/12-remote-approvals.md`, `docs/19-android-offline-notifications.md`.

**Acceptance criteria:**

- Push payload contains identifiers only, never sensitive content.
- Opening a resolved/expired approval shows its final state.
- Approve/deny requires confirmation and handles duplicate requests safely.

## M2-07 — Implement reconnect and replay

**Acceptance criteria:**

- Client persists per-session cursors.
- Reconnect replays without gaps or duplicates.
- Resync is visible and does not silently overwrite a newer local draft.

