# Alignment Notes — S72 Draft Updates

**Date**: February 27, 2026
**Status**: Phase 4 Protocol Cross-References
**Scope**: Monad Foundation draft-05, Sophia Dictionary draft-02, Wotan Memory draft-02

---

## Monad Foundation (draft-bellis-unheaded-protocol-foundation-05)

### Overview

The Monad Foundation specification defines the 20-byte protocol metadata register, IPv6 HbH extension header integration, TLV container format, and Anamnesis ring buffer event structure.

| Spec Section | Implementation File | Status | Notes |
|---|---|---|---|
| IPv6 Hop-by-Hop Extension Header | ebpf/xdp_ingress.c | Partial | Foundation in place; draft-05 formalizes format |
| Monad Register (20-byte) | pkg/protocol/monad.go, monad-common/src/lib.rs | Complete | All field types, endianness, checksum validated |
| Flags Bitfield (chaos, canary, traced, encrypted) | services/wotan/flags.go | Complete | All 8 flag bits implemented and tested |
| Checksum Field (CRC-16/CCITT) | pkg/protocol/integrity/crc16.go | Complete | Verified on 20-byte header, integrated with Monad |
| Shield Ingress/Egress Processing | ebpf/xdp_ingress.c, ebpf/tc_egress.c | Complete | XDP stamps header, TC strips it; no regressions |
| Sophia Dictionary System | services/sophia/, monad-sophia/src/ | Aligned | Exponent field lookups working; dict distribution via Wotan |
| Anamnesis Ring Buffer Events | pkg/observability/anamnesis.go | Complete | 64-byte event struct, per-CPU buffers, EVE JSON transform |
| BPF Map Implementation | ebpf/maps.c | Complete | admissions_policy, rate_limit_buckets, egress_policy all pinned |
| TLV Extension Container | ebpf/tlv.c | Planned | draft-05 defines format; needs integration test. See roadmap. |
| TLV Type Registry (M6 Extension) | docs/protocol/EXTENSIONS.md | Planned | Registry started, Ring Path Counter (M8) needs eBPF hook. See roadmap. |
| Event Types (0x00-0x05) | pkg/observability/event_types.go | Complete | BIRTH, COMPUTED, DEATH, ANOMALY, CHAOS, ROLLBACK all defined |
| Ring Buffer Configuration | services/wotan/ring_config.go | Complete | Per-CPU sizing, retention, overflow handling |
| BPF Helper Functions | ebpf/helpers.c | Complete | bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas (bounds check enabled) |
| Error Codes (0x00-0x0C) | docs/protocol/error-registry.md | Complete | 13 normative codes, Monad error propagation active |
| Monitoring & Logging | pkg/monitoring/shield.go | Complete | BPF errors, Wotan access errors, chaos events logged |

---

## Sophia Dictionary (draft-bellis-unheaded-sophia-dictionary-02)

### Overview

The Sophia specification defines a hierarchical exponent-based dictionary system for decoding the 8 exponent fields in the Monad register. Draft-02 expands entry types and synchronization protocol.

| Spec Section | Implementation File | Status | Notes |
|---|---|---|---|
| Root Dictionary (256 entries max) | services/sophia/root_map.go | Complete | All 256 slots allocated with collision detection |
| Sub-Dictionary Maps (Array of Maps) | services/sophia/sub_dicts.go | Complete | 256 sub-dictionaries, each 256 entries max |
| Lookup Chain (O(depth) per packet) | services/sophia/lookup.go | Complete | Optimized with L1 cache; <100ns per lookup |
| Dictionary Entry Structure | monad-sophia/src/entry.rs | Complete | Size limits (1 MB per-flow, 100 MB global) enforced |
| CBOR Serialization (Wire Format) | services/sophia/encoding.go | Complete | Round-trip tested for all entry types |
| Per-Flow Dictionary Capacity (1 MB) | services/sophia/limits.go | Complete | Rejection on overflow returns error code 0x09 |
| Global Dictionary Capacity (100 MB) | services/sophia/global_limit.go | Complete | System-wide quota managed, alerts on >80% |
| Root Entry Schema (exponent + sub_dict_id) | services/sophia/root_schema.go | Complete | 8 exponent fields mapped to 8 root entries |
| Sub-Dictionary Entry Schema | services/sophia/sub_schema.go | Complete | All entry types: QoS, service, action, ring, circuit, mesh |
| Exponent Rule Entry Type | services/sophia/rule_entry.go | Complete | Policy rules for flow processing |
| PQC Key Entry Type | services/sophia/pqc_entry.go | Aligned | ML-DSA-65 keys stored, verification in progress |
| Routing Entry Type (NEW in draft-02) | services/sophia/routing_entry.go | Complete | Backend selection, path tracking |
| Firewall Entry Type (NEW in draft-02) | services/sophia/firewall_entry.go | Complete | Allow/deny rules, stateful inspection |
| Observability Entry Type (NEW in draft-02) | services/sophia/obs_entry.go | Complete | Event sampling, metric emission |
| IDS Entry Type (NEW in draft-02) | services/sophia/ids_entry.go | Complete | Signature matching, anomaly detection |
| Health Entry Type (NEW in draft-02) | services/sophia/health_entry.go | Complete | Service health indicators, liveness checks |
| BPF Map Pinning Paths | ebpf/pins/ | Complete | /sys/fs/bpf/sophia/ with versioning |
| Wotan Distribution Channel | services/wotan/distribution.go | Complete | Topic-based sync, atomic updates, version counters |
| Version Negotiation (field versioning) | services/sophia/versioning.go | Complete | Forward/backward compatibility, schema evolution |
| Bulk Synchronization (Crash Recovery) | services/sophia/recovery.go | Complete | Full dictionary snapshot recovery within TTL |
| Conflict Resolution (Last-Writer-Wins) | services/sophia/lww.go | Complete | CRC-32 for collision detection |
| REQUIRED Entries (Root 0x00-0x0F) | services/sophia/required.go | Complete | All standard entries present |
| Standard Service Identity Sub-Dict #1 | services/sophia/subdict_1.go | Complete | captain, timeguru, architect, micromanager, monad, sophia, etc. |
| Standard Flow Action Sub-Dict #2 | services/sophia/subdict_2.go | Complete | forward, trace, sample, mirror, drop actions |
| Standard QoS Class Sub-Dict #3 | services/sophia/subdict_3.go | Complete | realtime, interactive, best-effort, background |

---

## Wotan Memory (draft-bellis-unheaded-wotan-memory-02)

### Overview

The Wotan specification defines the memory hierarchy, ring buffer structure, per-flow state machine, and gRPC topic subscription protocol. Draft-02 expands triple-role isolation and reliability guarantees.

| Spec Section | Implementation File | Status | Notes |
|---|---|---|---|
| L1 Cache (64-byte per-flow) | ebpf/wotan_cache.c | Complete | Composite key addressing, LRU eviction, prefetch hints |
| L2 Ring Buffer (Per-Flow RAM) | ebpf/wotan_ringbuf.c | Complete | Configurable per-flow size (8 KB), WAL format |
| Memory Hierarchy (L1 → L2 → L3) | services/wotan/hierarchy.go | Complete | Miss rate targets <5%, latency SLAs enforced |
| Data Memory Region (0x00000000-0x0000BFFF) | ebpf/memory/data.c | Complete | Per-flow state allocation |
| I/O Memory Region (0x0000C000-0x0000FFFE) | ebpf/memory/io.c | Complete | MMIO write addresses for async offloading |
| Extended Memory (0x00010000-0x00FFFFFF) | ebpf/memory/extended.c | Planned | Large allocations; needs heap management. See roadmap. |
| Cache Line Structure (Composite Key) | ebpf/wotan_key.c | Complete | (flow_label, addr) composite addressing |
| L1 Cache Composite Key (PATCH W2) | ebpf/wotan_key.c | Complete | 64-bit flow_label + 16-bit offset = 80-bit key |
| Cache Line Size Configuration | services/wotan/cache_line.go | Complete | 64-byte lines, configurable via BPF map |
| Prefetch Model (bpf_wotan_prefetch) | ebpf/helpers.c | Planned | Helper function definition needed. See roadmap. |
| Cache Miss Handling | services/wotan/miss_handler.go | Complete | Stall mechanism via MMIO, latency limited |
| Write-Back Policy (Buffered Write) | services/wotan/writeback.go | Complete | TTL-based flush, configured per-flow |
| LRU Eviction | ebpf/wotan_lru.c | Complete | Counter overflow handling, version tagging |
| Ring Buffer Structure (PATCH W1) | ebpf/wotan_ringbuf.c | Complete | Per-flow allocation, metadata (timestamp, addr, valid) |
| Ring Buffer Entry Size | ebpf/wotan_ringbuf.c | Complete | 64-byte entries, cache-line aligned |
| Allocation (per Flow Label) | services/wotan/allocation.go | Complete | On-demand, returned on CLOSED state |
| Sizing (--ring-size configuration) | services/wotan/config.go | Complete | Default 8 KB per ACTIVE, 256 bytes per IDLE |
| Overflow Policy | services/wotan/overflow.go | Complete | Proactive drain + backpressure; capacity planning |
| WAL Format (PATCH W1, W4) | ebpf/wotan_wal.c | Complete | Seqno, timestamp, addr, value, CRC-32 |
| WAL Flush Policy (TTL-based) | services/wotan/wal_flush.go | Complete | Configurable TTL (default 100ms) |
| WAL Recovery on Restart | services/wotan/wal_recovery.go | Complete | Replay from last seqno, gap detection |
| WAL Compaction (PATCH W5) | services/wotan/compaction.go | Aligned | Reduces storage; version counter incremented |
| SETTINGS Exchange (PATCH W7) | services/wotan/settings.go | Aligned | eBPF program configuration handshake |
| GOAWAY Frame (PATCH W8) | services/wotan/goaway.go | Planned | Graceful shutdown signaling for ring buffers. See roadmap. |
| Topic Naming Convention | services/wotan/topics.go | Complete | anamnesis.*, compute.screen.*, compute.input.* |
| Memory-Mapped I/O Addresses | ebpf/memory/mmio.go | Complete | 0x0000C000-0x0000FFFE range for async writes |
| Screen Topic (compute.screen.{flow_label}) | services/wotan/topics/screen.go | Complete | eBPF → userspace computation results |
| Input Topic (compute.input.{flow_label}) | services/wotan/topics/input.go | Planned | Userspace to eBPF computation inputs. See roadmap. |
| Dictionary Topic (sophia.dictionary.v{N}) | services/wotan/topics/dictionary.go | Complete | Sophia dictionary distribution channel |
| Anamnesis Topics (anamnesis.{event_type}) | services/wotan/topics/anamnesis.go | Complete | Ring buffer event streaming |
| Cache Miss Event Structure | pkg/observability/miss_event.go | Complete | Timestamp, flow_label, addr, evicted_value |
| Cache Miss Stall Mechanism | services/wotan/stall.go | Complete | MMIO blocking until userspace responds |
| Cache Miss Rate Limiting (PATCH W6) | services/wotan/rate_limit.go | Complete | Max misses/sec per flow, configurable |
| Program Memory (ROM via Sophia) | ebpf/program_memory.c | Complete | BPF program code stored in Sophia dictionary |
| Data Memory (RAM via Wotan Ring) | ebpf/data_memory.c | Complete | Per-flow heap, 1 MB quota per program |
| Stack (top-of-RAM) | ebpf/stack.c | Complete | Configurable stack depth |
| Heap (configurable region) | ebpf/heap.c | Planned | Needs allocation tracker. See roadmap. |
| Ring Buffer Capacity and Layout | services/wotan/capacity.go | Complete | Head/tail pointers, wrap-around handling |
| Oldest-First Eviction (Overflow) | services/wotan/eviction.go | Complete | LRU order, timestamp-based ordering |
| Lock-Free Read Path | services/wotan/lockfree.go | Complete | BPF ringbuf consumer interface |
| Memory Layout Alignment | services/wotan/alignment.go | Complete | 64-byte alignment for cache line operations |
| Topic Subscription Protocol | services/wotan/subscription.go | Complete | gRPC streams, backpressure handling |
| Stream Lifecycle (subscribe → events → unsubscribe) | services/wotan/stream.go | Complete | Idempotency tokens, reconnection policy |
| Backpressure Handling | services/wotan/backpressure.go | Complete | Bidirectional flow control |
| Reconnection Policy (Exponential Backoff) | services/wotan/reconnect.go | Complete | Max 5 retries, 30-second timeout |
| Triple-Role Isolation (PATCH W9) | services/wotan/triple_role.go | Complete | Role 1 (Ring Buffer), Role 2 (Event Bus), Role 3 (Protocol RAM) separate |
| Role 1: Ring Buffer (Per-Service Log Aggregation) | services/wotan/role1_ringbuf.go | Complete | Service-specific ring buffer, isolated goroutine |
| Role 2: Event Bus (Pub/Sub Message Routing) | services/wotan/role2_eventbus.go | Complete | Topic-based routing, subscriber isolation |
| Role 3: Protocol RAM (Monad State Backing) | services/wotan/role3_protocol_ram.go | Complete | Composite key access, flow-level coherency |
| Role Isolation Guarantees | services/wotan/role_isolation.go | Complete | Separate memory regions, per-role concurrency limits |
| Ring Buffer Overflow Failure Mode | services/wotan/failure_modes.go | Complete | Backpressure → -ENOMEM → Shim stall |
| PublishWithAck (At-Least-Once) | services/wotan/publish.go | Complete | Acknowledgment receipt, retry logic |
| IdempotencyCache (24-Hour TTL) | services/wotan/idempotency.go | Complete | Prevents duplicate event emission |
| OrderedPublisher (Per-Destination FIFO) | services/wotan/ordered_pub.go | Complete | Linearization token tracking |
| Dead Letter Queue (DLQ) | services/wotan/dlq.go | Complete | Unparseable events, 30-day retention |
| L1 Hit Latency Target | docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md | Complete | <100ns (cache hit SLA) |
| L2 Access Latency Target | docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md | Complete | ~1-10µs (ring buffer access) |
| L3 Access Latency Target | docs/protocol/PROTOCOL_TECHNICAL_SUMMARY.md | Complete | ~100µs-1ms (userspace/network) |
| Hit Rate Target (>95%) | services/wotan/monitoring.go | Complete | Tracked per-service, alerted on degradation |

---

## Cross-Protocol Alignment

### Monad ↔ Sophia Integration

- **Exponent Fields**: All 8 exponent fields in Monad (offset 0x01, 0x02, 0x08, 0x09, 0x0A, 0x0E, 0x0F) map to Sophia root dictionary entries (0x01-0x0F)
- **Dictionary Lookups**: Every hop performs O(2) lookups per exponent field
- **Field Updates**: BPF programs atomically update exponent values via Sophia LWW merge
- **Status**: ✓ Complete

### Monad ↔ Wotan Integration

- **Ring Buffer Events**: Each Anamnesis event captures full 20-byte Monad snapshot (input + output)
- **Memory Access**: Wotan helpers (bpf_wotan_read/write) store access metadata in ring buffer
- **Flow State**: Wotan WAL preserves flow transitions (IDLE → ACTIVE → CLOSING → CLOSED)
- **Status**: ✓ Complete

### Sophia ↔ Wotan Integration

- **Distribution Channel**: Sophia dictionaries distributed via `sophia.dictionary.v{N}` topic
- **Version Tracking**: Wotan maintains version counter, incremented on atomic updates
- **Synchronization**: Bulk recovery via Wotan, conflict resolution via LWW
- **Status**: ✓ Complete

---

## New Sections in Draft-05

### M1-M8: Monad Foundation Patches

| Patch | Section | Implementation | Status |
|-------|---------|-----------------|--------|
| M1 | CRC-16/CCITT Checksum Extension | pkg/protocol/integrity/ | Complete |
| M2 | Multiple HbH Header Detection | ebpf/parser.c | Complete |
| M4 | Wotan Bounds Checking | ebpf/helpers.c | Complete |
| M5 | Version Field Validation | ebpf/xdp_ingress.c | Complete |
| M6 | TLV Extension Container Format | ebpf/tlv.c | Planned |
| M8 | Ring Path Counter Extension | ebpf/extensions.c | Planned |

### S1-S8: Sophia Dictionary Patches

| Patch | Section | Implementation | Status |
|-------|---------|-----------------|--------|
| S1 | BPF Map Types and Schemas | ebpf/maps.c | Complete |
| S2 | Per-Flow Dictionary Limits | services/sophia/limits.go | Complete |
| S5 | Version Monotonicity | services/sophia/versioning.go | Complete |
| S7 | Dictionary Poisoning Mitigation | services/sophia/security.go | Complete |
| S8 | Cross-Reference with Monad/Wotan | services/sophia/integration.go | Aligned |

### W1-W9: Wotan Memory Patches

| Patch | Section | Implementation | Status |
|-------|---------|-----------------|--------|
| W1 | Ring Buffer Structure Formalization | ebpf/wotan_ringbuf.c | Complete |
| W2 | Composite Key Addressing | ebpf/wotan_key.c | Complete |
| W3 | CAS Alignment Enforcement | ebpf/helpers.c | Complete |
| W4 | WAL Tampering Detection | services/wotan/wal_recovery.go | Complete |
| W5 | Compaction Policy | services/wotan/compaction.go | Aligned |
| W6 | Cache Miss Rate Limiting | services/wotan/rate_limit.go | Complete |
| W7 | SETTINGS Exchange Protocol | services/wotan/settings.go | Aligned |
| W8 | GOAWAY Frame Signaling | services/wotan/goaway.go | Planned |
| W9 | Triple-Role Isolation | services/wotan/triple_role.go | Complete |

---

## Next Steps

### Critical (Phase 4 Complete)

1. ✓ OpenAPI 2.0.0 updated with Shield, Anamnesis, Kingdom endpoints
2. ✓ Error registry includes codes from all 3 drafts
3. ✓ ALIGNMENT_NOTES created (this document)

### High Priority (Phase 5)

1. TLV Extension integration (M6, M8)
2. GOAWAY frame implementation (W8)
3. Input Topic completion (compute.input.*)
4. Extended Memory management (Heap)

### Documentation

- All 3 draft versions formally published
- RFC alignment report updated with draft-05, sophia-02, wotan-02
- Technical summary reflects Age 1 → Age 2 evolution

---

**End of Alignment Notes**
