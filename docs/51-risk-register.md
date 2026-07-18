# Risk register

| ID | Risk | Impact | Mitigation | Owner stage |
| --- | --- | --- | --- | --- |
| R-01 | Claude Code output changes | Adapter stops normalizing events | Version detection, fixtures, degraded mode | M1 |
| R-02 | Mobile network loss during command | Duplicate prompt or unclear outcome | Idempotency keys and reconciliation | M2 |
| R-03 | VPS compromise | Exposure of transport metadata | Stateless gateway and WireGuard | M3 |
| R-04 | Secret in transcript/log | Credential compromise | Redaction before persistence and delivery | M1/M4 |
| R-05 | SQLite file corruption | Lost session state | Transactional writes, backups, restore drills | M1/M3 |
| R-06 | Approval spoofing | Unauthorized command execution | Server authorization and device binding | M2 |
| R-07 | Scope creep to multi-agent platform | Delayed usable MVP | M1 Claude adapter first, ADR for expansion | All |

Risks are reviewed at every milestone gate and closed only with evidence, not with a design statement alone.

