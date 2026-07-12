# ADR-001: Wrap Claude Code instead of forking it

**Status:** Accepted

## Decision

CAR launches and supervises the installed Claude Code CLI through an adapter and wrapper. It does not fork, patch or replace Claude Code.

## Rationale

This preserves the user's provider setup and native feature set while keeping CAR responsible only for remote control and durable state. A fork would inherit an unstable, high-maintenance compatibility burden.

## Consequences

The adapter must tolerate CLI-output changes and expose compatibility diagnostics. Some agent state may remain opaque; CAR must not invent unsupported capabilities.

