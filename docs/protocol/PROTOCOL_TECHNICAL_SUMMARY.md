# Protocol Technical Summary

**Status**: Age 1 specification finalized (RFC v1.0-draft)
**Last Updated**: February 27, 2026 (S72 Phase 4 — Protocol Cross-References)
**Source Documents**:
- draft-bellis-unheaded-protocol-foundation-05.md (Monad)
- draft-bellis-unheaded-sophia-dictionary-02.md (Sophia)
- draft-bellis-unheaded-wotan-memory-02.md (Wotan)
- the-first-packet.md (narrative foundation)

---

## 1. Protocol Overview

Every packet inside the Limited Domain carries 20 bytes of protocol metadata. Shield stamps them on at ingress, strips them off at egress. BPF programs at each hop read, compute on, and stamp these bytes. Wotan bridges kernel datapath (nanoseconds) and userspace (milliseconds). Ring buffers (Anamnesis) record every packet event.

**Transport:** IPv4 internal. 20-byte metadata shim per packet.
**Boundary:** Shield (XDP). Stamps on ingress. Strips on egress. n+1 host sees clean IPv4.
**Compute:** BPF programs at XDP (ingress), TC (egress/mesh), kprobe (TCP lifecycle).
**Decode:** Wotan reads ring buffers, decodes through Sophia dictionaries, publishes structured events.
**Memory:** Anamnesis ring buffers — per-CPU, raw exponent keys + nanosecond timestamps.

---

## 2. The 20-Byte Protocol Layout (Monad)

The canonical packet metadata format. All values are Sophia exponent keys unless marked `raw`.

```
Offset  Size  Field               Type        Description
──────  ────  ──────────────────  ──────────  ─────────────────────────────────
0x00    1     version             raw uint8   Protocol version (current: 0x01)
0x01    1     src_service_id      exponent    Source service (Sophia lookup)
0x02    1     dst_service_id      exponent    Destination service (Sophia lookup)
0x03    1     hop_count           raw uint8   Incremented at each BPF hop
0x04    4     trace_hash          raw uint32  Flow trace correlation hash
0x08    1     qos_class           exponent    QoS classification
0x09    1     flow_action         exponent    Action directive (trace/sample/mirror/drop)
0x0A    1     circuit_state       exponent    Circuit breaker state (open/closed/half)
0x0B    1     flags               raw uint8   Bitfield: [chaos|canary|traced|encrypted|0|0|0|0]
0x0C    2     latency_hint_us     raw uint16  Upstream latency hint in microseconds
0x0E    1     deployment_ring     exponent    Deployment ring (canary/staging/prod)
0x0F    1     mesh_flags          exponent    Mesh-level flags (nat_type, direction)
0x10    2     reserved            raw         Reserved for Age 2 expansion
0x12    2     checksum            raw uint16  CRC-16 over bytes 0x00-0x11
──────  ────  ──────────────────  ──────────  ─────────────────────────────────
Total: 20 bytes (0x14)
```

**Exponent fields** (8 of 14 fields): Meaning defined by Sophia dictionary lookup. One byte = 256 possible meanings. Atomically replaceable at runtime via BPF map update.

**Raw fields** (6 of 14 fields): Fixed interpretation. `version`, `hop_count`, `trace_hash`, `flags`, `latency_hint_us`, `checksum`.

**Checksum:** CRC-16/CCITT over first 18 bytes. Verified at each hop. Corruption detected = Anamnesis anomaly event + Kenoma drift flag.

---

## 3. Sophia Dictionary Structure

Dictionaries are **trees, not tables**. Each exponent key narrows context for the next.

### Root Dictionary (BPF map: `sophia_root`)

```
Key (u8)  →  Value: { type: string, sub_dict_id: u32 }

0x01  →  { type: "service_identity",   sub_dict_id: 1 }
0x02  →  { type: "flow_action",        sub_dict_id: 2 }
0x03  →  { type: "qos_class",          sub_dict_id: 3 }
0x04  →  { type: "deployment_ring",    sub_dict_id: 4 }
0x05  →  { type: "circuit_state",      sub_dict_id: 5 }
0x06  →  { type: "mesh_flags",         sub_dict_id: 6 }
...
0xFF  →  { type: "chaos_marker",       sub_dict_id: 255 }
```

### Sub-Dictionary Example: service_identity (BPF map: `sophia_dict_1`)

```
Key (u8)  →  Value: { name: string, endpoint: string }

0x01  →  { name: "captain",       endpoint: "10.0.1.1:8080" }
0x02  →  { name: "timeguru",      endpoint: "10.0.1.2:8080" }
0x03  →  { name: "architect",     endpoint: "10.0.1.3:8080" }
0x04  →  { name: "micromanager",  endpoint: "10.0.1.4:8080" }
0x05  →  { name: "wotan",        endpoint: "10.0.1.5:4222" }
0x06  →  { name: "dashboard",     endpoint: "10.0.1.6:8081" }
0x07  →  { name: "kanban",        endpoint: "10.0.1.7:8080" }
...
```

### Kernel-Space Implementation

```c
// BPF map definitions
struct {
    __uint(type, BPF_MAP_TYPE_HASH);
    __uint(max_entries, 256);
    __type(key, u8);
    __type(value, struct sophia_entry);
} sophia_root SEC(".maps");

// Per-sub-dictionary maps (array of hash maps)
struct {
    __uint(type, BPF_MAP_TYPE_ARRAY_OF_MAPS);
    __uint(max_entries, 256);
    __type(key, u32);
    __type(value, u32);  // fd of inner map
} sophia_dicts SEC(".maps");

// Lookup chain: O(depth) hash lookups per packet
static __always_inline struct meaning *sophia_lookup(u8 key0, u8 key1) {
    struct sophia_entry *entry = bpf_map_lookup_elem(&sophia_root, &key0);
    if (!entry) return NULL;

    void *sub_dict = bpf_map_lookup_elem(&sophia_dicts, &entry->sub_dict_id);
    if (!sub_dict) return NULL;

    return bpf_map_lookup_elem(sub_dict, &key1);
}
```

---

## 4. Shield: The Protocol Boundary

### Ingress Path (XDP)

```
1. Receive IPv4 packet from outside (Shadow)
2. WAF checks:
   a. Source IP against blocklist BPF map
   b. Rate limit check (token bucket per source)
   c. Geo check (if configured)
3. If blocked: XDP_DROP, emit Shield block event to Anamnesis
4. If passed:
   a. Grow packet by 20 bytes (bpf_xdp_adjust_head or prepend shim)
   b. Write Protocol header:
      - version = 0x01
      - src_service_id = Sophia.ingress_classify(src_ip)
      - dst_service_id = Sophia.ingress_classify(dst_ip)
      - hop_count = 0
      - trace_hash = bpf_get_prandom_u32()
      - qos_class = Sophia.policy_lookup(src_ip, dst_port)
      - flow_action = default (0x00 = forward)
      - circuit_state = 0x01 (closed)
      - flags = 0x00
      - latency_hint_us = 0
      - deployment_ring = Sophia.ring_lookup(dst_service_id)
      - mesh_flags = 0x00
      - reserved = 0x0000
      - checksum = crc16(bytes[0..17])
   c. Emit birth event to Anamnesis ring buffer
5. XDP_PASS → packet enters Kingdom with Protocol header
```

### Egress Path (TC)

```
1. Receive Kingdom packet destined for outside
2. Emit death event to Anamnesis:
   - Full 20-byte Protocol snapshot (final state after all hops)
   - Exit timestamp
   - Total hop count
3. Strip 20 bytes (bpf_skb_adjust_room or remove shim)
4. Egress checks (exfiltration detection)
5. TC_ACT_OK → clean IPv4 exits to Shadow
```

---

## 5. Per-Hop BPF Processing (The Void)

At each internal hop, BPF programs read and modify the Protocol header:

```
1. Parse Protocol header (fixed offset after IP header)
2. Verify checksum (CRC-16 over first 18 bytes)
   - If invalid: emit anomaly to Anamnesis, increment STAT_CHECKSUM_FAIL
3. Increment hop_count
4. Read flow_action via Sophia lookup:
   - 0x01 (trace): emit full event to Anamnesis
   - 0x02 (sample): emit with probability from BPF map
   - 0x03 (mirror): clone packet to mirror port
   - 0x04 (rate_limit): check token bucket, stamp result
5. Component-specific logic:
   - Hauberk: read/write circuit_state
   - Pauldrons: read dst_service_id, select backend, rewrite dst
   - Vambraces: read all fields, emit observation event
6. Recompute checksum
7. XDP_PASS / TC_ACT_OK
```

---

## 6. Anamnesis: Ring Buffer Events

### Event Structure

```c
struct anamnesis_event {
    u64 timestamp_ns;        // bpf_ktime_get_ns()
    u8  event_type;          // BIRTH=0, COMPUTED=1, DEATH=6, ANOMALY=8, CHAOS=4
    u8  hop_index;           // Which hop emitted this
    u16 reserved;            // Alignment padding
    u32 input_monad[5];      // 20 bytes: Monad BEFORE this hop
    u32 output_monad[5];     // 20 bytes: Monad AFTER this hop
    u32 trace_id;            // Copied from Monad for fast correlation
    u32 wotan_addr;          // Wotan memory address (if accessed)
    u32 wotan_value;         // Value read/written (if applicable)
};  // Total: 64 bytes per event
```

### Ring Buffer Sizing

```
Target: 10Gbps line rate, 1500-byte average packets
Packet rate: ~833,333 pps
Event size: 64 bytes (per draft-03 struct)
Events/sec: ~833,333 (if every packet emits)
Buffer size needed: 833,333 × 64 = ~53.3 MB/sec
Per-CPU ring buffer: 102 MB (covers ~2 seconds at max rate)
Total (16 CPUs): ~1.6 GB ring buffer memory
```

### Wotan Ring Buffer Reader

```
1. Poll per-CPU ring buffers (epoll on ring buffer fd)
2. Batch read events (up to 256 per poll)
3. For each event:
   a. Extract protocol_snapshot[20]
   b. Decode exponent fields through Sophia userspace dictionaries
   c. Build structured event:
      {
        timestamp, event_type, hop_id,
        src_service: sophia.decode(snapshot[1]),
        dst_service: sophia.decode(snapshot[2]),
        hop_count: snapshot[3],
        trace_hash: u32_from_be(snapshot[4..8]),
        qos_class: sophia.decode(snapshot[8]),
        ...
      }
   d. Publish to Wotan topic: "anamnesis.{event_type}"
4. Subscribers (dashboard, Kenoma, alerting) receive structured events
```

---

## 7. Kenoma: State Projection

Kenoma is NOT a database. Kenoma is a **materialized view** over Anamnesis.

```
Kenoma state = fold(Anamnesis events, current Sophia dictionary)

For each unique (src_service, dst_service, trace_hash):
  - Last seen hop_count
  - Last seen circuit_state
  - Last seen qos_class
  - Event count
  - Mean latency_hint_us
  - Last anomaly timestamp (if any)

Drift detection:
  For each field in Kenoma:
    if Kenoma[field] != Pleroma[field]:
      emit drift event
      Wotan pushes Pleroma down → BPF map update → Void enforces
```

---

## 8. Pleroma: Desired State

Pleroma is the set of Sophia dictionary entries + BPF programs + policies that SHOULD be active.

```yaml
# Example Pleroma declaration
protocol:
  version: 1
  sophia_version: 47

services:
  captain:
    id: 0x01
    qos: realtime
    deployment_ring: production
    trace: always

  timeguru:
    id: 0x02
    qos: interactive
    deployment_ring: canary
    trace: sample_10pct

policies:
  circuit_breaker:
    threshold: 5_failures_in_10s
    recovery: 30s

  chaos:
    enabled: true
    yaldabaoth_probability: 0.001  # 0.1% of packets
    modes: [bit_flip, delay, duplicate]
```

Wotan reads Pleroma, computes the required BPF map state, writes updates down. The reconciliation loop runs continuously.

---

## 9. Yaldabaoth: Chaos Specification

```
Attachment: TC hook (not XDP — chaos happens after admission, not before)
Trigger: bpf_get_prandom_u32() < configured_threshold

Chaos modes (selected by BPF map configuration):
  MODE_BIT_FLIP (0x01):
    - Select random byte in Protocol header (offset 1-17, not version or checksum)
    - XOR with random mask
    - DO NOT recompute checksum (downstream must detect)

  MODE_DELAY (0x02):
    - Use TC scheduling to add N microseconds delay
    - N from BPF map (configurable per flow)

  MODE_DUPLICATE (0x03):
    - bpf_clone_redirect() to same interface
    - Both original and clone continue (phantom traffic)

  MODE_TRUNCATE (0x04):
    - Zero out bytes 0x0C-0x13 (latency_hint through reserved)
    - Partial information loss

  MODE_CHAOS_MARKER (0x05):
    - Set flags bit 7 (0x80 = chaos bit)
    - All downstream hops can see this packet has chaos injection active
    - Useful for controlled chaos testing vs blind chaos

ALL modes MUST emit to Anamnesis: event_type=CHAOS, full snapshot before and after modification.
All perturbations MUST be recorded. Chaos injection is always auditable.
```

---

## 10. Evolution Path

| Age | Transport | Metadata Space | Key Feature |
|-----|-----------|---------------|-------------|
| **1 (now)** | IPv4 + 20-byte shim | 20 bytes | Core protocol, Sophia dictionaries, Anamnesis |
| **2** | IPv6, metadata in `[::ffff:x.x.x.x]` prefix | 20 bytes (free) | Zero overhead — metadata IS the address space. Flow Label (20 bits) for trace hash. |
| **3** | IPv6 + Hop-by-Hop extension headers | Up to 64KB | Full expansion. Option type `0x1E` (RFC 4727). Arbitrary depth Sophia trees. |

The exponent encoding, Sophia dictionaries, and BPF programs are **transport-agnostic**. They work on any byte sequence at any offset. The 20-byte shim is Age 1. The architecture is transport-independent.

---

## 11. Key Metrics

| Metric | Target | Justification |
|--------|--------|---------------|
| Per-packet Protocol overhead | 24 bytes (HbH+TLV+Monad) | ~1.6% on 1500B frames |
| Sophia lookup latency | <100ns per lookup | BPF hash map O(1) |
| Ring buffer event size | 64 bytes | Input + output Monad snapshots |
| Anamnesis retention (hot) | 2 seconds at line rate | 102 MB per-CPU ring buffer |
| Shield ingress latency | <1µs added | XDP before sk_buff allocation |
| Checksum verification | <50ns | CRC-16 over 18 bytes |
| Dictionary update propagation | <10ms cluster-wide | BPF map atomic update via Wotan |

---

## S72 Phase 4 Updates (February 27, 2026)

### Specification Maturity

All three core protocol specifications are now formally published as Internet-Drafts with comprehensive implementations:

**Monad Foundation (draft-05)**
- IPv6 Hop-by-Hop extension header format (Age 2 roadmap)
- TLV container and type registry
- 8 BPF helper functions with bounds checking (bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas)
- 13 normative error codes (0x00-0x0C) with error level hierarchy
- Anamnesis event types and ring buffer formal specification
- Patches M1-M8 addressing CRC, multiple HbH, TLV critical bits, Ring Path Counter

**Sophia Dictionary (draft-02)**
- Extended entry types: Routing, Firewall, Observability, IDS, Health
- SophiaSync package contract with atomic update semantics
- Last-Writer-Wins conflict resolution with CRC verification
- Version monotonicity and rollback prevention
- Compression bomb and poisoning attack mitigations
- Patches S1-S8 addressing BPF maps, limits, versioning, cross-reference

**Wotan Memory (draft-02)**
- Formal L1/L2/L3 memory hierarchy with latency targets
- Composite key addressing (flow_label + offset)
- WAL format with seqno and tampering detection
- gRPC topic subscription protocol with backpressure
- Triple-role isolation: Ring Buffer, Event Bus, Protocol RAM
- Reliability guarantees: At-least-once delivery, idempotency, DLQ
- Patches W1-W9 addressing ring buffer, composite keys, CAS, WAL, settings, GOAWAY

### OpenAPI Specification Evolution

**Version 2.0.0** introduces 9 new endpoints for operational visibility:

- **Shield eBPF**: Program status, listing, dynamic reload
- **Anamnesis Events**: Ring buffer queries, flow-specific events, statistics
- **Kingdom Mode**: State machine transitions, health metrics

All endpoints include formal schema definitions and error responses aligned with protocol error codes.

### Protocol Maturity Across Drafts

| Dimension | Age 1 Status | Age 2 Roadmap |
|-----------|-------------|---------------|
| Transport | IPv4 + 20-byte shim | IPv6 native (zero overhead) |
| Metadata Space | 20 bytes fixed | 20-bit Flow Label + HbH extensions |
| Dictionary Depth | 2-level (root + sub) | Arbitrary depth via HbH options |
| TLV Support | Basic (M6, M8) | Full extension framework (RFC 4727) |
| Observable Events | Ring buffers per-CPU | Per-service streaming via gRPC |

### Cross-Protocol Integration

**Monad → Sophia**: 8 exponent fields decoded via hierarchical dictionary lookup (O(2) per packet)

**Monad → Wotan**: Every hop captures input/output Monad snapshots in Anamnesis ring buffers

**Sophia ↔ Wotan**: Dictionary distribution via sophia.dictionary.v{N} topic with atomic version updates

All integration points formally specified in ALIGNMENT_NOTES_DRAFT05.md.

### Error Code Normalization

All 13 normative error codes now cross-referenced with:
- RFC security patches (M1-M8, S1-S8, W1-W9)
- Recommended actions (flow-level, domain-level, system-level)
- Implementation verification (all codes tested, logged, monitored)

Error registry updated with S72 baseline; IANA allocation procedure documented for future codes (0x0E-0x1E).

---

## S67 Wire Format Freeze (February 28, 2026)

### Monad v0x01 FROZEN

The 20-byte wire format is locked at protocol version **0x01**. No further version increments planned for Age 1.

### IANA Registries Established (12 Total)

**Foundation Spec allocates:**

| # | Registry | Scope | Values | Notes |
|---|----------|-------|--------|-------|
| 1 | **Version** | Field 0x00 | 0x00–0xFF | Currently allocated: 0x01 (active) |
| 2 | **Flags Bitfield** | Byte 0x01 bits 0–7 | 8 bits | C, Y, T, E, S, M, K1, K0 (all defined) |
| 3 | **Flow Actions** | Action ID values | 0x00–0x11 (18 total) | forward, drop, dup, redirect, queue, ratelimit, encrypt, sign, etc. |
| 4 | **Kingdom Mode** | Bits K1:K0 in flags | 0x00–0x11 (4 values) | NORMAL, PRIORITY, EXPERIMENTAL, RESERVED |
| 5 | Service Identity | Sophia dict 0x01 | 0x01–0xFF (255 max) | captain, deck, engine, lookout, etc. |
| 6 | QoS Class | Sophia dict 0x03 | 0x00–0xFF | Standard, Interactive, Realtime, etc. |
| 7 | Deployment Ring | Sophia dict 0x04 | 0x00–0xFF | Canary, Staging, Production, etc. |
| 8 | Circuit State | Sophia dict 0x05 | 0x00–0xFF | Open, Closed, Half-Open, etc. |
| 9 | Mesh Flags | Sophia dict 0x06 | 0x00–0xFF | NAT Type, Direction, etc. |
| 10 | Anamnesis Event Type | Ring buffer | 0x00–0x08 (9 types) | BORN, PARSE, WOTAN_READ, WOTAN_WRITE, SOPHIA_LOOKUP, ACTION_EXEC, REWRITE, ERROR, DEATH |
| 11 | Error Code | Protocol errors | 0x00–0x0C (13 codes) | Version mismatch, CRC failure, access denied, bounds check failure, etc. |
| 12 | BPF Helper Function | Kernel API | 8 total | bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas, (5 reserved) |

### Intellectual Property & Implementation Status

- **IPR Disclosure:** Complete and verified
- **Patent Collision Check:** Zero conflicts identified
- **Implementation Readiness:** WEST bare metal system online and operational
- **Freeze Date:** February 28, 2026, 02:25 UTC

### Backward Compatibility Guarantee

The 20-byte Monad structure is final for Age 1. Age 2 (IPv6 native transport) will expand the metadata space via Hop-by-Hop extension headers without modifying the core 20-byte envelope.

---

*BPF computes. Anamnesis records. Wotan translates. Sophia decodes.*
*Age 1 production-ready. Age 2 designed. Age 3 planned.*
