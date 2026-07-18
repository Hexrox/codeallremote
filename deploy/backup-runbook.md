# Backup and restore runbook (M3-03)

CAR ships `carctl` for encrypted backup and restore of the database and
artifact directory. Back up these together (database + event journal +
approval audit + artifact metadata). Workspace repositories and provider
credentials follow their own backup policies and are NOT included.

## Recovery objectives (per deployment)

Record the owner's targets per deployment. Suggested starting point
(from `docs/28-disaster-recovery.md`):

- RPO 24h for historical artifacts
- RPO 1h for database state
- RTO 4h for the CAR service

An interrupted run is NEVER silently resumed after restore; it must be
reconciled as `recovering` and resumed explicitly.

## Backup

```sh
carctl backup \
  --source /var/lib/car \
  --out /backup/car-$(date +%Y%m%d).carbak \
  --passphrase-file /etc/car/backup.passphrase
```

The archive is AES-256-GCM encrypted (magic `CARBACK01`). The passphrase is
read from a file — never the command line, never logged.

## Verify (no extraction)

```sh
carctl verify \
  --in /backup/car-20260718.carbak \
  --passphrase-file /etc/car/backup.passphrase
```

## Restore drill (clean instance)

Provision a clean host, restricted network, then:

```sh
carctl restore \
  --in /backup/car-20260718.carbak \
  --target /var/lib/car \
  --passphrase-file /etc/car/backup.passphrase
```

Then:
1. Run schema migrations (`car` runs them on start; they are forward-only
   and idempotent — verified by `TestMigrations_Idempotency`).
2. Start CAR; sessions reconcile as `recovering` per
   `docs/10-session-lifecycle.md`. Any agent run requires explicit resume.
3. Reconnect WireGuard/VPS only after local `/ready` passes.

## Drill cadence

A restore drill is performed before production release and quarterly
thereafter. The result records: duration, missing data, unresolved sessions,
and corrective tasks.

## Safety

- Restore rejects symlinks and path-traversal entries in the archive.
- Wrong passphrase → GCM authentication failure (restore aborts cleanly).
- The backup passphrase is stored separately from the archive, on
  owner-controlled encrypted storage.

## Negative invariants

- Restore MUST NOT silently resume an interrupted run.
- Restore MUST NOT overwrite a newer local draft (drafts are client-local;
  server state is authoritative per the protocol contract).
