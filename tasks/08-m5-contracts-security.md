# M5 tasks — contracts and security

## M5-01 — Publish OpenAPI and schemas

**Inputs:** `docs/25-openapi-contract.md`, `docs/26-json-schemas.md`.

**Acceptance criteria:**

- Every implemented endpoint has an OpenAPI operation and error model.
- REST/WebSocket fixtures validate in CI.
- Schema compatibility checks prevent accidental breaking changes.

## M5-02 — Security review implementation

**Inputs:** `docs/27-security-threat-model.md`.

**Acceptance criteria:**

- Workspace/path and process isolation tests pass.
- Secret redaction is tested with representative provider credentials.
- Approval, token and audit invariants have automated tests.
- Findings are tracked with severity and remediation status.

## M5-03 — Recovery and release drill

**Inputs:** `docs/28-disaster-recovery.md`, `docs/29-qa-checklists.md`.

**Acceptance criteria:**

- A clean-instance restore is repeatable.
- Interrupted sessions require explicit operator action after restore.
- Release checklist blocks deployment when backup or security checks fail.

