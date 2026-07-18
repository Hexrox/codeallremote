# Path to real end-to-end functionality

`accepted` (2026-07-18) means the documented scope is implemented and both CI
workflows are green. It does **not** yet mean a person can drive a real Claude
Code session from an Android phone in a homelab. This task enumerates the gaps
between the accepted control plane and a working product, grouped by area and
tagged by who/what can close them:

- **[code-now]** — implementable and testable now against the deterministic fake
  rig; no real provider, no operator environment. Safe to hand to the agent.
- **[needs-claude]** — requires verification/iteration against a real `claude`
  binary; cannot be finished with the fake rig (CLAUDE.md forbids requiring real
  provider credentials in tests). Agent can prepare/research + structure the
  change; final confirmation is an operator step.
- **[ops]** — deployment/provisioning, not code.

Do not silently expand scope. Each item below is its own change with its own
acceptance; land them incrementally.

---

## A. Make the real Claude Code integration work — HIGHEST VALUE

The adapter is exercised only through the `sh` rig. Three concrete risks:

### A-1 [needs-claude] Verify/adjust the `claude` CLI flags
`internal/adapter/claude/claude.go` `buildArgs` (~line 433) runs
`claude -p --output-format stream-json --verbose`. `-p`/`--print` may be a
one-shot mode; multi-turn prompting over stdin must be confirmed. Verify against
the current `claude --help`; adjust `buildArgs` (and any session/turn handling)
to the real invocation. **Acceptance:** documented mapping from CAR run/turn
semantics to real `claude` flags; unit tests for `buildArgs` cover the chosen
mode; a manual operator smoke-run recorded (not in CI).

### A-2 [needs-claude] Confirm the approval/permission protocol
`DecideApproval` (~line 372) writes `{"decision":"approve"|"deny"}` on stdin.
This is a guess; real Claude Code negotiates tool permissions differently (e.g.
a permission-prompt tool / structured control protocol in stream-json), not a
freeform stdin JSON line. **Without this, remote approvals — a core CAR feature —
will not work against real Claude Code.** Research the real mechanism, redesign
`DecideApproval` + the approval-detection side of the parser to match it, keep
the CAR-facing contract unchanged. **Acceptance:** the approval round-trip
(detect permission request → phone approves/denies → agent proceeds/stops)
demonstrated against a real `claude`, recorded by the operator; fake-rig tests
updated to model the same shape.

### A-3 [needs-claude] Validate the OutputParser on a real stream
Confirm the `stream-json` event → CAR signal mapping (output/status/approval/
completion) on a real Claude Code stream, including partial-line and error
framing. **Acceptance:** parser handles a captured real transcript; regression
fixtures added from that capture (secrets scrubbed).

> A-1..A-3 are the gate to real usefulness. They need a real binary; the agent
> can prepare the code and research, but final sign-off is an operator smoke test.

---

## B. Remote notifications while away from the phone — currently not working in background

Today notifications use a local `NotificationManager` (`NotificationRouter.kt`)
and only fire while the app holds a live WebSocket. `FOREGROUND_SERVICE`
permissions are declared but no foreground `Service` exists, and there is no
push (FCM/ntfy). This misses the "phone-first, notified when away" product goal.

- **B-1 [code-now]** Foreground service that owns the WS connection with
  reconnect/backoff, so events/approvals arrive while the app is backgrounded.
  **Acceptance:** instrumented/robolectric test that the service survives
  backgrounding and re-establishes the WS; notification fires on an approval
  event received in background.
- **B-2 [ops/code]** Optional server-initiated push (FCM or self-hosted ntfy)
  for delivery when the app/process is killed. Server emits identifiers-only
  payloads (no transcript). **Acceptance:** documented push path + a delivery
  test with a stub push endpoint.

---

## C. Deploy in the homelab and make it reachable — [ops]

- **C-1** Run the server (Docker or systemd unit from `deploy/`). Fill
  `config.json`: real `workspaces` paths, `adapters.exec_path` → the real
  `claude`, and secrets (`ANTHROPIC_API_KEY` or the CCR base URL) in the server
  environment (never in config/backup — see `CCR_WIRING_CHANGES.md`).
- **C-2** Stand up WireGuard + the VPS reverse proxy (Caddy TLS). `deploy/` ships
  only `.example` files; provision a real VPS, keys, DNS and firewall.
- **Acceptance:** `/health` + `/ready` green through the VPS; a paired phone can
  reach the API/WS over WSS.

---

## D. Ship an installable Android app

`android/app/build.gradle.kts` has no `signingConfig` ("signing configured
per-environment"). To install on a phone you need a signed build.

- **D-1 [code-now]** Add a release `signingConfig` wired from environment/
  keystore properties (no secrets committed). **Acceptance:** `assembleRelease`
  produces a signed APK from CI/local given a provided keystore; unsigned build
  still works without the keystore.
- **D-2 [ops]** Install on the device and complete the pairing flow.

---

## E. Persistence & robustness (functional follow-ups) — mostly [code-now]

- **E-1 [code-now]** Persist access tokens. `internal/identity/service.go` keeps
  `tokens` in-memory (~line 53), so a server restart invalidates active sessions
  (paired devices survive in DB). Persist tokens or implement a clean re-auth on
  restart. **Acceptance:** token survives (or cleanly refreshes after) a restart;
  regression test.
- **E-2 [code-now]** CI-05 hardening follow-up: drain `ErrorChannel()` in the
  claude adapter (or make `errCh` backpressure explicit) so a child that floods
  stderr past the buffer cannot stall process reaping. **Acceptance:**
  `-race` test with a stderr-flooding child completes and reaps.
- **E-3 [code-now]** FR-6: subscribe-before-replay in the WS gateway to close the
  event-loss window. **Acceptance:** test proving no event appended during the
  replay window is dropped.
- **E-4 [code-now]** FR-8: `ApprovalBridge.Cancel` on run exit so pending
  approvals resolve as `cancelled` (not silently expired). **Acceptance:**
  pending approval on run exit → `cancelled`; test.
- **E-5 [code-now]** FR-2: re-check device revocation on the WS heartbeat, so a
  revoked device's live socket is dropped promptly. **Acceptance:** revoking a
  device closes its live WS within one heartbeat; test.
- **E-6 [code-now, optional]** FR-9: real Codex adapter (process spawn + parser),
  if a second agent is wanted. **Acceptance:** Codex adapter drives a real (or
  faked-at-the-rig) Codex process end to end.

---

## Suggested order

Minimum path to a first real "I steer Claude from my phone":
**A-1 → A-2 → A-3** (real integration) → **C** (deploy + WireGuard/VPS) →
**D** (signed APK + pairing). **B** and **E** are quality/robustness — B is
important for the product promise but does not block a first bring-up.

The **[code-now]** items (B-1, D-1, E-1..E-5) can proceed in parallel with the
operator-dependent A/C work.
