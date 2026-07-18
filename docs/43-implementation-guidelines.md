# Implementation guidelines

## General rules

- Keep domain logic independent from HTTP, WebSocket and Android UI.
- Validate at boundaries; keep internal domain objects invariant-safe.
- Prefer explicit typed errors over string matching.
- Make retries safe with idempotency keys or documented non-retryable behavior.
- Persist an authority-changing state transition before publishing its event.
- Never log secrets, full prompts, environment variables or unredacted workspace content.

## Adapter rules

Agent-specific parsing belongs in an adapter. Core code must consume normalized signals and must not branch on terminal escape sequences, provider names or Claude-specific prompt text.

## API rules

Responses are authoritative snapshots or documented command acknowledgements. Do not return internal filesystem paths, process arguments containing secrets or unbounded log payloads. Additive contract changes require fixtures.

## Android rules

The app renders server state; it does not reimplement lifecycle transitions. Optimistic UI is allowed only for local drafts and must be visibly reconciled when the server responds.

