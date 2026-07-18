# Session synchronization

## Client model

Android keeps a local projection of each authorized session. The projection is disposable: the server snapshot and event journal are authoritative. Local drafts and UI preferences are separate and are never overwritten by event replay.

## Reconciliation

```text
connect -> authenticate -> send cursors -> replay or resync
       -> apply contiguous events -> acknowledge cursor -> live stream
```

If an event sequence is missing, the client stops applying later events for that session, requests replay, and falls back to a snapshot if the cursor has expired. It must show a resync indicator while doing so.

## Command reconciliation

Commands carry idempotency keys and a local pending status. On reconnect, the client asks the server for command outcome before retrying. A timeout is not proof of failure and must not cause blind duplicate prompts or approval decisions.

## Conflict policy

The server wins for session state. The client wins for unsent prompt drafts and local navigation. A server-rejected command remains visible with the rejection reason and is not silently removed.

