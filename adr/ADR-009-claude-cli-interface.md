# ADR-009: Claude Code CLI interface (flags, stream-json, approvals)

**Status:** proposed (A-1 flags implementable now; A-2 approvals require design + review)
**Date:** 2026-07-19
**Supersedes guesses in:** `internal/adapter/claude/claude.go` (`buildArgs`, stdin prompt delivery, `DecideApproval`, `approvalDecisionLine`) and `tasks/28` A-1..A-3.

## Context

The Claude Code adapter was written against an **assumed** `claude` CLI
interface and never verified against a real binary (tests use an `sh` rig). Two
assumptions are wrong or incomplete. This ADR records the **authoritative**
interface from the official docs (code.claude.com, fetched 2026-07-19) so the
adapter can be corrected, and states which changes need an operator smoke-test
with a real `claude` before they can be trusted.

## Findings (authoritative)

### Flags (A-1)

- `-p` / `--print` — non-interactive print mode.
- `--output-format` ∈ {`text`, `json`, `stream-json`}; `stream-json` is
  newline-delimited JSON and, in print mode, requires `--verbose`.
- `--input-format` ∈ {`text`, `stream-json`}. **This is the gap:** the adapter
  submits multiple prompts over stdin (multi-turn), but without
  `--input-format stream-json`, `-p` treats stdin as a **single** prompt
  (one-shot). Multi-turn over stdin requires `--input-format stream-json`, and a
  message sent while Claude is working is queued and run as its own turn.
- `--bare` — recommended for scripted/SDK calls; skips auto-discovery of hooks,
  skills, plugins, MCP, CLAUDE.md. Deterministic for a supervised runner.
- Auth via `ANTHROPIC_API_KEY` (or a base URL for a router). CAR already passes
  env to the child; keep secrets out of argv/logs (unchanged).

Current `buildArgs` adds `-p --output-format stream-json --verbose`. **Correct**
for output; **missing** `--input-format stream-json` (and optionally `--bare`)
for multi-turn stdin.

### stream-json input message shape (A-1, needs verification)

With `--input-format stream-json`, each stdin line is a JSON user message of the
Anthropic message shape, approximately:

```json
{"type":"user","message":{"role":"user","content":[{"type":"text","text":"<prompt>"}]}}
```

The adapter currently writes the **raw prompt string** to stdin
(`WriteInputString(input.InitialPrompt)` / `SubmitInput`). Under
`--input-format stream-json` that is invalid — prompts must be wrapped in the
JSON envelope above, one per line. **Operator must confirm** the exact top-level
`type` token and content-block schema against the installed `claude` version
before this is trusted (docs summaries were ambiguous on `"user"` vs
`"user_message"`).

### stream-json OUTPUT events (A-3)

Output is NDJSON; each line has a `type`:
- `system` (subtype `init` first — session metadata, model, tools, capabilities;
  also `api_retry`, `plugin_install`),
- `assistant` / `user` (message blocks, incl. `tool_use` / `tool_result`;
  `parent_tool_use_id` distinguishes subagent messages),
- `stream_event` (partial deltas, `event.delta.type == "text_delta"`; only with
  `--include-partial-messages`),
- `result` — **last line**, final response text + cost + session metadata.

The parser must map: `assistant` text → output signal; `result` → completion;
`system/init` → status/active; tool-permission prompts → approval (see A-2).
Regression fixtures should be captured from a real transcript (secrets scrubbed).

### Approvals / permissions (A-2) — the current mechanism is wrong

`DecideApproval` writes `{"decision":"approve"|"deny"}` to stdin. **Claude Code
has no such stdin decision protocol.** Non-interactive permission handling is one
of:
- `--permission-prompt-tool <mcp-tool>` — Claude **calls an MCP tool** to ask for
  permission; the tool's response allows/denies. Claude waits for that MCP
  server to connect (≤ `MCP_TIMEOUT`, ~30s).
- `--allowedTools` / `--permission-mode` (`acceptEdits`, `dontAsk`,
  `bypassPermissions`) — static pre-approval.
- `--dangerously-skip-permissions` — skip all (not acceptable for CAR).

**Correct CAR design:** run an in-process **MCP server** exposing a
permission-prompt tool, launch `claude … --permission-prompt-tool <name>
--mcp-config <cfg>`, and route each permission request from that tool into the
`ApprovalBridge` → phone → return the decision as the tool result. This changes
the **approval trust boundary** (a new local MCP surface) and the approval data
flow, so per `CLAUDE.md` it must not be silently rewritten — it needs its own
design/ADR increment and an operator smoke-test.

## Decision

1. **A-1 (flags):** add `--input-format stream-json` (and `--bare`) in
   `buildArgs`, and deliver stdin prompts as stream-json user-message envelopes.
   Implementable now; the exact envelope `type`/schema is **operator-verified**
   against a real `claude` before release.
2. **A-3 (parser):** map the documented output event types above; add fixtures
   from a real transcript. Prepare now; verify against a real stream.
3. **A-2 (approvals):** replace the stdin decision with the
   `--permission-prompt-tool` + CAR MCP-server flow. **Do not ship the stdin
   approach.** This is a separate, reviewed increment (new trust boundary);
   until then the Claude adapter's approvals are **not functional against a real
   `claude`**, and this is recorded as a known limitation.

## Operator verification checklist (needs a real `claude`)

- Confirm `claude -p --output-format stream-json --input-format stream-json
  --verbose --bare` accepts multi-turn stdin and the exact user-message JSON.
- Capture a real stream-json transcript (with a tool-permission prompt) → parser
  fixtures.
- Confirm the `--permission-prompt-tool` MCP round-trip shape (request/response).

## Consequences

- Honest status: real Claude Code integration is **not** yet functional;
  approvals especially require the MCP redesign. The adapter's `sh`-rig tests
  remain valid for the process/streaming plumbing but do not prove the real CLI
  contract.
- CAR-facing REST/WS/approval contracts are unaffected by A-1/A-3; A-2 changes
  only how the adapter talks to `claude`, not how the phone talks to CAR.
