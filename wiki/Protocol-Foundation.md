# Protocol Foundation — Monad Wire Format

The Unheaded Protocol encodes a 20-byte Monad (5 × u32 register file) in the IPv6 Hop-by-Hop Options extension header. At each hop, an eBPF Shim reads and writes the Monad. The packet itself is the working memory of a distributed computation.

## Key Properties

```
Monad:       20 bytes, 5 registers (R0–R4), CRC-16 integrity
Encoding:    Exponent-encoded fields via Sophia dictionaries
Processing:  Per-hop eBPF (XDP/TC), O(1) per packet
Scope:       Limited Domain [RFC 8799] — every hop is controlled
Heritage:    ARINC 429 → I2C → CAN Bus → BGP → BPF → IPv6 → uIP → Unheaded
```

## Wire Format Freeze Status (S67)

**FROZEN at v0x01** — February 28, 2026

The 20-byte Monad wire format is locked at specification version 0x01 with **12 IANA registries** in the foundation spec:

1. **Version Registry** — Protocol version field (0x00–0xFF)
2. **Flags Registry** — Bitfield interpretations (C, Y, T, E, S, M, K1, K0)
3. **Flow Actions Registry** — Action directive values (forward, drop, dup, redirect, etc.)
4. **Kingdom Mode Registry** — Privilege level encodings (NORMAL, PRIORITY, EXPERIMENTAL, RESERVED)
5. Service Identity Registry
6. QoS Class Registry
7. Deployment Ring Registry
8. Circuit State Registry
9. Mesh Flags Registry
10. Event Type Registry (Anamnesis)
11. Error Code Registry
12. BPF Helper Function Registry

**IPR Status:** Clear. No collisions. WEST bare metal online.

## Kingdom Mode (Optional, EVPN-VXLAN)

Deterministic IPv6 address bits reclaimed as Extended Register Space:

| Mode | Bits Reclaimed | Hosts |
|------|---------------|-------|
| /8 | 208 bits (26 bytes) | 16.7M |
| /12 | 216 bits (27 bytes) | 1M |
| /16 | 224 bits (28 bytes) | 65K |

Combined with the Monad, a /16 Kingdom carries **48 bytes** of computational register state per packet with zero wire overhead.

---

> **Source:** [docs/protocol/PROTOCOL_FOUNDATION.md](../docs/protocol/PROTOCOL_FOUNDATION.md)
> **Spec:** [draft-bellis-unheaded-protocol-foundation-04](../docs/protocol/draft-bellis-unheaded-protocol-foundation-04.md)
