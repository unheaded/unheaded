# Protocol Math & ASCII Maps

**Companion to:** `draft-bellis-unheaded-protocol-foundation-03.md`
**Date:** February 19, 2026
**Purpose:** Byte-level visual maps, mathematical proofs, encoding reference, and heritage lineage

---

## 1. The 20-Byte Monad: Field Layout

The Monad is a 20-byte fixed structure carried as an IPv6 Hop-by-Hop Option (RFC 8200 §4.3, RFC 9673). It contains 14 fields organized by offset, with 8 fields using exponent encoding (Sophia lookup) and 6 raw fields.

```
          THE MONAD — 20 bytes (0x14) carried in IPv6 Hop-by-Hop Option
          Stamped at ingress Shield, stripped at egress Shield.

          ┌───────────────────────────────────────────┐
          │  IPv6 Hop-by-Hop Extension Header         │
          │  Next Header = 0, Hdr Ext Len = 2         │
          │  ┌─────────────────────────────────────┐   │
          │  │  Option TLV                         │   │
          │  │  Type = TBD (IANA), Len = 20        │   │
          │  │  ┌─────────────────────────────────┐│   │
          │  │  │  THE MONAD (20 bytes)           ││   │
          │  │  │                                  ││   │
Offset    │  │  │  0x00  0x01  0x02  0x03         ││   │
          │  │  │ ┌──────┬──────┬──────┬──────┐   ││   │
   0x00   │  │  │ │ vers │ src  │ dst  │ hop  │   ││   │
   0x04   │  │  │ ├──────┬──────┬──────┬──────┤   ││   │
          │  │  │ │ qos  │ act  │ state│ flags│   ││   │
   0x08   │  │  │ ├──────┬──────┬──────┬──────┤   ││   │
          │  │  │ │  latency_hint (u16)  │ ring │   ││   │
   0x0C   │  │  │ ├──────┬──────┬──────┬──────┤   ││   │
          │  │  │ │mesh_fl│src_px│dst_px│scratch[0]  ││   │
   0x0E   │  │  │ ├──────┬──────┬──────┬──────┤   ││   │
          │  │  │ │  scratch[1-3]  │  checksum  │   ││   │
   0x12   │  │  │ ├──────────────────────────┤   ││   │
          │  │  │ │                          │   ││   │
          │  │  │ └──────────────────────────┘   ││   │
          │  │  └─────────────────────────────────┘│   │
          │  └─────────────────────────────────────┘   │
          └───────────────────────────────────────────┘

  All fields network byte order (big-endian).
  8 exponent-encoded fields (Sophia lookup): src_service_id, dst_service_id,
  qos_class, flow_action, circuit_state, deploy_ring, mesh_flags
  6 raw fields: version, hop_count, flags, latency_hint, src_prefix_lo, dst_prefix_lo
  4 scratch bytes and 2-byte checksum (raw, but may be exponent-encoded when CUSTOM flag is set)
  The Monad is the ONLY state that travels with the packet.
  All other memory lives in Wotan (see Memory Hierarchy below).
```

### Field Reference

```
OFFSET  SIZE  FIELD               TYPE        DESCRIPTION
──────  ────  ─────────────────   ───────────  ───────────────────────────
0x00    1B    version             raw u8      Protocol version (current: 0x01)
0x01    1B    src_service_id      exponent    Source service (Sophia lookup)
0x02    1B    dst_service_id      exponent    Destination service (Sophia)
0x03    1B    hop_count           raw u8      Incremented at each hop
0x04    1B    qos_class           exponent    QoS classification
0x05    1B    flow_action         exponent    Action directive
0x06    1B    circuit_state       exponent    Circuit breaker state
0x07    1B    flags               raw u8      Bitfield (C,Y,T,E,S,M,CUSTOM,RSVD)
0x08    2B    latency_hint        raw u16     Latency hint in microseconds
0x0A    1B    deploy_ring         exponent    Deployment ring
0x0B    1B    mesh_flags          exponent    Mesh-level flags
0x0C    1B    src_prefix_lo       raw u8      Source routing prefix low octet
0x0D    1B    dst_prefix_lo       raw u8      Destination routing prefix low octet
0x0E    4B    scratch[0-3]        raw u8      Scratch registers (4 bytes)
0x12    2B    checksum            raw u16     CRC-16/CCITT over bytes 0x00-0x11
──────  ────  ─────────────────   ───────────  ───────────────────────────
TOTAL   20B                                    14 fields, 8 exponent + 6 raw
```

### FLAGS Bitfield (0x07)

```
Bit 7 (MSB)                                              Bit 0 (LSB)
┌─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┬─────────┐
│    C    │    Y    │    T    │    E    │    S    │    M    │  CUST   │  RSVD   │
│  0x80   │  0x40   │  0x20   │  0x10   │  0x08   │  0x04   │  0x02   │  0x01   │
└─────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────┴─────────┘

  C (CHAOS,   bit 7, 0x80): Chaos injection marker. Visible downstream.
  Y (CANARY,  bit 6, 0x40): Packet belongs to canary deployment path.
  T (TRACED,  bit 5, 0x20): Full trace active — every hop emits to Anamnesis.
  E (ENCRYPT, bit 4, 0x10): Payload is encrypted (intra-Kingdom TLS).
  S (SAMPLED, bit 3, 0x08): Packet selected for statistical sampling.
  M (MIRROR,  bit 2, 0x04): This is a mirror copy (not the original).
  CUST (CUSTOM, bit 1, 0x02): Scratch & checksum carry exponent-encoded values.
  RSVD (bit 0, 0x01): Reserved. MUST be zero.
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
  │                  │ = 2 (24 bytes)    │                   │
  │                  │                   │                   │
  │  Unheaded Option TLV:                                   │
  │  ┌──────────┬──────────┬──────────────────────────────┐ │
  │  │ Type=TBD │ Len=20   │    Monad (20 bytes)          │ │
  │  │ (1B)     │ (1B)     │    14 fields                 │ │
  │  │ 001xxxxx │          │    (no additional metadata)  │ │
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

  Hop-by-Hop Header Structure:
    Total size: (HEL+1) x 8 = (2+1) x 8 = 24 bytes
    HEL (Hdr Ext Len) = 2: represents 24 bytes total
    Formula: HEL = (total_size / 8) - 1
    Note: HEL increases if optional metadata fields are added

  Option Type byte:
    Bits 7-6 = 00: skip if unrecognized (backward compatible)
    Bit 5    = 1:  option data MAY change en-route
    Bits 4-0 = TBD: IANA-assigned option type

  This ensures:
    - Option can be modified at each hop (per RFC 8200)
    - Unaware IPv6 routers skip the option harmlessly
    - No IHL hacks, no IP checksum recomputation
    - Standards-compliant per RFC 8200 and RFC 9673
```

---

## 3. Packet Lifecycle: ASCII Flow

```
                    PACKET LIFECYCLE (LIMITED DOMAIN)

  SHADOW (outside)           THE KINGDOM                      SHADOW (outside)
  (clean IPv6 or v4)    (IPv6 + HBH Monad + Wotan)         (clean IPv6 or v4)

    ┌──────┐           ┌────────────────────────────┐          ┌──────┐
    │ IPv6 │──ingress─▶│IPv6 │ HBH: 20-byte Monad  │─egress──▶│ IPv6 │
    │      │           │     │ version, service ids │          │      │
    │ PL   │           │ PL  │ trace_id, qos, etc   │          │ PL   │
    └──────┘           │     │ + checksum            │          └──────┘
  no Monad              │     │ (fields 0x00-0x13)   │        no Monad
                        └────────────────────────────┘
                        ▲    ▲    ▲    ▲    ▲
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
  to Anamnesis          3. Execute BPF program    to Anamnesis
  Wotan memory         4. Read/write Wotan mem  (final snapshot)
  pre-allocated         5. Update Monad in-place  Wotan memory
                        6. Verify checksum        deallocated
                        7. Log to Anamnesis        (after grace)
                        8. Forward packet
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
Let K = number of exponent key positions in the Monad
  K = 8 (exponent-encoded fields: src_service_id, dst_service_id,
         qos_class, flow_action, circuit_state, deploy_ring, mesh_flags)
  Note: K=14 counts ALL fields including raw ones, but only K=8 participate
        in Sophia exponent lookup. Raw fields have fixed semantics.

Let D = 256 (possible values per byte, since u8)
Let S(k) = the Sophia sub-dictionary selected by key byte k
Let |S(k)| = number of entries in sub-dictionary S(k), where |S(k)| <= D = 256
```

### Theorem 1: Expressible Meanings (Flat)

With K=8 independent exponent key positions, each selecting from D possible meanings:

```
  M_flat = D^K = 256^K

  K=1:  M = 256                    (8 bits)
  K=2:  M = 65,536                 (16 bits)
  K=3:  M = 16,777,216             (24 bits)
  K=8:  M = 1.844 x 10^19         (64 bits)  <- exponent fields only
```

With 8 exponent fields (K=8), we can express 1.844 x 10^19 unique meanings — approximately 2.5 times the estimated number of grains of sand on Earth (~7.5 × 10^18).

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

  Wire format for equivalent metadata:
    Hop-by-Hop header (HBH):  2 bytes (Next Header + Hdr Ext Len)
    Option TLV:               2 bytes (Type + Length)
    Monad:                   20 bytes (14 fields, all inline)
    Total:                   24 bytes on the wire

  Comparison to other metadata formats:
    HTTP/2 HEADERS frame:     ~280 bytes for similar metadata (with HPACK)
    OpenTelemetry span:       ~400 bytes serialized
    Envoy access log:         ~500 bytes

  Density ratios:
    vs HTTP headers:  24:280 = ~12x more compact
    vs OTel span:     24:400 = ~17x more compact
    vs Envoy log:     24:500 = ~21x more compact

  And critically: Monad operates at kernel datapath speed (~320 ns per hop).
                  HTTP/OTel/Envoy operate at application speed (milliseconds).
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
  Let E = event size (bytes) = 64 bytes (timestamp, type, hop, monad, wotan, trace)
  Let S = sampling rate (fraction of packets emitting events)
  Let T = retention time (seconds)

  Ring buffer memory required:
    M_ring = R x S x E x T_hot

  Where T_hot = hot retention in ring buffer (before drain to Wotan WAL)

  Example at 10 Gbps with 1500B average packets:
    R = 10 x 10^9 / (1500 x 8) = 833,333 pps
    S = 1.0 (every packet, worst case)
    E = 64 bytes
    T_hot = 2 seconds

    M_ring = 833,333 x 1.0 x 64 x 2 = 106,666,624 bytes ~ 101.7 MB

  Per-CPU ring buffers (16 CPUs):
    M_total = 101.7 MB x 16 = 1,627 MB ~ 1.6 GB

  Long-term Anamnesis storage (Wotan WAL):
    Events per day at various speeds, full trace (S=1.0):
      1 Gbps:   ~83K pps × 64B = 5.3 MB/s = 461 GB/day
      10 Gbps:  ~833K pps × 64B = 53.3 MB/s = 4.6 TB/day
      100 Gbps: ~8.3M pps × 64B = 533 MB/s = 46 TB/day

    With sampling at S = 0.01 (1%):
      1 Gbps:   4.6 GB/day
      10 Gbps:  46 GB/day
      100 Gbps: 461 GB/day

    With compression (LZ4, ~4:1 on structured binary):
      Divide results above by 4
```

### Theorem 6: Checksum Error Detection

```
  CRC-16/CCITT-FALSE over Monad bytes 0x00-0x11:

  Polynomial: x^16 + x^12 + x^5 + 1  (0x1021)
  Initial value: 0xFFFF
  Reflection (input): false
  Reflection (output): false
  XOR output: 0x0000

  Detection capability:
    - All single-bit errors: 100%
    - All double-bit errors: 100%
    - All odd-number-of-bit errors: 100%
    - Burst errors up to 16 bits: 100%
    - Burst errors of 17 bits: 99.997%
    - Random errors (>=3 bits): 99.998%

  Computation cost:
    Verification: ~50ns for 20 bytes on modern x86
    Recomputation: ~50ns for 20 bytes on modern x86
    (can use BPF helper or inline CRC table)

  Chaos injection (single-bit-flip):
    Guaranteed detection by next hop's checksum verification.
    All injected perturbations are detectable.
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
  │  ┌─────────────────────────────────────────────┐         │
  │  │ Next Header (orig NH) │ Hdr Ext Len = 2    │         │
  │  │ (Total size: 24 bytes)                      │         │
  │  │                                             │         │
  │  │  THE MONAD — 20 bytes (14 fields)           │         │
  │  │  version (u8)                               │         │
  │  │  src_service_id (exponent)                  │         │
  │  │  dst_service_id (exponent)                  │         │
  │  │  hop_count (u8)                             │         │
  │  │  qos_class (exponent)                       │         │
  │  │  flow_action (exponent)                     │         │
  │  │  circuit_state (exponent)                   │         │
  │  │  flags (u8)                                 │         │
  │  │  latency_hint (u16)                         │         │
  │  │  deploy_ring (exponent)                     │         │
  │  │  mesh_flags (exponent)                      │         │
  │  │  src_prefix_lo (u8)                         │         │
  │  │  dst_prefix_lo (u8)                         │         │
  │  │  scratch[0-3] (4 bytes)                     │         │
  │  │  checksum (u16, CRC-16/CCITT over 0x00-11) │         │
  │  │                                             │         │
  │  │  PadN alignment to 8-byte boundary          │         │
  │  └─────────────────────────────────────────────┘         │
  ├─────────────────────────────────────────────────────────┤
  │                       Payload ...                        │
  └─────────────────────────────────────────────────────────┘

  Shield ingress XDP/TC operation:
    1. Allocate Wotan memory for this flow (keyed by Flow Label)
    2. Construct Hop-by-Hop extension header with Monad
    3. Initialize Monad fields (version, service IDs, qos, flags, etc.) per program
    4. Set metadata: hop_count=0, flags, circuit_state
    5. Compute CRC-16/CCITT checksum over Monad bytes 0x00-0x11
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
    2. Extract final Monad state (all 20 bytes)
    3. Verify CRC-16/CCITT checksum — discard if failed
    4. Emit DEATH event to Anamnesis (full Monad snapshot)
    5. Strip Hop-by-Hop extension header from packet
    6. Restore original IPv6 Next Header
    7. Update IPv6 Payload Length -= HBH header size
    8. Deallocate Wotan memory (after grace period)
    9. Forward clean IPv6 packet to Shadow

  The packet exits the Limited Domain as standard IPv6.
  External nodes never observe the Monad or Hop-by-Hop header.
```

---

## 7. Wotan Memory Hierarchy: The Architecture

```
  THE MEMORY HIERARCHY — Wotan as the Memory Model

  The Monad carries ONLY fields (20 bytes, pure compute bus).
  All memory lives in Wotan, accessed by Flow Label.

  ┌──────────────────────────────────────────────────────────────┐
  │ L0: Monad Fields (20 bytes)                                  │
  │     Fields: version, src_service_id, dst_service_id,         │
  │            hop_count, qos_class, flow_action,                │
  │            circuit_state, flags, latency_hint,               │
  │            deploy_ring, mesh_flags, src_prefix_lo,           │
  │            dst_prefix_lo, scratch[0-3], checksum             │
  │     Size: 20 bytes (fixed)                                   │
  │     Latency: per-hop (~320 ns including CRC)                  │
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

  L0: 20 bytes     (14 fields in Monad)
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
  1. Registers       Monad fields (20 bytes)                 In-packet, per-hop
                     + BPF r0-r10 (per-hop scratch)         Per RFC 9669

  2. ALU             BPF instruction set (RFC 9669)         64-bit arithmetic,
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

  2. STATE: Monad circuit_state field = current state q in Q
            Monad latency_hint or scratch field = head position

  3. TRANSITION: Shim BPF program implements delta:

     current_symbol = wotan_read(head_pos);      // Read tape at head
     action = lookup_transition(circuit_state, current_symbol); // Sophia
     wotan_write(head_pos, action.write_symbol); // Write tape
     circuit_state = action.next_state;          // Update state
     head_pos = head_pos + action.head_move;     // Move head

  4. ITERATION: Packet circulation via BPF_REDIRECT
                Each circulation = one Turing machine step
                Flow Label identifies the computation

  5. HALTING: if (circuit_state == qf) → stop circulating, emit result

  THEREFORE: Unheaded + Wotan is Turing-complete.  QED.

  PRACTICAL NOTE: BPF verifier bounds each individual Shim execution
  (no infinite loops per hop), but the packet circulation loop provides
  unbounded iteration. The bound is resource (ring buffer size, packet
  lifetime), not computational.
```

### Performance Budget

```
  Can it actually run programs at useful speed?

  Single Shim execution:
    BPF overhead:            ~100 ns (XDP fast path)
    Monad read:              ~10 ns (packet memory access)
    BPF map lookup (L1):     ~100 ns
    Checksum verification:   ~50 ns
    Monad write:             ~10 ns
    Checksum recomputation:  ~50 ns
    Total per-hop:           ~320 ns

  With redirect (same-host circulation):
    Redirect overhead:       ~50 ns
    Total per cycle:         ~370 ns
    Effective MHz:           ~2.7 MHz

  For comparison:
    Original Doom (1993):    ~2-3.5 M instructions/second needed
    Monad effective rate:    ~2.7 M instructions/second
    Headroom:                ~0.77-1.35x (tight, feasible with batching)

  Multi-instruction per hop (batch execution):
    If Shim executes 4-8 instructions per hop:
    Effective rate:          ~11-21 MHz
    Headroom:                ~3-7x (comfortable)
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
  Each meaning is atomically replaceable by updating the leaf's BPF map entry.

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

  MONAD + OPTION (The Pattern):
  ┌──────────────────────────────────────────────────┐
  │ Hop-by-Hop header (2B): Next Header, Hdr Ext Len│
  │ Option TLV (2B): Type, Length                    │
  │ Monad (20B): version, src_service_id,            │
  │              dst_service_id, hop_count,          │
  │              qos_class, flow_action,             │
  │              circuit_state, flags,               │
  │              latency_hint, deploy_ring,          │
  │              mesh_flags, src_prefix_lo,          │
  │              dst_prefix_lo, scratch[0-3],        │
  │              checksum                            │
  └──────────────────────────────────────────────────┘
                                                      = 24 bytes total
                                                        (on the wire)

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

  ENVOY ACCESS LOG (equivalent info):
                                                     ~ 500 bytes

  RATIO:
  ──────
  Monad : HTTP headers    = 24 : 280  = ~12x more compact
  Monad : OTel span       = 24 : 400  = ~17x more compact
  Monad : Envoy log       = 24 : 500  = ~21x more compact

  AND: Monad operates at kernel datapath speed (~320 ns per hop).
       HTTP/OTel/Envoy operate at application speed (milliseconds).
       That is a ~10^6 speed difference on top of the 12-21x size difference.

  MTU overhead:
    Monad wire size: 24 bytes / 1500 MTU = 1.6% overhead
    (Previous measurement of 1.3% counted only the 20-byte Monad,
     not the 4-byte HBH+TLV headers)
```

---

## 11. Ring Buffer Math: When to Sample

```
  FULL TRACE (S=1.0, every packet):
    At 1 Gbps:   ~83K pps × 64B = 5.3 MB/s = 461 GB/day
    At 10 Gbps:  ~833K pps × 64B = 53.3 MB/s = 4.6 TB/day
    At 100 Gbps: ~8.3M pps × 64B = 533 MB/s = 46 TB/day

  1% SAMPLE (S=0.01):
    At 1 Gbps:   4.6 GB/day    <- easily fits on local SSD
    At 10 Gbps:  46 GB/day     <- manageable with compression
    At 100 Gbps: 461 GB/day    <- needs dedicated storage tier

  HEAD-BASED SAMPLING (sample by trace_id):
    if (trace_id % 100 < sample_rate) → emit event
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
      Option C: Backpressure via BPF map flag → BPF reduces emission rate

    Recommended policy: Option A for most traffic.
    Exception: CHAOS events → Option B (never lose chaos audit trail).
```

---

## 12. Heritage Table: The Lineage

The Unheaded Protocol builds on decades of prior art in bus-based metadata transport.

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

  PATTERN: metadata co-located with data, hop-by-hop accumulation,
           bus-as-compute. A recurring architectural pattern across
           aerospace, automotive, and networking domains.
```

---

## 13. Quick Reference Card

```
+==================================================================+
|                THE MONAD — QUICK REFERENCE                        |
+==================================================================+
|                                                                   |
|  TRANSPORT:   IPv6 Hop-by-Hop Options (RFC 8200, RFC 9673)       |
|  MONAD SIZE:  20 bytes (14 fields)                               |
|  OPTION SIZE: 24 bytes (HBH + TLV + Monad)                       |
|  COMPUTE:     BPF per RFC 9669 (BPF ISA)                         |
|                                                                   |
|  EXPONENT FIELDS: 8 (Sophia lookup)                              |
|    src_service_id, dst_service_id, qos_class, flow_action,       |
|    circuit_state, deploy_ring, mesh_flags                        |
|                                                                   |
|  RAW FIELDS: 6 (fixed semantics)                                 |
|    version, hop_count, flags, latency_hint,                      |
|    src_prefix_lo, dst_prefix_lo, scratch[0-3], checksum          |
|                                                                   |
|  CHECKSUM: CRC-16/CCITT-FALSE (polynomial 0x1021,                |
|            init 0xFFFF, no reflection)                           |
|                                                                   |
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
|  ANAMNESIS:   64 bytes/event, per-CPU ring buffer                |
|  RETENTION:   2s hot (ring), unlimited cold (Wotan WAL)          |
|  REPLAY:      Through any Sophia dictionary version               |
|                                                                   |
|  COMPLETENESS: Turing-complete (Monad + Wotan)                   |
|  EFFECTIVE:    ~2.7 MHz single-instruction, ~11-21 MHz batched   |
|  DOOM:         Feasible (~2.7 MHz vs ~2-3.5 MHz required)         |
|                                                                   |
|  NORMATIVE RFCs:                                                  |
|    RFC 8200 — IPv6 Specification                                  |
|    RFC 9673 — IPv6 Hop-by-Hop Options Processing                  |
|    RFC 9669 — BPF Instruction Set Architecture                    |
|    RFC 2119 — Key words for RFCs                                  |
|    RFC 8174 — Ambiguity of Uppercase in RFC 2119                  |
|                                                                   |
|  INFORMATIVE RFCs:                                                |
|    RFC 8799 — Limited Domains and Internet Protocols              |
|    RFC 1105 — BGP (original)                                      |
|    RFC 1883 — IPv6 (original)                                     |
|    RFC 9098 — Operational Implications of IPv6 Packets            |
|    RFC 9288 — Recommendations on Filtering of IPv6 Packets        |
|                                                                   |
+==================================================================+
```

---

*20 bytes of fields. Wotan for memory. BPF for compute. IPv6 Hop-by-Hop for transport. All standards-compliant.*
