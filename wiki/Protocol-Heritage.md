# Protocol Heritage — Lineage from ARINC 429 to Unheaded

The Unheaded Protocol inherits design principles from a lineage of data bus and packet processing systems spanning 50 years of engineering.

## The Lineage

```
ARINC 429 (1977)  →  fixed 32-bit words, unidirectional bus
    ↓
I2C (1982)        →  master/slave, shared bus, address space
    ↓
CAN Bus (1986)    →  broadcast frames, priority arbitration
    ↓
BGP (1989)        →  path attributes, policy routing, autonomous systems
    ↓
BPF (1992)        →  kernel packet filtering, register machine
    ↓
IPv6 (1998)       →  128-bit addresses, extension headers, HbH Options
    ↓
uIP (2001)        →  microcontroller IP stack, minimal footprint
    ↓
eBPF (2014)       →  extended BPF, maps, ring buffers, XDP
    ↓
Unheaded (2025)   →  Monad register file over IPv6 HbH Options
```

## What Each Ancestor Contributed

| Principle | Source | Unheaded Implementation |
|-----------|--------|------------------------|
| Fixed-width atomic data | ARINC 429 | 20-byte Monad (5 × u32) |
| Shared message bus | I2C | Wotan (gRPC pub/sub) |
| Broadcast with priority | CAN Bus | Wotan topic priority |
| Per-hop policy attributes | BGP | Monad registers + eBPF Shim |
| Kernel register machine | BPF | eBPF programs processing Monad |
| Extension header transport | IPv6 | HbH Options (RFC 8200) |
| Minimal footprint | uIP | < 4096 BPF instructions per hop |
| Programmable data plane | eBPF | XDP/TC/kprobe/tracepoint programs |

---

> **Source:** [docs/lore/HERITAGE.md](../docs/lore/HERITAGE.md)
