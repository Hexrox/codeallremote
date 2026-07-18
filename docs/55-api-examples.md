# API examples

Examples use synthetic identifiers and are illustrative contract fixtures, not production credentials.

## Create and start

```http
POST /api/v1/sessions
Idempotency-Key: 3f4f6f7e-0001
Authorization: Bearer <access-token>
Content-Type: application/json

{"workspace_id":"ws_demo","adapter_id":"claude-code","title":"Demo"}
```

The response contains a session snapshot. Starting a run is a separate command so clients can render the created session before process startup.

## Replay

```http
GET /api/v1/sessions/ses_demo/events?after=12&limit=100
Authorization: Bearer <access-token>
```

The response includes `events`, `next_after` and `resync_required`. Clients must not assume an empty page means the session has ended.

## Safe errors

```json
{
  "code": "approval_expired",
  "message": "The approval is no longer actionable.",
  "details": {"approval_id":"apr_demo"},
  "request_id":"req_demo"
}
```

