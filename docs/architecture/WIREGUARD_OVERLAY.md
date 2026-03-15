# WireGuard IPv6 Overlay Design

**Status:** Age 2 Epic
**Sprint:** S77
**Author:** Architect + Developer
**Date:** 2026-03-15

---

## Why

WEST and EAST communicate over a direct point-to-point Ethernet link
(192.168.13.0/30). This link carries unencrypted IPv4 traffic today. As
Kingdom services expand across both hosts, we need:

1. **Encryption** -- all cross-host traffic must be encrypted at rest and in
   transit. The P2P link is a physical cable but defense-in-depth demands we
   treat it as untrusted.
2. **IPv6 overlay** -- the Monad wire format (v0x01, 20 bytes) embeds
   flow-level metadata in IPv6 Hop-by-Hop extension headers. An IPv6 overlay
   lets services communicate natively with Monad-tagged packets.
3. **Unified addressing** -- a single ULA prefix (`fd00:dead:beef::/48`) gives
   every Kingdom service a stable, routable IPv6 address regardless of which
   host it runs on.
4. **BPF flow graph continuity** -- the cross-host BPF flow graph (already
   operational over IPv4) gains full IPv6 Monad tracing once the overlay is
   live.

WireGuard is the obvious choice: it is in-kernel on Linux 5.6+, stateless,
auditable (~4K LOC), and adds sub-millisecond overhead on a P2P link.

---

## Network Topology

```
                     P2P Ethernet (192.168.13.0/30)
    WEST ─────────────────────────────────────────── EAST
  192.168.13.2                                   192.168.13.1
  (enp16s0)                                      (eth0)
      │                                              │
      │  WireGuard tunnel (UDP 51820)                │
      │  encrypted, authenticated                    │
      ▼                                              ▼
    wg0                                            wg0
  fd00:dead:beef::1/64                    fd00:dead:beef::2/64
      │                                              │
      ├── lxdbr0 (10.10.10.0/24)                     ├── containers
      ├── docker (172.28.x.0/24)                     └── services
      └── Kingdom services
```

### Interface Summary

| Host | Physical | P2P IPv4 | WireGuard | Overlay IPv6 |
|------|----------|----------|-----------|--------------|
| WEST | wlp17s0 (192.168.69.184) | enp16s0 (192.168.13.2) | wg0 | fd00:dead:beef::1/64 |
| EAST | -- | eth0 (192.168.13.1) | wg0 | fd00:dead:beef::2/64 |

### Port

WireGuard uses UDP port **51820** (IANA default). Only the P2P interfaces
need this port open; no exposure to the wider LAN.

---

## IPv6 Address Plan

Prefix: `fd00:dead:beef::/48` (ULA, RFC 4193)

| Subnet | Range | Purpose |
|--------|-------|---------|
| ::/64 | fd00:dead:beef::1 - ::ffff | Host endpoints (WEST=::1, EAST=::2) |
| 0:1::/80 | fd00:dead:beef:0:1::0/80 | WEST containers / services |
| 0:2::/80 | fd00:dead:beef:0:2::0/80 | EAST containers / services |
| 0:f::/80 | fd00:dead:beef:0:f::0/80 | Infrastructure (Wotan, gateway) |

### docker0 Migration Note

The `fd00:dead:beef::/48` prefix is currently assigned to `docker0` for
Docker's IPv6 networking. To avoid conflicts:

1. **Phase 1 (now):** WireGuard uses only the host endpoints (::1, ::2) with
   /64 scope. Docker's /48 assignment on docker0 is narrowed to
   `fd00:dead:beef:0:d::/80` (the "d" subnet for Docker).
2. **Phase 2 (post-validation):** Docker containers that need overlay access
   join the `wg0` routing domain via `PostUp` rules or `--network host`.
3. **Phase 3 (production):** Docker IPv6 is fully managed through the
   WireGuard overlay. The docker0 bridge reverts to IPv4-only.

Update `/etc/docker/daemon.json` to narrow Docker's IPv6 pool:
```json
{
  "ipv6": true,
  "fixed-cidr-v6": "fd00:dead:beef:0:d::/80"
}
```

---

## WireGuard Configuration

### Key Management

Two hosts, pre-shared keys. Simple and appropriate for a point-to-point
deployment.

```
# On each host (one-time):
wg genkey | tee /etc/wireguard/privatekey | wg pubkey > /etc/wireguard/publickey
chmod 600 /etc/wireguard/privatekey

# Generate a pre-shared key (on either host, share securely):
wg genpsk > /etc/wireguard/presharedkey
chmod 600 /etc/wireguard/presharedkey
```

Private keys MUST NOT be committed to Git. The config files in
`config/wireguard/` use `<PLACEHOLDER>` values. The `scripts/wireguard/setup-wireguard.sh`
script generates real keys at deploy time.

### WEST Configuration (config/wireguard/west.conf)

```ini
[Interface]
Address = fd00:dead:beef::1/64
ListenPort = 51820
PrivateKey = <WEST_PRIVATE_KEY>
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
# EAST
PublicKey = <EAST_PUBLIC_KEY>
PresharedKey = <PRESHARED_KEY>
AllowedIPs = fd00:dead:beef::2/128, fd00:dead:beef:0:2::/80
Endpoint = 192.168.13.1:51820
PersistentKeepalive = 25
```

### EAST Configuration (config/wireguard/east.conf)

```ini
[Interface]
Address = fd00:dead:beef::2/64
ListenPort = 51820
PrivateKey = <EAST_PRIVATE_KEY>
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.%i.forwarding=1
PostDown = ip -6 route del fd00:dead:beef::/48 dev %i; sysctl -w net.ipv6.conf.%i.forwarding=0

[Peer]
# WEST
PublicKey = <WEST_PUBLIC_KEY>
PresharedKey = <PRESHARED_KEY>
AllowedIPs = fd00:dead:beef::1/128, fd00:dead:beef:0:1::/80
Endpoint = 192.168.13.2:51820
PersistentKeepalive = 25
```

---

## Systemd Integration

WireGuard ships with `wg-quick@.service`, a templated systemd unit. Enable
with:

```bash
sudo systemctl enable --now wg-quick@wg0
```

This reads `/etc/wireguard/wg0.conf` and brings the interface up at boot.

For environments where wg-quick is unavailable, a standalone unit is provided
at `config/wireguard/wg-quick@.service`.

---

## NixOS Integration

A declarative NixOS module is provided at `nix/modules/wireguard.nix`. It
supports:

- Peer configuration via options
- Private key file path (never inline)
- PostUp/PostDown routing rules
- Firewall port opening
- Integration with the existing `networking.nix` module

Usage in a host configuration:

```nix
{ config, ... }:
{
  imports = [ ./modules/wireguard.nix ];

  unheaded.wireguard = {
    enable = true;
    role = "west";  # or "east"
    privateKeyFile = "/etc/wireguard/privatekey";
    presharedKeyFile = "/etc/wireguard/presharedkey";
    peerPublicKey = "<EAST_PUBLIC_KEY>";
  };
}
```

---

## Docker / Container Integration

Containers that need to communicate across the overlay have three options:

### Option 1: Host Networking (simplest)

```yaml
services:
  cross-host-service:
    network_mode: host
    # Service binds to fd00:dead:beef::1 directly
```

### Option 2: Overlay Routing (recommended)

Add a route from the container network to the WireGuard interface via PostUp:

```ini
PostUp = ip -6 route add fd00:dead:beef::/48 dev %i; \
         ip6tables -A FORWARD -i br-unhe-data -o %i -j ACCEPT; \
         ip6tables -A FORWARD -i %i -o br-unhe-data -j ACCEPT
```

Containers on the `data` network (172.28.1.0/24) can then reach
`fd00:dead:beef::/48` addresses through the host's routing table.

### Option 3: Macvlan (advanced)

Attach containers directly to `wg0` via a macvlan sub-interface. This gives
each container its own overlay IPv6 address. Reserved for production use.

---

## FRR Integration (Future Enhancement)

FRR is already running on WEST for routing. When the overlay grows beyond two
hosts, FRR can advertise WireGuard overlay routes via OSPFv3 or BGP:

```
# /etc/frr/frr.conf (future)
router ospf6
  redistribute connected
  interface wg0 area 0.0.0.0
```

For the current two-host topology, static routes in `AllowedIPs` are
sufficient. FRR integration is deferred until a third host joins the Kingdom.

---

## BPF Flow Graph Integration

The cross-host BPF flow graph currently operates over IPv4 (192.168.13.0/30).
With the WireGuard overlay:

1. **Monad IPv6 injection:** eBPF programs attach HbH extension headers to
   packets traversing `wg0`. The UNHEADED_METRIC_V1 (Type 0x2A) option
   carries 52 bytes of flow metadata.
2. **trace-collector bridge:** The Rust trace-collector reads from the `wg0`
   perf buffer (or AF_XDP ring) and forwards Monad events to Wotan.
3. **Dashboard visualization:** The dashboard's host selector dropdown already
   supports WEST/EAST. Overlay traffic appears as a distinct flow class with
   encrypted transport markers.

The existing `ebpf/flow_tracker/` and `ebpf/packet_marker/` programs need
minor updates to attach to `wg0` in addition to `lxdbr0` and physical
interfaces.

---

## Security Considerations

- **Key rotation:** Rotate WireGuard keys quarterly. The setup script supports
  re-keying by tearing down and redeploying.
- **Firewall:** Only UDP 51820 is opened on the P2P interfaces. The overlay
  IPv6 traffic is subject to the same ip6tables rules as local traffic.
- **No key material in Git:** Config files use placeholders. Real keys live in
  `/etc/wireguard/` with mode 0600.
- **Pre-shared key:** Provides post-quantum resistance (symmetric key on top
  of Curve25519 DH). Aligns with the PQC work in S-PQC (SLH-DSA).

---

## Verification

Run the verification script after deployment:

```bash
./scripts/wireguard/verify-wireguard.sh
```

This checks:
- Interface existence on both hosts
- Bidirectional ping6 over the overlay
- WireGuard handshake status
- HbH extension header passthrough (tcpdump)
- Latency comparison (P2P direct vs. WireGuard overhead)

Expected overhead: < 1ms on the P2P link.

---

## Files

| Path | Purpose |
|------|---------|
| `docs/architecture/WIREGUARD_OVERLAY.md` | This design document |
| `config/wireguard/west.conf` | WEST WireGuard config (placeholder keys) |
| `config/wireguard/east.conf` | EAST WireGuard config (placeholder keys) |
| `config/wireguard/wg-quick@.service` | Systemd unit (fallback) |
| `nix/modules/wireguard.nix` | NixOS declarative module |
| `scripts/wireguard/setup-wireguard.sh` | Deployment script (generates keys) |
| `scripts/wireguard/verify-wireguard.sh` | Post-deploy verification |
