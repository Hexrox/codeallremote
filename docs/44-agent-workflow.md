# Agent implementation workflow

## Before coding

1. Read the task, referenced documents and applicable ADRs.
2. Identify the domain/API contracts being consumed or changed.
3. List assumptions and unresolved choices in the task branch notes.
4. Define tests for each acceptance criterion and failure path.

## During coding

Implement only one task at a time. Keep changes small enough to review. If implementation reveals a contract conflict, stop and update the specification/ADR before making a workaround.

## After coding

Run formatting, static analysis, unit tests, contract tests and the task-specific integration tests. Report changed files, commands run, results, known limitations and any migration impact.

## Handoff format

```text
Task: Mx-yy
Implemented:
Tests:
Contract changes:
Migration impact:
Known limitations:
Follow-up tasks:
```

## Prohibited shortcuts

Do not bypass approval checks, disable TLS verification, expose CAR publicly for debugging, hard-code credentials, swallow adapter parse failures, or mark a task complete without its acceptance tests.

