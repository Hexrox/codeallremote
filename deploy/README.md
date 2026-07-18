# CAR deployment (M3)

This directory packages CAR for homelab deployment. The CAR server binds
only to the intended local/WireGuard interface; database and artifacts use
explicit persistent paths; health and readiness checks are available to the
local supervisor.

## Files

- `Dockerfile` — stateless image running the server binary as a non-root
  `car` user. Config and data are volume-mounted (no secrets in the image).
- `car.service` — systemd unit with hardening (NoNewPrivileges, ProtectSystem,
  etc.). Owns `/var/lib/car` (durable data) and `/etc/car` (config).
- `config.example.json` — minimal config binding to 127.0.0.1 with explicit
  SQLite path and a workspace pointing only at homelab directories.

## Health vs readiness

- `/health` — process liveness (always 200 while the process runs).
- `/ready` — readiness: storage reachable (503 otherwise).

The systemd unit and Dockerfile `HEALTHCHECK` probe `/health`; orchestration
that needs to gate traffic should probe `/ready`.

## Persistent paths

- `/etc/car/config.json` — configuration
- `/var/lib/car/car.db` — SQLite database
- `/var/lib/car/artifacts/` — terminal/transcript artifacts

These MUST be backed up together (encrypted). See `docs/16-storage-and-retention.md`.

## Interface binding

`config.example.json` sets `server.host = "127.0.0.1"`. For WireGuard-only
access, set `server.host` to the WireGuard interface address. Never bind CAR
to the public Internet interface; the VPS reverse proxy handles public TLS.
