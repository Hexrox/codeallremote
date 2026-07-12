# WebSocket protocol

## Purpose

The WebSocket connection distributes real-time session events after authentication. It is an optimization for live use, not the sole source of truth: a client recovers through snapshots plus REST event replay.

## Connection

The client connects to `/api/v1/ws` with a short-lived access token. It sends a `hello` envelope containing supported protocol versions, device ID and last received cursors. The server replies with `welcome`, then subscribes the client to permitted session events.

```json
{
  "type": "hello",
  "protocol_version": 1,
  "device_id": "dev_01",
  "cursors": [{ "session_id": "ses_01", "after": 39 }]
}
```

## Envelope

Every message uses this shape:

```json
{
  "type": "session.output",
  "message_id": "msg_01",
  "occurred_at": "2026-07-12T09:30:01Z",
  "session_id": "ses_01",
  "sequence": 40,
  "payload": {}
}
```

`sequence` is strictly increasing within a session. Clients deduplicate by `(session_id, sequence)` and acknowledge the highest contiguous sequence after persistence to local UI state.

## Core event types

| Type | Meaning |
| --- | --- |
| `session.state_changed` | Durable lifecycle transition. |
| `run.output` | Terminal or transcript chunk; may be batchable. |
| `run.started` / `run.completed` | Run boundary and outcome. |
| `approval.requested` | A decision is required; fetch details through REST. |
| `approval.resolved` | Final approval state and actor summary. |
| `workspace.changed_files` | Best-effort changed-file summary. |
| `adapter.diagnostic` | Compatibility or wrapper diagnostic. |

## Reconnect algorithm

1. Android stores the last contiguous sequence per session.
2. On reconnect, it sends those cursors in `hello`.
3. Server replays available events, then switches to live delivery.
4. If a cursor is older than retention, server emits `resync_required`; the client fetches a snapshot and timeline page before subscribing again.

## Backpressure

Terminal output can be high volume. The server MAY coalesce adjacent `run.output` chunks but MUST preserve byte order within a stream. A slow client is disconnected with a resumable cursor rather than causing unbounded server memory growth.

