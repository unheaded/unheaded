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
