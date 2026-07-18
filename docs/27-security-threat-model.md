# Security threat model

## Assets

Workspace source, agent transcripts, approval authority, Git credentials, provider credentials, device tokens, audit history and homelab network access.

## Main threats and controls

| Threat | Control |
| --- | --- |
| Stolen phone | Device revocation, short-lived tokens, biometric confirmation for approvals |
| Compromised VPS | Stateless gateway, WireGuard, end-to-end CAR authorization |
| Malicious prompt/action | Explicit approvals, workspace policy, audit trail |
| Path traversal | Registered canonical workspace paths and server-generated artifact paths |
| Secret leakage | Environment redaction, notification minimization, log filters |
| Replay/duplicate command | TLS, device tokens, idempotency keys and server sequence checks |
| Agent process escape | Process-group ownership, least-privilege service account and filesystem policy |

## Security invariants

- No public endpoint can execute a shell command directly.
- An approval decision is bound to one approval ID and one authorized actor.
- The VPS never stores workspace content or durable credentials.
- Audit records identify authority-changing actions without duplicating secrets.

## Review cadence

Threat assumptions are reviewed before enabling multi-user access, unattended task queues, third-party plugins or arbitrary remote file editing.

