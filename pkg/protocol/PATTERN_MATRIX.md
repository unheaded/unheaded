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

| ID  | RFC Source | Section | Go Package | BPF Map Name (Rust) | Map Type | Status | Description |
|-----|-----------|---------|------------|--------------|----------|--------|-------------|
| Q1  | RFC 9000  | §8.1    | integrity  | `HMAC_KEYS` | HASH | Pending | Per-flow HMAC-SHA256 keys for replay protection |
| Q2  | RFC 9000  | §5.1.1  | sequence   | `SEQ_COUNTERS` | PERCPU_HASH | ✅ WIRED | Per-namespace monotonic sequence counters |
| Q4  | RFC 9000  | §21.3   | amplification | (no Rust map) | HASH | Pending | Ring path counters, 3× amplification limit |
| Q5  | RFC 9000  | §9      | migration  | `MIGRATION_TOKENS` | LRU_HASH | ✅ WIRED (Flow Tracker) | Flow migration validation tokens |
| Q6  | RFC 9000  | §8.1.2  | migration  | `RETRY_TOKENS` | LRU_HASH | Pending | Shield retry tokens (HMAC-signed) |
| Q8  | RFC 9000  | §10.2   | lifecycle  | `GOAWAY_STATE` | HASH | Pending | Stateless reset / GOAWAY tracking |
| Q9  | RFC 9000  | §16     | encoding   | (shared) | — | — | Variable-length integer encoding |
| H1  | RFC 9114  | §4.1    | sophiasync | (no Rust map) | HASH | Pending | Encoder/decoder ACK for dictionary sync |
| H2  | RFC 9114  | §8      | errors     | `ERROR_COUNTERS` | PERCPU_ARRAY | Pending | Error code counters per type |
| H3  | RFC 9114  | §9      | tlv        | (no Rust map) | ARRAY | Pending | TLV type → handler dispatch |
| H4  | RFC 9114  | §7.2.4  | settings   | `SETTINGS` | HASH | ✅ WIRED | Per-connection capability negotiation |
| H5  | RFC 9114  | §10     | dos        | `DOS_STATE` | PERCPU_HASH | ✅ WIRED | Drop rate / backpressure tracking |
| H6  | RFC 9114  | §6      | flowtype   | `FLOW_TYPES` | ARRAY | ✅ WIRED | Flow type classification (control/data/prefetch) |
| H7  | RFC 9114  | §7.2.6  | lifecycle  | `GOAWAY_STATE` | HASH | Pending | GOAWAY monotonicity enforcement |
| H8  | RFC 9114  | §4.1.1  | lifecycle  | `CANCEL_FLOWS` | LRU_HASH | ✅ WIRED (Flow Tracker) | Per-flow cancellation state |
| H9  | RFC 7240  | §2      | prefetch   | (no Rust map) | LRU_HASH | Pending | Explicit prefetch hint tracking |
| H10 | RFC 8200  | §4      | intermediary | `HOP_VALIDATORS` | HASH | Pending | Per-hop packet validation |
| H11 | RFC 8200  | §4.2    | intermediary | `AUTHORITY` | HASH | Pending | Dictionary authority enforcement |
| H12 | RFC 9114  | §7.2.4  | settings   | `SETTINGS` | HASH | ✅ WIRED | Sophia compression negotiation |
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

Centralizes the Go mirrors of the BPF map key/value structs. The Rust eBPF
source is the source of truth: `rust_source_test.go` derives sizes, field
offsets, map names and map key/value sizes from `ebpf/*/src` at test time
(S-TRUE-WIRE Phase 3), so this file no longer lists sizes. Map names are the
names Aya gives the maps (the `#[map]` static identifier); the earlier
lowercase `unhd_*` names never matched a loaded map. Rows marked "(no Rust
map)" are design placeholders.

---

## BPF Program Integration Points

### Shield (XDP Ingress)
Consumes: `HMAC_KEYS`, (no Rust map), `RETRY_TOKENS`, `HOP_VALIDATORS`
Pattern: Validate → Authenticate → Admit → Route

### Shield (TC Egress)
Consumes: `GOAWAY_STATE`, `ERROR_COUNTERS`, `AUTHORITY`
Pattern: Enforce → Strip → Emit → Log

### Shim (Per-Hop TC)
Consumes: `SEQ_COUNTERS`, `FLOW_TYPES`, `SETTINGS`, `DOS_STATE`
Pattern: Read Monad → Apply Policy → Update Counters → Forward

### Wotan (Memory Helpers)
Consumes: (no Rust map), (no Rust map), `CANCEL_FLOWS`
Pattern: Cache Read → Validate Version → Apply Delta → Write Back
