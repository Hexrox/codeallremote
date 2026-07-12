# M2 tasks — remote API and Android prerequisites

## M2-01 — Implement REST session API

**Inputs:** `docs/13-rest-api.md`.

**Acceptance criteria:**

- Endpoints expose snapshots, session commands, event replay and approval decisions.
- All write endpoints require an idempotency key.
- Error responses follow the documented shape.
- API tests cover authorization and retry behavior.

## M2-02 — Implement WebSocket replay and backpressure

**Inputs:** `docs/14-websocket-protocol.md`.

**Acceptance criteria:**

- Reconnect from a cursor yields no duplicates and no gaps within retained events.
- Old cursors return `resync_required`.
- Slow clients are disconnected safely without growing memory without bound.

## M2-03 — Implement device pairing and token revocation

**Inputs:** `docs/15-authentication-and-pairing.md`.

**Acceptance criteria:**

- A pairing challenge is single-use and expires.
- A revoked device cannot refresh or establish a WebSocket connection.
- Logs and notification payloads contain no tokens or secrets.

