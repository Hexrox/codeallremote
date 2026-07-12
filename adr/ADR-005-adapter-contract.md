# ADR-005: Adopt an adapter contract for agents

**Status:** Accepted

## Decision

CAR core communicates with agents only through a versioned adapter contract.

## Minimum contract

An adapter declares capabilities and supports: validate workspace, start run, submit input, interrupt, observe output, surface approval requests, report lifecycle, and restore or explicitly decline restoration.

## Consequences

Clients consume CAR concepts rather than Claude-specific output. Adapter work is bounded but cannot conceal incompatible agent behavior.

