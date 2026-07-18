# Agent task contract

## Required input

An implementation agent receives one task ID, the referenced documents, repository state, permitted commands and the expected verification scope. It must not infer broad product goals from a single task.

## Required output

The agent reports:

- files changed and why;
- acceptance criteria evidence;
- tests and commands run;
- migration/schema/protocol impact;
- security considerations;
- known limitations and follow-up task IDs.

## Decision boundary

The agent may choose internal names, test fixtures and mechanical structure. It must stop for user/architectural review when behavior changes a protocol, trust boundary, persistence model, approval semantic, workspace policy or compatibility guarantee.

## Completion rule

No task is complete because code compiles. It is complete only when the Definition of Done is satisfied and the review template is filled.

