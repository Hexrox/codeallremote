# Milestone gates

## Gate M0 → M1

No unresolved conflict exists between ADRs, domain model and task contracts. The agent can implement a fake adapter without inventing client-visible behavior.

## Gate M1 → M2

Core lifecycle, event journal, approval semantics and adapter wrapper pass failure-path tests. The local API exposes enough authoritative state for Android without terminal parsing.

## Gate M2 → M3

Android reconnect, token revocation and approval expiry are verified. No command can be duplicated by a network retry. Notification payload privacy is reviewed.

## Gate M3 → M4

WireGuard/proxy topology, encrypted backup and restore drill are proven. CAR is not publicly reachable except through intended transport.

## Gate M4 → M5

Security findings are triaged, CI gates are enforced and protocol compatibility is documented. Adding an adapter does not require changing Android domain models.

