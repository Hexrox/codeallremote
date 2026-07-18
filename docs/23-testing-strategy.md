# Testing strategy

## Test layers

1. **Unit:** state transitions, authorization, redaction, cursor handling and parsers.
2. **Contract:** adapter signals, REST schemas and WebSocket envelopes.
3. **Integration:** database transactions, wrapper process groups and event replay.
4. **End-to-end:** a fake CLI through CAR API and an Android test client.
5. **Operational:** restart, backup/restore, WireGuard loss and notification expiry.

## Determinism

Tests must use a fake agent executable and controllable clock where timing matters. No test may require a real model provider, personal credentials or public network access.

## Required failure cases

- duplicate idempotency key;
- dropped WebSocket and cursor gap;
- adapter crash during approval;
- malformed agent output;
- database failure between state and event writes;
- revoked device using an old refresh token;
- VPS/WireGuard unavailable while a local run continues.

## Definition of done

A task is complete only when its acceptance criteria, negative paths and relevant contract tests are present. A green unit suite alone does not establish reconnect, authorization or process-isolation safety.

