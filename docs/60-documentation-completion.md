# Documentation completion status

The foundation documentation is complete for beginning M1 implementation. It includes product requirements, architecture, ADRs, domain model, protocol contracts, OpenAPI, JSON Schemas, Android UX, deployment, security, testing, operations and task governance.

## Remaining work is implementation

Future documentation changes should be driven by discovered implementation facts, security review findings or explicit new product scope. The next work item is M0 review, followed by M1 task execution; writing more high-level descriptions is not a prerequisite for starting.

## Change rule

When implementation discovers a contract gap, update the smallest relevant specification and task before changing behavior. Preserve the current document set as the baseline for the first implementation milestone.

## Implementation completion (M1–M5)

All milestones M1 through M5 are implemented with tests. Status is `in_review`
per `docs/54-progress-reporting.md`: acceptance evidence is available, but a
milestone is only `accepted` after reviewer sign-off and a green CI run.

| Milestone | Scope | State |
| --- | --- | --- |
| M1 | Single-user local core: bootstrap, storage/event journal, session manager, adapter boundary + fake, wrapper, output normalization, approval bridge, workspace policy, snapshot projection, cursor repository, audit, graceful shutdown/reconciliation | in_review |
| M2 | REST API, WebSocket gateway (replay/backpressure/resync), device pairing & revocation, API error catalog, WS hardening; Android client SDK + screens + ViewModels + notifications | in_review |
| M3 | Packaging (Dockerfile/systemd/health+ready), WireGuard+VPS reverse proxy, backup/restore tooling (`carctl`) | in_review |
| M4 | Plugin registry, observability, failure test suite, CI/CD gates, command dispatcher, protocol negotiation, synchronization harness | in_review |
| M5 | Adapter SDK contract, non-Claude adapter skeleton (Codex), real Claude Code adapter | in_review |

Verification (2026-07-18): Go `go test -race ./...` (23 packages, 0 races);
Android JVM tests (29) + APK; CI workflows under `.github/workflows/`. The
one gate not run locally is Android instrumented tests (require an emulator)
— they must run green in CI before `accepted`. Full evidence and follow-ups:
`tasks/25-post-review-closure.md`.

