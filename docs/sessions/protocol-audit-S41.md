# S41 Protocol Audit — Phase 3

**Date:** 2026-02-24
**Auditor:** Claude Opus 4.6 (automated)
**Scope:** BPF wire formats, map entries, CRC/checksum, port usage, RFC references

---

## 1. BPF-Related Go Packages Inventory

100 Go files reference BPF/eBPF/XDP. Key packages:

| Package | Role |
|---------|------|
| `pkg/ebpf/` | From-scratch BPF loader (direct syscalls), Anamnesis event types, mapfreeze, maploader |
| `pkg/protocol/bpfschema/` | Canonical Go struct definitions for all BPF map key/value types |
| `pkg/protocol/encoding/` | Wire encoding primitives (varint, exponent, CRC-16/CCITT, CRC-32/MPEG-2, TLV) |
| `cmd/trace-collector-go/` | TraceEntry, FlowKey, FlowState, LatencyKey, LatencyEntry decoders |
| `cmd/dashboard-backend/internal/ebpf/` | Dashboard Anamnesis consumer, flow/latency ingestors |
| `pkg/netlink/` | Netlink interface for XDP/TC attachment |
| `pkg/validation/` | Syscall validator, bounds checks |

## 2. Rust eBPF Program Sources

Seven eBPF programs under `ebpf/`:

| Program | Type | File |
|---------|------|------|
| **shield-ebpf** | XDP ingress + TC egress | `ebpf/shield-ebpf/src/main.rs` |
| **hop-ebpf** | XDP per-hop processor | `ebpf/hop-ebpf/src/main.rs` |
| **yaldabaoth-ebpf** | TC egress chaos injector | `ebpf/yaldabaoth-ebpf/src/main.rs` |
| **flow-tracker** | TC ingress+egress flow tracking | `ebpf/flow-tracker/src/main.rs` |
| **latency-probe** | Kprobe/kretprobe TCP latency | `ebpf/latency-probe/src/main.rs` |
| **syscall-tracer** | Raw tracepoint syscall audit | `ebpf/syscall-tracer/src/main.rs` |
| **monad-cpu-ebpf** | XDP MBC virtual CPU (Doom) | `ebpf/monad-cpu-ebpf/src/main.rs` |

Shared type libraries:
- `ebpf/common/src/lib.rs` — TraceId, FlowKey, FlowState, PacketEvent, LatencyEvent, SyscallEvent (IPv4-oriented)
- `ebpf/monad-common/src/lib.rs` — Monad, AnamnesisEvent, HopByHopHeader, CrcCcitt, SophiaEntry (IPv6-oriented)
- `ebpf/latency-probe/src/common.rs` — LatencyKey, LatencyEntry (LATENCY_MAP schema)
- `ebpf/flow-tracker/src/common.rs` — Documentation-only; canonical types in `ebpf/common/src/lib.rs`

---

## 3. Wire Format Alignment Status

### 3.1 Monad Register File (20 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `monad_common::Monad` | 20B (compile-time assertion) | Reference |
| Go `pkg/ebpf.MonadState` | 20B (field-by-field decode) | MATCH |
| Go `pkg/protocol/bpfschema.MonadRegister` | 20B (`MonadRegisterSize = 20`) | MATCH |

All three representations agree on field offsets 0x00-0x13. Encoding/decoding in Go uses `binary.BigEndian` for LatencyHint (0x08-0x09) and Checksum (0x12-0x13), matching Rust's `to_be_bytes()` usage.

### 3.2 AnamnesisEvent (32 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `monad_common::AnamnesisEvent` | 32B (compile-time assertion) | Reference |
| Go `pkg/ebpf.AnamnesisEvent` (decoder) | 32B (`AnamnesisEventSize = 32`) | MATCH |
| Go `pkg/protocol/bpfschema.AnamnesisEvent` | 32B (`AnamnesisEventSize = 32`) | MATCH |

Wire layout: `[0..8] timestamp_ns LE, [8] event_type, [9] hop_id, [10..12] flow_label_lo BE, [12..32] monad`.

### 3.3 FlowKey (16 bytes) -- ALIGNED

| Source | Size | Notes |
|--------|------|-------|
| Rust `unheaded_common::FlowKey` | 16B | IPv4 5-tuple: src_addr(4) + dst_addr(4) + src_port(2) + dst_port(2) + proto(1) + pad(3) |
| Go `cmd/trace-collector-go/flow_reader.go FlowKey` | 16B (`FlowKeySize = 16`) | MATCH |
| Go `pkg/protocol/bpfschema.FlowKey` | 16B (`FlowKeySize = 16`) | MATCH |

**Note:** `flow_reader.go` decodes all fields as LittleEndian even though the comment says "network byte order as stored by BPF." The Rust struct stores addresses in network byte order (big-endian) as received from the kernel. The Go side reads them with `binary.LittleEndian` at offsets 0:4 and 4:8, which means the u32 value will be byte-swapped relative to the BPF-stored value. This is a **potential byte-order mismatch** for address display (but the SrcIP/DstIP methods then convert with `binary.BigEndian.PutUint32`, partially compensating). The decode-as-LE then display-as-BE approach is internally consistent but confusing and could break if the kernel stores addresses differently on big-endian architectures.

### 3.4 FlowState (72 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `unheaded_common::FlowState` | 72B | 16 (TraceId) + 6*8 (u64s) + 1 (state) + 7 (pad) = 72 |
| Go `cmd/trace-collector-go/flow_reader.go FlowStateEntry` | 72B (`FlowStateSize = 72`) | MATCH |
| Go `pkg/protocol/bpfschema.FlowState` | 72B (`FlowStateSize = 72`) | MATCH |
| Rust `ebpf/flow-tracker/src/common.rs` documentation | 72B | MATCH |

### 3.5 FlowEvent (104 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `unheaded_common::FlowEvent` | 104B (8 + 16 + 72 + 1 + 7) | Reference |
| Go `cmd/trace-collector-go/flow_reader.go FlowEventEntry` | 104B (`FlowEventSize = 104`) | MATCH |

### 3.6 LatencyKey (40 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `latency-probe/common.rs LatencyKey` | 40B (compile-time assertion) | Reference |
| Go `cmd/trace-collector-go/latency_reader.go LatencyKey` | 40B (`LatencyKeySize = 40`) | MATCH |

### 3.7 LatencyEntry (40 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `latency-probe/common.rs LatencyEntry` | 40B (compile-time assertion) | Reference |
| Go `cmd/trace-collector-go/latency_reader.go LatencyEntry` | 40B (`LatencyEntrySize = 40`) | MATCH |

### 3.8 TraceEntry (68 bytes) -- UNVERIFIABLE

The Go `cmd/trace-collector-go/types.go TraceEntry` is 68 bytes and uses IPv6 (16-byte) addresses. There is **no corresponding Rust struct** -- this appears to be a Go-only format for the packet_marker BPF program which is not yet implemented in Rust. The comment says it mirrors `ebpf/common/src/lib.rs` but no matching struct exists there. The `ebpf/common` crate defines a `PacketEvent` (different layout) instead.

**FINDING: TraceEntry has no Rust counterpart.** The Go decoder at `cmd/trace-collector-go/types.go` defines a 68-byte TraceEntry with IPv6-width addresses (16B each), while `ebpf/common/src/lib.rs` defines `PacketEvent` with IPv4-width addresses (inside FlowKey with u32 src/dst). These are structurally different types serving different subsystems and should not be confused.

### 3.9 SyscallEvent -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `unheaded_common::SyscallEvent` | 8-byte aligned (test verified) | Reference |
| Go consumer | N/A (no explicit Go decoder found in trace-collector-go) | N/A |

### 3.10 HopByHopHeader (24 bytes) -- ALIGNED

| Source | Size | Status |
|--------|------|--------|
| Rust `monad_common::HopByHopHeader` | 24B (compile-time assertion) | Reference |
| Go (no explicit struct) | Decoded inline in doom-go-injector | Consistent |

### 3.11 MigrationTokenValue (48 bytes) -- ALIGNMENT CONCERN

| Source | Size | Repr |
|--------|------|------|
| Rust `flow-tracker/main.rs MigrationTokenValue` | 48B | `#[repr(C, packed)]` |
| Go `pkg/protocol/bpfschema.FlowMigrationTokenValue` | 48B | Standard Go alignment |

**FINDING:** The Rust struct uses `#[repr(C, packed)]` which means NO padding. The Go struct uses standard alignment. Fields are: `token[16]` + `expiry_ns(u64)` + `new_src_addr(u32)` + `new_dst_addr(u32)` + `new_src_port(u16)` + `new_dst_port(u16)` + `flags(u32)` + `_pad[4]`. With `packed`, `expiry_ns` sits at offset 16; in Go, the `ExpiryNs uint64` at offset 16 is naturally 8-byte aligned (16 is divisible by 8), so **no padding is inserted by Go**. The layouts happen to be compatible despite different packing semantics.

### 3.12 CancelFlowValue (24 bytes) -- ALIGNMENT CONCERN

| Source | Size | Repr |
|--------|------|------|
| Rust `flow-tracker/main.rs CancelFlowValue` | 24B | `#[repr(C, packed)]` |
| Go `pkg/protocol/bpfschema.FlowCancelValue` | 24B | Standard Go alignment |

**FINDING:** The Rust struct is `packed`: `reason(u32)` at offset 0, `timestamp_ns(u64)` at offset 4, `flags(u32)` at offset 12, `_pad[4]` at offset 16 = 20 bytes total. But the comment says 24 bytes with alignment padding. The Go struct explicitly documents Go alignment padding: `Reason(4)` + `(4 implicit padding)` + `TimestampNs(8)` + `Flags(4)` + `_pad(4)` = 24 bytes. **The Rust `packed` struct is 20 bytes while Go expects 24 bytes.** The Go code's own comment at line 393-404 of `core_maps.go` acknowledges this discrepancy and documents it. This is a **real wire format mismatch** that would cause incorrect decoding if Go reads a Rust-packed value.

---

## 4. Map Entry Format Consistency

### 4.1 TRACE_MAP / STATS / FLOW_STATE / LATENCY_MAP / PACKET_EVENTS

The `BPFLoader` interface in `cmd/trace-collector-go/loader.go` defines accessors for:
- `GetTraceMap()` -- STATS hash map (confusingly named)
- `GetStatsMap()` -- STATS map
- `GetFlowStateMap()` -- FLOW_STATE map (FlowKey -> FlowState)
- `GetLatencyMap()` -- LATENCY_MAP (LatencyKey -> LatencyEntry)
- `GetPacketEventsCh()` -- PACKET_EVENTS ring buffer

### 4.2 Map Name Consistency

The `pkg/protocol/bpfschema/bpfschema.go` defines canonical map names with `unhd_` prefix:
- `unhd_hmac_keys`, `unhd_seq_counters`, `unhd_ring_path`, etc.

The Rust eBPF programs use Aya's `#[map]` attribute names:
- `ANAMNESIS`, `SOPHIA`, `CIRCUIT_ERRORS`, `CONFIG`, `STATS`, `FLOWS`, `TRACE_ASSOC`, `FLOW_EVENTS`, etc.

**FINDING:** There is a naming gap between the canonical `unhd_*` names in `bpfschema.go` and the actual Aya map names in Rust. The `unhd_*` names are intended for pinned map paths; the Aya `#[map]` names become the ELF section names. The `pkg/ebpf/maploader/` package bridges this gap, but there is no automated test verifying that pinned names match loader expectations.

### 4.3 Map Type Consistency

| Map | Rust Type | bpfschema Documented Type | Status |
|-----|-----------|--------------------------|--------|
| FLOWS | `LruHashMap<FlowKey, FlowState>` | Not listed (HASH implied) | LRU vs HASH differs |
| SOPHIA | `HashMap<u16, SophiaEntry>` | `SophiaMapKey{u16}` -> `SophiaMapValue{[32]byte}` | MATCH |
| LATENCY_MAP | `HashMap<LatencyKey, LatencyEntry>` | (defined in latency_reader.go) | MATCH |
| ANAMNESIS | `RingBuf` 8 MiB | `AnamnesisRingSize = 8*1024*1024` | MATCH |
| PACKET_EVENTS | `RingBuf` 256 KiB | `PacketEventsRingSize = 256*1024` | MATCH |
| STATS (all programs) | `HashMap<u32, u64>` | `StatsEntry{u64}` | MATCH |
| CONFIG (all programs) | `HashMap<u32, u64>` | `ConfigEntry{u64}` | MATCH |

---

## 5. CRC/Checksum Status

### 5.1 CRC-16/CCITT-FALSE (Monad Integrity)

Two independent implementations:

| Location | Polynomial | Init | Reflect | Final XOR |
|----------|-----------|------|---------|-----------|
| Rust `monad_common::CrcCcitt::compute()` | 0x1021 | 0xFFFF | No | 0x0000 |
| Go `pkg/ebpf.CRC16CCITT()` | 0x1021 | 0xFFFF | No | 0x0000 |
| Go `pkg/protocol/encoding.CRC16CCITT()` | 0x1021 | 0xFFFF | No | 0x0000 |

**Status: ALIGNED.** Both Rust and Go implementations use identical parameters. The Rust test verifies `CrcCcitt::compute(b"123456789") == 0x29B1` which is the standard CRC-16/CCITT-FALSE check value.

**FINDING: Duplicate CRC implementations.** `pkg/ebpf.CRC16CCITT()` and `pkg/protocol/encoding.CRC16CCITT()` are identical functions in different packages. This violates DRY; one should be the canonical implementation imported by the other.

### 5.2 CRC-32/MPEG-2 (Extended Checksum)

Implemented in `pkg/protocol/encoding.CRC32MPEG2()` only (Go side). No Rust counterpart found. Used for extended checksum scenarios not yet in eBPF programs.

### 5.3 Checksum Protection Region

Both Rust and Go agree: CRC covers Monad octets 0x00 through 0x11 (18 bytes). Checksum stored at 0x12-0x13 in big-endian.

### 5.4 CUSTOM Flag Bypass

Both `hop-ebpf` and `shield-ebpf` correctly skip CRC verification when `flags::CUSTOM` is set (checksum field carries exponent keys instead). The Go `MonadState.VerifyChecksum()` does NOT check for CUSTOM flag -- **callers must check `HasFlag(FlagCustom)` before calling.**

---

## 6. Exponent Encoding Status

Exponent encoding is used for Monad fields: SrcServiceID, DstServiceID, QoSClass, FlowAction, CircuitState, DeployRing, MeshFlags, Scratch registers.

- Go implementation: `pkg/protocol/encoding.EncodeExponent()` / `DecodeExponent()`
- Rust: Values are stored as raw u8 in the Monad wire format. The "exponent encoding" documented in `bpfschema` comments describes the semantic interpretation, not the wire encoding. Each Monad field IS a single byte.

**FINDING:** The exponent encoding in `pkg/protocol/encoding` produces a uint16 (2 bytes), but Monad fields are single bytes. The exponent encoding is used for Sophia dictionary lookups and user-facing APIs, NOT for the Monad wire format itself. The bpfschema comments saying fields are "exponent-encoded" are misleading -- the BPF programs treat these as raw u8 values.

---

## 7. Port Usage Audit

### 7.1 Canonical Port Registry

`pkg/ports/ports.go` is the single source of truth. All ports fall within the Doom Range (16666-26666).

### 7.2 Hardcoded Port Instances

The following files use literal port strings rather than the `pkg/ports` constants:

| File | Hardcoded Value | Should Use |
|------|----------------|------------|
| `cmd/trace-collector-go/main.go:62` | `":16670"` | `ports.DefaultAddr(ports.TraceCollectorHTTP)` |
| `cmd/trace-collector-go/main.go:72` | `":16671"` | `ports.DefaultAddr(ports.TraceCollectorMetrics)` |
| `cmd/dashboard-backend/main.go:74` | `":20000"` | `ports.DefaultAddr(ports.DashboardBackend)` |
| `cmd/dashboard-backend/internal/server/server.go:81,95` | `":20000"` | `ports.DefaultAddr(ports.DashboardBackend)` |
| `cmd/unheaded-daemon/main.go:1342` | `":17000"` | `ports.DefaultAddr(ports.DaemonHTTP)` |
| `cmd/unheaded-daemon/internal/config/config.go:194` | `":17000"` | `ports.DefaultAddr(ports.DaemonHTTP)` |
| `pkg/http/server.go:91,111,293` | `":20000"` | `ports.DefaultAddr(ports.DashboardBackend)` |
| `pkg/tracing/collector.go:1264` | `":16670"` | `ports.DefaultAddr(ports.TraceCollectorHTTP)` |
| `pkg/baremetal/pxe/pxe.go:44` | `":17000"` | `ports.DefaultAddr(ports.DaemonHTTP)` |
| `pkg/baremetal/image/image.go:38` | `":17000"` | `ports.DefaultAddr(ports.DaemonHTTP)` |
| `services/cape/cape.go:127-128` | `":20000"`, `":18001"` | `ports.DefaultAddr(...)` |

**FINDING:** 13+ files hardcode port values as string literals instead of using `pkg/ports` constants. This creates a maintenance burden and risks desynchronization if ports change.

---

## 8. Hop-by-Hop Extension Header Handling

### 8.1 Shield XDP Ingress (BIRTH)

- Strips ALL existing IPv6 extension headers (next_header values 0, 43, 44, 51, 60) from inbound Shadow traffic
- Inserts 24-byte HBH header using `bpf_xdp_adjust_head(delta=-24)`
- Sets IPv6 Next_Header = 0 (HBH)
- Sets Flow Label from `bpf_get_prandom_u32()`
- Correctly processes RFC 8200 section 4.2 option type action bits for Destination Options
- Bounded loop (`MAX_EXT_HDRS_TO_STRIP = 8`)

### 8.2 Shield TC Egress (DEATH)

- Verifies Monad option type (0x3E) and data length (20)
- Captures final Monad state for DEATH event
- Strips HBH header using `bpf_skb_adjust_room(delta=-24, BPF_ADJ_ROOM_NET)`
- Restores original Next_Header
- Decrements Payload_Length by 24

### 8.3 Hop XDP Processing

- Parses IPv6 + HBH to find Monad option via bounded scan (`find_monad_option`, max 16 iterations)
- Verifies CRC (unless CUSTOM flag set)
- Checks chaos guard, hop limit
- Performs Sophia dictionary lookups, circuit breaker enforcement
- Increments hop count (saturating)
- Applies flow action semantics
- Recomputes CRC
- Writes mutated Monad in-place (no packet resize)

### 8.4 Monad Option Type Encoding

`MONAD_OPT_TYPE = 0x3E`:
- Bits 7-6 (action): `00` -- skip if unrecognized (safe for non-Kingdom nodes)
- Bit 5 (change-en-route): `1` -- option data MAY change at each hop
- Bits 4-0 (number): `0x1E` (30) -- locally assigned in limited domain

This follows RFC 8200 section 4.2 and RFC 9673 section 6 correctly.

**Status: Correctly implemented.** The HBH handling is thorough and RFC-compliant for a limited-domain deployment.

---

## 9. RFC References Status

### 9.1 RFCs Referenced in Code

| RFC | Topic | Where Used |
|-----|-------|-----------|
| RFC 8200 | IPv6 Specification | Shield (ext header stripping, HBH option type encoding, Destination Options action bits), Hop (option parsing), bpfschema |
| RFC 9673 | IPv6 Hop-by-Hop Options in Limited Domains | Monad option type assignment |
| RFC 9000 | QUIC Transport | Varint encoding, HMAC keys, sequence counters, migration tokens, retry tokens, amplification limiting |
| RFC 9114 | HTTP/3 | Settings, error counters, GOAWAY, flow types, DoS backpressure, TLV registry, cancellation |
| RFC 9669 | BPF Instruction Set | "BPF" naming convention, map type selection |
| RFC 9562 | UUIDv7 | Trace ID generation |
| RFC 8126 | IANA Registry Procedures | Protocol registry allocation policies |
| RFC 7240 | HTTP Prefer Header | Prefetch hints |
| RFC 6298 | TCP RTO | ECN bit setting reference |

### 9.2 Observations

**FINDING: Misapplied RFC references.** Several BPF maps cite QUIC (RFC 9000) and HTTP/3 (RFC 9114) section numbers for features that are inspired by but not compliant with those protocols. For example:
- `SEQ_COUNTERS` cites "RFC 9000 section 5.1.1" but implements a simplified per-namespace counter, not QUIC packet number spaces
- `SETTINGS` cites "RFC 9114 section 7.2.4" but uses a different key encoding than HTTP/3 SETTINGS frames
- `GOAWAY_STATE` cites "RFC 9114 section 7.2.6" but tracks connection-level GOAWAY in a BPF map rather than HTTP/3 frame semantics

These references are best understood as design inspiration rather than protocol compliance claims. The code comments and `bpfschema.go` should clarify this distinction.

---

## 10. Summary of Findings

### Critical (wire format mismatch)

1. **CancelFlowValue Rust/Go size mismatch.** Rust `#[repr(C, packed)]` produces 20 bytes (no padding between `reason:u32` and `timestamp_ns:u64`). Go standard alignment inserts 4 bytes of padding, producing 24 bytes. The Go comment documents this but the actual BPF map value will be 20 bytes on the Rust side.

### Significant

2. **TraceEntry has no Rust counterpart.** The 68-byte TraceEntry in `cmd/trace-collector-go/types.go` is a Go-only format. The corresponding Rust `PacketEvent` has a different layout (IPv4 FlowKey instead of IPv6 addresses).

3. **Duplicate CRC-16/CCITT implementations.** `pkg/ebpf.CRC16CCITT()` and `pkg/protocol/encoding.CRC16CCITT()` are identical. One should import the other.

4. **13+ files hardcode port values** instead of using `pkg/ports` constants.

5. **Map name gap.** Canonical `unhd_*` names in `bpfschema.go` vs Aya `#[map]` names in Rust are not verified by automated tests.

### Minor

6. **Exponent encoding documentation is misleading.** The `bpfschema` comments describe Monad fields as "exponent-encoded" but BPF programs treat them as raw u8. Exponent encoding is a user-facing API concern, not a wire format concern.

7. **RFC references are inspirational, not compliance claims.** QUIC/HTTP/3 section citations describe design patterns borrowed from those protocols, not strict conformance.

8. **CUSTOM flag bypass not enforced in Go.** `MonadState.VerifyChecksum()` does not check the CUSTOM flag internally; callers must check `HasFlag(FlagCustom)` first.

9. **FlowKey byte order in flow_reader.go is confusing.** Addresses stored in network byte order by BPF are decoded with `binary.LittleEndian`, then re-interpreted with `binary.BigEndian` for display. Internally consistent but error-prone.

---

## Files Examined

- `ebpf/common/src/lib.rs` -- shared IPv4 types (FlowKey, FlowState, PacketEvent, etc.)
- `ebpf/monad-common/src/lib.rs` -- Monad, AnamnesisEvent, HopByHopHeader, CrcCcitt
- `ebpf/latency-probe/src/common.rs` -- LatencyKey, LatencyEntry
- `ebpf/latency-probe/src/main.rs` -- latency probe kprobe program
- `ebpf/flow-tracker/src/main.rs` -- flow tracker TC program
- `ebpf/flow-tracker/src/common.rs` -- documentation-only wire format
- `ebpf/hop-ebpf/src/main.rs` -- hop XDP program
- `ebpf/shield-ebpf/src/main.rs` -- shield XDP + TC programs
- `ebpf/yaldabaoth-ebpf/src/main.rs` -- chaos injection TC program
- `ebpf/syscall-tracer/src/main.rs` -- syscall audit raw tracepoint
- `cmd/trace-collector-go/types.go` -- TraceEntry, StatsEntry, FiveTuple
- `cmd/trace-collector-go/flow_reader.go` -- FlowKey, FlowStateEntry, FlowEventEntry
- `cmd/trace-collector-go/latency_reader.go` -- LatencyKey, LatencyEntry
- `cmd/trace-collector-go/reader.go` -- TraceReader (poll + ringbuf modes)
- `cmd/trace-collector-go/loader.go` -- BPFLoader interface
- `pkg/ebpf/loader.go` -- from-scratch BPF syscall loader
- `pkg/ebpf/anamnesis.go` -- MonadState, AnamnesisEvent Go types, CRC16CCITT
- `pkg/protocol/bpfschema/bpfschema.go` -- canonical map schemas
- `pkg/protocol/bpfschema/core_maps.go` -- MonadRegister, AnamnesisEvent, FlowKey, etc.
- `pkg/protocol/encoding/encoding.go` -- varint, exponent, CRC-16, CRC-32, TLV
- `pkg/ports/ports.go` -- canonical port registry
