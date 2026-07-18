#!/usr/bin/env bash
# CAR homelab firewall (M3-02): block direct Internet access to CAR.
#
# CAR is reachable ONLY via the WireGuard interface (10.10.0.1), never the
# public interface. Run as root on the homelab host.
#
# This script is idempotent and documents the policy; review before applying.

set -euo pipefail

WG_IFACE="${WG_IFACE:-wg0}"
WG_ADDR="${WG_ADDR:-10.10.0.1}"
CAR_PORT="${CAR_PORT:-8080}"
PUBLIC_IFACE="${PUBLIC_IFACE:-eth0}"

echo "==> CAR reachable only via $WG_IFACE ($WG_ADDR:$CAR_PORT)"

# Allow CAR on the loopback and WireGuard interfaces only.
iptables -C INPUT -i lo -p tcp --dport "$CAR_PORT" -j ACCEPT 2>/dev/null || \
  iptables -A INPUT -i lo -p tcp --dport "$CAR_PORT" -j ACCEPT
iptables -C INPUT -i "$WG_IFACE" -p tcp --dport "$CAR_PORT" -j ACCEPT 2>/dev/null || \
  iptables -A INPUT -i "$WG_IFACE" -p tcp --dport "$CAR_PORT" -j ACCEPT

# DROP direct Internet access to CAR on the public interface.
iptables -C INPUT -i "$PUBLIC_IFACE" -p tcp --dport "$CAR_PORT" -j DROP 2>/dev/null || \
  iptables -A INPUT -i "$PUBLIC_IFACE" -p tcp --dport "$CAR_PORT" -j DROP

echo "==> Blocked direct Internet access to CAR on $PUBLIC_IFACE"
echo "==> Verify: from the phone, https://car.example.invalid/health -> 200"
echo "==> Verify: direct http://<public-ip>:$CAR_PORT/health -> blocked"
