# Release checklist

## Before merge

- [ ] Scope is represented by a task and acceptance criteria.
- [ ] Relevant ADR/specification is updated.
- [ ] Unit, contract and failure-path tests are present.
- [ ] No credentials, real workspace paths or private transcripts appear in fixtures.
- [ ] OpenAPI/JSON Schema validation passes.
- [ ] Documentation links and Mermaid blocks are valid.

## Before deployment

- [ ] Database migration tested from the previous supported version.
- [ ] Backup completed and restore drill passed.
- [ ] Adapter version and Claude Code compatibility verified.
- [ ] CAR binds only to the intended local/WireGuard interface.
- [ ] VPS proxy has TLS, WebSocket forwarding and no persistent application data.
- [ ] Device revocation and approval expiry tested.

## After deployment

- [ ] Liveness and readiness checks pass.
- [ ] A synthetic session using a fake CLI completes end-to-end.
- [ ] Android reconnect and approval notification verified.
- [ ] Logs/metrics contain no secrets.
- [ ] Rollback owner and procedure are known.

