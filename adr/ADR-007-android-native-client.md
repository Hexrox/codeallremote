# ADR-007: Android is a native client, not a web wrapper

**Status:** Accepted

## Decision

The primary Android application is specified as a native mobile client with platform notifications, secure storage, offline projection and biometric integration.

## Consequences

The backend must expose normalized state and replay cursors. A browser/PWA may be built later but cannot define the mobile security or offline model.

