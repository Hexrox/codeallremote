# Task 30 — Research: native Remote Control endpoints + prior art

**Status:** research findings (lead-authored, 2026-07-23). Feeds the go/no-go on
tasks 28–29 and on CAR's overall direction. No code.

## 1. Where native Remote Control connects (reverse-engineered)

Claude Code v2.1.214 is a 265 MB compiled ELF (bundled Node). `strings` + a
public reverse-engineering writeup (frr.dev) give the control-plane endpoints:

- **Register a machine/session:** `POST https://api.anthropic.com/v1/environments/bridge`
  → returns `environment_id`, `environment_secret`, `organization_uuid`.
- **Live channel (bidirectional):** `wss://api.anthropic.com/v1/session_ingress/ws/{session_id}`
  (internally "HybridTransport").
- **Fallback polling:** `GET https://api.anthropic.com/v1/environments/{env_id}/work/poll`.
- **Chrome/browser bridge (separate feature):** `wss://bridge.claudeusercontent.com`
  (+ `-staging`) — used by Claude-in-Chrome automation, not the mobile RC channel.
- Auth: rotatable `environment_secret`; sessions get a 3-word slug; claude.ai
  shows `https://claude.ai/code/session_...`. E2E-encrypted; outbound-only (no
  inbound port). Control + tool results flow through Anthropic's cloud.

The bridge URL is **hardcoded** (no `CLAUDE_CODE_BRIDGE_URL`). But relevant env
knobs exist: `CLAUDE_CODE_API_BASE_URL`, `CLAUDE_CODE_ASSUME_FIRST_PARTY_BASE_URL`,
`CLAUDE_CODE_CUSTOM_OAUTH_URL`, `CLAUDE_CODE_FORCE_BRIDGE`,
`CLAUDE_CODE_BRIDGE_SESSION_ID`, `CLAUDE_CODE_MOCK_REMOTE_SETTINGS`. There is a
full internal RC subsystem (`remoteControlDaemonWorker`, `remoteControlSpawnMode`,
`getRemoteControlPolicyVerdict`, …).

### Can we "swap the endpoints for our own"?
Plausible but not free:
- **Control plane vs inference are separate.** Inference = `ANTHROPIC_BASE_URL`
  (already pointed at CCR→GLM). RC control plane = the `/v1/environments/*` +
  `/v1/session_ingress/*` endpoints, likely following `CLAUDE_CODE_API_BASE_URL`.
  If separable (TO VERIFY), you could keep inference on CCR/GLM **and** point the
  RC control plane at a self-hosted "bridge" server.
- **Self-host path (Option X):** point `CLAUDE_CODE_API_BASE_URL` +
  `ASSUME_FIRST_PARTY_BASE_URL` (+ maybe `CUSTOM_OAUTH_URL`) at a CAR-run bridge;
  implement the three endpoints above; CAR's app becomes the mobile client.
  Reuses claude's **native** RC (session sync, transcript, tool-approval UX) — far
  less code than parsing stream-json ourselves.
- **Hard blockers:** the message shapes on `session_ingress/ws` are undocumented
  (RE effort, fragile to updates); Anthropic's own mobile app is hardcoded to
  their cloud, so **we must supply the client** (CAR app) — the native app won't
  talk to our bridge; `ASSUME_FIRST_PARTY`/OAuth behavior with a custom base is
  unverified and may be locked down.

## 2. Prior art — this is a crowded space (evaluate before building)

Several open-source projects already deliver "control/read Claude Code from a
phone," all by owning the **process/terminal or the session files** — none hijack
Anthropic's bridge. Because they drive the CLI, they are backend-agnostic and
work with CCR/GLM/DeepSeek out of the box.

| Project | Approach | Relevance |
|---|---|---|
| **siteboon/claudecodeui** (CloudCLI) | Node+React/Electron; **auto-discovers sessions from `~/.claude`**, chat UI, embedded shell, git; mobile-responsive; AGPL-3.0 | Closest to task 29-A/B (discover+read sessions on mobile). Likely works with your CCR setup today. |
| **buckle42/claude-code-remote** | tmux + ttyd (web terminal) + Tailscale | The exact tmux/VPN approach; simplest; fully backend-agnostic |
| **Adoom666/CloudeCode** | terminal streaming, auto-tunnel, persistent sessions | Live terminal mirror |
| **cducote/remoteCC** | QR-paired mobile terminal focused on **approving changes** | Mobile approvals (a CAR selling point) |
| **decolua/9remote**, MobileCLI, vibe-remote, MuxAgent | phone/browser terminal for Claude/Codex/Gemini | Whole category exists |
| **JessyTsui/Claude-Code-Remote** | control via email/Discord/Telegram + notifications | Async angle |

## 3. Lead assessment / recommendation

- **CAR's current build largely reinvents this space.** Its differentiators
  (approvals routed to the phone; a self-hosted, VPN-only control plane) exist
  elsewhere (remoteCC for approvals; the tmux/ttyd projects for the transport).
- **Fastest path to the operator's actual goal** ("see and drive my GLM/DeepSeek
  sessions from the phone") is to **try an existing tool first** — evaluate
  `siteboon/claudecodeui` (reads `~/.claude`, mobile, drives the CLI → CCR-agnostic)
  and the `buckle42` tmux+ttyd+VPN recipe. One afternoon vs a multi-week build.
- **Option X (self-host the native bridge)** is the most elegant end state
  (native RC UX + CCR inference) but is a real reverse-engineering project on an
  undocumented, changeable protocol, and still needs our own client. Only pursue
  if the existing tools are unsatisfactory and polish matters.

### Decision gate (before any further CAR feature work)
1. Run the native-RC-vs-CCR spike (task 29 prerequisite).
2. Spend an afternoon trialing `siteboon/claudecodeui` (and/or the tmux/ttyd
   recipe) against the real CCR/GLM homelab setup. If either satisfies the goal,
   **stop building CAR features** and adopt/fork it.
3. Only if both native RC and the existing tools fail the goal, continue CAR —
   and even then prefer Option X (reuse native RC via a self-hosted bridge) over
   the current bespoke stream-json approach, if the RE cost is acceptable.

## Sources
- frr.dev — "Anatomy of Claude Code's Remote Control: The Hidden API You Can't
  Use Yet"
- github.com/siteboon/claudecodeui, github.com/buckle42/claude-code-remote,
  github.com/Adoom666/CloudeCode, github.com/cducote/remoteCC,
  github.com/decolua/9remote, github.com/JessyTsui/Claude-Code-Remote
- Binary strings (claude 2.1.214, /usr/local/lib/node_modules/@anthropic-ai/claude-code)
