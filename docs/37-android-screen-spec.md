# Android screen specification

## Home

Shows server connectivity, active sessions, pending approvals and recent activity. Primary actions are “open session” and “pair server”. Loading uses skeleton cards; empty state explains how to register a workspace; stale state shows last refresh time.

## Server detail

Shows server name, CAR version, connection status, paired-device identity and workspace list. Destructive actions (remove server, revoke device) require confirmation and never delete homelab data.

## Workspace detail

Shows workspace display name, health, configured adapter capabilities, recent sessions and changed-file summaries. The client does not expose arbitrary filesystem browsing; file views are scoped to server-approved workspace results.

## Session detail

The header shows session title, workspace, adapter, lifecycle state and connection indicator. Tabs contain Chat, Output, Timeline, Changes and Approvals. A persistent composer sends prompts and indicates unsent drafts.

## Approval detail

Shows action category, risk, redacted context, expiry countdown, session/workspace and approve/deny controls. When the approval is no longer pending, controls are replaced with the final decision and actor.

## Changes and files

Changes show file path, status, additions/deletions and diff. The app must support a plain-text fallback and must not claim that a diff is complete when the server reports a partial or stale snapshot.

## Settings

Contains notification preferences, biometric requirement, cache clearing, server management, diagnostics and privacy information. Provider credentials and raw server secrets never appear here.

