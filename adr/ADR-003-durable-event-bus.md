# ADR-003: Use a durable domain event stream

**Status:** Accepted

## Decision

CAR persists ordered session domain events before distributing them to clients.

## Rationale

Mobile clients disconnect often. Persisted events make timeline, audit, replay and deterministic reconnect possible without pretending a WebSocket is durable storage.

## Consequences

Event payloads require versioning and retention policy. High-volume terminal bytes may be chunked or stored as artifacts while retaining navigable transcript markers.

