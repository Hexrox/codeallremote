# Android product specification

## Role

Android is CAR's primary work client. It must expose the important agent workflows without requiring a desktop terminal: start/resume a session, read progress, answer approvals, inspect changes, and recover after network loss.

## Information architecture

```text
Home
├── Active sessions
├── Pending approvals
├── Recent activity
└── Server/device status

Workspace
├── Sessions
├── Files and diff
├── Git summary
└── Workspace policy/status

Session
├── Chat / prompts
├── Live output
├── Timeline
├── Changed files
├── Approvals
└── Session settings
```

## Primary workflows

### Start a session

The user selects a registered workspace, selects the adapter, optionally supplies a title, and confirms the session policy. The app shows a pending/start state immediately, then replaces it with server-authoritative state.

### Supervise a run

The session screen keeps the current state visible above a scrollable transcript. Output may continue while the user views the timeline or diff. The app must never discard unread approval or failure events when changing tabs.

### Resolve approval

The notification opens a protected approval detail screen. The user sees action category, redacted command/context, workspace and expiry. Approve/deny requires explicit confirmation and displays the final server result.

## UX states

Every screen defines loading, empty, error, disconnected, unauthorized and stale-data states. A disconnected session remains readable with a clear “last updated” marker; destructive buttons are disabled until authorization and connectivity are restored.

## Privacy

Notification previews are generic by default. Transcripts and diffs are cached only in app-private storage and can be cleared per server. Screenshots and Android backups must not include tokens or unredacted secrets.

