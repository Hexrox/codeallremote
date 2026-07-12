# MVP definition

## Scope

The MVP proves the complete remote-control loop for one trusted operator and Claude Code.

### Included

- One CAR server in the homelab.
- Registered local workspaces with explicit path policy.
- Claude Code process wrapper and adapter.
- Session creation, prompt submission, interrupt, reconnect and terminal/transcript viewing.
- Approval requests with approve/deny/expiry and audit entries.
- Android client with session list, session detail and actionable notifications.
- TLS endpoint through WireGuard/VPS gateway.

### Deferred

- Multi-user collaboration and role administration beyond the owner.
- Arbitrary remote filesystem editing.
- iOS application, marketplace plugins, task scheduling and cost analytics.
- Guaranteed support for all Claude Code versions or all third-party providers.

## Acceptance scenario

From Android, the owner opens a session in a registered homelab workspace, asks Claude Code to modify a file, sees streaming progress, resolves a requested approval, loses and restores network connectivity, and reviews the resulting changed-file list and audit trail.

