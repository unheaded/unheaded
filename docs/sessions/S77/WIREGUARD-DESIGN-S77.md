# WireGuard IPv6 Overlay Design — S77

**SPDX-License-Identifier: MIT**
**Sprint:** S77 Age 2 Acceleration, Phase 2
**Author:** Unheaded Team
**Date:** 2026-03-05

> **Corrected 2026-10-23 (S-QUANTUM-ARMOR, ADR-105).** This is the S77 session
> record; `docs/WIREGUARD-DESIGN-S77.md` is the current design. Deployed
> addresses are WEST `fd00:dead:beef::1/128`, EAST `fd00:dead:beef::2/128`
> (the tables below have them swapped). The PSK is on (one static key since
> setup) and is now rotated by `scripts/wireguard/rotate-psk.sh`, not by
> re-running setup: see "Post-Quantum" and "Key Rotation" below.

---

## Overview

This document describes the WireGuard-based IPv6 overlay network connecting the WEST and EAST bare metal hosts. The overlay provides an encrypted, high-performance IPv6 transport for Monad Hop-by-Hop (HbH) extension headers, which cannot traverse NAT or most IPv4 infrastructure.

---

## Network Layout

### Before (IPv4 P2P Only)

```
WEST (192.168.13.2) ──── P2P Ethernet ──── EAST (192.168.13.1)
       │                                          │
   lxdbr0                                     lxdbr0
 10.10.10.0/24                             10.10.10.0/24
       │                                          │
   [containers]                             [containers]
```

- Direct point-to-point link over 192.168.13.0/24
- IPv4 only — no IPv6 support on the P2P segment
- Monad HbH headers cannot be carried over IPv4
- No encryption on the P2P link

### After (WireGuard IPv6 Overlay)

```
WEST (192.168.13.2) ──── P2P Ethernet ──── EAST (192.168.13.1)
       │                    (IPv4)                │
       │                                          │
   wg0 (fd00:dead:beef::2) ═══════════ wg0 (fd00:dead:beef::1)
       │              WireGuard Tunnel            │
       │              (encrypted IPv6)            │
   lxdbr0                                     lxdbr0
 10.10.10.0/24                             10.10.10.0/24
       │                                          │
   [containers]                             [containers]

Port: 51820/UDP (WireGuard)
Subnet: fd00:dead:beef::/48
```

- WireGuard tunnel encapsulates IPv6 inside IPv4/UDP
- Full IPv6 support including extension headers
- ChaCha20-Poly1305 encryption on all tunnel traffic
- Monad HbH headers pass through natively over IPv6

---

## WireGuard Configuration

### Addressing

| Host | P2P (IPv4) | WireGuard (IPv6) | Role |
|------|-----------|-------------------|------|
| WEST | 192.168.13.2 | fd00:dead:beef::2/48 | Development/test cluster |
| EAST | 192.168.13.1 | fd00:dead:beef::1/48 | Staging environment |

### Tunnel Parameters

| Parameter | Value |
|-----------|-------|
| Interface | wg0 |
| Listen Port | 51820/UDP |
| Protocol | WireGuard (Noise_IKpsk2) |
| Cipher | ChaCha20-Poly1305 |
| Hash | BLAKE2s |
| Key Exchange | Curve25519 |
| Keepalive | 25 seconds |
| MTU | 1420 (default, auto-negotiated) |

### WEST Configuration (wg0.conf)

```ini
[Interface]
PrivateKey = <WEST_PRIVATE_KEY>
Address = fd00:dead:beef::2/48
ListenPort = 51820
PostUp = sysctl -w net.ipv6.conf.all.forwarding=1
PostDown = sysctl -w net.ipv6.conf.all.forwarding=0

[Peer]
PublicKey = <EAST_PUBLIC_KEY>
PresharedKey = <PRESHARED_KEY>
AllowedIPs = fd00:dead:beef::1/128
Endpoint = 192.168.13.1:51820
PersistentKeepalive = 25
```

### EAST Configuration (wg0.conf)

```ini
[Interface]
PrivateKey = <EAST_PRIVATE_KEY>
Address = fd00:dead:beef::1/48
ListenPort = 51820
PostUp = sysctl -w net.ipv6.conf.all.forwarding=1
PostDown = sysctl -w net.ipv6.conf.all.forwarding=0

[Peer]
PublicKey = <WEST_PUBLIC_KEY>
PresharedKey = <PRESHARED_KEY>
AllowedIPs = fd00:dead:beef::2/128
Endpoint = 192.168.13.2:51820
PersistentKeepalive = 25
```

---

## Monad HbH Passthrough

### Why WireGuard + IPv6?

The Monad wire format (v0x01, 20 bytes) is designed to be carried in IPv6 Hop-by-Hop (HbH) extension headers. HbH headers are processed by every router in the path and are only valid in IPv6 packets. Key constraints:

1. **IPv4 cannot carry HbH headers** — there is no equivalent mechanism
2. **NAT destroys extension headers** — middleboxes strip or reject them
3. **WireGuard preserves full IPv6 packets** — the tunnel carries the complete IPv6 packet including all extension headers, encrypted inside UDP/IPv4

### How It Works

```
Application Layer:
  [Monad Payload]

IPv6 + HbH:
  [IPv6 Header] [HbH: Monad v0x01 20B] [UDP/TCP] [Payload]

WireGuard Encapsulation:
  [IPv4: 192.168.13.x] [UDP:51820] [WG Header] [Encrypted IPv6+HbH+Payload]

Wire:
  [Ethernet] [IPv4+UDP+WG] ──── P2P Link ──── [Ethernet] [IPv4+UDP+WG]
```

The receiving WireGuard endpoint decapsulates and delivers the original IPv6 packet with HbH headers intact to the kernel. The kernel processes the HbH options (including Monad type 0x2A) according to RFC 8200 rules.

### HbH Option Format (UNHEADED_METRIC_V1)

Per the IPv6 metric research (docs/research/IPV6_METRIC_CAPACITY.md):

```
Type:   0x2A (UNHEADED_METRIC_V1, to be registered with IANA)
Length: 52 bytes
Data:   Monad v0x01 wire format (20B) + extended metrics (32B)
```

### Kernel Requirements

Both hosts must have:
- `net.ipv6.conf.all.forwarding=1` (set by WireGuard PostUp)
- IPv6 HbH processing enabled (default on Linux 5.x+)
- No firewall rules dropping HbH options on wg0

---

## Performance Targets

| Metric | Target | Rationale |
|--------|--------|-----------|
| P2P direct RTT | < 0.5 ms | Baseline (direct Ethernet) |
| WireGuard RTT | < 1.0 ms | Encryption overhead budget |
| WG overhead | < 0.5 ms | ChaCha20 on modern CPUs |
| HTTP health check | < 5 ms | Local loopback service |
| Wotan publish | < 10 ms | gRPC round-trip |
| Packet-to-browser | < 50 ms | End-to-end (eBPF to dashboard) |
| Tunnel throughput | > 500 Mbps | P2P link capacity |

---

## Security Considerations

### Encryption

- **Algorithm:** ChaCha20-Poly1305 (AEAD)
- **Key exchange:** Curve25519 ECDH
- **Additional:** Preshared key (on; one static key since setup) for post-quantum resistance
- **Perfect forward secrecy:** Yes (new session keys per handshake)

### Post-Quantum

Curve25519 alone does not survive a future quantum attacker who recorded the
tunnel. The PSK is hashed into every handshake (Noise_IKpsk2), so recorded
traffic stays confidential while the PSK stays secret. `scripts/wireguard/rotate-psk.sh`
rotates it (dry run by default; `--apply` sends the new key to east only over
an SSH session that negotiated `mlkem768x25519-sha256` or `sntrup761x25519-sha512`,
and rolls both hosts back if no handshake follows). A weekly timer
(`deploy/systemd/unheaded-wg-psk-rotate.timer`) ships disabled; the first live
rotation is ESCALATE E3 (`docs/security/pq-host-escalations.md`). Rosenpass is
parked (C/liboqs dependency, ADR-004).

### Key Management

- Private keys generated on deployment, never leave the host
- Preshared key distributed via SSH during setup
- `age` encryption used for key-at-rest protection when available
- Plaintext keys deleted after deployment if age is available

### Network Exposure

- WireGuard port (51820/UDP) exposed only on the P2P interface
- No public internet exposure
- Firewall rules should restrict 51820/UDP to 192.168.13.0/24

### Threat Model

| Threat | Mitigation |
|--------|-----------|
| P2P link eavesdropping | WireGuard encryption (ChaCha20-Poly1305) |
| Key compromise | Preshared key adds second factor; PSK rotation via `scripts/wireguard/rotate-psk.sh` |
| Replay attacks | WireGuard nonce-based replay protection |
| Denial of service | Rate limiting at kernel level; P2P link is isolated |
| Extension header injection | HbH options validated by Monad parser; invalid types rejected |

---

## Operational Procedures

### Deployment

```bash
# First-time setup (idempotent)
./scripts/wireguard/setup-wireguard.sh

# Verify health
./scripts/wireguard/verify-wireguard.sh
```

### Key Rotation

Re-running the setup script does not rotate anything while the overlay is up
(it exits "nothing to do"). Rotate the PSK with:

```bash
sudo scripts/wireguard/rotate-psk.sh           # dry run: preflight + plan
sudo scripts/wireguard/rotate-psk.sh --apply   # rotate (ESCALATE E3 for the first live run)
```

### Troubleshooting

```bash
# Check interface status
sudo wg show wg0

# Check IPv6 connectivity
ping6 fd00:dead:beef::1   # from WEST
ping6 fd00:dead:beef::2   # from EAST

# Check for handshake
sudo wg show wg0 | grep "latest handshake"

# Debug kernel IPv6 forwarding
sysctl net.ipv6.conf.all.forwarding
sysctl net.ipv6.conf.wg0.forwarding

# Capture tunnel traffic
sudo tcpdump -i wg0 -n ip6
```

---

## Future Work

- **Multi-host expansion:** Add more peers to the fd00:dead:beef::/48 network
- **Automated key rotation:** PSK rotation tool and weekly timer shipped 2026-10-23 (not enabled: E3); static keypair rotation still manual
- **WireGuard monitoring:** Export wg show metrics to Prometheus
- **MTU optimization:** Test jumbo frames on P2P link for higher throughput
- **IPv6-native services:** Bind Unheaded services directly to WireGuard IPv6 addresses
