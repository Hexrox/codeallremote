# Android navigation and state

## Navigation model

Use a single root navigation graph with stable deep links:

```text
/home
/servers/{serverId}/workspaces/{workspaceId}
/servers/{serverId}/sessions/{sessionId}
/servers/{serverId}/approvals/{approvalId}
/servers/{serverId}/pair
```

Deep links from notifications must validate the server, token and resource authorization before displaying private content.

## State ownership

- **Server state:** sessions, approvals, cursors and workspace summaries come from CAR APIs/events.
- **Device state:** selected server, navigation, draft prompt and notification preferences stay local.
- **Transient UI state:** scroll position, expanded cards and loading indicators are not synced.

## Event handling

The client persists the highest contiguous sequence per session. It applies events idempotently, then updates the screen from a derived snapshot. If a gap appears, it pauses live application for that session and requests replay or a full resync.

## Prompt drafts

Draft text is local to the selected session and is never sent until the user presses send. A draft survives rotation and temporary disconnection, but the app must clearly mark it unsent.

## Accessibility

Actions require accessible labels, sufficient contrast, large touch targets and screen-reader descriptions for state, risk and expiry. Terminal output must have a selectable plain-text alternative.

