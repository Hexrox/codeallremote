# Operator runbook

## CAR is unhealthy

Check local readiness, database writability, disk space, adapter executable and recent logs. If a run is active, do not repeatedly restart; determine whether the process is owned and recoverable first.

## Android cannot connect

Check VPS TLS, WireGuard handshake, homelab firewall and CAR readiness in that order. A failed gateway must not be “fixed” by exposing CAR directly to the Internet.

## Approval stuck

Fetch current approval state. If expired or cancelled, do not retry automatically. Inspect adapter/run diagnostics and create a new prompt only after confirming the original action did not proceed.

## Storage pressure

Check artifact retention and backup freshness. Reduce artifact retention through policy, never by deleting database rows or audit records manually.

