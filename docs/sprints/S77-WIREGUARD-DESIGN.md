# S77 — WireGuard IPv6 Overlay Design (sprint index)

**Sprint:** S77 — Age 2 Acceleration Campaign
**Phase:** 2 — IPv6 Overlay
**Subnet:** `fd00:dead:beef::/48` (ULA, RFC 4193)
**Status:** Design shipped; setup + verify scripts in-tree; live mesh validated WEST↔EAST
**Canonical doc:** [`docs/WIREGUARD-DESIGN-S77.md`](../WIREGUARD-DESIGN-S77.md)
**Gate test:** [`tests/s77/s77_verification_test.go::TestPhase2_WireGuard`](../../tests/s77/s77_verification_test.go)

---

## Purpose

Sprint-folder index for the S77 WireGuard overlay. For full crypto
parameters, peer addressing, NAT considerations, and authentication
posture, read the canonical doc. This file documents the *design intent*
and the *topology* in the sprint-accounting voice.

---

## Topology — WEST + EAST mesh

```
                    fd00:dead:beef::/48
                            │
        ┌───────────────────┼───────────────────┐
        │                   │                   │
   /64 = 0001          /64 = 0002          /64 = 00ff
    WEST                 EAST              service-mesh
  (Linux 6.17,         (Linux,             (wotan, sophia,
   dev/test)            4c/8GB DDR3,        monad floating
                        staging)            service IPs)
        │                   │
        └─── UDP/51820 ─────┘
         WireGuard transport
         ChaCha20-Poly1305 AEAD
```

`/64 = 0003 (NORTH)` and `/64 = 0004 (SOUTH)` are reserved blocks; the
`/52 = f000` block anchors user apps (kanban, demo, wiki).

## Addressing

`fd00:dead:beef::/48` carved into `/64`s, one per host. Within a `/64`,
host-local addresses follow EUI-64 derivation from the WireGuard
interface MAC — deterministic per peer public key, no DHCPv6 required.
Service-mesh `/64 = 00ff` is shared across hosts so floating service IPs
(wotan, sophia, monad) survive host reassignment.

## Authentication

S77 uses WireGuard's native static-public-key authentication. Each peer
holds its own Curve25519 keypair; peer public keys are catalogued in
`pkg/auth/wg-peer-registry.json` and **signed with ML-DSA-65 per
ADR-049** so a future PQC migration does not require regenerating the
WG keys themselves. The PSK layer is left available for future
federation between Kingdom instances and other Unheaded deployments.

ChaCha20-Poly1305 is built into WireGuard — there is no negotiated
ciphersuite to downgrade. Curve25519 / BLAKE2s round out the cryptobox.
The Kingdom relies on the protocol's inherent simplicity rather than
TLS-style cipher agility.

## Routing

WireGuard's `AllowedIPs` configuration is the routing table. Each peer
declares the `/64`s it owns; userspace pre-populates these from
`pkg/auth/wg-peer-registry.json` at `setup-wireguard.sh` time. There is
no dynamic routing protocol — adds and removes are config commits.

## MTU

WireGuard's UDP encapsulation overhead is 60 bytes (IPv6 header + UDP +
WG packet header + AEAD tag). On a path-MTU of 1500 this leaves 1440
bytes for the inner IPv6 packet. The Kingdom sets `MTU = 1420` on `wg0`
to leave headroom for any future second tunnel layer (e.g., a Monad
flow-label tag added by an XDP program before encryption).

## What this enables

- Cross-host BPF flow graph (WEST↔EAST) carries Monad protocol packets
  through encrypted ULA space.
- WireGuard does **not** replace `pkg/auth`; HTTP and gRPC traffic over
  the overlay still passes through APIKey or JWT authenticators.
- The overlay is private. Kingdom publishing surface remains IPv4 +
  global IPv6 via the gateway service on port 21000/21443.

---

## Status — what's shipped vs. PROPOSED / TBD per S77 close-out

- **Shipped:** `scripts/wireguard/setup-wireguard.sh`,
  `scripts/wireguard/verify-wireguard.sh`, canonical design doc, the
  Mimirs's-Law gleipnir bootstrap pattern that depends on this overlay.
- **PROPOSED / TBD per S77 close-out:** NORTH (`/64 = 0003`) and SOUTH
  (`/64 = 0004`) blocks are *reserved* but unallocated. The
  service-mesh `/64 = 00ff` block has no service registration mechanism
  yet — services are pinned by hand in S77; S78+ will couple it to
  `pkg/discovery` so service IPs become discoverable rather than
  configured.
- **PROPOSED / TBD per S77 close-out:** PSK federation. Deferred to
  whenever the second Kingdom instance lights up.

---

Free to use. Free to share.
