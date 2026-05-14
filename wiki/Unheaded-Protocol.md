# The Unheaded Protocol (UP)

The Unheaded Protocol is the wire format + memory model + microcode that turns IPv6 packet forwarding into computation. It is what the [UPC](UPC-Overview) dispatches on. This page ties together the three pillars — Monad, Sophia, Wotan — and points at the canonical RFC drafts.

The Internet-Drafts are the authoritative spec. Wiki pages summarize and link.

## The three pillars

| Pillar | What it is | Wire surface | Wiki | Draft |
|---|---|---|---|---|
| **Monad** | The 20-byte register-file packet header | IPv6 Hop-by-Hop Option type 0x3E | [Protocol Foundation](Protocol-Foundation) | [foundation-06](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) |
| **Sophia** | Exponent-encoded dictionaries (microcode) | Per-field encoding rules inside the Monad | [Sophia Dictionaries](Sophia-Dictionaries) | [sophia-03](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md) |
| **Wotan** | Distributed memory model (the "RAM") | BPF maps + gRPC topic streaming | [Wotan Memory](Wotan-Memory-Model) | [wotan-03](../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md) |

Plus three companion specs:

| Spec | What it covers | Wiki | Draft |
|---|---|---|---|
| **MBC-ISA** | The bytecode the UPC executes | [MBC ISA Reference](MBC-ISA-Reference) | [mbc-isa-00](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) |
| **Shim** | The eBPF execution pipeline (Monad → MBC dispatch → forward) | [Shim Pipeline](Draft-Shim-00) | [shim-00](../docs/protocol/draft-bellis-unheaded-shim-00.md) |
| **PQC** | Post-quantum authentication (ML-DSA-65 / SLH-DSA) | [PQC Authentication](PQC-Authentication) | [pqc-authentication-00](../docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md) |

## Monad — the wire format

20 bytes. 5 × u32 registers (R0–R4). Lives in an IPv6 Hop-by-Hop Options extension header. Frozen at version 0x01 since 2026-02-28.

```
 31     24 23     16 15      8 7       0
+---------+---------+---------+---------+
|  type   |  hop    | flags   | version |  R0
+---------+---------+---------+---------+
|              hop_metadata             |  R1
+---------------------------------------+
|              register_A               |  R2
+---------------------------------------+
|              register_B               |  R3
+---------------------------------------+
|     CRC-16     |    custom_or_ext     |  R4
+----------------+----------------------+
```

- **type** (8 bits): registered IANA type for the Monad option (= 0x3E in the IPv6 HbH registry).
- **hop**: monotonic per-hop counter. The "clock tick".
- **flags**: C(rypto) | Y(ield) | T(race) | E(rror) | S(ample) | M(irror) | CUST | R(eserved). IANA-registered bitfield.
- **version**: protocol spec version. Currently 0x01.
- **hop_metadata**: per-hop encoding context (Sophia dictionary index, kingdom mode).
- **register_A**, **register_B**: general-purpose 32-bit values. Carry compute state, addresses, intermediate results.
- **CRC-16**: integrity check across R0–R3. Computed at each hop on egress, verified on ingress.
- **custom_or_ext**: 16 bits for vendor extensions or IANA-registered sub-types.

The Foundation spec freezes 12 IANA registries: Monad Protocol Version Numbers, Flags Bitfield, Flow Actions (13 entries), Kingdom Mode Values, plus 8 others for extensibility. IPR clearance: RFC 8928/9927 CLEAR.

Wiki: [Protocol Foundation](Protocol-Foundation). Source: [`docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md`](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md).

## Sophia — exponent-encoded microcode

Sophia is the encoding layer. Each Monad register slot carries an exponent-encoded value: a base-N value where N is selected by a Sophia dictionary index in the hop_metadata field. This compresses common operations into 4-bit codes while preserving arbitrary 32-bit values via escape sequences.

Think of it as the protocol's microcode. A 4-bit code might mean "decrement R2 by 1" in dictionary A but "stamp current epoch into R3" in dictionary B. Dictionary selection rotates through the hop_metadata field; the same wire pattern means different things at different hops.

Sub-dictionary types + QPACK compression are landing in draft-03.

Wiki: [Sophia Dictionaries](Sophia-Dictionaries). Source: [`docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md`](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md).

## Wotan — distributed memory

Wotan is the memory hierarchy. The Monad is a register file (20 bytes, packet-resident); Wotan is everything bigger — flow state, control-plane configuration, observability events, audit log, page tables.

Implemented as a hybrid:

- **LOCAL**: BPF maps on each Shim. Per-flow state, per-process page tables, framebuffers, TTY rings. Sub-microsecond access from the XDP fast path.
- **DISTRIBUTED**: gRPC topic streaming with ML-DSA-65 signed messages on `config.*` topics. Cluster-wide consistency for control-plane state.

Topic taxonomy:

- `compute.*` — UPC events (TTY writes, framebuffer flush ticks, halts).
- `config.*` — desired-state changes. Signed-only.
- `state.*` — actual-state reports.
- `system.*` — health, drift, service discovery, log aggregation.
- `alerts.*` — percentage-based-consensus outage signals.

Draft-03 adds an error-code taxonomy.

Wiki: [Wotan Memory Model](Wotan-Memory-Model). Source: [`docs/protocol/draft-bellis-unheaded-wotan-memory-03.md`](../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md).

## MBC — the bytecode

The compute substrate's instruction set. 32-bit fixed-width. 256 possible opcodes (8-bit field). Designed for fast eBPF dispatch and easy translation from RV32I.

```
 31     24 23 20 19 16 15            0
+---------+-----+-----+----------------+
| opcode  | dst | src |     imm16      |
+---------+-----+-----+----------------+
```

Real programs are written in C, compiled to RV32I ELF, then translated to MBC via `crates/monad-mbc/`. The translator emits three artifacts:

- `.mbc` — the bytecode.
- `.rv2mbc` — RV byte addr → MBC PC map (consumed by indirect jumps).
- `.data` — TLV-format dump of `.rodata` + `.data` sections at their link-time byte addresses.

Wiki: [MBC ISA Reference](MBC-ISA-Reference). Source: [`docs/protocol/draft-bellis-unheaded-mbc-isa-00.md`](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md).

## Shim — the execution pipeline

The Shim is the eBPF program that lives at each hop. It:

1. Receives a packet on the XDP / TC ingress.
2. Parses the Monad option.
3. Verifies CRC-16, version, flags.
4. Looks up the active Sophia dictionary for this hop.
5. Decodes the register operations.
6. Dispatches MBC instructions (up to 256 per packet via tail-call chain).
7. Updates the Monad in-place. Recomputes CRC.
8. Forwards or drops based on Flow Actions.

The UPC's `monad-cpu-ebpf` is the canonical Shim implementation.

Wiki: [Shim Pipeline](Draft-Shim-00). Source: [`docs/protocol/draft-bellis-unheaded-shim-00.md`](../docs/protocol/draft-bellis-unheaded-shim-00.md).

## PQC — authentication

Post-quantum authentication for signed Wotan topics. Two algorithms:

- **ML-DSA-65** (FIPS 204) — fast, lattice-based. Default for `config.*` topic signing.
- **SLH-DSA** (FIPS 205) — hash-based, conservative. Belt-and-braces for high-value events (cluster join, key rotation).

Implementation via `cloudflare/circl` v1.6.3. 60 PQC tests passing. ADR-043 hard condition #2 (Wotan topic signing on `config.*`) satisfied since 2026-04-11.

Wiki: [PQC Authentication](PQC-Authentication). Source: [`docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md`](../docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md).

## The protocol as a computer

The reason there are three pillars + three companion specs is that, together, they describe a computer. Not metaphorically — operationally.

| Computer concept | Unheaded Protocol equivalent |
|---|---|
| Register file | Monad (20 bytes, 5 × u32) |
| Instruction encoding | MBC ISA |
| Microcode | Sophia dictionaries |
| Memory hierarchy | Wotan (LOCAL BPF + DISTRIBUTED gRPC) |
| CPU pipeline | Shim (XDP dispatch + tail-call chain) |
| Authentication / signing | PQC (ML-DSA-65 + SLH-DSA) |
| Clock | The packet arrival rate. Each hop is one cycle. |

The wire format is FROZEN. The MBC ISA is at v2. The Shim is implemented. Doom-on-Monad and xv6-on-UPC are the two extant programs running on this computer. Linux is the headline of the [Dream Ladder](UPC-Dream-Ladder) L6.

## Specifications

All six drafts are public Internet-Drafts:

- [`draft-bellis-unheaded-protocol-foundation-06`](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md) — Monad
- [`draft-bellis-unheaded-sophia-dictionary-03`](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md) — Sophia
- [`draft-bellis-unheaded-wotan-memory-03`](../docs/protocol/draft-bellis-unheaded-wotan-memory-03.md) — Wotan
- [`draft-bellis-unheaded-mbc-isa-00`](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) — MBC
- [`draft-bellis-unheaded-shim-00`](../docs/protocol/draft-bellis-unheaded-shim-00.md) — Shim
- [`draft-bellis-unheaded-pqc-authentication-00`](../docs/protocol/draft-bellis-unheaded-pqc-authentication-00.md) — PQC

Index: [Drafts Index](Drafts-Index).

## Cross-references

- [UPC Overview](UPC-Overview) — the substrate that dispatches the protocol
- [Linux on UPC](Linux-on-UPC) — current frontier program
- [Doom on UPC](Doom-on-UPC) — the computational-completeness proof
- [Protocol Heritage](Protocol-Heritage) — ARINC 429 → I2C → CAN Bus → BGP → BPF → IPv6 → Unheaded
- [The First Packet](The-First-Packet) — the bootstrapping story

---

> **Source:** [docs/protocol/](../docs/protocol/) · [draft-bellis-unheaded-protocol-foundation-06](../docs/protocol/draft-bellis-unheaded-protocol-foundation-06.md)
