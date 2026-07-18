# Milestone plan

## M0 — Documentation foundation

Exit criteria: architecture, ADRs, domain model, protocol, security baseline, Android flows and task governance are reviewed.

## M1 — Local single-user core

Exit criteria: a fake CLI session can start, stream output, request approval, finish, persist events and recover after a controlled restart through a local API.

## M2 — Android remote control

Exit criteria: paired Android can list workspaces/sessions, send prompts, receive live events, resolve approvals and reconnect without duplicates.

## M3 — Homelab remote deployment

Exit criteria: Android reaches CAR through VPS/WireGuard while CAR remains unreachable directly from the Internet; backup and restore are demonstrated.

## M4 — Production hardening

Exit criteria: observability, failure tests, schema gates, rate/policy controls, runbooks and security review are complete.

## M5 — Extensibility

Exit criteria: adapter SDK is versioned and a second fake or real adapter proves that clients remain agent-neutral.

