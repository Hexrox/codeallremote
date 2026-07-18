# Disaster recovery

## Recovery objectives

MVP targets are owner-defined but must be recorded per deployment. Suggested starting point: RPO 24 hours for historical artifacts, RPO 1 hour for database state, and RTO 4 hours for the CAR service. An interrupted run is never silently resumed after restore.

## Backup set

Back up database, event journal, approval audit records, artifact metadata and CAR configuration (excluding runtime secrets). Workspace repositories and provider credentials follow their own backup policies.

## Restore procedure

1. Provision a clean homelab host and restrict network access.
2. Restore encrypted database and artifacts.
3. Validate schema migrations and integrity checks.
4. Rotate service/device credentials if compromise is suspected.
5. Reconcile sessions as recovering; require explicit resume for any agent run.
6. Reconnect WireGuard/VPS only after local readiness passes.

## Recovery testing

A restore drill is performed before production release and quarterly thereafter. The result records duration, missing data, unresolved sessions and corrective tasks.

