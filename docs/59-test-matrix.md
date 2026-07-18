# Test matrix

| Area | Happy path | Failure path | Evidence |
| --- | --- | --- | --- |
| Session | start, prompt, complete | crash, restart, interrupt race | event journal + state assertions |
| Approval | approve/deny | expiry, duplicate, adapter exit | audit + final state |
| Transport | live WebSocket | loss, cursor gap, slow client | replay/resync logs |
| Auth | pair and refresh | revoke, expired token | authorization tests |
| Android | navigate and act | offline, stale, accessibility | device/UI tests |
| Storage | migration and backup | disk full, restore | migration/restore report |
| Deployment | proxy through WireGuard | VPS loss, CAR not ready | runbook evidence |

Every row must have a deterministic fixture or an explicitly documented manual test procedure.

