# JSON Schema policy

## Purpose

Machine-readable schemas validate REST bodies, WebSocket envelopes and persisted event payloads. Each schema carries a `$id`, protocol version and explicit required fields.

## Event envelope

```json
{
  "$id": "car://schemas/event/v1",
  "type": "object",
  "required": ["type", "message_id", "session_id", "sequence", "payload"],
  "properties": {
    "type": {"type": "string"},
    "message_id": {"type": "string"},
    "session_id": {"type": "string"},
    "sequence": {"type": "integer", "minimum": 1},
    "schema_version": {"type": "integer", "minimum": 1},
    "payload": {"type": "object"}
  },
  "additionalProperties": false
}
```

## Validation rules

Schemas are validated in CI against representative fixtures. Unknown event types are preserved as opaque events by clients; malformed envelopes are rejected and diagnosed. Persisted events retain their original schema version so migrations can be explicit.

