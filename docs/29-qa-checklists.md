# QA checklists

## Core

- [ ] Invalid session transitions are rejected.
- [ ] Duplicate idempotency keys do not duplicate actions.
- [ ] Event sequence remains ordered under concurrent writes.
- [ ] Adapter crash and restart are visible and recoverable.

## Remote control

- [ ] WebSocket reconnect replays without gaps or duplicates.
- [ ] Expired approvals cannot be approved later.
- [ ] Revoked devices lose access immediately.
- [ ] Push notifications contain no sensitive payload.

## Android

- [ ] Offline state is clearly marked.
- [ ] Unsynced prompt drafts are preserved.
- [ ] Deep links require authorization.
- [ ] Screen-reader labels and contrast pass accessibility review.

## Deployment

- [ ] CAR is unreachable from the public interface except through the proxy/tunnel.
- [ ] Backup restore succeeds on a clean instance.
- [ ] VPS contains no workspace or transcript data.
- [ ] Logs and diagnostics contain no credentials or raw prompts.

