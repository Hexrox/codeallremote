# M3 decomposed tasks

## M3-04 — Homelab service account

Define the OS account, workspace ownership, filesystem permissions and process limits used by CAR. Verify the account cannot read unrelated home directories.

## M3-05 — VPS proxy hardening

Document TLS renewal, WebSocket timeouts, request-size limits, access logs without payloads and a deny-by-default upstream policy.

## M3-06 — WireGuard rotation

Document peer key rotation, revocation, allowed IPs and recovery when the VPS key is lost. No client token is reused as a WireGuard credential.

## M3-07 — Monitoring runbook

Define alerts for CAR down, storage failure, stale backup, WireGuard loss, approval backlog and repeated adapter failures, with an owner action for each.

