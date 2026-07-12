# Product requirements

## Functional requirements

| ID | Requirement |
| --- | --- |
| FR-1 | The operator can register a workspace and start a Claude Code session. |
| FR-2 | A session streams normalized output and lifecycle events to connected clients. |
| FR-3 | The operator can submit prompts, interrupt a run, and resume a known session. |
| FR-4 | CAR exposes a pending approval with command, risk context and expiry; only an authorized user can decide it. |
| FR-5 | Android shows active sessions, pending approvals, transcript, terminal output and changed-file summary. |
| FR-6 | CAR records an append-only audit trail for authority-changing actions. |
| FR-7 | The server can host more than one adapter, even though only Claude Code is implemented first. |

## Non-functional requirements

- **Privacy:** workspace content and session history stay in the homelab.
- **Resilience:** transient client or tunnel loss does not terminate a running agent.
- **Security:** all remote access uses TLS plus authenticated, revocable clients.
- **Latency:** a connected client should normally receive live events within one second.
- **Compatibility:** adapter parsing must be versioned and fail visibly when unsupported.

## Success criteria for the MVP

One authenticated Android user can remotely run a Claude Code session in a registered workspace, receive an approval notification, approve or deny it, and review the resulting transcript and diff after reconnecting.

