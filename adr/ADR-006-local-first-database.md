# ADR-006: Start with SQLite for single-node local storage

**Status:** Accepted for MVP

## Decision

Use SQLite for the first single-node CAR deployment, with a repository boundary that permits a later PostgreSQL implementation.

## Rationale

The homelab deployment needs low operational overhead and transactional event/session writes. The application is not initially a cluster or multi-tenant service.

## Consequences

SQLite locking, backup and file permissions become explicit operational concerns. A future multi-node deployment requires a new ADR rather than silently sharing the database file.

