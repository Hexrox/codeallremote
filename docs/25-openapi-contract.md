# OpenAPI contract

## Contract policy

The REST contract is versioned under `/api/v1`. The generated OpenAPI document is a client contract, not an implementation detail. Every endpoint must define authentication, idempotency, success response, typed errors and authorization requirements.

## Required schemas

- `WorkspaceSummary`: id, display name, adapter capabilities and health.
- `SessionSnapshot`: lifecycle state, active run, pending approval and last event sequence.
- `RunSummary`: id, state, started/ended timestamps and outcome.
- `ApprovalSummary`: id, category, redacted context, state and expiry.
- `DomainEvent`: type, message ID, session sequence, schema version and payload.
- `ApiError`: stable code, safe message, details and request ID.

## Compatibility

New response fields are additive. Clients must ignore unknown fields. Removing or changing the meaning of a field requires a new API major version and migration documentation. Error codes are stable identifiers; human messages may change.

## Security requirements

The OpenAPI contract must not describe endpoints that return credentials, raw environment variables or unrestricted filesystem paths. Examples use synthetic IDs and content.

