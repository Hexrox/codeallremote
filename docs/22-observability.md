# Observability

## Goals

Operators must be able to answer: is CAR alive, can it start an agent, where did a run stop, and why did a phone fail to reconnect? Observability must not become a second copy of private workspace data.

## Logs

Use structured logs with timestamp, severity, component, request ID, session/run ID where safe, and outcome. Never log tokens, environment variables, full prompts, raw commands or file contents. Redaction is applied before log serialization.

## Metrics

Minimum metrics:

- active sessions/runs and state transitions;
- adapter start failures and compatibility-degraded runs;
- approval wait duration and expiry count;
- WebSocket connections, replay lag and resync count;
- API latency/error rates;
- storage size, backup age and artifact deletion count.

## Tracing

Trace IDs connect an Android request to core, adapter and wrapper logs. Trace payloads contain identifiers and timings, not transcript contents. Sampling may be reduced for high-volume output events.

## Operator diagnostics

The local administration view exposes readiness, adapter versions, WireGuard reachability, storage health and backup freshness. Public health endpoints expose only minimal liveness information.

