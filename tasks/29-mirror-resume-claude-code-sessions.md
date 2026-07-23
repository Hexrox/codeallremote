# Task 29 — Discover, read, and resume EXISTING Claude Code sessions from the phone

**Status:** proposed (lead-authored spec; not yet implemented)
**Owner decision required:** yes (see §Decisions)
**Relates to:** ADR-009 (claude CLI interface), ADR-010 (MCP approvals),
ADR-011 (transport), CCR_WIRING_CHANGES.md, task 28.

## PREREQUISITE SPIKE (do this BEFORE any 29 coding)

Claude Code has **native Remote Control** (`claude --remote-control [name]`,
`claude remote`) — control a live session from the Claude mobile app. It requires
a **claude.ai OAuth login** and relays through Anthropic's cloud. CAR's entire
premise (CCR_WIRING_CHANGES.md) is that native Remote Control **breaks when the
session is routed to a non-Anthropic backend via CCR**. As of 2026-07-23 that is
an *assumption*, not a verified fact.

Evidence leaning toward the premise: CCR routes via `ANTHROPIC_BASE_URL` +
`ANTHROPIC_AUTH_TOKEN=test` (API-key mode); the CLI states OAuth/keychain are not
read in API-key/`--settings` mode; Remote Control needs OAuth. Untested path:
OAuth login **plus** only `ANTHROPIC_BASE_URL` overridden (no AUTH_TOKEN), which
might keep Remote Control working while inference goes to CCR/GLM/DeepSeek.

**Spike (operator, ~2 min):** `claude auth login`, then
`ANTHROPIC_BASE_URL=http://127.0.0.1:3456 claude --remote-control`; in the Claude
mobile app check (a) the session appears and is controllable, and (b) inference
is served by DeepSeek/GLM (not Anthropic).

**Decision gate:**
- If BOTH hold → native Remote Control already delivers this task's goal for
  CCR-routed sessions. **Do NOT build task 29**; use native. Re-scope CAR to
  whatever native does not cover (or wind it down for this use case).
- If it fails (no connect / forces Anthropic inference) → the premise holds and
  task 29 is justified; proceed.

Everything below is contingent on this spike failing.

## Why (the real product goal)

The operator's expectation, verbatim: *"I run Claude Code in my homelab (e.g.
with GLM via CCR); I want to find that session on the phone, read its
conversation, and send a message to it."* — i.e. Claude Code's native
**Remote Control**, but for sessions routed to alternative backends (CCR/GLM),
which the Anthropic relay cannot control.

Today CAR only **owns sessions it launches itself** (`claude -p …`). It does not
see, read, or drive the operator's own interactive sessions (`claude --resume`
in tmux, `ccr code`). This task closes exactly that gap. It is the feature that
makes CAR match its own vision ("preserve remote control that breaks when you
leave the Anthropic cloud").

## Verified facts (homelab, 192.168.2.16, 2026-07-23)

- Claude Code persists every session as a JSONL transcript at
  `~/.claude/projects/<url-encoded-cwd>/<session-uuid>.jsonl`. The operator has
  **88 transcripts** across ~10 projects under `/home/hexan/.claude/projects/`.
  Subagent transcripts live under `<uuid>/subagents/*.jsonl`.
- Transcript lines are typed JSON: `type: user` / `assistant` carry the dialog
  (`message.role`, `message.content`); other line types include
  `permission-mode`, `file-history-snapshot`, `attachment`. Fully parseable.
- `claude` supports `-r, --resume [session-id]` and it **works with `--print`**
  — so a specific session can be continued headlessly and its new turns append
  to the **same** transcript.
- The operator's interactive sessions run as user **`hexan`**; CAR runs as
  **`car`**. Different HOMEs → this is the main access decision (see §Decisions).
- Live right now: 2 operator interactive Claude Code sessions (one plain
  `claude --resume`, one `ccr code`), the CCR router daemon, and CAR-managed
  sessions.

## The core constraint (must be honest in UX)

**One live driver per session.** `--resume` starts a process that continues a
session; a session cannot be safely driven from two live processes at once
(tmux TUI *and* a phone-resume would diverge). The supported model is **hand-off,
not co-driving**: drive in the terminal, then continue from the phone (or vice
versa). Claude Code is designed around resumability, so this is the natural flow.
The UI MUST make it clear that opening/sending from the phone resumes the session
(and MUST warn/disable if that session appears to have a live process attached).

## Design approaches (owner must choose — see §Decisions)

Verified on the homelab (2026-07-23): the operator runs interactive sessions in
**detached tmux** (`claude`, `claude2`, routed to DeepSeek/GLM via CCR, some with
`bypass permissions on`). `tmux capture-pane` and `tmux send-keys` work on a
detached session; the live process appends to the same `<uuid>.jsonl` transcript.
Note: `car` cannot reach `hexan`'s tmux socket (`/tmp/tmux-995/default`) — the
user/access decision below applies to tmux too.

- **Approach 1 — live tmux co-drive.** Read the pane with `capture-pane`, send
  input with `send-keys` to the *live* process. Pro: true live co-driving of the
  operator's existing session, no hand-off. Con: it screen-scrapes a TUI (ANSI,
  box-drawing, spinners, redraws, truncation) — lossy read; `send-keys` is blind
  keystroke injection dependent on TUI state; approvals become TUI-dialog driving
  (and vanish under `bypass permissions`); coupled to tmux + exact session name.
- **Approach 2 — transcript + resume** (the 29-A/B/C body below). Clean
  structured read (JSONL) + clean stream-json send via `claude -p --resume` +
  real MCP approvals. Con: hand-off model, not live co-drive of the tmux process.
- **Approach 3 — CAR owns the pty/tmux from launch.** Clean AND live AND locally
  attachable, but only for sessions CAR starts (not pre-existing ones).
- **Approach 4 — HYBRID (recommended to evaluate first).** Read from the clean
  `<uuid>.jsonl` (29-A/B), and for a session detected as LIVE in tmux, **send via
  `tmux send-keys` to co-drive it**; fall back to `--resume` (29-C) only when the
  session is not live. Gives clean read + live send without a resume fork, because
  the tmux process and the transcript are the same session. Con: still tmux-coupled
  for the send path; approval UX depends on the session's permission mode.

## Scope — three capabilities, land incrementally

### 29-A [code-now] Discover sessions from transcripts
Read-only enumeration of Claude Code sessions from the transcript directory.
- Server: a discovery service that scans a configured roots list
  (`session_discovery.roots`, e.g. `/home/hexan/.claude/projects`) for
  `<uuid>.jsonl` files; returns per session: `session_id` (uuid), project path
  (decoded from the dir name), derived **title** (first user message, truncated),
  `last_modified`, message count, and whether a live process appears attached
  (best-effort: match a running `claude`/`ccr` cwd/args — advisory only).
- New REST: `GET /api/v1/discovered-sessions` (owner-auth). Additive; does not
  touch the existing sessions model.
- **Acceptance:** against a fixture transcript dir, the endpoint lists sessions
  with correct id/title/project/last_modified/count; subagent files are excluded
  from the top-level list; malformed lines are skipped, not fatal. Unit tests use
  committed JSONL fixtures (no real provider).

### 29-B [code-now] Read a session transcript
- Server: `GET /api/v1/discovered-sessions/{id}/transcript?after=<n>&limit=<n>`
  returns the parsed dialog as an ordered list of normalized messages
  (`role`, `content` text, tool_use/tool_result summarized, timestamp). Redact
  nothing beyond what CAR already redacts; never log transcript bodies.
- App: a read-only conversation view (reuse/extend the session-detail transcript
  UI) that renders user/assistant turns. Distinct from CAR-owned sessions in the
  list (a "Claude Code (discovered)" section).
- **Acceptance:** a fixture transcript renders as the correct ordered dialog;
  large transcripts paginate; tool calls show as collapsible summaries, not raw
  JSON. Instrumented UI test against the fixture.

### 29-C [needs-claude] Send a message → resume the session
- Adapter: extend the claude adapter to start a run in **resume mode** —
  `claude -p --output-format stream-json --input-format stream-json --resume <uuid>`
  with the workspace = the session's project cwd and the same CCR env. Reuse the
  existing stream-json input + MCP-approval wiring (task 28 / ADR-010) unchanged.
- Server: `POST /api/v1/discovered-sessions/{id}/messages` (owner-auth,
  idempotency-key) — starts (or reuses) a resume run for `{id}`, delivers the
  message, and streams output/approvals over the existing WS/event path so the
  phone sees the reply and any tool approvals live. New turns append to the same
  `<uuid>.jsonl`.
- App: send box on the discovered-session view; live output + approvals.
- **Acceptance:** operator-recorded smoke run — resume a real prior GLM/CCR
  session by id, send a message from the phone, get the reply routed through CCR,
  and confirm the new turns landed in the same transcript file; a tool-using
  message surfaces an approval on the phone and proceeds on approve. Fake-rig
  tests model the resume-run shape without a real provider.

## Decisions the owner must make (blockers)

1. **User alignment / transcript access.** CAR (`car`) must read + resume
   `hexan`-owned transcripts. Pick one:
   - (a) run the operator's interactive Claude Code sessions as `car` (single
     user, cleanest);
   - (b) a shared `projects` dir / group with read+resume access for `car`;
   - (c) run the CAR claude adapter as `hexan` for discovered sessions (revisits
     the non-root decision — note `--dangerously-skip-permissions` is blocked as
     root but fine for a normal login user like `hexan`).
   Recommendation: (a) or (b).
2. **Co-driving policy.** Confirm "one live driver per session" is acceptable
   (hand-off model), and how aggressively the UI should block sending to a
   session that looks live in a terminal (warn vs hard-disable).
3. **Roots config.** Which project roots to expose (all of
   `~/.claude/projects`, or an allow-list of projects).
4. **Send path: live co-drive vs hand-off vs hybrid.** Choose the §Design
   approach: (1) tmux `send-keys` live co-drive, (2) `--resume` hand-off, or
   (4) hybrid (clean JSONL read + tmux send for live sessions, resume otherwise).
   Recommendation: prototype **Approach 4** — it matches the operator's mental
   model ("see and drive my live DeepSeek/GLM session") most closely while
   keeping the read clean. Accept that live send inherits the session's own
   permission mode (no CAR approval when the session runs `bypass permissions`).

## Non-goals / scope guards

- No change to CAR-owned session semantics, the approval contract, or the
  transport policy.
- If Approach 1/4 (tmux) is chosen, the read still comes from the JSONL
  transcript, NOT from parsing `capture-pane` output — TUI scraping for *display*
  is out of scope (too lossy); `capture-pane` is only for liveness detection.
- Discovery/read are additive read-only endpoints; they must not mutate
  transcripts. Only 29-C (resume) appends, via `claude` itself.

## Contract / ADR impact

- New ADR (ADR-012) for "discovered/resumed sessions": the new endpoints, the
  discovery trust boundary (reading another user's transcripts), and the
  single-driver constraint. Additive to the REST contract with fixtures for the
  new payloads; no breaking changes to existing endpoints.

## Suggested order

29-A (discover) → 29-B (read) → 29-C (resume/send). A and B are fake-rig-testable
now and deliver immediate value (see your sessions + read them on the phone); C
needs the owner decision + a real-claude smoke run.
