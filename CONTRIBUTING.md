# Contributing

CAR is specified before implementation. Changes that affect a durable contract—protocol, trust boundary, session semantics, or plugin interface—must update the relevant documentation and add or amend an ADR.

## Working agreement

1. Keep Android workflows equal to desktop workflows unless an explicit constraint is documented.
2. Do not send workspace contents, transcripts, secrets or credentials through the VPS.
3. Prefer a typed domain event over UI-specific parsing of terminal output.
4. Make destructive or externally visible actions reviewable and auditable.
5. Document acceptance criteria before implementing a non-trivial module.

## Documentation convention

Specifications use **MUST**, **SHOULD**, and **MAY** in their usual normative sense. Open choices are called out rather than silently assumed.

