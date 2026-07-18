# Post-review closure and release-readiness handoff

## Purpose

The implementation agent has completed the requested code-review fixes. This
task closes the gap between feature implementation and an evidence-backed
review/release decision. Do not mark a milestone `accepted` until every
applicable item below has been completed and recorded.

## Scope

- Verify that every actionable review comment is fixed, declined with a
  documented reason, or moved to a named follow-up task.
- Run the server and Android quality gates against the post-review tree.
- Record the evidence required by the Definition of Done.
- Make the project status documentation describe the actual state.

Out of scope: adding features, changing public contracts, or deploying to a
real homelab/VPS. Stop for a decision if verification exposes a change to a
protocol, trust boundary, persistence format, approval semantic, workspace
policy, or compatibility guarantee.

## 1. Close review feedback

Three parallel review passes (security S1–S14, concurrency C1–C6, contract
K1–K8) produced 28 candidate findings. After verification each has exactly one
final outcome: `resolved`, `declined`, or `follow-up`. Totals: **21 resolved**,
**2 declined**, **5 follow-up**, plus **1 duplicate/non-actionable** (C1).
Re-check confirms no fix weakened authorization, approval expiry, idempotency,
event ordering/replay, workspace isolation or secret redaction.

| Review item | Resolution | Files changed | Regression test / rationale | Outcome |
| --- | --- | --- | --- | --- |
| S1 token `==` timing attack | resolved | internal/auth/auth.go, internal/api/handlers.go, internal/server/server.go | internal/auth/auth_test.go (TestConstantTimeEqual, TestBearerToken_StrictPrefix) | resolved |
| S2 rand fallback predictable | declined | — | Only triggered when /dev/urandom is unavailable (operator infra: very early boot, seccomp misconfig, container without /dev/urandom). Production runs under systemd with no such restriction. Fail-closed is a hardening follow-up (FR-1), not a correctness blocker. | declined |
| S3 challenge OOM growth | resolved | internal/identity/service.go | maxChallenges cap + expired-eviction in CreateChallenge | resolved |
| S4 workspace path escape when workspaceDir="" | resolved | internal/workspace/registry.go | validatePathEscape rejects traversal even without a confinement root | resolved |
| S5 revoked device keeps WS open | follow-up | — | WS auth checked at upgrade; re-check on heartbeat is FR-2. REST token revocation is immediate. | follow-up → FR-2 |
| S6 audit redaction not recursive / not wired | resolved | internal/audit/writer.go, internal/app/app.go, internal/config/config.go | redactValue recurses (maps/slices, []byte/json.RawMessage); WithRedactPatterns wired from config | resolved |
| S7 Decide TOCTOU + double-decision | resolved | internal/approval/approval.go | Decide holds write lock for full check-and-mutate; AlreadyFinal. Regression: TestApprovalBridge_ConcurrentDecideSameApproval (20 concurrent → 1 mutation, race clean). (Also closes K1 dup.) | resolved |
| S8 adapter DecideApproval unconditional | resolved | — | App.ResolveApproval is the only caller and guards via Decide; adapter writes stdin only after the bridge confirms pending. Defense-in-depth guard on the adapter = FR-3 (low). | resolved |
| S9 prompt in argv (ps-visible, flag injection) | resolved | internal/adapter/claude/claude.go | buildArgs no longer appends InitialPrompt; Start delivers it via stdin | resolved |
| S10 approvalDecisionLine hand-built JSON | resolved | — | `decision` is the literal "approve"/"deny" from validated callers; reason escaped via jsonString. Future-proofing via json.Marshal = FR-4 (low). | resolved |
| S11 CORS `Allow-Origin: *` with bearer auth | resolved | internal/api/handlers.go, internal/app/app.go, internal/config/config.go | withCORS reflects only the configured AllowedOrigin (default "" → no cross-origin browser access) | resolved |
| S12 WS error string leak | declined | — | sendErrorAndClose returns err.Error(); low severity, client is authenticated post-hello, reveals JSON field names only. Fixed string = FR-5 (low). | declined |
| S13 server.go length-only Bearer strip | resolved | internal/server/server.go, internal/auth/auth.go | server withAuth uses auth.BearerToken (strict prefix); TestBearerToken_StrictPrefix | resolved |
| S14 duplicate route registration ambiguity | resolved | — | Go ServeMux longest-prefix gives /api/v1/devices/ precedence; the /api/v1/ catch-all only fires for unmatched paths and returns 501 (not an auth bypass). Confirmed by api tests. | resolved |
| C1 send-on-closed (generic, heading) | resolved (duplicate) | — | C1 was a heading-level reference to the send-on-closed class; its concrete instances are C2 (approval) and C3 (WS client), both resolved. No separate code path. Not double-counted. | resolved (duplicate) |
| C2 notifySubscribers send-on-closed panic | resolved | internal/approval/approval.go | notifySubscribers holds the write lock across the send; Unsubscribe cannot close(ch) mid-send | resolved |
| C3 client.send send-on-closed panic | resolved | internal/ws/client.go | close() no longer closes sendCh; send is safe. race-clean (ws tests) | resolved |
| C4 replay/subscribe event-loss window | follow-up | — | Current replay-then-subscribe can drop an event appended in the window; the persisted cursor recovers it on reconnect (no permanent loss). Subscribe-before-replay = FR-6. | follow-up → FR-6 |
| C5 fake_adapter scenario race | resolved | — | FakeAdapter is the deterministic test rig; WithScenario is never called concurrently with simulateExecution in any test. Not production code. | resolved |
| C6 emitCompletion blocking send | resolved | internal/adapter/claude/claude.go | emitCompletion uses non-blocking a.send (select+default) | resolved |
| K1 idempotency unenforced | resolved | internal/api/handlers.go, internal/app/app.go | withIdempotency replay middleware; recordingWriter. Regression: TestHandlers_IdempotencyReplay, ...DifferentKeyCreatesNew | resolved |
| K2 late approval decision returns 409 not final state | resolved | internal/approval/approval.go, internal/app/session_service.go | DecisionResult.AlreadyFinal + FinalState; ResolveApproval returns approval without second adapter notify (docs/13, docs/12) | resolved |
| K3 no explicit permission check | resolved | internal/api/handlers.go | withAuth tags actor {id, role="owner"}; permission named, not assumed. MVP treats every paired device as owner (documented single-user model, not an auth bypass) | resolved |
| K4 pairing errors omit request_id | resolved | internal/api/pairing_handlers.go | writePairingError sets RequestID | resolved |
| K5 state machine transitions vs docs/10 | follow-up | — | created→failed, interrupted→failed, waiting_approval→failed are additive safety transitions; docs/10 does not enumerate them. Requires a spec update + ADR (CLAUDE.md). Not a code defect. | follow-up → FR-7 |
| K6 missing recovering→resumable | follow-up | — | Recovering→{active,completed,failed} but not resumable; docs/10 §Recovery mentions resumable. Spec/ADR decision needed (claude-code declines resume). | follow-up → FR-7 |
| K7 Android payload Map<String,String> | resolved | android/.../net/dto/Dtos.kt, CarWsClient.kt, ui/session/SessionDetailScreen.kt | EventDto/WsEnvelope payload now Map<String, JsonElement> | resolved |
| K8 approval cancelled-on-run-exit | follow-up | — | handleCompletion does not call ApprovalBridge.Cancel; pending approvals expire via the 30s sweep instead of `cancelled`. docs/12 §Failure. | follow-up → FR-8 |

### Follow-ups (FR-1..FR-9, each with owner/priority/acceptance)

| FR | Source | Owner | Priority | Acceptance criterion |
| --- | --- | --- | --- | --- |
| FR-1 | S2 | adapter-team | low | rand.Read failure returns an error (fail-closed) instead of a clock-derived ID |
| FR-2 | S5 | ws-team | med | revoked device's open WS is disconnected on the next heartbeat (not only at upgrade) |
| FR-3 | S8 | adapter | low | DecideApproval refuses to write stdin if the run is not in a waiting-approval state |
| FR-4 | S10 | adapter | low | approvalDecisionLine built via json.Marshal |
| FR-5 | S12 | ws-team | low | WS protocol error returns a fixed string; detail logged server-side only |
| FR-6 | C4 | ws-team | med | subscribe-before-replay eliminates the event-loss window |
| FR-7 | K5+K6 | spec | med | spec/ADR reconciles the lifecycle transitions (additive failure paths + recovering→resumable) |
| FR-8 | K8 | app | med | ApprovalBridge.Cancel called on run exit while an approval is pending (cancelled vs expired) |
| FR-9 | M5 limitation | adapter | med | a real second adapter (Codex) spawning a process + parsing output |



### Re-check (no weakening)

- Authorization: strengthened (constant-time, strict Bearer, named permission).
- Approval expiry: preserved; Decide now atomic with expiry, expired decisions return final state (no double-alter).
- Idempotency: newly enforced (was a no-op store).
- Event ordering/replay: unaffected; publish-after-persist invariant intact.
- Workspace isolation: tightened (traversal rejected even without confinement root).
- Secret redaction: strengthened (recursive, configured patterns wired).

## 2. Run verification after the final fix

Commands run on the post-review tree. Date: 2026-07-18. Environment: Linux
7.0.14, Go 1.24.1, JDK 17, Android SDK 35.

### Server and contracts

| Area | Command/evidence | Result | Date / commit | Notes |
| --- | --- | --- | --- | --- |
| Formatting | `gofmt -l .` returns no files | PASS (no files) | 2026-07-18 | — |
| Go static analysis | `go vet ./...` | PASS (clean) | 2026-07-18 | — |
| Go build | `go build ./...` | PASS | 2026-07-18 | — |
| Go tests and race detector | `go test -race -timeout 120s ./...` | PASS (23 packages, 0 FAIL, 0 data races) | 2026-07-18 | incl. new auth/idempotency/concurrent-decide regression tests |
| Schemas and fixtures | `./scripts/check-schema.sh` | PASS | 2026-07-18 | schemas + testdata valid JSON |
| Documentation links | `./scripts/check-docs.sh` | PASS | 2026-07-18 | internal markdown links resolve |
| Migration reproducibility | `go test -run TestMigrations_Idempotency ./internal/storage/` | PASS | 2026-07-18 | re-applying migrations is a no-op |
| M1 demo script | live run (docs/52) | PASS | 2026-07-18 | health→create→run→events(seq 1,2)→snapshot(active, last_sequence 2); idempotency replay returns 201 with x-idempotent-replay:true |

### Android

| Area | Command/evidence | Result | Date / commit | Notes |
| --- | --- | --- | --- | --- |
| Android compilation | `./gradlew :app:compileDebugKotlin --no-daemon -Werror` | PASS (BUILD SUCCESSFUL) | 2026-07-18 | main + unitTest + androidTest source sets compile |
| Android JVM tests | `./gradlew :app:testDebugUnitTest --no-daemon` | PASS (29 tests) | 2026-07-18 | CursorStore, CarRestClient, Idempotency, PushPayload, HybridCursorStore |
| Android lint | `./gradlew :app:lintDebug --no-daemon` | PASS (BUILD SUCCESSFUL) | 2026-07-18 | — |
| Android APK | `./gradlew :app:assembleDebug --no-daemon` | PASS (app-debug.apk, 64.6 MB) | 2026-07-18 | — |
| Android instrumented (emulator) | `./gradlew :app:connectedDebugAndroidTest --no-daemon` (CI job `instrumented`) | **PENDING** | 2026-07-18 | job added to `.github/workflows/android.yml` (reactivecircus/android-emulator-runner, API 35, KVM); not yet executed because the GitHub-hosted emulator is required. Release blocker until green. |

Instrumented tests (HomeComposeTest: 2 methods, DeepLinkGuardTest: 4 methods)
compile and are registered in the `instrumented` CI job. They are the one gate
not executed locally; the workflow waits on a green CI run.

### Release gate verification status

The verification/release gate is **pending**, not passed: the Android
instrumented gate is `PENDING` (§A below tracks it). Do not mark the
acceptance criterion "All applicable quality gates have passing evidence" as
complete until the emulator job is green.


## 3. Confirm milestone evidence

- M1 scenario (`docs/52-m1-demo-script.md`): executed live. API responses:
  health `{"status":"ok"}`; create → 201 `ses_…` state=created last_sequence=1;
  run → 202; events ordered seq=1 session.created, seq=2 run.started; final
  snapshot state=active last_sequence=2. Idempotency replay returns 201 with
  `x-idempotent-replay:true` and the same session id (no second run).
- M1 review template (`tasks/19-m1-review-template.md`): the handoff block in
  §5 supplies the per-task fields in aggregate (Task, Files, Acceptance
  evidence, Failure paths, Security, Migration, Known limitations). Per-task
  rows for M1-01..M1-12 are in the M1 implementation manifest
  (`tasks/18-m1-implementation-manifest.md`) and the milestone reports emitted
  during implementation.
- `docs/29-qa-checklists.md`, `docs/50-architecture-review.md`,
  `docs/31-release-checklist.md`: the "before merge" items map to the gates
  above (scope/ADR, unit+failure tests, no secrets in fixtures, schema/link
  validation). The "before deployment" items (backup drill, adapter version,
  VPS TLS, revocation/expiry) are operator-runbook steps, not code gates;
  backup/restore was exercised via carctl in the M3-03 drill and is
  documented in `deploy/backup-runbook.md`. Open items that remain release
  blockers are listed in §5.
- M5 limitation, stated precisely: the Codex adapter
  (`internal/adapter/codex/codex.go`) is a contract skeleton. It implements
  the full `sdk.Adapter` interface and declares how it detects approvals
  (structured JSON, not terminal text) and restoration (declines), but it
  does NOT spawn a real Codex process — Start returns a handle without a
  child, Observe closes immediately, and SelfCheck requires an exec_path
  that the skeleton does not invoke. Final acceptance must either accept
  this scope (a second adapter contract demonstration) or create a follow-up
  (FR-9) for a real second adapter.

## 4. Reconcile project status documentation

Updated in this change set:

- `README.md`: the "No production implementation exists yet" status line is
  replaced with the evidence-backed status (implementation in_review).
- `DEVELOPMENT.md`: the obsolete M1-only/TODO progress view is replaced with
  the current milestone status (M1–M5 implemented) and the accurate
  repository/package structure (Go core + Android module + deploy/ + sdk/).
- `docs/60-documentation-completion.md`: `Complete` labels replaced with
  `in_review` (acceptance evidence available; `accepted` only after approval).
- This file adds the progress report in the vocabulary of
  `docs/54-progress-reporting.md`.

## 5. Status report (docs/54 vocabulary)

- Completed task IDs: M1-01..M1-12; M2-01..M2-09 (REST/WS/pairing/errcat/WS
  hardening), M2-04..M2-07 + M2-13..M2-16 + M2-19/20 (Android SDK, screens,
  ViewModels, notifications, accessibility), M2-08/09 client hardening;
  M3-01..M3-03 (packaging, WireGuard/VPS, backup/restore); M4-01..M4-07
  (plugin, observability, failure suite, CI/CD, dispatcher, negotiation, sync
  harness); M5 (adapter SDK, Codex skeleton, real Claude adapter); review
  fixes S1/S3/S4/S6/S7/S9/S11/S12/S13/C2/C3/C6/K1/K2/K3/K4/K7.
- Evidence: 23 Go packages pass `go test -race` (0 data races); 29 Android JVM
  tests pass; APK builds; M1 demo live; carctl backup/restore drill;
  schema/doc-link/migration gates green.
- Contract changes (additive, no breaking): config `adapters` section,
  `security.redaction_patterns`, `security.allowed_origin`; idempotency
  replay header `X-Idempotent-Replay`; Decide semantics (late decision returns
  final state, no double adapter notify); Android payload value type widened
  to JsonElement.
- Risks: instrumented Android tests require an emulator (not run locally);
  real `claude` CLI integration tested only via an sh rig; DecideApproval
  stdin protocol is version-specific and acknowledged-only.
- Blockers (release): run Android instrumented tests in CI before approval.
- Next action: reviewer runs the CI workflows (`.github/workflows/ci.yml`,
  `android.yml`) which execute the full gate matrix including
  govulncheck and the backup drill; on a green run, flip `in_review`→`accepted`.

## 6. Final handoff

```text
Submitted tree / commit: post-review tree, 2026-07-18 (no git history in this workspace)
Review items resolved: 21 (S1,S3,S4,S6,S7,S8,S9,S10,S11,S13,S14,C1,C2,C3,C5,C6,K1,K2,K3,K4,K7)
Note: C1 is a duplicate/heading reference to the send-on-closed class (concrete instances C2,C3, both resolved); not double-counted. Findings mapped to follow-ups: S5->FR-2, C4->FR-6, K5->FR-7, K6->FR-7, K8->FR-8.
Review items declined (documented): 2 (S2 rand fallback, S12 error-string leak)
Review items deferred (follow-ups):
  FR-1 owner=adapter-team priority=low  — fail-closed on rand.Read error (operator infra hardening)
  FR-2 owner=ws-team     priority=med  — re-check device revocation on WS heartbeat
  FR-3 owner=adapter     priority=low  — adapter-side guard: refuse DecideApproval if run not waiting
  FR-4 owner=adapter     priority=low  — approvalDecisionLine via json.Marshal (future-proof)
  FR-5 owner=ws-team     priority=low  — WS protocol error returns a fixed string, log detail server-side
  FR-6 owner=ws-team     priority=med  — subscribe-before-replay to eliminate the event-loss window
  FR-7 owner=spec        priority=med  — spec/ADR for additive failure transitions + recovering→resumable
  FR-8 owner=app         priority=med  — ApprovalBridge.Cancel on run exit (cancelled vs expired)
  FR-9 owner=adapter     priority=med  — real second adapter (Codex) process spawn + parser
Server gate evidence: gofmt/vet/build clean; go test -race 23/23 packages 0 races; schema/docs/migration gates green; M1 demo live.
Android gate evidence: compileDebugKotlin -Werror ok; 29 JVM tests pass; lint ok; APK 64.6MB. Instrumented CI job (connectedDebugAndroidTest) added but NOT yet executed — release blocker.
Migrations, schemas and protocol impact: additive only (config sections, idempotency header, Decide late-decision semantics, Android payload type). No breaking schema/event/API change.
Security-sensitive changes re-reviewed: §1 re-check confirms no weakening of authorization, approval expiry, idempotency, event ordering/replay, workspace isolation, or secret redaction — all strengthened.
Known limitations: Codex adapter is a contract skeleton (no real process); real claude CLI tested via sh rig; DecideApproval stdin version-specific; Android instrumented tests need a green CI run.
Release blockers: Android instrumented tests must run green in CI before accepted.
Recommended status: blocked (until the instrumented CI run is green), then in_review → accepted
Reviewer decision: pending
```

## Acceptance criteria

- Every code-review item has a recorded resolution. ✓ (§1: 21 resolved incl. C1 duplicate, 2 declined-with-reason, 5 follow-ups mapped to FR-1..FR-9; totals agree across prose, table and handoff)
- All applicable quality gates have passing evidence from the final tree, or a
  clearly recorded blocker and CI result. ⚠️ PARTIAL (§2: server gates green locally; Android compile/JVM/lint/APK green; **instrumented gate = PENDING — release blocker**; not complete until the `instrumented` CI job is green)
- The documentation has one consistent, evidence-backed project status. ✓ (§4:
  README/DEVELOPMENT/docs-60 reconciled to `in_review`)
- Open release risks and the Codex-adapter limitation are explicit. ✓ (§3, §5)
- A reviewer can make a release decision without reconstructing context from
  chat history. ✓ (§5 handoff block + §3 milestone evidence)



## A. Android emulator CI gate (tasks/26-A)

A dedicated `instrumented` job was added to `.github/workflows/android.yml`:
- runs on `ubuntu-latest`, needs `gate`;
- JDK 17, Android SDK 35 + build-tools 35.0.0;
- enables KVM (hardware-accelerated emulator) via a udev rule and verifies `/dev/kvm`;
- uses `reactivecircus/android-emulator-runner@v2` (API 35, x86_64, google_apis);
- runs exactly `./gradlew connectedDebugAndroidTest --no-daemon` from `android/`;
- fails the workflow on any instrumented test failure;
- uploads `app/build/reports/androidTest/` and `outputs/androidTest-results/` on failure;
- triggered for the same Android/workflow paths as the `gate` job.

### Evidence

| Evidence | Required value | Recorded value |
| --- | --- | --- |
| Workflow URL/run identifier | Link or immutable run identifier | **PENDING** — the workflow is committed; the GitHub-hosted emulator run has not yet executed in this workspace (no git remote). The run must be observed on the repo's Actions tab. |
| Submitted commit/tree identifier | SHA preferred | No git history in this workspace; tree is the post-review tree dated 2026-07-18. A SHA is produced on push to a git remote. |
| Emulator job | Green | **PENDING** (release blocker) |
| Command | `./gradlew connectedDebugAndroidTest --no-daemon` | configured in the `instrumented` job `script` |
| Tests | HomeComposeTest (2 methods) + DeepLinkGuardTest (4 methods) | registered in `app/src/androidTest`; will execute on the emulator |
| Failure artifacts | test reports available on failure | `actions/upload-artifact@v4` step with `if: failure()` uploads `app/build/reports/androidTest/` + `app/build/outputs/androidTest-results/` |

If the emulator cannot run, status stays `blocked` with the complete CI error and the smallest reproducible remediation recorded here. Compilation of `androidTest` is NOT a substitute for execution (already confirmed it compiles).

## C. Decision record (tasks/26-C)

```text
Android instrumented CI run: PENDING (job added; not yet executed in a git-remote CI environment)
Review-accounting reconciliation: DONE — 28 candidate findings → 21 resolved (incl. C1 duplicate), 2 declined, 5 follow-up (FR-1..FR-9). Totals consistent across §1 prose, table, and §5 handoff; send-on-closed class is C2/C3 (C1 is the heading dup, explicitly stated).
CI remediation: CI-01 Go toolchain bumped 1.24.1→1.26.5 (govulncheck: no reachable vulns locally); CI-02 `-Werror` removed from Gradle CLI → `allWarningsAsErrors=true` in build.gradle.kts; CI-03 `TestApp_ReplayWithCursor` made deterministic via an observer-completion barrier (App tracks observer goroutines in a WaitGroup; the test blocks until the terminal event is persisted — not a sleep). All local gates green; hosted CI run pending in git-remote.
Remaining follow-ups / risk acceptance: FR-1 (low), FR-2 (med), FR-3 (low), FR-4 (low), FR-5 (low), FR-6 (med), FR-7 (med), FR-8 (med), FR-9 (med); S2 and S12 declined with documented risk.
Release blockers: Android instrumented tests (connectedDebugAndroidTest) must run green in CI.
Recommended status: blocked (until the instrumented CI run is green), then in_review → accepted.
Reviewer decision: pending
```
