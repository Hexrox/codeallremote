# Decision log

This is a compact index of durable decisions. Detailed rationale belongs in `adr/`.

| Decision | ADR | Current position |
| --- | --- | --- |
| Integrate through a wrapper | ADR-001 | Do not fork Claude Code. |
| Android is primary | ADR-002 | Design API and UX mobile-first. |
| Durable events | ADR-003 | Persist ordered events before publish. |
| Local-first gateway | ADR-004 | VPS transports; homelab stores. |
| Agent-neutral boundary | ADR-005 | Core talks to versioned adapters. |

Open decisions requiring a future ADR include backend language, Android persistence library, exact token protocol, artifact encryption implementation and plugin process isolation.

