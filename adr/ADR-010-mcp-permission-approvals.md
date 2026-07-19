# ADR-010: Real Claude Code approvals via an MCP permission-prompt server

**Status:** accepted (protocol verified; implementation incremental)
**Date:** 2026-07-19
**Depends on:** ADR-009 (A-2). Changes the **approval trust boundary**, so this
is its own reviewed increment per `CLAUDE.md`.

## Problem

In headless `claude -p`, tool uses **auto-run with no prompt** under
`--permission-mode default` and `manual` (verified: a Bash `rm` executed,
`permission_denials: []`). So the adapter's terminal-text approval detection
never fires — there is nothing to parse. The only interception point is
`--permission-prompt-tool <tool>` backed by an MCP server the operator provides.

## Verified protocol (claude 2.1.214, live)

Running `claude -p … --permission-mode default --strict-mcp-config
--permission-prompt-tool mcp__car__approve --mcp-config <cfg>` where the MCP
server `car` exposes a tool `approve`, claude drives this exchange over the MCP
transport (newline-delimited JSON-RPC 2.0 on stdio, confirmed):

1. `initialize` → server returns `{protocolVersion, capabilities:{tools:{}},
   serverInfo:{name,version}}`. clientInfo is `claude-code` 2.1.214.
2. `notifications/initialized` (no id).
3. `tools/list` → server returns the `approve` tool (name + inputSchema).
4. `tools/call` for each tool use needing permission, with:

```json
{"name":"approve","arguments":{
  "tool_name":"Bash",
  "input":{"command":"rm -f …","description":"…"},
  "tool_use_id":"toolu_…"
}}
```

The server MUST reply with an MCP tool result whose text content is a JSON
decision (verified both branches):

- **Allow** → tool runs:
  `{"content":[{"type":"text","text":"{\"behavior\":\"allow\",\"updatedInput\":{…}}"}]}`
  (`updatedInput` echoes/edits `input`).
- **Deny** → tool is blocked and appears in the final `result.permission_denials`
  (`[{tool_name, tool_use_id, tool_input}]`):
  `{"content":[{"type":"text","text":"{\"behavior\":\"deny\",\"message\":\"<reason>\"}"}]}`

Note: `--bare` and inherited `CLAUDE_CODE_*`/`CLAUDECODE` env force
auto-approval and bypass the permission tool; a clean env + `default` mode is
required for the permission tool to be consulted.

## Design

- **`internal/mcpperm`**: a reusable Go MCP permission server implementing the
  handshake + the `approve` tool over an `io.Reader`/`io.Writer` (stdio-shaped),
  with a decision callback
  `Decide(toolName string, input json.RawMessage, toolUseID string) Decision`,
  where `Decision{Allow bool; Message string; UpdatedInput any}`. Fully
  unit-testable by feeding JSON-RPC lines — no `claude` needed. (Increment 1.)
- **Transport wiring**: CAR spawns `claude` with `--permission-prompt-tool
  mcp__car__approve --mcp-config <generated>` where the config runs a small CAR
  helper (`cmd/car-mcp-perm`) as a stdio MCP server. The helper reaches the
  running CAR server's `ApprovalBridge` over a local socket to raise a real
  approval, waits for the phone's decision, and returns allow/deny. (Increment
  2 — the IPC + adapter flag wiring.)
- **ApprovalBridge routing**: the permission request becomes an `ApprovalBridge`
  request (category from `tool_name`, human-readable context from `input`);
  the operator's phone decision becomes the allow/deny reply. (Increment 3.)

## Consequences / trust boundary

- New local surface: an MCP server claude talks to. It is loopback/stdio only,
  never exposed to the network; it carries tool names + inputs (which may
  include file paths / commands) — these reach the ApprovalBridge and phone as
  approval context, consistent with existing approval data. No secrets are added
  to logs.
- Until increments 2–3 land, the Claude adapter's `DecideApproval` stdin path
  stays a no-op (ADR-009); real approvals are not functional. Increment 1
  (the verifiable server core) does not change runtime behavior on its own.
- End-to-end verification uses a real authenticated `claude` (available) with
  the Go server replacing the Python stub used to capture this protocol.

## Increment 2/3 design (how the decision reaches the phone)

The permission decision must flow through the EXISTING approval architecture,
not a new side-channel, so the phone/audit/expiry semantics are reused. Chosen
flow (fits the adapter→app→ApprovalBridge path):

1. **Adapter owns the socket.** When the Claude adapter starts a real `claude`
   run it: creates a per-run unix socket, writes an `--mcp-config` naming
   `cmd/car-mcp-perm` with `--socket <path> --session <id>`, and passes
   `--permission-prompt-tool mcp__car__approve --mcp-config <cfg>` (Increment 3).
2. **car-mcp-perm is a thin client.** Its Decide callback dials the socket,
   sends `{session, tool_name, input, tool_use_id}` (one JSON line), and blocks
   reading one JSON reply `{allow, message}` (Increment 2). No policy lives here
   — it is pure transport, still fail-closed if the socket is unavailable.
3. **The adapter turns a socket request into a normal approval.** On a socket
   request it emits a `SignalApprovalRequest` (approval id = `tool_use_id`,
   category from `tool_name`, human context from `input`) and PARKS the pending
   socket reply keyed by `tool_use_id`. The app routes the signal to the
   `ApprovalBridge` → phone exactly as today.
4. **DecideApproval resolves the parked request.** `App.ResolveApproval` calls
   `adapter.DecideApproval(runID, tool_use_id, approved)`; the adapter looks up
   the parked socket reply for that id and writes `{allow, message}` back over
   the socket → car-mcp-perm returns it to claude. This REPLACES the bogus stdin
   write with a real resolution, and closes ADR-009's A-2 gap. Approval expiry /
   cancel (E-4) map to a fail-closed deny on the socket.

Properties: loopback unix socket only; the tool name + input become approval
context (same class of data as existing approvals, no new secrets in logs);
fail-closed on any transport error, timeout, or expiry. Verified end to end with
a real authenticated `claude` once wired.

Increment status: **1 done + verified** (mcpperm library, car-mcp-perm skeleton).
**2** = socket request/response transport (reusable, unit-testable). **3** =
adapter socket owner + mcp-config generation + DecideApproval rewrite.
