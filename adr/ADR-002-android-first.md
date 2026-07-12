# ADR-002: Android is a first-class client

**Status:** Accepted

## Decision

CAR's API, approval flow and information architecture are designed first for Android. The Web dashboard is complementary, not the source of mobile functionality.

## Consequences

Every MVP workflow must be usable with constrained screen space, intermittent connectivity and push notifications. Terminal emulation alone is insufficient; CAR exposes normalized state for mobile-native views.

