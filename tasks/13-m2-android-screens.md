# M2 tasks — Android screens

## M2-13 — Home and server detail

**Inputs:** `docs/37-android-screen-spec.md`, `docs/39-android-api-mapping.md`.

**Acceptance criteria:**

- Active sessions and pending approvals are visible without opening each workspace.
- Connectivity, stale data and authorization states are distinct.
- Server removal does not delete remote sessions or data.

## M2-14 — Workspace and session detail

**Acceptance criteria:**

- Session header always shows workspace, adapter, lifecycle and connection state.
- Chat, output, timeline, changes and approvals share one authoritative session projection.
- Navigation preserves unsent prompt drafts.

## M2-15 — Diff and file review

**Acceptance criteria:**

- Partial/stale change data is clearly labelled.
- Paths are rendered as server-provided display values, not used as local filesystem paths.
- Large output remains usable on a phone and has a selectable text fallback.

## M2-16 — Settings and privacy controls

**Acceptance criteria:**

- Biometric approval requirement, notification privacy and cache clearing are configurable.
- Clearing cache never revokes server authorization accidentally.
- No token, provider credential or raw secret is displayed.

