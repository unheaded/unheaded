# Protocol Math & ASCII Maps

**Companion to:** `draft-bellis-unheaded-protocol-foundation-01.md`
**Date:** February 18, 2026 (updated)
**Purpose:** Byte-level visual maps, mathematical proofs, encoding reference, and heritage lineage

---

## 1. The 20-Byte Monad: Register File Map

The Monad is a 20-byte register file carried as an IPv6 Hop-by-Hop Option (RFC 8200 §4.3, RFC 9673).  It is organized as five 32-bit words:

```
          THE MONAD — 20 bytes (0x14) carried in IPv6 Hop-by-Hop Option
          Born at Shield ingress. Dies at Shield egress. Shadow never sees.

          ┌───────────────────────────────────────────┐
          │  IPv6 Hop-by-Hop Extension Header         │
          │  Next Header = 0, Hdr Ext Len = 5         │
          │  ┌─────────────────────────────────────┐   │
          │  │  Option TLV                         │   │
          │  │  Type = TBD (IANA), Len = 20        │   │
          │  │  ┌─────────────────────────────────┐│   │
          │  │  │  THE MONAD (Register File)      ││   │
          │  │  │                                  ││   │
Offset    │  │  │  0x00  0x01  0x02  0x03         ││   │
          │  │  │ ┌──────┬──────┬──────┬──────┐   ││   │
   0x00   │  │  │ │            R0 (u32)        │   ││   │
          │  │  │ │       Accumulator           │   ││   │
          │  │  │ ├──────┼──────┼──────┼──────┤   ││   │
   0x04   │  │  │ │            R1 (u32)        │   ││   │
          │  │  │ │       Argument              │   ││   │
          │  │  │ ├──────┼──────┼──────┼──────┤   ││   │
   0x08   │  │  │ │            R2 (u32)        │   ││   │
          │  │  │ │       Result                │   ││   │
          │  │  │ ├──────┼──────┼──────┼──────┤   ││   │
   0x0C   │  │  │ │            R3 (u32)        │   ││   │
          │  │  │ │       Counter               │   ││   │
          │  │  │ ├──────┼──────┼──────┼──────┤   ││   │
   0x10   │  │  │ │            R4 (u32)        │   ││   │
          │  │  │ │       Caller / Error        │   ││   │
          │  │  │ └──────┴──────┴──────┴──────┘   ││   │
          │  │  └─────────────────────────────────┘│   │
          │  └─────────────────────────────────────┘   │
          └───────────────────────────────────────────┘

  All fields network byte order (big-endian).
  Semantics are program-defined — the protocol does not prescribe use.
  The Monad is the ONLY state that travels with the packet.
  All other memory lives in Wotan (see Memory Hierarchy below).
```

### Register Reference

```
OFFSET  SIZE  NAME   TYPE   PURPOSE
──────  ────  ─────  ─────  ─────────────────────────────────
0x00    4B    R0     u32    General-purpose accumulator
0x04    4B    R1     u32    Argument or secondary value
0x08    4B    R2     u32    Result or output register
0x0C    4B    R3     u32    Counter or loop variable
0x10    4B    R4     u32    Caller ID or error code
──────  ────  ─────  ─────  ─────────────────────────────────
TOTAL   20B                 5 registers × 32 bits = 160 bits
```

### Optional Metadata Fields (following the Monad)

Following the 20-byte Monad, additional metadata fields may be packed using exponent encoding within the same Hop-by-Hop option (up to 255 bytes total):

```
FIELD           SIZE    ENCODING         DESCRIPTION
──────────────  ────    ───────────────  ─────────────────────
hop_count       1B      raw u8           Hops since Shield birth
latency_hint    2B      exp (base=2)     2^exp ns upstream latency
loss_rate       1B      exp (base=2)     2^-exp / 256 loss fraction
queue_depth     2B      exp (base=2)     2^exp packets
flags           1B      bitfield         See FLAGS below
trace_hash      4B      raw u32          Flow trace correlation
checksum        2B      CRC-16/CCITT     Over Monad + metadata
```

### FLAGS Bitfield

```
Bit 7 (MSB)                                              Bit 0 (LSB)
┌─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┐
│  CHAOS  │ CANARY  │ TRACED  │ ENCRYPT │ SAMPLED │ MIRRORED│  RSVD   │  RSVD   │
│  0x80   │  0x40   │  0x20   │  0x10   │  0x08   │  0x04   │  0x02   │  0x01   │
└─────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────┘

  CHAOS    (bit 7): Yaldabaoth touched this packet. Visible to all downstream hops.
  CANARY   (bit 6): Packet belongs to canary deployment path.
  TRACED   (bit 5): Full trace active — every hop emits to Anamnesis.
  ENCRYPT  (bit 4): Payload is encrypted (intra-Kingdom TLS).
  SAMPLED  (bit 3): Packet selected for statistical sampling.
  MIRRORED (bit 2): This is a mirror copy (not the original).
  RSVD     (bit 1): Reserved.
  RSVD     (bit 0): Reserved.
```

---

## 2. IPv6 Hop-by-Hop Option: Wire Format

The Monad is carried in an IPv6 Hop-by-Hop Options extension header per RFC 8200 §4.3 and RFC 9673.

```
                   IPv6 HOP-BY-HOP OPTION WIRE FORMAT
                   (RFC 8200 §4.3, RFC 9673)

  IPv6 Fixed Header (40 bytes):
  ┌─────────────────────────────────────────────────────────┐
  │ Version=6 │ Traffic Class │ Flow Label (20 bits)         │
  │ Payload Length │ Next Header = 0 │ Hop Limit             │
  │ Source Address (128 bits)                                │
  │ Destination Address (128 bits)                           │
  └─────────────────────────────────────────────────────────┘
                    │
                    │  Next Header = 0 (Hop-by-Hop)
                    ▼
  Hop-by-Hop Extension Header:
  ┌─────────────────────────────────────────────────────────┐
  │ Next Header (1B) │ Hdr Ext Len (1B) │                   │
  │                  │ = 5 (48 bytes)    │                   │
  │                  │                   │                   │
  │  Unheaded Option TLV:                                   │
  │  ┌──────────┬──────────┬──────────────────────────────┐ │
  │  │ Type=TBD │ Len=20+  │    Monad (20 bytes)          │ │
  │  │ (1B)     │ (1B)     │    R0, R1, R2, R3, R4        │ │
  │  │ 00T00000 │          │    + optional metadata        │ │
  │  └──────────┴──────────┴──────────────────────────────┘ │
  │                                                         │
  │  Padding to 8-byte alignment (PadN if needed)           │
  └─────────────────────────────────────────────────────────┘
                    │
                    │  Next Header → Transport (TCP/UDP/etc.)
                    ▼
  ┌─────────────────────────────────────────────────────────┐
  │                  Transport Header + Payload              │
  └─────────────────────────────────────────────────────────┘

  Option Type byte:
    Bits 7-6 = 00: skip if unrecognized (backward compatible)
    Bit 5    = 0:  unchanged in transit (compliant routers leave it)
    Bits 4-0 = TBD: IANA-assigned option type

  This ensures:
    - Unaware IPv6 routers skip the option harmlessly
    - No IHL hacks, no IP checksum recomputation
    - Standards-compliant per RFC 8200 and RFC 9673
```

---

## 3. Packet Lifecycle: ASCII Flow

```
                    THE LIFECYCLE OF A KINGDOM PACKET

  SHADOW (outside)           THE KINGDOM                      SHADOW (outside)
  (clean IPv6 or v4)    (IPv6 + HBH Monad + Wotan)         (clean IPv6 or v4)

    ┌──────┐           ┌────────────────────────────┐          ┌──────┐
    │ IPv6 │──ingress─▶│IPv6 │ HBH: Monad [R0..R4] │─egress──▶│ IPv6 │
    │      │           │     │ + metadata + flags    │          │      │
    │ PL   │           │ PL  │ + checksum            │          │ PL   │
    └──────┘           └────────────────────────────┘          └──────┘
  no Monad              ▲    ▲    ▲    ▲    ▲                no Monad
                        │    │    │    │    │
                     Shield  S1   S2   S3  Shield
                     (birth) Shim Shim Shim (death)
                              │    │    │
                              ▼    ▼    ▼
                        ┌─────────────────────┐
                        │    Wotan Memory     │
                        │  (ring buffers, WAL) │
                        │  keyed by Flow Label │
                        └─────────────────────┘
                              │    │    │
                              ▼    ▼    ▼
                        ┌─────────────────────┐
                        │   Anamnesis Events   │
                        │  (per-CPU ring buf)  │
                        └─────────────────────┘

  Shield stamps         Each Shim:                Shield strips
  Monad ON              1. Parse HBH option       Monad OFF
  Birth event           2. Extract Monad           Death event
  to Anamnesis          3. Execute eBPF program   to Anamnesis
  Wotan memory         4. Read/write Wotan mem  (final snapshot)
  pre-allocated         5. Update Monad in-place  Wotan memory
                        6. Log to Anamnesis        deallocated
                        7. Forward packet          (after grace)
```

---

## 4. Sophia Dictionary Tree: ASCII Map

```
                      SOPHIA ROOT MAP
                    ┌─────────────────┐
                    │ sophia_root     │
                    │ BPF_MAP_HASH    │
                    │ key: u8         │
                    │ val: dict_id    │
                    └────────┬────────┘
                             │
            ┌────────────────┼────────────────┬──────────────────┐
            ▼                ▼                ▼                  ▼
     key=0x01          key=0x02         key=0x03           key=0xFF
  ┌──────────────┐ ┌──────────────┐ ┌──────────────┐  ┌──────────────┐
  │ dict_1:      │ │ dict_2:      │ │ dict_3:      │  │ dict_255:    │
  │ service_id   │ │ flow_action  │ │ qos_class    │  │ chaos_marker │
  │              │ │              │ │              │  │              │
  │ 0x01=captain │ │ 0x01=forward │ │ 0x01=bulk    │  │ 0x01=bit_flip│
  │ 0x02=timeguru│ │ 0x02=trace   │ │ 0x02=interact│  │ 0x02=delay   │
  │ 0x03=architct│ │ 0x03=sample  │ │ 0x03=realtime│  │ 0x03=dup     │
  │ 0x04=microman│ │ 0x04=mirror  │ │ 0x04=critical│  │ 0x04=truncate│
  │ 0x05=wotan  │ │ 0x05=drop    │ │              │  │ 0x05=marker  │
  │ 0x06=dashbrd │ │ 0x06=ratelim │ │              │  │              │
  │ 0x07=kanban  │ │              │ │              │  │              │
  └──────────────┘ └──────────────┘ └──────────────┘  └──────────────┘

  LOOKUP CHAIN (2 hops, O(1) each):
  ──────────────────────────────────
  key[0] = 0x01 → sophia_root → dict_id = 1 (service_identity)
  key[1] = 0x03 → dict_1      → "architect"

  SAME key[1], DIFFERENT key[0]:
  ──────────────────────────────
  [0x01, 0x03] → service   = "architect"
  [0x02, 0x03] → action    = "sample"
  [0x03, 0x03] → qos       = "realtime"
  [0xFF, 0x03] → chaos     = "duplicate"
```

---

## 5. Exponential Composition: The Math

### Definitions

```
Let K = number of exponent key positions in the metadata
Let D = 256 (possible values per byte, since u8)
Let S(k) = the Sophia sub-dictionary selected by key byte k
Let |S(k)| = number of entries in sub-dictionary S(k), where |S(k)| <= D = 256
```

### Theorem 1: Expressible Meanings (Flat)

With K independent exponent key positions, each selecting from D possible meanings:

```
  M_flat = D^K = 256^K

  K=1:  M = 256                    (8 bits)
  K=2:  M = 65,536                 (16 bits)
  K=3:  M = 16,777,216             (24 bits)
  K=8:  M = 1.844 x 10^19         (64 bits)
  K=14: M = 5.192 x 10^33          (112 bits)  <- all 14 fields
```

With 8 exponent fields (K=8), we can express 1.844 x 10^19 unique meanings — more than every grain of sand on Earth multiplied by a thousand.

### Theorem 2: Expressible Meanings (Composed)

When dictionaries are trees (maps of maps), composition multiplies:

```
  M_composed = product (i=1 to depth) |S(key_i)|

  For uniform dictionaries where |S| = D = 256:
  M_composed = D^depth = 256^depth
```

The key insight: **the same bytes carry different semantics depending on context**. Byte `0x03` means "architect" in the service dictionary, "sample" in the flow action dictionary, and "realtime" in the QoS dictionary. The first byte selects the dictionary; the second byte selects the entry.

### Theorem 3: Information Density

```
  Information per exponent byte = log2(D) = log2(256) = 8 bits

  Information per Monad + metadata:
    Monad registers:  5 x 32 bits = 160 bits (program state)
    Metadata fields:  variable, exponent-encoded
    Total (20B min):  160 bits of register state
    Total (with metadata): up to 2040 bits (255B option payload)

  For comparison:
    HTTP/2 HEADERS frame: ~50-200 bytes for similar metadata (with HPACK)
    OpenTelemetry span:   ~200-500 bytes serialized
    Envoy access log:     ~500-2000 bytes
    Our Monad:            20 bytes. At wire speed. At every hop.
```

### Theorem 4: Dictionary Update Propagation

```
  Let N = number of Kingdom hosts (hops)
  Let P = Sophia dictionary update payload size (bytes)
  Let B = Wotan fanout factor (concurrent BPF map updates)

  Propagation time:
    T_prop = T_wotan_encode + (N/B) x T_bpf_map_update

  Where:
    T_wotan_encode  ~ 1us     (serialize Sophia entry to BPF map format)
    T_bpf_map_update ~ 100ns   (single bpf_map_update_elem syscall)
    B                = N        (Wotan writes to all maps in parallel)

  Therefore:
    T_prop ~ 1us + 100ns = ~1.1us  (cluster-wide, regardless of N)

  With safety margin and network latency:
    T_prop_real ~ 1-10ms (conservative target)
```

### Theorem 5: Anamnesis Storage Requirements

```
  Let R = packet rate (packets/second)
  Let E = event size (bytes) = 56 bytes (see draft §Anamnesis)
  Let S = sampling rate (fraction of packets emitting events)
  Let T = retention time (seconds)

  Ring buffer memory required:
    M_ring = R x S x E x T_hot

  Where T_hot = hot retention in ring buffer (before drain to Wotan WAL)

  Example at 10 Gbps with 1500B average packets:
    R = 10 x 10^9 / (1500 x 8) = 833,333 pps
    S = 1.0 (every packet, worst case)
    E = 56 bytes
    T_hot = 2 seconds

    M_ring = 833,333 x 1.0 x 56 x 2 = 93,333,296 bytes ~ 89 MB

  Per-CPU ring buffers (16 CPUs):
    M_total = 89 MB x 16 = 1,424 MB ~ 1.4 GB

  Long-term Anamnesis storage (Wotan WAL):
    Events per day at 10 Gbps full trace:
      833,333 x 56 x 86,400 = 4.03 TB/day (raw)

    With sampling at S = 0.01 (1%):
      40.3 GB/day (manageable)

    With compression (LZ4, ~4:1 on structured binary):
      10.1 GB/day
```

### Theorem 6: Checksum Error Detection

```
  CRC-16/CCITT over Monad + metadata bytes:

  Polynomial: x^16 + x^12 + x^5 + 1  (0x1021)
  Initial value: 0xFFFF

  Detection capability:
    - All single-bit errors: 100%
    - All double-bit errors: 100%
    - All odd-number-of-bit errors: 100%
    - Burst errors up to 16 bits: 100%
    - Burst errors of 17 bits: 99.997%
    - Random errors (>=3 bits): 99.998%

  Computation cost: ~50ns for 20 bytes on modern x86
  (can use BPF helper or inline CRC table)

  Yaldabaoth single-bit-flip:
    Guaranteed detection by next hop's checksum verification.
    The chaos is always caught.
```

---

## 6. Shield Transform: Byte-Level View (IPv6 Hop-by-Hop)

### Ingress (Shadow -> Kingdom)

```
BEFORE (standard IPv6 — 40-byte fixed header + payload):

  ┌─────────────────────────────────────────────────────────┐
  │                  IPv6 Fixed Header (40 bytes)            │
  │  Ver=6 | TC | Flow Label | Payload Len | NH | Hop Limit │
  │  Source Address (128 bits)                               │
  │  Destination Address (128 bits)                          │
  ├─────────────────────────────────────────────────────────┤
  │                       Payload ...                        │
  └─────────────────────────────────────────────────────────┘


AFTER (Kingdom packet — IPv6 + Hop-by-Hop + Monad + payload):

  ┌─────────────────────────────────────────────────────────┐
  │                  IPv6 Fixed Header (40 bytes)            │
  │  Ver=6 | TC | Flow Label | Payload Len* | NH=0* | HL   │
  │  Source Address (ULA prefix, 128 bits)                   │
  │  Destination Address (ULA prefix, 128 bits)              │
  │  (* NH changed to 0 = Hop-by-Hop, Payload Len updated)  │
  ├─────────────────────────────────────────────────────────┤
  │  Hop-by-Hop Extension Header                             │
  │  ┌─────────────────────────────────────────────────┐     │
  │  │ Next Header (orig NH) │ Hdr Ext Len = 5        │     │
  │  │ Option Type = TBD     │ Option Len = 20+       │     │
  │  │                                                 │     │
  │  │  THE MONAD — 20 bytes (Register File)           │     │
  │  │  R0: Accumulator                                │     │
  │  │  R1: Argument                                   │     │
  │  │  R2: Result                                     │     │
  │  │  R3: Counter                                    │     │
  │  │  R4: Caller / Error                             │     │
  │  │                                                 │     │
  │  │  Optional metadata (exponent-encoded fields)    │     │
  │  │  Checksum (CRC-16/CCITT)                        │     │
  │  │  PadN alignment                                 │     │
  │  └─────────────────────────────────────────────────┘     │
  ├─────────────────────────────────────────────────────────┤
  │                       Payload ...                        │
  └─────────────────────────────────────────────────────────┘

  Shield ingress XDP/TC operation:
    1. Allocate Wotan memory for this flow (keyed by Flow Label)
    2. Construct Hop-by-Hop extension header with Monad
    3. Initialize Monad registers (R0-R4) per program
    4. Set metadata: hop_count=0, trace_hash, flags
    5. Compute checksum over Monad + metadata
    6. Update IPv6 Next Header = 0 (Hop-by-Hop)
    7. Update IPv6 Payload Length += HBH header size
    8. Emit BIRTH event to Anamnesis ring buffer
    9. Forward packet

  NOTE: No IHL hacks. No IP checksum recomputation.
  This is a standards-compliant IPv6 extension header per RFC 8200.
  Unaware routers skip the option (Type bits 7-6 = 00).
  The Limited Domain boundary (RFC 8799) ensures containment.
```

### Egress (Kingdom -> Shadow)

```
  Shield egress TC operation (reverse of ingress):
    1. Parse Hop-by-Hop extension header
    2. Extract final Monad state
    3. Verify checksum — discard if failed
    4. Emit DEATH event to Anamnesis (full Monad snapshot)
    5. Strip Hop-by-Hop extension header from packet
    6. Restore original IPv6 Next Header
    7. Update IPv6 Payload Length -= HBH header size
    8. Deallocate Wotan memory (after grace period)
    9. Forward clean IPv6 packet to Shadow

  The packet exits the Kingdom as standard IPv6.
  Shadow never sees the Monad. Shadow never knows.
```

---

## 7. Wotan Memory Hierarchy: The Architecture

```
  THE MEMORY HIERARCHY — Wotan as the Memory Model

  The Monad carries ONLY registers (20 bytes, pure compute bus).
  All memory lives in Wotan, accessed by Flow Label.

  ┌──────────────────────────────────────────────────────────────┐
  │ L0: Monad Registers (R0-R4)                                 │
  │     Size: 20 bytes (fixed)                                   │
  │     Latency: wire speed (nanoseconds)                        │
  │     Location: IN the packet (Hop-by-Hop option)              │
  │     Analogy: CPU registers                                   │
  ├──────────────────────────────────────────────────────────────┤
  │ L1: Per-Hop BPF Map Cache                                   │
  │     Size: variable (pages prefetched by Wotan)              │
  │     Latency: ~100-200 ns (kernel BPF map lookup)             │
  │     Location: kernel memory, per-hop                         │
  │     Analogy: L1/L2 CPU cache                                 │
  │     Managed by: Wotan pre-stages pages before packet arrives│
  ├──────────────────────────────────────────────────────────────┤
  │ L2: Wotan Ring Buffer (RAM)                                 │
  │     Size: configurable via --ring-size flag                  │
  │     Latency: ~1-10 us (userspace ring buffer access)         │
  │     Location: userspace memory, per-flow (keyed by Flow Lbl) │
  │     Analogy: Main memory (RAM)                               │
  │     Features: backpressure, pub/sub, configurable allocation │
  ├──────────────────────────────────────────────────────────────┤
  │ L3: Wotan WAL (Persistent Storage)                          │
  │     Size: disk-bounded                                       │
  │     Latency: ~100 us - 1 ms (disk I/O)                      │
  │     Location: local disk, per-flow                           │
  │     Analogy: Swap / disk                                     │
  │     Features: durable, survives restarts, TTL-based eviction │
  ├──────────────────────────────────────────────────────────────┤
  │ L4: Sophia Dictionaries (Instruction Decode Only)            │
  │     Size: variable (BPF maps)                                │
  │     Latency: ~100-200 ns (BPF map lookup)                    │
  │     Location: kernel BPF maps                                │
  │     Analogy: Microcode ROM                                   │
  │     NOT data memory — instruction decode only                │
  └──────────────────────────────────────────────────────────────┘

  CACHE MISS FLOW:
  ────────────────
  1. Shim executes LD instruction → reads L1 BPF map cache
  2. Cache miss (address not in L1)
  3. Shim emits compute.mem.miss event → Wotan handles
  4. Shim sets stalled=1, returns XDP_PASS
  5. Wotan reads from L2 ring buffer (or promotes from L3 WAL)
  6. Wotan writes page to L1 BPF map cache
  7. Next packet circulation finds data cached → cache hit
  8. Execution resumes

  MEMORY BUDGET (example: Doom over IPv6):
  ─────────────────────────────────────────
  Doom requires ~4 MB RAM (128KB screen + heap + stack + WAD index)

  L0: 20 bytes     (5 registers)
  L1: 64 KB        (16 pages x 4 KB, hot working set)
  L2: 4 MB         (--ring-size=4194304, full Doom RAM)
  L3: 12 MB        (WAD data, cold pages, on disk)
  L4: 16 KB        (Sophia instruction decode tables)
```

### Address Space Layout

```
  Address space layout (configurable per program):

  0x00000000 +------------------+
             |  Data Memory     |  <- Wotan ring buffer channel
             |  (general RAM)   |     per-flow, keyed by Flow Label
  0x0000BFFF +------------------+
  0x0000C000 |  Screen / Output |  <- Wotan I/O topic
             |  (I/O region)    |     Wotan reads -> dashboard
  0x0000FFFE +------------------+
  0x0000FFFF |  Input (1 word)  |  <- Wotan I/O topic
             |  (keyboard/etc)  |     Wotan writes <- external
             +------------------+

  Wotan Memory Service Config:

    type MemoryServiceConfig struct {
        RingSize    int64  `yaml:"ring_size"`    // --ring-size flag
        PageSize    int    `yaml:"page_size"`    // L1 cache page size
        PrefetchN   int    `yaml:"prefetch_n"`   // Pages to prefetch
        WALEnabled  bool   `yaml:"wal_enabled"`
        WALPath     string `yaml:"wal_path"`
    }
```

---

## 8. Computational Completeness

### The Five Primitives

Any general-purpose computer requires exactly five things. The Monad + Wotan provides all five:

```
  PRIMITIVE          MONAD/WOTAN PROVIDES                  HOW
  ────────────────   ───────────────────────────────         ──────────────────────
  1. Registers       Monad R0-R4 (20 bytes)                 In-packet, wire speed
                     + eBPF r0-r10 (per-hop scratch)        Per RFC 9669

  2. ALU             eBPF instruction set (RFC 9669)        64-bit arithmetic,
                     ADD, SUB, MUL, DIV, AND, OR, XOR,      logic, shifts, jumps
                     LSH, RSH, NEG, MOD, JEQ, JGT, JGE...  at every hop

  3. Memory          Wotan ring buffers (L1-L3)            --ring-size scales
                     Addressable, per-flow, persistent       from KB to GB

  4. I/O             Wotan I/O topics                      Memory-mapped regions
                     Screen, keyboard, sensors               bridged to dashboard

  5. Clock           Per-hop processing                     Each hop = one tick
                     Packet circulation for loops            BPF_REDIRECT for repeat
```

### Turing Completeness Proof Sketch

```
  CLAIM: Unheaded + Wotan can simulate any Turing machine M.

  PROOF:
  Let M = (Q, A, delta, q0, qf) be a Turing machine with:
    Q = finite set of states
    A = tape alphabet
    delta = transition function Q x A -> Q x A x {L, R}
    q0 = initial state
    qf = halting state

  CONSTRUCTION:

  1. TAPE:  Wotan ring buffer (L2) = infinite tape
            Address i holds symbol at tape position i
            --ring-size bounds physical tape (resource limit, not fundamental)

  2. STATE: Monad R3 = current state q in Q
            Monad R4 = head position (tape offset)

  3. TRANSITION: Shim eBPF program implements delta:

     current_symbol = wotan_read(R4);         // Read tape at head
     action = lookup_transition(R3, current_symbol);  // Sophia table
     wotan_write(R4, action.write_symbol);    // Write tape
     R3 = action.next_state;                   // Update state
     R4 = R4 + action.head_move;               // Move head

  4. ITERATION: Packet circulation via BPF_REDIRECT
                Each circulation = one Turing machine step
                Flow Label identifies the computation

  5. HALTING: if (R3 == qf) → stop circulating, emit result

  THEREFORE: Unheaded + Wotan is Turing-complete.  QED.

  PRACTICAL NOTE: eBPF verifier bounds each individual Shim execution
  (no infinite loops per hop), but the packet circulation loop provides
  unbounded iteration. The bound is resource (ring buffer size, packet
  lifetime), not computational.
```

### Performance Budget

```
  Can it actually run programs at useful speed?

  Single Shim execution:
    eBPF overhead:           ~100 ns (XDP fast path)
    Monad read:              ~10 ns (packet memory access)
    BPF map lookup (L1):     ~100 ns
    Monad write:             ~10 ns
    Total per-hop:           ~220 ns

  Effective clock speed:
    1 hop = 1 instruction = ~220 ns
    Effective MHz = 1 / 220e-9 = ~4.5 MHz

  With BPF_REDIRECT circulation (same host):
    Redirect overhead:       ~50 ns
    Total per cycle:         ~270 ns
    Effective MHz:           ~3.7 MHz

  For comparison:
    Original Doom (1993):    ~2-3.5 M instructions/second needed
    Monad effective rate:    ~3.7 M instructions/second
    Headroom:                ~1.06-1.85x (tight but feasible)

  Multi-instruction per hop (batch execution):
    If Shim executes 4-8 instructions per hop:
    Effective rate:          ~15-30 MHz
    Headroom:                ~4-15x (comfortable)
```

---

## 9. Exponential Composition: Visual Proof

```
  WHY 2 BYTES = 65,536 MEANINGS (not 512)

  WRONG (additive):
    byte_0 has 256 meanings + byte_1 has 256 meanings = 512 meanings

  RIGHT (multiplicative / compositional):
    byte_0 SELECTS A DICTIONARY (256 possible dictionaries)
    byte_1 SELECTS AN ENTRY (256 entries per dictionary)
    Total = 256 x 256 = 65,536 unique (dictionary, entry) pairs

  VISUAL:

  byte_0 = 0x01 ──┐     ┌── byte_1 = 0x01 → captain
  (service_id)     ├─────┤── byte_1 = 0x02 → timeguru
                   │     ├── byte_1 = 0x03 → architect
                   │     └── byte_1 = ... (256 entries)
                   │
  byte_0 = 0x02 ──┐     ┌── byte_1 = 0x01 → forward
  (flow_action)    ├─────┤── byte_1 = 0x02 → trace
                   │     ├── byte_1 = 0x03 → sample
                   │     └── byte_1 = ... (256 entries)
                   │
  byte_0 = 0x03 ──┐     ┌── byte_1 = 0x01 → bulk
  (qos_class)      ├─────┤── byte_1 = 0x02 → interactive
                   │     ├── byte_1 = 0x03 → realtime
                   │     └── byte_1 = ... (256 entries)
                   │
  ...              │
  (256 dicts)      │
                   │
  byte_0 = 0xFF ──┐     ┌── byte_1 = 0x01 → bit_flip
  (chaos)          ├─────┤── byte_1 = 0x02 → delay
                         ├── byte_1 = 0x03 → duplicate
                         └── byte_1 = ... (256 entries)

  256 branches x 256 leaves = 65,536 total meanings
  And each meaning is HOT-SWAPPABLE by updating the leaf's BPF map entry.

  SCALING LAW:
  ─────────────
  Depth 1:  256^1 = 256                 meanings
  Depth 2:  256^2 = 65,536              meanings
  Depth 3:  256^3 = 16,777,216          meanings
  Depth 4:  256^4 = 4,294,967,296      meanings
  Depth 5:  256^5 = 1,099,511,627,776  meanings
  ...
  Depth K:  256^K meanings

  General formula: M = D^K where D=256 (values per byte), K=depth
```

---

## 10. Information Density Comparison

```
  BYTES NEEDED TO EXPRESS "packet from timeguru to captain, QoS realtime,
  traced, canary deployment, circuit closed, no chaos":

  MONAD + METADATA (The Pattern):
  ┌──────────────────────────────────────────────────┐
  │ R0=program_state  R1=args  R2=result  R3=ctr    │ = 20 bytes (Monad)
  │ R4=caller                                        │
  │ + hop=0 trace=A3F1B2C4 qos=03 act=01 flags=60   │ + ~13 bytes metadata
  └──────────────────────────────────────────────────┘ = ~35 bytes total
                                                         (in HBH option)

  HTTP HEADERS (equivalent info):
  ┌────────────────────────────────────────────────┐
  │ X-Service-Source: timeguru                     │
  │ X-Service-Dest: captain                        │
  │ X-QoS-Class: realtime                          │
  │ X-Trace-ID: a3f1b2c4                           │
  │ X-Flow-Action: trace                           │
  │ X-Circuit-State: closed                        │
  │ X-Deploy-Ring: canary                          │
  │ X-Flags: traced                                │
  └────────────────────────────────────────────────┘ ~ 280 bytes

  OPENTELEMETRY SPAN (equivalent info):
  ┌────────────────────────────────────────────────┐
  │ {                                              │
  │   "traceId": "a3f1b2c4...",                    │
  │   "spanId": "...",                             │
  │   "operationName": "...",                      │
  │   "tags": {                                    │
  │     "service.source": "timeguru",              │
  │     "service.dest": "captain",                 │
  │     ...                                        │
  │   }                                            │
  │ }                                              │
  └────────────────────────────────────────────────┘ ~ 400+ bytes

  RATIO:
  ──────
  Monad+meta : HTTP headers  = 35 : 280  = 8x more compact
  Monad+meta : OTel span     = 35 : 400  = 11x more compact
  Monad+meta : Envoy log     = 35 : 500  = 14x more compact

  AND: Monad operates at wire speed (nanoseconds).
       HTTP/OTel/Envoy operate at application speed (milliseconds).
       That's a 10^6 speed difference on top of the 8-14x size difference.
```

---

## 11. Ring Buffer Math: When to Sample

```
  FULL TRACE (S=1.0, every packet):
    At 1 Gbps:   ~83K pps x 56B = 4.7 MB/s = 400 GB/day
    At 10 Gbps:  ~833K pps x 56B = 47 MB/s = 4.0 TB/day
    At 100 Gbps: ~8.3M pps x 56B = 466 MB/s = 40 TB/day

  1% SAMPLE (S=0.01):
    At 1 Gbps:   4.0 GB/day    <- easily fits on local SSD
    At 10 Gbps:  40 GB/day     <- manageable with compression
    At 100 Gbps: 400 GB/day    <- needs dedicated storage tier

  HEAD-BASED SAMPLING (sample by trace_hash):
    if (trace_hash % 100 < sample_rate) → emit event
    Guarantees all events for a given flow are either ALL sampled or NONE.
    No partial traces. No orphan spans.

  ADAPTIVE SAMPLING (via Sophia dictionary):
    qos_class = realtime → always trace (S=1.0)
    qos_class = interactive → sample 10% (S=0.1)
    qos_class = bulk → sample 1% (S=0.01)
    flags & CHAOS → always trace (need full chaos audit trail)

    Sophia dictionary update changes sampling policy instantly.
    No code change. No restart.

  MEMORY SAFETY:
    Ring buffer is bounded. If Wotan falls behind:
      Option A: Drop oldest events (ring overwrites)
      Option B: Drop new events (ring returns -ENOSPC)
      Option C: Backpressure via BPF map flag → eBPF reduces emission rate

    Kingdom policy (Pleroma declares): Option A for most traffic.
    Exception: CHAOS events (Yaldabaoth) → Option B (never lose chaos audit).
```

---

## 12. Heritage Table: The Lineage

The Unheaded Protocol did not emerge from nothing. It is the latest inscription of a Pattern that has existed since the first bus carried the first bit with metadata attached.

```
  TECHNOLOGY          YEAR  PATTERN ELEMENT           RFC/STANDARD
  ─────────────────   ────  ───────────────────────   ─────────────────
  ARINC 429           1977  Self-contained words,     ARINC Spec 429
                            every bit position
                            meaningful, bus as
                            compute

  I2C                 1982  Two-wire bus, address      Philips/NXP
                            in first byte, every
                            device reads every
                            clock pulse

  CAN Bus             1986  Two wires, no central     ISO 11898
                            controller, identifier
                            = address + priority,
                            bus as backplane

  BGP                 1989  Path attributes riding     RFC 1105 (orig)
                            with routes, hop-by-hop    RFC 4271 (current)
                            accumulation, AS_PATH
                            as breadcrumb trail

  BPF                 1992  Packet filter in kernel,   McCanne/Jacobson
                            evolved to eBPF            RFC 9669 (ISA)
                            general-purpose VM

  IPv6                1995  Extension headers,         RFC 1883 (orig)
                            typed/length-prefixed,     RFC 8200 (current)
                            extensible computation
                            space in the IP header

  IPv6 HBH Options    2024  Hop-by-Hop processing      RFC 9673
                            rehabilitated, per-hop
                            read and act, the
                            Pattern's grammar

  Unheaded Protocol   2026  Mapped data bus model,     This document
                            packet as memory,          (I-D)
                            20-byte register file,
                            Wotan memory hierarchy,
                            computational completeness

  PATTERN: metadata riding with data. The bus as the computer.
           The wire as the processor. Hop-by-hop accumulation.
           The same design, drawn over and over, in cars and planes
           and routers and kernels. Different Shadows, same Pattern.
```

---

## 13. Quick Reference Card

```
+==================================================================+
|                THE MONAD — QUICK REFERENCE                        |
+==================================================================+
|                                                                   |
|  TRANSPORT:   IPv6 Hop-by-Hop Options (RFC 8200, RFC 9673)       |
|  MONAD SIZE:  20 bytes (5 x u32 registers)                       |
|  OPTION MAX:  255 bytes (Monad + metadata + checksum)             |
|  COMPUTE:     eBPF per RFC 9669 (BPF ISA)                        |
|                                                                   |
|  REGISTERS:   R0 (accum), R1 (arg), R2 (result),                 |
|               R3 (counter), R4 (caller/error)                     |
|  MEMORY:      Wotan ring buffers (L1-L3), per-flow              |
|  I/O:         Wotan topics (screen, keyboard, sensors)           |
|  CLOCK:       Per-hop processing (each hop = 1 tick)              |
|                                                                   |
|  BOUNDARY:    Shield (XDP/TC ingress / egress)                    |
|  STAMPED:     At ingress (birth) — HBH option added              |
|  STRIPPED:    At egress (death) — HBH option removed              |
|  LEAKED:      NEVER. Shadow never sees the Monad.                 |
|  DOMAIN:      Limited Domain per RFC 8799                         |
|                                                                   |
|  SOPHIA:      O(1) per BPF map, O(depth) per chain               |
|  UPDATE:      ~1us (BPF map atomic update via Wotan)             |
|  PROPAGATE:   <10ms cluster-wide                                  |
|                                                                   |
|  ANAMNESIS:   56 bytes/event, per-CPU ring buffer                 |
|  RETENTION:   2s hot (ring), unlimited cold (Wotan WAL)          |
|  REPLAY:      Through any Sophia dictionary version               |
|                                                                   |
|  COMPLETENESS: Turing-complete (Monad + Wotan)                   |
|  EFFECTIVE:    ~3.7 MHz single-instruction, ~30 MHz batched       |
|  DOOM:         Yes. (after the dashboard)                         |
|                                                                   |
|  NORMATIVE RFCs:                                                  |
|    RFC 8200 — IPv6 Specification                                  |
|    RFC 9673 — IPv6 Hop-by-Hop Options Processing                  |
|    RFC 2119 — Key words for RFCs                                  |
|    RFC 8174 — Ambiguity of Uppercase in RFC 2119                  |
|                                                                   |
|  INFORMATIVE RFCs:                                                |
|    RFC 9669 — BPF Instruction Set Architecture                    |
|    RFC 8799 — Limited Domains and Internet Protocols              |
|    RFC 1105 — BGP (original)                                      |
|    RFC 1883 — IPv6 (original)                                     |
|    RFC 9098 — Operational Implications of IPv6 Packets            |
|    RFC 9288 — Recommendations on Filtering of IPv6 Packets        |
|                                                                   |
+==================================================================+
```

---

*The math doesn't lie. 20 bytes of registers. Wotan for memory. eBPF for compute. IPv6 Hop-by-Hop for transport. All standards-compliant. The Pattern is real.*
