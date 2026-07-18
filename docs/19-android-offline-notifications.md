# Android offline and notifications

## Offline policy

CAR is not an offline agent. Android may display cached state and compose drafts offline, but it must not claim that a prompt, approval or interrupt succeeded until the server acknowledges it.

## Reconnect

1. Detect transport loss without immediately logging out.
2. Keep cached session state visible with a stale marker.
3. Refresh access token if needed.
4. Reconnect WebSocket with per-session cursors.
5. Apply replay, or fetch a snapshot when the server requests resync.
6. Reconcile queued idempotent commands and show conflicts explicitly.

## Push notifications

Push is a hint, not the source of truth. The payload contains a server ID, resource ID, event category and notification nonce; it contains no command text, filename, transcript or credential.

Notification categories:

- approval requested;
- run completed or failed;
- server/device security event.

Opening a notification always fetches current state. A stale or already-resolved approval opens its final result rather than presenting an actionable approve button.

## Battery and reliability

The app uses push to wake for important events and a foreground connection only while the user is actively viewing a session. Backoff is bounded and jittered; repeated failures are visible in server status rather than hidden in a tight retry loop.

