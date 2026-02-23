# Heritage — Protocol Lineage

The Unheaded Protocol did not appear from nothing. It inherits design
principles from a lineage of data bus and packet processing systems
spanning 50 years of engineering.

## The Lineage

```
ARINC 429 (1977)  →  fixed 32-bit words, unidirectional bus
    ↓
I2C (1982)        →  master/slave, shared bus, address space
    ↓
CAN Bus (1986)    →  broadcast frames, priority arbitration, automotive
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

### ARINC 429 — Fixed-Width Words

ARINC 429 is an avionics data bus standard. Each transmission is exactly
32 bits: 8-bit label, 2-bit SDI, 19-bit data, 1-bit parity. Fixed-width.
No negotiation. Every receiver knows exactly where every field is.

**Contribution to Unheaded**: The Monad is a fixed 20-byte (5 × u32) register
file. No variable-length fields. No parsing ambiguity. Every eBPF program
knows the exact offset of every register. O(1) access, always.

### I2C — Shared Bus with Addressing

I2C uses a shared two-wire bus where devices have 7-bit addresses. Any master
can communicate with any slave. The bus is the shared medium.

**Contribution to Unheaded**: Wotan as a shared message bus with topic-based
addressing. Services publish and subscribe to topics. The bus is the shared
medium. No point-to-point connections.

### CAN Bus — Broadcast Frames with Priority

CAN (Controller Area Network) broadcasts every frame to all nodes. Priority
is determined by the message identifier — lower ID wins arbitration. Used in
every modern automobile.

**Contribution to Unheaded**: Wotan topic priority. Critical alerts
(`alerts.critical`) have higher priority than informational updates
(`timeline.updates`). Broadcast to all subscribers. The bus decides priority,
not the sender.

### BGP — Path Attributes and Policy

BGP (Border Gateway Protocol) carries path attributes with every route
announcement. Each autonomous system applies policy to decide what to accept,
prefer, and propagate. The internet runs on it.

**Contribution to Unheaded**: The Monad carries attributes with every packet.
Each hop applies policy (the eBPF Shim) to decide how to process, transform,
and forward. Network fabric uses EVPN-VXLAN with BGP control plane — actual
BGP, not a metaphor.

### BPF — Kernel Register Machine

The original Berkeley Packet Filter (1992) is a register-based virtual machine
in the kernel. Programs load into the kernel, attach to network events, and
process packets at kernel speed. Two registers (A and X), a small instruction
set, guaranteed termination.

**Contribution to Unheaded**: The Monad IS a register file processed by eBPF
programs. The lineage is direct: BPF's register machine running on BPF's
successor (eBPF) processing register files encoded in packets. The Shim is
literally a BPF program processing a register file.

### IPv6 — Extension Headers

IPv6 introduced extension headers: additional protocol data chained between
the IPv6 header and the payload. Hop-by-Hop Options (HbH) are processed at
every router along the path — the only extension header with this property.

**Contribution to Unheaded**: The Monad lives in the IPv6 HbH Options header.
This means every hop processes it. This is not optional behavior we bolted on —
it is the designed purpose of HbH Options (RFC 8200 §4.3). We are using
IPv6 as intended.

### uIP — Minimal IP Stack

uIP is Adam Dunkels' microcontroller IP stack. Full TCP/IP in ~5KB of code.
Proves that a complete network stack can fit in extremely constrained
environments.

**Contribution to Unheaded**: The Monad is 20 bytes. The Shim program is
< 4096 BPF instructions (the verifier limit). The entire per-hop processing
fits in the eBPF instruction budget with room to spare. Minimalism is a
design constraint, not an accident.

### eBPF — Maps, Ring Buffers, XDP

Extended BPF (2014+) added: BPF maps (key-value stores in kernel), ring
buffers (zero-copy kernel→userspace), XDP (earliest possible packet hook),
TC (traffic control hooks), kprobes, tracepoints. A full programmable
data plane in the Linux kernel.

**Contribution to Unheaded**: Everything. The Monad is processed by eBPF
programs. Sophia dictionaries are BPF maps. Anamnesis events flow through
BPF ring buffers. Packet marking happens at XDP. Flow tracking at TC.
Latency measurement via kprobes. The entire Unheaded data plane is eBPF.

## The Synthesis

Unheaded is the synthesis of these eight traditions:

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
