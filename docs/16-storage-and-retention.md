# Storage and retention

CAR stores all durable application data in the homelab. MVP uses SQLite for transactional state and an artifact directory for terminal/transcript payloads; PostgreSQL is a later migration option.

## Required records

- `workspaces`, `sessions`, `runs`, `approvals`, `approval_decisions` and paired `devices`.
- Append-only per-session `events` with sequence number and payload schema version.
- `artifacts` with content address, local path, size, MIME type and retention class.

## Retention and backup

Audit records are retained indefinitely. Session metadata is retained indefinitely. Terminal and transcript artifacts default to 90 days and can have a shorter per-workspace policy. Database and artifact backups MUST be encrypted and restored together. CAR does not back up workspace repositories themselves.

## Security

Client-supplied paths are never used for artifact writes. Secret redaction occurs before event persistence. Deleting an artifact leaves an audit-preserving deletion marker.

