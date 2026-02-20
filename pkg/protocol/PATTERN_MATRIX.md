# Unheaded Protocol Pattern Matrix

## RFC Cross-Pollination → Go Package → BPF Map Standardization

This document is the canonical mapping between RFC patterns (QUIC §9000, HTTP/3 §9114,
BPF ISA §9669), the Go protocol packages that implement them, and the BPF map schemas
that expose them to the eBPF data plane.

### Design Principles

1. **One Pattern, One Package, One Map** — Each RFC pattern maps to exactly one Go
   package and one BPF map schema. No duplication, no ambiguity.
2. **Shared Wire Encoding** — All packages use `pkg/protocol/encoding` for varint,
   exponent, CRC, and TLV encoding. No package reimplements encoding.
3. **BPF Map Contract** — Every package defines its BPF map key/value structs in a
   `bpf.go` file using `encoding/binary` compatible layouts. Shield/Shim programs
   consume these directly.
4. **Registry Pattern** — Packages with extensible code points (errors, flowtype,
   settings, tlv) use a shared registry interface from `pkg/protocol/registry`.
5. **RFC Traceability** — Every exported type and function has a doc comment citing
   the specific RFC section it implements.

---

## Pattern Matrix

| ID  | RFC Source | Section | Go Package | BPF Map Name | Map Type | Status | Description |
|-----|-----------|---------|------------|--------------|----------|--------|-------------|
| Q1  | RFC 9000  | §8.1    | integrity  | `unhd_hmac_keys` | HASH | Pending | Per-flow HMAC-SHA256 keys for replay protection |
| Q2  | RFC 9000  | §5.1.1  | sequence   | `unhd_seq_counters` | PERCPU_HASH | ✅ WIRED | Per-namespace monotonic sequence counters |
| Q4  | RFC 9000  | §21.3   | amplification | `unhd_ring_path` | HASH | Pending | Ring path counters, 3× amplification limit |
| Q5  | RFC 9000  | §9      | migration  | `unhd_migration_tokens` | LRU_HASH | ✅ WIRED (Flow Tracker) | Flow migration validation tokens |
| Q6  | RFC 9000  | §8.1.2  | migration  | `unhd_retry_tokens` | LRU_HASH | Pending | Shield retry tokens (HMAC-signed) |
| Q8  | RFC 9000  | §10.2   | lifecycle  | `unhd_goaway_state` | HASH | Pending | Stateless reset / GOAWAY tracking |
| Q9  | RFC 9000  | §16     | encoding   | (shared) | — | — | Variable-length integer encoding |
| H1  | RFC 9114  | §4.1    | sophiasync | `unhd_sophia_sync` | HASH | Pending | Encoder/decoder ACK for dictionary sync |
| H2  | RFC 9114  | §8      | errors     | `unhd_error_counters` | PERCPU_ARRAY | Pending | Error code counters per type |
| H3  | RFC 9114  | §9      | tlv        | `unhd_tlv_registry` | ARRAY | Pending | TLV type → handler dispatch |
| H4  | RFC 9114  | §7.2.4  | settings   | `unhd_settings` | HASH | ✅ WIRED | Per-connection capability negotiation |
| H5  | RFC 9114  | §10     | dos        | `unhd_dos_state` | PERCPU_HASH | ✅ WIRED | Drop rate / backpressure tracking |
| H6  | RFC 9114  | §6      | flowtype   | `unhd_flow_types` | ARRAY | ✅ WIRED | Flow type classification (control/data/prefetch) |
| H7  | RFC 9114  | §7.2.6  | lifecycle  | `unhd_goaway_state` | HASH | Pending | GOAWAY monotonicity enforcement |
| H8  | RFC 9114  | §4.1.1  | lifecycle  | `unhd_cancel_flows` | LRU_HASH | ✅ WIRED (Flow Tracker) | Per-flow cancellation state |
| H9  | RFC 7240  | §2      | prefetch   | `unhd_prefetch_hints` | LRU_HASH | Pending | Explicit prefetch hint tracking |
| H10 | RFC 8200  | §4      | intermediary | `unhd_hop_validators` | HASH | Pending | Per-hop packet validation |
| H11 | RFC 8200  | §4.2    | intermediary | `unhd_authority` | HASH | Pending | Dictionary authority enforcement |
| H12 | RFC 9114  | §7.2.4  | settings   | `unhd_settings` | HASH | ✅ WIRED | Sophia compression negotiation |
| B1  | RFC 9669  | §3      | encoding   | (ISA) | — | — | BPF instruction encoding baseline |
| B2  | RFC 9669  | §5.4    | (all maps) | (schema) | — | — | BPF map type selection per pattern |
| B3  | RFC 9669  | §7      | registry   | (IANA) | — | — | Conformance group registration model |

---

## Shared Infrastructure (New Packages)

### `pkg/protocol/encoding` — Wire Encoding Primitives

Consolidates all encoding/decoding used across protocol packages:

| Function | RFC Source | Used By |
|----------|-----------|---------|
| `EncodeVarint(v uint64) []byte` | RFC 9000 §16 | settings, tlv, lifecycle |
| `DecodeVarint(b []byte) (uint64, int)` | RFC 9000 §16 | settings, tlv, lifecycle |
| `EncodeExponent(v uint32) uint16` | Monad spec | amplification, flowtype |
| `DecodeExponent(e uint16) uint32` | Monad spec | amplification, flowtype |
| `CRC16CCITT(data []byte) uint16` | RFC 1071 / Monad | integrity, intermediary |
| `CRC32MPEG2(data []byte) uint32` | Monad spec | integrity, intermediary |
| `EncodeTLV(t, l uint8, v []byte) []byte` | RFC 9114 §7 | tlv, settings |
| `DecodeTLV(b []byte) (Type, Length, Value)` | RFC 9114 §7 | tlv, settings |

### `pkg/protocol/registry` — Shared Registry Interface

Unifies the registration pattern used by errors, flowtype, settings, tlv:

```go
// Registry is the shared interface for extensible code point registries.
// Implements the IANA "Specification Required" pattern from RFC 8126 §4.6.
type Registry[K comparable, V any] interface {
    Lookup(key K) (V, bool)
    Register(key K, value V) error
    Range(fn func(K, V) bool)
    Len() int
}
```

### `pkg/protocol/bpfschema` — BPF Map Schema Definitions

Centralizes BPF map key/value struct definitions for all patterns:

| Struct | Size | Map | Package |
|--------|------|-----|---------|
| `SeqCounterKey` | 8B | unhd_seq_counters | sequence |
| `SeqCounterValue` | 4B | unhd_seq_counters | sequence |
| `RingPathKey` | 16B | unhd_ring_path | amplification |
| `RingPathValue` | 4B | unhd_ring_path | amplification |
| `HMACKeyEntry` | 40B | unhd_hmac_keys | integrity |
| `ErrorCounter` | 8B | unhd_error_counters | errors |
| `FlowTypeEntry` | 2B | unhd_flow_types | flowtype |
| `SettingsEntry` | 16B | unhd_settings | settings |
| `GoawayState` | 16B | unhd_goaway_state | lifecycle |
| `MigrationToken` | 48B | unhd_migration_tokens | migration |
| `PrefetchHintEntry` | 24B | unhd_prefetch_hints | prefetch |
| `HopValidator` | 32B | unhd_hop_validators | intermediary |
| `DoSState` | 16B | unhd_dos_state | dos |
| `SophiaSyncState` | 24B | unhd_sophia_sync | sophiasync |
| `TLVHandler` | 8B | unhd_tlv_registry | tlv |

All structs use `encoding/binary` compatible layouts (no padding, explicit endianness).

---

## BPF Program Integration Points

### Shield (XDP Ingress)
Consumes: `unhd_hmac_keys`, `unhd_ring_path`, `unhd_retry_tokens`, `unhd_hop_validators`
Pattern: Validate → Authenticate → Admit → Route

### Shield (TC Egress)
Consumes: `unhd_goaway_state`, `unhd_error_counters`, `unhd_authority`
Pattern: Enforce → Strip → Emit → Log

### Shim (Per-Hop TC)
Consumes: `unhd_seq_counters`, `unhd_flow_types`, `unhd_settings`, `unhd_dos_state`
Pattern: Read Monad → Apply Policy → Update Counters → Forward

### Wotan (Memory Helpers)
Consumes: `unhd_sophia_sync`, `unhd_prefetch_hints`, `unhd_cancel_flows`
Pattern: Cache Read → Validate Version → Apply Delta → Write Back
