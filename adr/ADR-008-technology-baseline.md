# ADR-008: Establish a Go/Kotlin baseline

**Status:** Proposed baseline

## Decision

Use Go for CAR server/core and Kotlin with Jetpack Compose for Android. Keep storage and protocol behind interfaces/contracts.

## Rationale

The baseline matches local process supervision, a small homelab deployment, native Android notifications/secure storage and deterministic contract testing.

## Consequences

The team must maintain Go and Android toolchains. This ADR does not select a web framework, container runtime or push provider; those choices remain bounded follow-up decisions.

