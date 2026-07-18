# Protocol versioning

## Version layers

CAR tracks API major version, WebSocket protocol version, event schema version and adapter SDK version independently. A client advertises the versions it supports during connection; the server selects one compatible version or rejects the connection with a safe diagnostic.

## Compatibility rules

- Additive fields are backward compatible.
- Unknown event types are ignored after being recorded for diagnostics.
- Changing field meaning, requiredness or ordering requires a new version.
- A deprecated version remains readable for one documented migration window.
- Persisted events retain their original schema version.

## Migration

Every breaking change includes a migration note, fixture updates, minimum client version and rollback behavior. The server may support multiple read versions while writing only the current version.

