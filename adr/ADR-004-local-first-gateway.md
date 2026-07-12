# ADR-004: Local-first deployment with a stateless VPS gateway

**Status:** Accepted

## Decision

CAR server, agent processes, storage and workspaces reside in the homelab. A VPS may terminate public TLS and forward traffic through WireGuard, but stores no CAR application data.

## Consequences

The homelab remains the primary security and availability boundary. VPS compromise must not expose stored transcripts, repositories or credentials; end-to-end authorization remains enforced by CAR.

