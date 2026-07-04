# draft-bellis-unheaded-sophia-dictionary-03

**Status:** IETF Experimental · Independent Submission · March 2026

The Sophia Dictionary Format defines the serialization, storage, and distribution mechanism for semantic metadata that accompanies the Unheaded Protocol. Sophia dictionaries are exponent-decoding tables that translate compact byte values (0x00-0xFF) into meaningful human-readable categories (service identifiers, QoS classes, flow actions, etc.) and their associated metadata. This memo specifies the CBOR serialization format, BPF map representation for in-kernel storage, atomic update protocol for cluster-wide distribution via the Wotan memory bus, and minimum required dictionary entries. Draft-03 introduces sub-dictionary type systems for hierarchical knowledge representation and QPACK compression headers for efficient dictionary entry encoding over the wire.

## Key Sections

- **Introduction** — Problem statement (where Monad field semantics come from); Sophia as the semantic layer; cross-references to Foundation and Wotan
- **Terminology** — Root Dictionary, Sub-Dictionary, Nested Sub-Dictionary, Sophia Lookup, Dictionary Version, Atomic Update, Wotan Topic, QPACK
- **Dictionary Model / Tree Structure** — Compositional context: same byte value resolves differently depending on field position; 256^K expressible meanings with K key positions
- **Sub-Dictionary Type System (NEW in draft-03)** — Type codes: LEAF (0x00), BRANCH (0x01), COMPOSITE (0x02), ALIAS (0x03); nested lookup chain; maximum nesting depth (8 levels); circular reference detection (visited bitmask); use cases (service topology, policy hierarchies, geographic routing, tenant isolation); CBOR encoding; BPF map representation (indices 0-63 top-level, 64-191 nested)
- **Root Dictionary** — 256-slot BPF hash map; key ranges (standard 0x01-0x0F, operator 0x10-0xFE, reserved 0xFF); initialization guarantee
- **Dictionary Size Constraints** — Per-flow: 128 entries / 1 MB max; global: 100 MB cap
- **QPACK Compression Headers (NEW in draft-03)** — Static table (24 entries); dynamic table (4096 bytes); encoding format with Compression Flags byte; compression ratios by entry type; decompression limits; backward compatibility with draft-02 raw CBOR
- **Dictionary Distribution** — Wotan topic sophia.dictionary.v{N}; version negotiation (2 concurrent versions minimum); atomic update protocol (9-step sequence; <10ms cluster-wide propagation)
- **Security Considerations** — Dictionary poisoning (ML-DSA-65 signatures, timestamp validation, CRC32, source authentication); nested dictionary security (depth limits, circular reference prevention, namespace partitioning); QPACK decompression security (bomb mitigation, dynamic table poisoning)
- **PQC Key Dictionary Integration (NEW in draft-03)** — PQC_SIG_MAP and PQC_KEY_MAP BPF map definitions; signature lookup procedure; key rotation protocol; algorithm support matrix (SLH-DSA, ML-DSA, FN-DSA, ML-KEM, HQC)
- **UPC Opcode Dictionary (NEW in draft-03)** — Sophia-driven MBC ISA instruction decode (root key 0x10, "code" category); 10 instruction class types; 32-byte BPF map entry format; use cases (tracing, dynamic dispatch, profiling, validation)
- **IANA Considerations** — Sophia Root Key Registry (0x01-0x14 initial entries); Sub-Dictionary Type Registry (NEW); QPACK Static Table Registry (NEW)

## Related

- [[Protocol Foundation|Protocol-Foundation]]
- [[Draft Protocol Foundation 06|Draft-Protocol-Foundation-06]]
- [[Wotan Memory Model|Wotan-Memory-Model]]
- [[Draft Wotan Memory 03|Draft-Wotan-Memory-03]]
- [[Drafts Index|Drafts-Index]]

---

> **Source:** [docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md](../docs/protocol/draft-bellis-unheaded-sophia-dictionary-03.md)
