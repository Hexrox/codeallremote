# CAR remote access: WireGuard + VPS reverse proxy (M3-02)

CAR binds only to the homelab's loopback/WireGuard interface. Public access
flows phone → VPS (TLS) → WireGuard → CAR. The VPS is **stateless transport**:
it must not receive workspace mounts, database files, transcripts, or
long-lived CAR secrets.

## Topology

```
Android ──HTTPS/WSS──> VPS (reverse proxy, TLS) ──WireGuard──> CAR (homelab, loopback)
```

Rules (docs/20-deployment-homelab.md):
- Only the CAR host and VPS are WireGuard peers.
- Public proxy exposes TLS plus the documented WebSocket upgrade.
- Direct Internet access to CAR is blocked by firewall rules.
- VPS configuration contains no workspace data or durable CAR secrets.

## WireGuard peers

Two peers only: `vps` and `homelab`. Keys are generated on each host and
exchanged out of band (never committed). See `wg-homelab.conf.example` and
`wg-vps.conf.example`.

```sh
# Generate keys on each host (never share the private key):
wg genkey | tee private.key | wg pubkey > public.key
```

## Reverse proxy (Caddy)

`Caddyfile.example` fronts CAR with automatic TLS + WebSocket upgrade. Only
the documented routes (`/health`, `/ready`, `/api/v1/*`) are forwarded; the
proxy never serves workspace content or transcripts.

## Firewall

`firewall.sh` ensures CAR is reachable only via the WireGuard interface, not
the public Internet interface. Direct CAR internet access is blocked.

## Stateless VPS

The VPS runs only Caddy + WireGuard. It keeps:
- TLS certificates (Caddy-managed)
- the WireGuard private key for its peer
- the reverse-proxy config

It keeps NONE of:
- the SQLite database
- transcripts/artifacts
- workspace mounts
- CAR API tokens (tokens live on the phone and the homelab only)

## Verification

1. From the phone: `https://car.example.invalid/health` → 200 (via VPS).
2. Direct `https://<homelab-public-ip>:8080/health` → blocked (firewall).
3. `wg show` lists only the two peers; handshake succeeds.
4. VPS disk has no `car.db`, no `artifacts/`, no CAR config.
