# Technology baseline

## Recommendation

This is a documented baseline, not permission to implement every component immediately.

- CAR server/core: Go, chosen for a small deployable binary, process control and concurrency.
- Android: Kotlin with Jetpack Compose and platform secure storage.
- MVP database: SQLite behind a repository interface.
- Transport: HTTPS and WebSocket, terminated at the CAR-aware reverse proxy path.
- Homelab packaging: container or supervised local process; final choice requires deployment validation.
- Schemas: OpenAPI plus JSON Schema fixtures checked in CI.

## Selection criteria

The stack must support deterministic tests, clear process ownership, secure secret handling, low homelab operations overhead and a future adapter SDK. A technology change requires an ADR that compares migration cost and contract impact.

## Constraints

Do not introduce a cloud dependency for core state. Do not select a framework merely because it generates code; generated artifacts remain subordinate to the documented protocol.

