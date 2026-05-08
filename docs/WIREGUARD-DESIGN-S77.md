# S77 — Phase 2: WireGuard IPv6 Overlay Design

**Sprint:** S77 (Age 2 Acceleration)
**Phase:** 2 — IPv6 Overlay
**Status:** Shipped (setup + verify scripts in-tree; bare-metal validation on WEST/EAST)
**Subnet:** `fd00:dead:beef::/48` (ULA, RFC 4193)

---

## Why

Cross-host BPF flow graph (WEST ↔ EAST) needs a private encrypted overlay that:

1. Survives NAT and arbitrary intermediate networks (point-to-point or via internet).
2. Provides cryptographic identity per host (no shared secrets).
3. Stays out of the public IPv4 space (avoids ISP filtering, future-proof for v6-only fabric).
4. Carries Monad protocol packets without per-flow tunneling overhead.

WireGuard delivers on all four. The Kingdom uses ChaCha20-Poly1305 AEAD (built-in, not configurable) over UDP/51820, with a single ULA `/48` carved into per-host `/64` subnets.

## Address plan — fd00:dead:beef::/48

| Block | Host | Notes |
|-------|------|-------|
| `fd00:dead:beef:0001::/64` | WEST | Linux 6.17, primary dev/test |
| `fd00:dead:beef:0002::/64` | EAST | Linux, 4-core / 8GB DDR3, staging |
| `fd00:dead:beef:0003::/64` | NORTH | Reserved (CI/jenkins runner) |
| `fd00:dead:beef:0004::/64` | SOUTH | Reserved (future bare-metal) |
| `fd00:dead:beef:00ff::/64` | Service-mesh | wotan, sophia, monad service IPs |
| `fd00:dead:beef:f000::/52` | User apps | demo / kanban / wiki anchor block |

Within each `/64`, host-local addresses follow EUI-64 derivation off the WireGuard interface MAC (deterministic per peer key).

## Crypto

- **Curve:** Curve25519.
- **Symmetric:** ChaCha20-Poly1305 AEAD.
- **MAC:** BLAKE2s-128.
- **PSK option:** off in S77 — peer authentication via static public key only (sufficient for closed Kingdom mesh; PSK layer reserved for future federation).
- **Rekey interval:** WireGuard default (180s key, 120s rejection threshold, 90s rekey send threshold).

## Setup + verify scripts

- `scripts/wireguard/setup-wireguard.sh` — generates the host keypair, writes `/etc/wireguard/unheaded.conf`, brings up `wg0` with the host's `fd00:dead:beef:NNNN::/64`, and pre-populates AllowedIPs per peer table. Hardened: `ListenPort = 51820`, `Endpoint =` per peer, `PersistentKeepalive = 25` for NAT traversal.
- `scripts/wireguard/verify-wireguard.sh` — pings each peer's anycast `::1` address, confirms `wg show` reports `latest handshake` < 60s, parses `transfer:` to assert nonzero rx/tx.

Both scripts emit JSON status to stdout for ingestion by the dashboard host-selector dropdown.

## Cross-host BPF flow graph

The flow-tracker BPF program emits Monad packets carrying `(src_ipv6, dst_ipv6, flow_label, latency_µs)` tuples. Userspace ingestion in `cmd/trace-collector` decodes them and republishes to wotan topic `flow.cross_host`. WireGuard ULA addresses are recognized as kingdom-internal and labeled as such in the trace UI.

## Security considerations

- WireGuard pubkeys are stored in `pkg/auth/wg-peer-registry.json`, signed with ML-DSA-65 per ADR-049.
- WireGuard does NOT replace pkg/auth; HTTP/gRPC traffic over the overlay still passes through APIKey or JWT authenticators.
- The overlay is `fd00:dead:beef::/48` (private). Kingdom publishing surface remains ipv4 + global ipv6 via the gateway service.

## References

- `scripts/wireguard/setup-wireguard.sh`, `scripts/wireguard/verify-wireguard.sh`.
- `pkg/auth/wg-peer-registry.json`.
- ADR-049 (transport security baseline).
- `tests/s77/s77_verification_test.go::TestPhase2_WireGuard`.
