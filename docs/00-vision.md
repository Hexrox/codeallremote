# Vision

## Problem

Coding agents run where repositories, credentials and build tools live, while their operator is often away from that machine. Existing remote-terminal tools expose bytes, not agent state: they do not make pending approvals, diffs, session history or task progress easy to operate from a phone.

## Product statement

CAR is a self-hosted control plane that turns a local AI coding-agent session into a durable, observable and remotely controllable session. It preserves the native agent as the execution engine and adds a common API, event stream and mobile experience around it.

## Primary user

A technical homelab owner who runs Claude Code with a chosen provider and wants Android to be the daily interface for supervising and directing work across multiple repositories.

## Outcomes

- Start or resume an agent session from Android.
- Read streamed output, inspect changed files and supply the next prompt.
- Receive a high-priority approval request and resolve it safely.
- Reconnect after a mobile-network interruption without duplicating work.
- Add a future agent integration without redesigning clients.

## Non-goals for the first release

- Reimplementing Claude Code or its model/provider protocol.
- Exposing a raw homelab service directly to the public Internet.
- Multi-tenant SaaS, collaboration, or iOS support.

