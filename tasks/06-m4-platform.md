# M4 tasks — platform quality

## M4-01 — Implement plugin manifest and registry

**Inputs:** `docs/21-plugin-sdk.md`.

**Acceptance criteria:**

- Invalid or incompatible manifests prevent plugin activation.
- Plugin capabilities are visible through diagnostics.
- Shutdown drains active plugin work within a bounded timeout.
- Core tests run with a fake plugin and a rejected plugin.

## M4-02 — Add structured observability

**Inputs:** `docs/22-observability.md`.

**Acceptance criteria:**

- Logs contain correlation identifiers but no secrets or workspace content.
- Metrics cover sessions, approvals, replay, API errors and storage.
- A local diagnostic view distinguishes liveness from readiness.

## M4-03 — Build failure-oriented test suite

**Inputs:** `docs/23-testing-strategy.md`.

**Acceptance criteria:**

- Fake CLI tests cover process, parser and approval races.
- Replay tests prove no gaps/duplicates after reconnect.
- Restore test runs against a clean temporary instance.

## M4-04 — Establish CI/CD gates

**Inputs:** `docs/24-ci-cd.md`.

**Acceptance criteria:**

- Pull requests fail on formatting, tests, schema drift or broken links.
- Release artifacts are reproducible and versioned.
- Migration and rollback checks run before deployment.

