# Change management

## Change classes

- **Patch:** documentation, diagnostics or internal behavior with no contract change.
- **Minor:** additive API/event/capability change; existing clients remain valid.
- **Major:** changed meaning, removed field, security boundary or migration requirement.

## Required records

Every major change includes an ADR or amended ADR, migration note, compatibility statement, fixture updates and rollback plan. Security-boundary changes also require threat-model review.

## Release notes

Release notes summarize user-visible Android behavior, server/API changes, adapter compatibility, migrations, security changes and known limitations. They must not include private workspace names, command output or credentials.

