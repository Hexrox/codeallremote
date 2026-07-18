# Backlog index for implementation agent

## Order of execution

1. M1 core: `tasks/01-m1-core.md`.
2. M1 Claude adapter: `tasks/02-m1-claude-adapter.md`.
3. M2 API and protocol: `tasks/03-m2-remote-api.md`.
4. M2 Android: `tasks/04-m2-android.md`.
5. M3 deployment: `tasks/05-m3-deployment.md`.
6. M4 platform quality: `tasks/06-m4-platform.md`.

## Agent operating rules

- Read the referenced specification and ADRs before changing code.
- Implement one task at a time; do not silently expand scope.
- Add tests for acceptance criteria and failure paths.
- Update the task status and document deviations.
- Ask for a decision when an implementation would change a durable contract.

## Explicit non-goals

Do not add provider routing, multi-user collaboration, arbitrary remote shell access, unattended task execution or marketplace plugins unless a new ADR authorizes it.

