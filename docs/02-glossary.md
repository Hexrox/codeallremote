# Glossary

| Term | Meaning |
| --- | --- |
| **CAR server** | Homelab service that owns sessions, storage, API and event distribution. |
| **Adapter** | Integration translating a specific coding agent into CAR's common domain model. |
| **Wrapper** | The process-control layer that launches and supervises an agent CLI. |
| **Session** | Durable CAR record for one agent run in one workspace. |
| **Run** | A bounded period of active execution within a session. |
| **Workspace** | Registered local project directory and its execution policy. |
| **Approval** | A time-bounded request for a user decision before an action proceeds. |
| **Domain event** | Immutable fact emitted by CAR, such as `approval.requested`. |
| **Client** | Android, Web or CLI consumer of CAR's API. |
| **Gateway** | VPS reverse proxy that transports encrypted traffic through WireGuard. |

