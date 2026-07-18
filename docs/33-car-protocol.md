# CAR Protocol

## Purpose

CAR Protocol is the agent-neutral contract between CAR core and clients. REST provides snapshots and commands; WebSocket provides ordered live events. Agent adapters never expose their private wire format to clients.

## Message classes

- **Command:** an authenticated client intent, such as `session.start`, `session.prompt`, `session.interrupt` or `approval.decide`.
- **Event:** an immutable fact emitted by the server, such as `run.started`, `run.output`, `approval.requested` or `run.completed`.
- **Snapshot:** authoritative current state for a resource, used after reconnect or resync.
- **Diagnostic:** non-secret compatibility, health or policy information.

## Command envelope

```json
{
  "command_id": "cmd_01",
  "type": "session.prompt",
  "session_id": "ses_01",
  "idempotency_key": "client-generated-uuid",
  "payload": { "text": "Run the focused tests." }
}
```

Commands are accepted or rejected synchronously, but their effects are observed through events. A client must not infer that a run started merely because an HTTP request returned `202`.

## Event guarantees

Events are ordered per session, durable according to the retention policy, and immutable. Delivery is at-least-once; clients deduplicate by session sequence. Cross-session ordering is not guaranteed.

## Approval semantics

An approval request pauses or constrains the relevant adapter operation. Only the server can transition an approval to `approved`, `denied`, `expired` or `cancelled`. Clients cannot manufacture approval events.

