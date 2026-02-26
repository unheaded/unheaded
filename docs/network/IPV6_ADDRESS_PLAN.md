# Unheaded Kingdom — IPv6 Address Plan

## Allocation Block

Base /48: `fd00:dead:beef::/48`

This is a ULA (Unique Local Address) block — RFC 4193 compliant.
Not routable on the public internet. Used for east-west control plane only.

## Subnet Allocation

| Subnet | CIDR | Purpose |
|--------|------|---------|
| LAN Forge | `fd00:dead:beef:0001::/64` | Host-A LAN interface |
| LAN Outpost | `fd00:dead:beef:0002::/64` | Host-B LAN interface |
| WireGuard Tunnel | `fd00:dead:beef:ee77::/64` | wg0 east-west tunnel |
| Services Forge | `fd00:dead:beef:aa01::/64` | Unheaded service virtual IPs (forge) |
| Services Outpost | `fd00:dead:beef:aa02::/64` | Unheaded service virtual IPs (outpost) |
| Kingdom Mode | `fd00:dead:beef:f000::/52` | Reserved for Kingdom Mode flow encoding |
| Future | `fd00:dead:beef:0100::/40` | Reserved for additional hosts |

## Host Addresses

| Host | Interface | IPv6 Address |
|------|-----------|-------------|
| Host-A (forge) | eth0 | `fd00:dead:beef:0001::1/64` |
| Host-A (forge) | wg0 | `fd00:dead:beef:ee77::1/64` |
| Host-B (outpost) | eth0 | `fd00:dead:beef:0002::1/64` |
| Host-B (outpost) | wg0 | `fd00:dead:beef:ee77::2/64` |

## WireGuard Configuration Summary

```
# Host-A (server/forge)
Interface:
  PrivateKey = <generated on host>
  Address = fd00:dead:beef:ee77::1/64
  ListenPort = 51820
  MTU = 1380

Peer (Host-B):
  PublicKey = <host-b pubkey>
  AllowedIPs = fd00:dead:beef:ee77::2/128
  PersistentKeepalive = 25

# Host-B (client/outpost)
Interface:
  PrivateKey = <generated on host>
  Address = fd00:dead:beef:ee77::2/64
  MTU = 1380

Peer (Host-A):
  PublicKey = <host-a pubkey>
  Endpoint = <host-a-public-ip>:51820
  AllowedIPs = fd00:dead:beef::/48
  PersistentKeepalive = 25
```

## MTU Calculation

```
Physical MTU:          1500 bytes
IPv6 header:            -40 bytes
WireGuard overhead:     -32 bytes  (Poly1305 auth tag + nonce)
Unheaded HbH header:    -24 bytes  (Monad 20-byte register + HbH framing)
                       ─────────
Available payload MTU:  1404 bytes
Recommended WG MTU:     1380 bytes  (safety margin for extension header stacking)
```

## Kingdom Mode Address Encoding

In Kingdom Mode, a /48 block can be reinterpreted as:
- 16 bits: subnet discriminator (standard routing)
- 64 bits: flow ID (64-bit unique flow identifier)
- 32 bits: Monad register overflow (additional computation state)

This allows 2^64 distinct flow IDs within a single /48, enabling per-flow
observability without additional tunnel or overlay protocols.

## Key Management

Keys are generated on each host at first boot via the `unheaded-wg-keygen.service` systemd unit.
Keys are stored in `/etc/unheaded/wg/` (mode 0700, root only).
Public keys are exchanged out-of-band and hardcoded in the NixOS configuration.
Key rotation: manual, coordinated between both hosts.
