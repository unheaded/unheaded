# Unheaded Protocol Innovations

**Date:** 2026-02-25
**Scope:** Catalogue of key technical innovations in the Unheaded protocol stack
**RFC:** draft-bellis-unheaded-protocol-foundation-04 (primary)

---

## Overview

Unheaded implements a custom IPv6 protocol stack built on Hop-by-Hop extension
headers, eBPF-powered per-hop computation, and a novel state encoding system.
This document catalogues the 10 key innovations that differentiate Unheaded
from conventional networking stacks.

---

## Innovation 1: Inverse Mask (Double Address Space)

**Status:** Core protocol feature
**RFC:** Section 5.2 / RFC 0950 wildcard mask formalism
**Implementation:** `pkg/protocol/monad.go` (Go), `ebpf/monad-common/src/lib.rs` (Rust)
**Documentation:** [docs/protocol/INVERSE_MASK.md](INVERSE_MASK.md)

A single IPv6 address interpreted in two distinct, valid ways within a /48
block. One bit in the Monad register (Inverse Mask Indicator, bit 5 of flags)
selects the interpretation, effectively doubling static address space without
consuming additional IP allocation. Deterministic: NOT(NOT(x)) == x.

---

## Innovation 2: Kingdom Mode (Four-State Flow Machine)

**Status:** Core security mechanism
**RFC:** Section 12 of draft-bellis-unheaded-protocol-foundation-04
**Implementation:** `pkg/protocol/monad.go`, `proto/unheaded/v1/protocol.proto`
**Documentation:** [docs/protocol/KINGDOM_MODE.md](KINGDOM_MODE.md)

A 2-bit (K1|K0) state machine encoding 4 flow states directly in the packet:
IDLE (00), ACTIVE (01), CLOSING (10), CLOSED (11). State lives in the packet,
not in kernel connection tables. Each transition generates an Anamnesis event.
eBPF enforces transitions at wire speed with sub-microsecond latency.

---

## Innovation 3: Monad Register File (20-Byte Packet-Carried State)

**Status:** Core protocol foundation
**RFC:** Section 5.2 — wire format definition
**Implementation:** `pkg/protocol/bpfschema/core_maps.go:48` (Go), `ebpf/monad-common/src/lib.rs:331` (Rust)

A 20-byte register file carried in every packet via IPv6 Hop-by-Hop extension
header. Contains version, service IDs, hop count, QoS class, flow action,
circuit state, flags, latency hint, deployment ring, mesh flags, prefix bytes,
scratch space, and CRC-16 checksum. Every field is accessible at each hop.

---

## Innovation 4: Exponential Encoding (Sophia Dictionary)

**Status:** Core encoding mechanism
**RFC:** Section 6 — exponent encoding rules
**Implementation:** `pkg/protocol/encoding/encoding.go` (Go)
**Documentation:** See [Sophia Dictionaries wiki](../../wiki/Sophia-Dictionaries.md)

Single-byte fields encode exponential values: `decoded = base^exponent * multiplier`.
Sophia dictionary provides base, multiplier, and semantic context per field.
O(1) BPF hash map lookup at ~100-200ns. Hot-swappable dictionaries propagate
in <10ms via Wotan topic distribution. Same byte = different meaning in
different contexts.

---

## Innovation 5: Hop-by-Hop Shim Processing

**Status:** Core architecture
**RFC:** Sections 3-5, 8-9
**Implementation:** `ebpf/hop-ebpf/src/main.rs` (Rust/Aya), `ebpf/shield-ebpf/src/main.rs`

At each hop: extract Monad, execute BPF Shim program, write updated Monad,
record Anamnesis event, forward. Shield boundary adds/strips the header.
Total per-hop overhead: ~2-12us dominated by Shim execution. CRC-16 integrity
check at every hop prevents silent corruption.

---

## Innovation 6: Anamnesis Event Sourcing

**Status:** Core observability
**RFC:** Section 10 — event types and ring buffer specification
**Implementation:** `services/anamnesis/anamnesis.go`, `pkg/ebpf/anamnesis.go`

Non-blocking eBPF ring buffer recording every state change with full packet
snapshots. 9 event types (BORN, COMPUTED, WOTAN_RD/WR, CHAOS, ROLLBACK,
DIED, KEY_OP, ANOMALY). 64-byte fixed event structure with before/after
Monad snapshots. Flow label correlation enables full path reconstruction.

---

## Innovation 7: MBC Instruction Set Architecture

**Status:** Proof of computational completeness
**RFC:** Section 15
**Implementation:** `crates/monad-mbc/src/lib.rs`, `ebpf/monad-common/src/lib.rs:922`
**Documentation:** [MBC ISA Reference](../../wiki/MBC-ISA-Reference.md)

43-opcode instruction set (32-bit fixed encoding) proving Turing completeness
of the protocol. Practical clock speed ~2.7 MHz single-instruction, 11-21 MHz
batched. Validated by running DOOM in the MBC virtual machine. CPU state
struct: 104 bytes (16 registers, PC, flags, halted, stalled, counters).

---

## Innovation 8: Wotan Memory Model (Per-Flow Storage)

**Status:** Core infrastructure
**RFC:** draft-bellis-unheaded-wotan-memory-01 (dedicated RFC)
**Implementation:** `services/wotan/internal/compute/memory.go`

5-level memory hierarchy: L0 (Monad, 20B wire), L1 (BPF hash map, ~200ns),
L2 (ring buffer, ~1-10us), L3 (WAL, ~100us-1ms), L4 (Sophia dictionaries).
Per-flow addressable memory with MMIO regions for screen/input I/O.
Cache miss protocol: BPF signals userspace to stage data into L1.

---

## Innovation 9: Post-Quantum Cryptographic Identity Binding

**Status:** Specified, not yet implemented
**RFC:** Section 13 — ML-KEM-768 + ML-DSA-65
**Implementation:** Not yet (deferred to post-alpha)

Each service_id cryptographically bound to quantum-resistant keypair.
Key lifecycle via flow_action codes 0x10-0x16 (ANNOUNCE, ROTATE, REVOKE).
32-bit SHA3-256 fingerprint truncation for BPF-level verification.
Hybrid mode: CONCATENATE (both PQ+classical), PQC_ONLY, CLASSICAL_ONLY.

---

## Innovation 10: Limited Domain Protocol Design

**Status:** Architectural principle
**RFC:** Section 1, references RFC 8799
**Implementation:** Shield boundary enforcement

Protocol operates within an operator-controlled Limited Domain where all
intermediate nodes support IPv6 Hop-by-Hop processing. Shield enforces
boundary (adds header at ingress, strips at egress). No cross-domain
routing. Clear trust boundary enables aggressive optimization.

---

## Performance Summary

| Operation | Latency | Notes |
|-----------|---------|-------|
| CRC-16 verification | ~50ns | Per-hop integrity check |
| Sophia dictionary lookup | ~100-200ns | O(1) BPF hash map |
| Kingdom Mode transition | <1us | In-packet state change |
| Hop-by-Hop Shim | 2-12us | Full per-hop processing |
| Anamnesis event write | ~100-500ns | Non-blocking ring buffer |
| MBC instruction | ~370ns | Including checksum |
| Wotan L1 cache hit | ~100-200ns | BPF hash map access |
| Wotan L2 access | ~1-10us | Go heap ring buffer |

## Security Summary

- **In-packet state:** No connection table exhaustion attacks
- **Per-hop CRC:** Silent corruption detection at every hop
- **Kingdom Mode:** Prevents state confusion attacks
- **Anamnesis:** Complete audit trail, tamper-evident
- **PQC (future):** Harvest-now-decrypt-later protection
- **Limited Domain:** Clear trust boundary, no cross-domain leakage

## Standards Compliance

- 3 Internet-Drafts documenting the protocol (foundation, dictionary, memory)
- IPv6 Hop-by-Hop extension headers per RFC 8200
- CRC-16/CCITT-FALSE per established standard
- Limited Domain per RFC 8799
- Wildcard mask per RFC 0950
- PQC per FIPS 203 (ML-KEM-768) and FIPS 204 (ML-DSA-65)

---

*Generated by S45 Phase 3 automation, 2026-02-25*
