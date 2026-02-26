# RFC Alignment Findings (S45 Phase 2)

**Date:** 2026-02-25
**Auditor:** Claude Opus 4.6 (automated cross-reference)
**Scope:** All 3 Internet-Drafts vs Go/Rust implementation

---

## 1. Protocol Foundation (draft-bellis-unheaded-protocol-foundation-04.md)

### Verified Sections

- [x] §5.1: IPv6 Hop-by-Hop Extension Header — Rust has HopByHopHeader (24B), Go missing wrapper
- [x] §5.2: Monad Register File (20 bytes) — **MATCH** across Go/Rust/RFC
- [x] §5.3: Flags Bitfield — bits 7-2 MATCH; **bits 1-0 MISMATCH** (Go uses Kingdom Mode, RFC says CUSTOM/RSVD)
- [x] §5.4: CRC-16/CCITT checksum — algorithm MATCH; coverage PARTIAL (18B vs 20B, functionally equivalent)
- [x] §6: Exponent Encoding — on-wire format MATCH (raw u8); Go utility uses u16 extension
- [x] §8: Shield Processing — blocklist/rate-limit maps MATCH; event types MATCH between Go/Rust
- [x] §9: Per-Hop Processing — CRC verify MATCH; circuit states MATCH
- [x] §10: Anamnesis Events — **MISMATCH** (code=32B, RFC=64B; event codes renumbered)
- [x] §14: Chaos Injection — **MISMATCH** (5 modes vs RFC's 6; codes 0x02-0x05 have different names)
- [x] §15: MBC Virtual Machine — MbcCpuState **MATCH** (104 bytes, all field offsets verified)

### Critical Discrepancies (Severity 1-2)

| # | RFC Section | Issue | Severity |
|---|------------|-------|----------|
| 1 | §5.2 | `hop_count` semantics: RFC=decrement/TTL, code=increment/counter | 2 |
| 2 | §5.3 | Flag bits 0-1: Go uses FlagK0/K1 for Kingdom Mode; RFC reserves bit 0 (MUST be zero), bit 1=CUSTOM | 2 |
| 3 | §10.1 | AnamnesisEvent: code=32B, RFC=64B (missing output_monad, wotan_addr/value) | 1 |
| 4 | §10.2 | Event type codes: code uses compact 0x01-0x05, RFC uses sparse 0x00-0x08 | 2 |
| 5 | N/A | FlowCancelValue: Go=24B (aligned), Rust=20B (packed) — BPF map r/w will corrupt | 1 |
| 6 | §14 | Chaos modes: code 0x02=Delay vs RFC 0x02=VALUE_MUTATE (semantic mismatch) | 3 |

### Not Implemented (Deferred)

- §12: Kingdom Mode address reclamation (explicitly deferred in RFC)
- §13: Post-Quantum Cryptographic Identity Binding (PQC)
- §5.2: Extended Register Option / inverse mask (companion draft)
- §11: Wotan BPF helpers (bpf_wotan_read/write)

### Action Items

1. **DOCUMENT** hop_count direction mismatch — defer fix to S48+ (requires BPF changes)
2. **DOCUMENT** flag bits 0-1 divergence — Go Kingdom Mode is experimental, not RFC-compliant
3. **DOCUMENT** AnamnesisEvent 32B vs 64B — current code is a simplified subset
4. **DOCUMENT** event type code numbering — both Go/Rust are internally consistent
5. **FIX** FlowCancelValue parity — add serialization adapter in pkg/protocol/bpfschema

---

## 2. Sophia Dictionary (draft-bellis-unheaded-sophia-dictionary-01.md)

### Verified Sections

- [x] §4: CBOR Serialization — **NOT IMPLEMENTED** (no CBOR library in codebase)
- [x] §5.1: Root Map (sophia_root) — **NOT IMPLEMENTED**
- [x] §5.2: Sub-Dictionary Maps — PARTIAL (flat HashMap vs RFC's ARRAY_OF_MAPS)
- [x] §6: Dictionary Distribution — PARTIAL (sophiasync has ACK scaffolding, no BPF plumbing)
- [x] §7: Minimum Required Dictionary — dict IDs **MISMATCH** (0x01=qos vs RFC 0x01=service_identity)
- [x] §8: Version Negotiation — PARTIAL (uint64 vs RFC uint8, no modular arithmetic)
- [x] §9: Security — **MISSING** (no ML-DSA-65, non-constant-time comparison)

### Critical Discrepancies

| # | RFC Section | Issue | Severity |
|---|------------|-------|----------|
| 1 | §4 | No CBOR wire format — entire Section 4 unimplemented | Critical |
| 2 | §5 | Flat HashMap vs two-level ARRAY_OF_MAPS — no atomic dict replacement | Critical |
| 3 | §5.2 | SophiaEntry: code=32B, RFC BPF struct=80B (missing fingerprint, PQC fields) | Major |
| 4 | §7 | Dict ID numbering: RFC 0x01=service_identity, code 0x01=qos_class | Major |
| 5 | §7.2 | Flow action values off-by-one from RFC (Forward=0x01 vs RFC 0x00) | Major |
| 6 | §9 | Non-constant-time byte comparison; HMAC instead of ML-DSA-65 | Major |
| 7 | §6 | Missing entry policy inverted (code silently passes vs RFC DROP+ANOMALY) | Significant |

### Correctly Implemented

- Grace period default (60 seconds) — matches RFC §8.3
- Grace period cleanup logic — correct
- ACK tracking scaffolding (Patch S6) — present
- Sophia key encoding concept (dict_id<<8|value) — correct
- SophiaEntry size enforcement (32 bytes, compile-time assert) — correct

### Action Items

1. **DOCUMENT** CBOR as future work — not needed for alpha
2. **DOCUMENT** flat map architecture decision — atomic replacement deferred
3. **FIX** dict ID numbering comment discrepancy in code
4. **FIX** `bytesEqual` → `crypto/subtle.ConstantTimeCompare`
5. **DEFER** PQC, ML-DSA-65, 80-byte entries to post-alpha

---

## 3. Wotan Memory (draft-bellis-unheaded-wotan-memory-01.md)

### Verified Sections

- [x] §3.2: Memory Hierarchy (L0-L4) — conceptually MATCH, L0 size comment wrong
- [x] §5.1: L1 Cache Line — **DIVERGENCE** (metadata in Go-side, not BPF map)
- [x] §5.3: Cache Miss Events — struct fields differ (no len/access_type, adds hop_id/timestamp)
- [x] §6: L2 Ring Buffer — **MAJOR** (flat []byte vs RFC's 80-byte structured entries)
- [x] §7: L3 WAL — **CRITICAL** (entirely unimplemented, stubs only)
- [x] §8: Topic I/O — generic pub/sub, no MMIO routing
- [x] §9: Miss Handler — D006 path uses 64B lines (RFC-correct); legacy path uses 4096B pages

### Critical Discrepancies

| # | RFC Section | Issue | Severity |
|---|------------|-------|----------|
| 1 | §5.1 | L1 cache line metadata (tag/valid/dirty/lru) in Go heap, not BPF map | Significant |
| 2 | §5.1 | No per-flow BPF map key (flat word-addr only) | Significant |
| 3 | §6.1 | L2 is flat []byte, not 80-byte structured ring entries | Major |
| 4 | §7 | L3 WAL entirely unimplemented (stub/TODO) | Critical |
| 5 | §7.2 | WAL recovery on restart — absent | Critical |
| 6 | §7.3 | WAL compaction — absent | Critical |
| 7 | §4 | MMIO address map (0xC000-0xFFFF) not routed | Major |
| 8 | §4.6 | No per-flow ring buffer allocation (BLAKE3 composite key) | Major |
| 9 | Patch W6 | No per-program cache-miss rate limiting | Significant |
| 10 | Patch W7/W8 | SETTINGS/GOAWAY frames absent | Significant |

### Correctly Implemented

- Cache line size: 64B default, configurable — **MATCH**
- Prefetch on cache miss — **MATCH** (D006 path)
- LRU eviction on L1 full — **MATCH** (different mechanism, same behavior)
- Dirty write-back before eviction — **MATCH**
- Write widths (1/2/4 bytes) — **MATCH**
- Little-endian byte order — **MATCH**
- Topic pub/sub model — **MATCH** (generic)
- Non-blocking publish (drop on full) — **MATCH**

### Action Items

1. **DOCUMENT** L3 WAL as post-alpha critical gap
2. **DOCUMENT** L2 flat memory as deliberate simplification for alpha
3. **DOCUMENT** MMIO as blocked on BPF integration
4. **FIX** L0 size comment (4 bytes → 20 bytes)
5. **DEFER** per-flow ring buffers, WAL, MMIO to bare-metal sprints

---

## Summary

| RFC | Sections Verified | Critical Issues | Major Issues | Match/Partial |
|-----|------------------|-----------------|--------------|---------------|
| Protocol Foundation | 12/18 | 2 | 4 | 8 MATCH, 4 PARTIAL |
| Sophia Dictionary | 9/9 | 2 | 4 | 3 MATCH, 6 PARTIAL/MISSING |
| Wotan Memory | 10/12 | 3 | 3 | 6 MATCH, 7 DIVERGENT |

### Resolution Strategy

**For Alpha (S45):** Document all discrepancies. Fix only non-breaking items (comments, constant-time compare).
**For Beta:** Align event type codes, fix hop_count direction, implement CBOR.
**For Production:** L3 WAL, per-flow isolation, PQC, MMIO routing, atomic dict replacement.

---

*Generated by S45 Phase 2 automation, 2026-02-25*
