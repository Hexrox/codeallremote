# REST API

## Scope

REST provides snapshots and commands. Live output travels through WebSocket; a REST response never requires a connected socket to be useful.

## Conventions

- Base path: `/api/v1`.
- JSON request and response bodies use UTF-8.
- Authenticated write requests require `Idempotency-Key`.
- Every response includes `request_id`; errors use `{ "code", "message", "details", "request_id" }`.
- Server IDs are opaque UUIDs. A client MUST NOT infer semantics from them.

## Resources

| Method | Path | Purpose |
| --- | --- | --- |
| GET | `/health` | Unauthenticated liveness check for local monitoring. |
| GET | `/me` | Authenticated user and device summary. |
| GET / POST | `/workspaces` | List or register permitted workspaces. |
| GET | `/workspaces/{id}` | Workspace policy and current session summary. |
| GET / POST | `/sessions` | List sessions or create a session. |
| GET | `/sessions/{id}` | Authoritative session snapshot. |
| POST | `/sessions/{id}/runs` | Start or resume a run. |
| POST | `/sessions/{id}/prompts` | Submit operator input to an active run. |
| POST | `/sessions/{id}/interrupt` | Request interruption of the active run. |
| GET | `/sessions/{id}/events?after={sequence}` | Replay durable events. |
| GET | `/approvals/{id}` | Full approval context. |
| POST | `/approvals/{id}/decision` | Approve or deny a pending approval. |

## Create session

`POST /sessions`

```json
{
  "workspace_id": "ws_01",
  "adapter_id": "claude-code",
  "title": "Fix authentication regression"
}
```

The server validates workspace policy and adapter availability. It returns `201 Created` with a `SessionSnapshot`; creation does not start a process until the client calls `POST /sessions/{id}/runs`.

## Submit a prompt

`POST /sessions/{id}/prompts`

```json
{
  "text": "Inspect the failing login test and propose a minimal fix."
}
```

The response is `202 Accepted`. It means the core has accepted the command; subsequent lifecycle and output events establish the run outcome.

## Resolve approval

`POST /approvals/{id}/decision`

```json
{
  "decision": "deny",
  "reason": "Do not push directly to main."
}
```

Allowed values are `approve` and `deny` for the MVP. A late or duplicate decision returns the final approval state and MUST NOT alter the adapter twice.

## Snapshot shape

```json
{
  "id": "ses_01",
  "workspace_id": "ws_01",
  "adapter_id": "claude-code",
  "state": "waiting_approval",
  "active_run": { "id": "run_01", "state": "active" },
  "last_sequence": 42,
  "pending_approval_id": "apr_01",
  "updated_at": "2026-07-12T09:30:00Z"
}
```

## Authorization

The owner role may register workspaces, start and control sessions, and decide approvals. Read-only roles are future work; endpoints MUST already check an explicit permission rather than assume every authenticated identity is an owner.

