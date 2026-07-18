# M3 tasks — homelab deployment

## M3-01 — Package CAR for local deployment

**Inputs:** `docs/20-deployment-homelab.md`.

**Acceptance criteria:**

- CAR binds only to the intended local/WireGuard interface.
- Database and artifacts use explicit persistent paths.
- Health and readiness checks are available to the local supervisor.

## M3-02 — Configure WireGuard and VPS proxy

**Acceptance criteria:**

- Only the CAR host and VPS are WireGuard peers.
- Public proxy exposes TLS plus the documented WebSocket upgrade.
- Direct Internet access to CAR is blocked by firewall rules.
- VPS configuration contains no workspace data or durable CAR secrets.

## M3-03 — Backups and restore drill

**Acceptance criteria:**

- Database and artifacts are backed up together and encrypted.
- Restore is tested on a separate instance.
- Recovery documentation states RPO/RTO assumptions and how sessions are reconciled.

