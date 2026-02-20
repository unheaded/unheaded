# BPF/IPv6 Interface Map — Top-Down RFC Standardization

## Overview

This document maps EVERY existing BPF program and BPF map in the Unheaded Kingdom
to the RFC standards they implement, the Go protocol packages that manage them from
userspace, and identifies gaps where standardization is needed.

**Purpose**: Ensure every eBPF data-plane component can trace its behavior back to a
specific RFC section, and every Go userspace component knows exactly which BPF maps
it owns.

---

## 1. BPF Program → RFC → Go Package Matrix

### Shield XDP (Ingress Boundary)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| Strip IPv6 extension headers | RFC 8200 | §4 | pkg/ebpf/loader | — |
| Insert HBH Options header (24B) | RFC 8200 | §4.2 | pkg/ebpf/loader | — |
| Set Monad Option Type 0x3E | RFC 8200/9673 | §4.2/§6 | pkg/protocol/intermediary | — |
| Initialize Monad (20B) | Monad spec | §3 | pkg/protocol/encoding | — |
| CRC-16/CCITT computation | RFC 1071 (pattern) | — | pkg/protocol/encoding | — |
| Set IPv6 Flow Label (20-bit) | RFC 8200 | §6 | pkg/protocol/flowtype | — |
| Emit BIRTH event | Monad spec | §6.1 | pkg/ebpf/anamnesis | ANAMNESIS |
| Source blocklist check | — | — | (custom) | BLOCKLIST |
| Rate limiting (token bucket) | RFC 9000 | §21.3 | pkg/protocol/amplification | RATE_TOKENS |
| **GAP: No HMAC validation** | RFC 9000 | §8.1 | pkg/protocol/integrity | unhd_hmac_keys |
| **GAP: No retry token check** | RFC 9000 | §8.1.2 | pkg/protocol/migration | unhd_retry_tokens |
| **GAP: No hop validator** | RFC 8200 | §4 | pkg/protocol/intermediary | unhd_hop_validators |

### Shield TC (Egress Boundary)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| Capture final Monad state | Monad spec | §3 | pkg/protocol/encoding | — |
| Emit DEATH event | Monad spec | §6.1 | pkg/ebpf/anamnesis | ANAMNESIS |
| Strip HBH header (24B removal) | RFC 8200 | §4.2 | pkg/ebpf/loader | — |
| Restore transport Next Header | RFC 8200 | §4 | pkg/ebpf/loader | — |
| **GAP: No GOAWAY enforcement** | RFC 9114 | §7.2.6 | pkg/protocol/lifecycle | unhd_goaway_state |
| **GAP: No error counter update** | RFC 9114 | §8 | pkg/protocol/errors | unhd_error_counters |
| **GAP: No authority check** | RFC 8200 | §4.2 | pkg/protocol/intermediary | unhd_authority |

### Hop XDP (Per-Hop ALU)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| Verify Monad CRC-16 | RFC 1071 | — | pkg/protocol/encoding | — |
| Enforce hop limits | RFC 8200 | §4 (TTL analogy) | pkg/protocol/amplification | — |
| Update QoS class | Monad spec | §4 | pkg/protocol/flowtype | SOPHIA |
| Update circuit state | Monad spec | §4 | (custom) | SOPHIA |
| Apply flow action | Monad spec | §4.2 | pkg/protocol/flowtype | — |
| Increment hop count | RFC 8200 | §4 | — | — |
| Emit HOP events | Monad spec | §6.1 | pkg/ebpf/anamnesis | ANAMNESIS |
| Sophia dictionary lookup | Sophia spec | §3 | pkg/protocol/sophiasync | SOPHIA |
| Circuit breaker tracking | — | — | (custom) | CIRCUIT_ERRORS |
| ✅ Seq counter update | RFC 9000 | §5.1.1 | pkg/protocol/sequence | SEQ_COUNTERS |
| ✅ Settings check | RFC 9114 | §7.2.4 | pkg/protocol/settings | SETTINGS |
| ✅ DoS backpressure | RFC 9114 | §10 | pkg/protocol/dos | DOS_STATE |
| ✅ Flow type dispatch | RFC 9114 | §6 | pkg/protocol/flowtype | FLOW_TYPES |

### Yaldabaoth TC (Chaos Injection)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| Bit flip (corruption) | — | — | (Black Mage) | CHAOS_TARGETS |
| Delay injection | — | — | (Black Mage) | CHAOS_TARGETS |
| Packet duplication | — | — | (Black Mage) | CHAOS_TARGETS |
| Truncation | — | — | (Black Mage) | CHAOS_TARGETS |
| Chaos marker flag | Monad spec | §4 (flags) | (Black Mage) | CHAOS_TARGETS |
| Emit CHAOS events | Monad spec | §6.1 | pkg/ebpf/anamnesis | ANAMNESIS |

### Flow Tracker (Bidirectional)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| 5-tuple flow tracking | RFC 8200 | §3 (flow def) | (custom) | FLOWS |
| TCP state machine | RFC 9293 | §3.3 | (custom) | FLOWS |
| Trace ID correlation | Monad spec | §6 | pkg/ebpf/anamnesis | TRACE_ASSOC |
| ✅ Flow migration token check | RFC 9000 | §9 | pkg/protocol/migration | unhd_migration_tokens |
| ✅ Per-flow cancellation check | RFC 9114 | §4.1.1 | pkg/protocol/lifecycle | unhd_cancel_flows |

### Monad CPU (Doom-over-IPv6)

| Behavior | RFC | Section | Go Package | BPF Map |
|----------|-----|---------|------------|---------|
| MBC instruction execution | RFC 9669 | §3 (ISA) | (custom) | ROM_MAP, CPU_MAP |
| L1 cache management | — | — | (custom) | L1_CACHE |
| RAM read/write | — | — | (custom) | RAM_MAP |
| Framebuffer I/O | — | — | (custom) | SCREEN_MAP |
| Keyboard input | — | — | (custom) | KBD_MAP |
| Compute events | Monad spec | §12 | pkg/ebpf/anamnesis | COMPUTE_EVENTS |

---

## 2. Existing BPF Maps → Standardized bpfschema Migration Path

### Maps Already Covered by bpfschema

These maps have corresponding struct definitions in `pkg/protocol/bpfschema/`:

| Existing Map | bpfschema Struct | Status | Action |
|-------------|------------------|--------|--------|
| (new) unhd_hmac_keys | HMACKeyKey/Value | Scaffolded | Wire to Shield XDP |
| (new) unhd_seq_counters | SeqCounterKey/Value | Scaffolded | Wire to Hop XDP |
| (new) unhd_ring_path | RingPathKey/Value | Scaffolded | Wire to Shield XDP |
| (new) unhd_migration_tokens | FlowMigrationTokenValue* | ✅ Wired | Flow Tracker (IPv4-based) |
| (new) unhd_retry_tokens | MigrationTokenKey/Value | Scaffolded | Wire to Shield XDP (IPv6) |
| (new) unhd_sophia_sync | SophiaSyncKey/Value | Scaffolded | Wire to Hop XDP |
| (new) unhd_error_counters | ErrorCounterKey/Value | Scaffolded | Wire to Shield TC |
| (new) unhd_tlv_registry | TLVHandlerEntry | Scaffolded | Wire to Shield XDP |
| (new) unhd_settings | SettingsKey/Value | Scaffolded | Wire to Hop XDP |
| (new) unhd_dos_state | DoSStateKey/Value | Scaffolded | Wire to Hop XDP |
| (new) unhd_flow_types | FlowTypeEntry | Scaffolded | Wire to Hop XDP |
| (new) unhd_goaway_state | GoawayStateKey/Value | Scaffolded | Wire to Shield TC |
| (new) unhd_cancel_flows | FlowCancelValue* | ✅ Wired | Flow Tracker (IPv4-based) |
| (new) unhd_prefetch_hints | PrefetchHintKey/Value | Scaffolded | Wire to Hop XDP |
| (new) unhd_hop_validators | HopValidatorKey/Value | Scaffolded | Wire to Shield XDP |
| (new) unhd_authority | AuthorityKey/Value | Scaffolded | Wire to Shield TC |

### Maps Needing bpfschema Definitions (Existing eBPF Maps)

| Existing Map | Program | Key/Value | bpfschema Action |
|-------------|---------|-----------|------------------|
| ANAMNESIS | All | Ring buf (32B events) | Add AnamnesisEvent struct |
| BLOCKLIST | Shield | u64 → u8 | Add BlocklistKey/Value |
| RATE_TOKENS | Shield | u64 → u32 | Add RateTokenKey/Value |
| STATS | All | u32 → u64 | Add StatsEntry |
| SOPHIA | Hop | u16 → SophiaEntry(32B) | Add SophiaMapKey/Value |
| CIRCUIT_ERRORS | Hop | u16 → u32 | Add CircuitErrorKey/Value |
| CONFIG | Multiple | u32 → u64 | Add ConfigEntry |
| FLOWS | Flow Tracker | FlowKey → FlowState | Add FlowKey/FlowState |
| TRACE_ASSOC | Flow Tracker | u32 → TraceId | Add TraceAssocKey/Value |
| CHAOS_TARGETS | Yaldabaoth | u32 → u64 | Add ChaosTargetKey/Value |
| ROM_MAP | Monad CPU | u32 → u32 | Add ROMEntry |
| RAM_MAP | Monad CPU | u32 → u32 | Add RAMEntry |
| CPU_MAP | Monad CPU | u32 → MbcCpuState(80B) | Add CpuStateKey/Value |
| L1_CACHE | Monad CPU | u32 → [u8;64] | Add CacheLineKey/Value |

---

## 3. IPv6 Extension Header Parsing → RFC Compliance Checklist

Per RFC 8200 §4, the following extension header processing rules MUST be implemented:

| Rule | RFC 8200 Section | Shield Status | Hop Status |
|------|-----------------|--------------|------------|
| Process HBH options at every hop | §4.2 | ✅ strip_extension_headers | ✅ find_monad_option |
| Recognize option types 0 (Pad1) and 1 (PadN) | §4.2 | ✅ | ✅ |
| Act on unknown option type per high-order 2 bits | §4.2 | ✅ (act=00 skip) | ✅ |
| Support option data change-en-route (bit 5) | §4.2 | ✅ (chg=1 for Monad) | ✅ |
| Chain Next Header values correctly | §4 | ✅ | ✅ |
| Bounded iteration for verifier safety | RFC 9669 §3 | ✅ (MAX=8) | ✅ (MAX=16) |
| Destination Options: IMPLEMENTED ✅ | §4.6 | ✅ process_destination_options | — |
| Routing Header: DEFERRED (see ADR-013) | §4.4 | ⏸️ skip (no validation) | ⏸️ skip |
| Fragment Header: DEFERRED (see ADR-014) | §4.5 | ⏸️ drop on fragment | ⏸️ drop on fragment |

---

## 4. RFC Reuse Opportunities

### RFCs Currently Referenced in eBPF Code

| RFC | Where | How |
|-----|-------|-----|
| RFC 8200 | monad-common, shield, hop | IPv6 header parsing, HBH option format |
| RFC 9673 | monad-common | Option Type encoding bits |
| RFC 1071 | encoding package | CRC checksum patterns |
| RFC 9669 | All BPF programs | BPF ISA conformance, map types |

### RFCs FROM Protocol Packages That SHOULD Be Referenced in eBPF Code

| RFC | Pattern | Where to Add | What It Enables |
|-----|---------|-------------|-----------------|
| RFC 9000 §8.1 | Q1 | Shield XDP | HMAC validation on ingress |
| RFC 9000 §5.1.1 | Q2 | Hop XDP | Sequence counter per namespace |
| RFC 9000 §21.3 | Q4 | Shield XDP | Standardized 3× amplification ratio |
| RFC 9000 §9 | Q5 | Flow Tracker | Flow migration token validation |
| RFC 9000 §8.1.2 | Q6 | Shield XDP | Retry token on first packet |
| RFC 9114 §8 | H2 | Shield TC | Error code emission |
| RFC 9114 §7.2.4 | H4 | Hop XDP | Settings-based behavior |
| RFC 9114 §6 | H6 | Hop XDP | Flow type dispatch |
| RFC 9114 §7.2.6 | H7 | Shield TC | GOAWAY monotonicity |
| RFC 9114 §4.1.1 | H8 | Flow Tracker | Per-flow cancellation |
| RFC 9114 §10 | H5 | Hop XDP | DoS backpressure signals |

### New BPF Map Lookups Required (Per Program)

**Shield XDP (add 3 map lookups):**
```
unhd_hmac_keys    → Validate HMAC before admitting packet
unhd_retry_tokens → Check retry token on initial flows
unhd_hop_validators → Validate Monad fields per hop policy
```

**Shield TC (add 3 map lookups):**
```
unhd_goaway_state    → Enforce GOAWAY monotonicity on egress
unhd_error_counters  → Increment error counters
unhd_authority       → Verify dictionary authority
```

**Hop XDP (add 4 map lookups):**
```
unhd_seq_counters → Update namespace sequence
unhd_settings     → Check capability negotiation
unhd_dos_state    → Report backpressure level
unhd_flow_types   → Dispatch by flow type
```

**Flow Tracker (add 2 map lookups):**
```
unhd_migration_tokens → Validate migration
unhd_cancel_flows     → Check cancellation state
```

---

## 5. Rust Struct → Go Struct Parity Table

These are the structs that need IDENTICAL memory layouts in both Rust (eBPF) and Go (userspace):

### Phase 3 Verification Status

| Rust Struct (source) | Go Struct (bpfschema) | Size | Verified | Test File |
|-----|-----------|------|----------|-----------|
| Monad (monad-common:301) | MonadRegister | 20B | ✅ | parity_test.go |
| AnamnesisEvent (monad-common:657) | AnamnesisEvent | 32B | ✅ | parity_test.go |
| FlowKey (common:61) | FlowKey | **16B** | ✅ | parity_test.go |
| FlowState (common:106) | FlowState | **56B** | ✅ | parity_test.go |
| MbcCpuState (monad-common:913) | MbcCpuState | 80B | ✅ | parity_test.go |
| SophiaEntry (monad-common) | SophiaMapValue | 32B | ⏳ | (pending) |

### Breaking Changes (Fixed in Phase 3)

**FlowKey**: Rust uses **IPv4** (u32 src_addr, u32 dst_addr), **NOT IPv6**.
- Old Go definition: [16]byte src/dst (40 bytes total) — **WRONG**
- New Go definition: uint32 src/dst (16 bytes total) — **CORRECT**
- Impact: Flow Tracker TC program uses IPv4 5-tuple tracking.

**FlowState**: Added 16-byte TraceID field for distributed trace correlation.
- Old Go definition: Missing TraceID — **INCOMPLETE**
- New Go definition: TraceID [16]byte at offset 0x00 — **CORRECT**
- Impact: Trace context propagation now tracked per-flow.

**MbcCpuState**: Added halted, stalled, insn_count, cache_hits, cache_misses fields.
- Old Go definition: Missing all cache/halt state — **INCOMPLETE**
- New Go definition: Full 80-byte layout with cache statistics — **CORRECT**
- Impact: Doom-over-IPv6 PoC CPU state persistence now works.

---

## 6. Implementation Priority

### Phase 1: Core Map Integration (Shield + Hop)
1. Add `MonadRegister` and `AnamnesisEvent` to bpfschema
2. Add `BlocklistKey`, `RateTokenKey`, `StatsEntry`, `ConfigEntry`
3. Add `SophiaMapKey/Value`, `CircuitErrorKey/Value`
4. Create `pkg/ebpf/maploader/` that uses bpfschema structs to populate maps

### Phase 2: Protocol Map Wiring (New Maps → eBPF Programs)
5. Define Rust-side structs matching bpfschema for Q1/Q2/Q4/Q5/Q6
6. Add map definitions to shield-ebpf (HMAC, retry, hop validators)
7. Add map definitions to hop-ebpf (seq counters, settings, DoS, flow types)
8. Add map definitions to flow-tracker (migration, cancel flows)

### Phase 3: Full RFC Compliance
9. Add RFC comments to ALL eBPF map definitions
10. Update monad-common with bpfschema struct parity tests
11. Run Lich campaigns against new map interactions

---

## 7. IPv4-Based Flow Tracker Extensions (Phase 2)

The Flow Tracker eBPF program operates on IPv4 5-tuple flows, not IPv6.
Therefore, migration and cancellation state must use IPv4-based key/value pairs:

| Map | Key | Value | Location |
|-----|-----|-------|----------|
| unhd_migration_tokens | FlowKey (IPv4, 16B) | FlowMigrationTokenValue (48B) | pkg/protocol/bpfschema/core_maps.go |
| unhd_cancel_flows | FlowKey (IPv4, 16B) | FlowCancelValue (24B) | pkg/protocol/bpfschema/core_maps.go |

**Key Difference**: The IPv6-based `MigrationTokenKey` (bpfschema.go) uses FlowID+SrcAddr[16].
The IPv4-based `FlowKey` (core_maps.go) uses SrcAddr[4]+DstAddr[4]+SrcPort+DstPort+Protocol.

This separation allows Q5/Q6 (Shield XDP/QUIC) and Flow Tracker patterns to coexist
without conflicts or name collisions in the BPF map namespace.
