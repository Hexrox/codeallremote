# Architecture review checklist

## Boundaries

- [ ] Core does not parse Claude-specific terminal output.
- [ ] Adapter does not own authorization or persistence.
- [ ] Android does not become the source of truth for session state.
- [ ] VPS does not store CAR application data.

## Failure behavior

- [ ] Client disconnect does not stop a run accidentally.
- [ ] Server restart reconciles active processes.
- [ ] Duplicate commands are safe.
- [ ] Approval expiry is enforced server-side.
- [ ] Unknown protocol versions fail safely.

## Security

- [ ] Workspace paths are registered and canonicalized.
- [ ] Logs, events and notifications redact secrets.
- [ ] Device revocation is tested.
- [ ] Direct public CAR access is blocked.

## Operability

- [ ] Readiness differs from liveness.
- [ ] Backup and restore are documented and tested.
- [ ] Metrics identify adapter, storage, transport and client failures.

