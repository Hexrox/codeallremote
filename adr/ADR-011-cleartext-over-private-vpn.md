# ADR-011: Cleartext transport to allow-listed private-VPN hosts

- **Status:** Accepted
- **Date:** 2026-07-21
- **Supersedes/relates:** ADR-004 (local-first gateway), ADR-007 (Android native client)

## Context

The CAR Android client was TLS-only: the pairing screen required a `https://`
base URL and `network_security_config` forbade cleartext everywhere. The
documented remote path is *phone → VPS gateway (TLS) → WireGuard → homelab*.

The operator's actual deployment terminates encryption at the VPN, not at the
application:

```
phone ──WireGuard──> VPS (public IP) ──tunnel──> homelab LAN ──> CAR server (192.168.2.16)
                 (alternatively ZeroTier: 172.22.89.70/16)
```

All phone↔server traffic travels **inside a private, already-encrypted overlay**
(WireGuard, or ZeroTier). The CAR Go server binds plain HTTP and does not
terminate TLS itself (ADR-004: the gateway/proxy owns TLS). Requiring
application-layer TLS *in addition* to the VPN would force either a public
domain + trusted certificate or an internal CA installed on the device — pure
friction with no confidentiality gain over the VPN that already encrypts the
link.

Two facts made the old policy both stricter than necessary and partly broken:

1. `network_security_config` was referenced via `<meta-data android:name="android.security.net.config">`, which is **not** the mechanism Android honors, so the intended `user` trust anchor never applied.
2. The pairing UI hard-required `https://`, so a homelab reachable only over a VPN by IP could not be entered at all.

## Decision

Permit cleartext (`http`/`ws`) **only to an explicit allow-list of homelab/VPN
hosts**; keep TLS-only as the default for everything else.

- `AndroidManifest.xml`: wire the config with the correct
  `android:networkSecurityConfig="@xml/network_security_config"` attribute
  (replacing the no-op `meta-data`).
- `network_security_config.xml`: `base-config cleartextTrafficPermitted="false"`
  (unchanged default) plus a `domain-config cleartextTrafficPermitted="true"`
  listing the specific hosts: `192.168.2.16` (WireGuard/LAN), `172.22.89.70`
  (ZeroTier), and `127.0.0.1`/`localhost` (instrumented MockWebServer tests).
- Pairing UI/ViewModel: accept a base URL starting with `http://` **or**
  `https://`. The WS client already maps `http→ws` / `https→wss`.

Adding a new homelab host is a one-line edit to `network_security_config.xml`.

## Consequences

**Positive**
- The app works over the operator's WireGuard/ZeroTier deployment with no
  certificate, CA, or public domain.
- Cleartext is scoped to named hosts; any other destination (a public URL, a
  typo'd host) is still refused, so a stray token cannot leak over plaintext to
  an arbitrary server.
- Fixes the latent `network_security_config` wiring bug; the `user` trust
  anchor now actually applies, keeping the internal-CA (HTTPS) option open.

**Negative / risks**
- Confidentiality of phone↔server traffic now depends on the VPN, not TLS. This
  is acceptable **only** because the listed hosts are reachable exclusively over
  WireGuard/ZeroTier, both of which authenticate peers and encrypt transport.
  If any listed host were ever exposed on an untrusted network, its traffic
  (including the bearer token and prompts) would be in cleartext. The allow-list
  must therefore contain only VPN-private addresses.
- The pairing endpoints remain unauthenticated (a device can self-pair); VPN
  membership is the access-control boundary. Do not expose the CAR server on a
  public interface. An owner-approval step for pairing is future work.

## Scope

Client transport policy only. No change to the REST/WebSocket contract, the
persistence model, the approval semantics, or the server. The server continues
to bind plain HTTP behind the VPN.
