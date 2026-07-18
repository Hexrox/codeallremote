# CI/CD and release policy

## Pull request checks

Every change runs formatting, static analysis, unit tests, contract tests, dependency vulnerability checks and documentation link validation. Integration tests use ephemeral data directories and fake agents.

## Release artifacts

A release includes the CAR server package, migration files, adapter manifests, Android build artifact and deployment documentation. Artifacts are versioned together when a protocol change affects more than one component.

## Migration policy

Database migrations are forward-only and tested from the previous supported release. Protocol changes are additive within a major version; removal requires a deprecation period and documented client minimum version.

## Deployment gates

Production deployment requires a clean backup, successful restore drill for the release candidate, health/readiness verification and a rollback note. The VPS gateway is updated only after the homelab CAR instance is ready.

