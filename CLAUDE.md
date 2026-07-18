# Instructions for the implementation agent

You are implementing Code All Remote (CAR), a self-hosted, Android-first control plane for coding agents. Read this file before any task, then read the selected task and every document listed under its Inputs.

## Mission

Implement the documented CAR contracts incrementally. The first milestone is a local, single-user core that can supervise a deterministic fake CLI and then Claude Code through an adapter. Android and remote deployment follow the M1 gates.

## Required reading order

1. `README.md`
2. `docs/00-vision.md`
3. `docs/03-architecture.md`
4. `docs/04-domain-model.md`
5. applicable ADRs under `adr/`
6. the selected task under `tasks/`
7. `docs/43-implementation-guidelines.md`
8. `docs/45-definition-of-ready-done.md`

## How to work

- Implement one task at a time, in the order documented by `tasks/21-m1-order.md`.
- Do not expand scope silently.
- Treat the server as authoritative for state.
- Keep Claude-specific parsing inside the adapter.
- Persist authority-changing state before publishing its event.
- Use idempotency keys for commands that may be retried.
- Write tests for every acceptance criterion and failure path.
- Use a deterministic fake CLI in tests; never require real provider credentials.
- Keep changes reviewable and explain all changed files in the handoff report.

## Security rules

- Never log tokens, credentials, environment variables, raw prompts, full commands or private file contents.
- Never expose CAR directly to the public Internet for debugging.
- Validate and canonicalize registered workspace paths; reject traversal and escaping symlinks.
- Run agent processes under the least-privileged service identity and own only their process group.
- Do not bypass approval, authorization, device revocation or audit requirements.
- Keep the VPS gateway stateless; workspace and transcript data stay in the homelab.

## Contract rules

- Do not change an API, event, lifecycle state, persistence model or trust boundary without updating the relevant specification and ADR.
- Additive schema changes require fixtures. Breaking changes require a version, migration note and compatibility review.
- Unknown future event types must be handled safely, not treated as success.
- A `202 Accepted` response means accepted for asynchronous processing, not completed.

## When to stop and ask for review

Stop before changing behavior if:

- two specifications or ADRs conflict;
- a task requires an unapproved technology or new dependency;
- a feature needs broader filesystem, network or credential access;
- an approval semantic or security boundary must change;
- the adapter cannot safely determine what happened;
- acceptance criteria cannot be tested deterministically.

Do not invent a workaround and hide the conflict in code. Record the issue and propose the smallest documentation/ADR change.

## Required verification

Before marking a task complete, run formatting, static analysis, unit tests, contract tests and relevant integration/failure tests. Check that logs and fixtures contain no secrets. Verify migrations from the previous schema where applicable.

## Required handoff report

```text
Task:
Summary:
Files changed:
Acceptance criteria evidence:
Tests/commands run:
Protocol/schema/API impact:
Migration impact:
Security considerations:
Known limitations:
Follow-up tasks:
Reviewer decision: pending
```

Never claim a task is complete because it compiles or because only the happy path works. Mark partial work as partial and leave the repository in a buildable, reviewable state.

