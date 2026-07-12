# Design principles

## Server authoritative

Clients request actions; they do not mutate session state locally. A reconnect obtains a snapshot and resumes from an event cursor.

## Normalize at the edge

Agent-specific text, escape sequences and control prompts stay in the adapter. The rest of CAR receives typed events such as `run.output`, `tool.requested`, `approval.requested`, and `run.completed`.

## Prefer durable events

If an event changes what an operator needs to know, persist it before publishing it. Ephemeral terminal chunks may be batched, but the transcript and action timeline must remain reconstructable.

## Least privilege by default

Each workspace defines allowed paths and command/approval policy. Credentials are never returned by the API or copied into mobile notifications.

## Capability negotiation

Clients and adapters advertise supported protocol versions and capabilities. Unsupported functionality is explicitly unavailable, never silently degraded into unsafe behavior.

