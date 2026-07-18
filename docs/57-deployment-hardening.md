# Deployment hardening

## Host

Run CAR under a dedicated non-login service account with a restricted filesystem view, resource limits and no interactive shell. Keep database/artifacts on encrypted storage with controlled permissions.

## Network

Bind CAR to loopback/WireGuard only. Restrict reverse proxy routes and methods. Enforce TLS, request-size limits, WebSocket idle timeouts and rate limits for authentication/pairing endpoints.

## Secrets

Store provider/service secrets outside the repository and configuration examples. Rotate device, WireGuard and service credentials independently. Never copy secrets into Android notifications, audit events or support bundles.

## Updates

Verify release artifact integrity, back up before migration, deploy to a test instance first, and retain a documented rollback version. Do not update the gateway before the homelab service is ready.

