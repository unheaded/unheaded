# Protocol Specification Changelog — S77

**Sprint:** S77 (Phase 4: Protocol Spec Advancement)
**Date:** March 5, 2026
**Author:** Stevie Bellis / Claude (Anthropic)

---

## Overview

S77 Phase 4 advances all three Unheaded Protocol specifications by one draft version:

- Foundation: draft-05 -> draft-06
- Sophia: draft-02 -> draft-03
- Wotan: draft-02 -> draft-03

All changes are purely additive. No existing wire formats, processing rules, or normative requirements are modified or removed. The Monad wire format remains FROZEN at v0x01 (20 bytes).

---

## Foundation draft-06 (from draft-05)

**File:** `draft-bellis-unheaded-protocol-foundation-06.md`

### New Sections

1. **IANA Registration Procedures (Section 9)**
   - Step-by-step guide for registering new metric types
   - Covers all 12 IANA registries with registration policies
   - Includes preparation, submission, and non-conflict verification steps

2. **UNHEADED_METRIC_V1 Example (Section 9.2)**
   - Complete example registration: Type 0x2A, 52 bytes
   - Carried as separate HbH option TLV (does NOT modify Monad)
   - Full wire format diagram with field definitions
   - Registration template for IANA submission

3. **Security Considerations — Wire Format Immutability (Section 10.1)**
   - Parser divergence attack threat model
   - Wire format modification via extension threat
   - Five-step verification procedure for implementations

4. **Backwards Compatibility Statement (Section 11)**
   - Formal statement that draft-05 to draft-06 is non-breaking
   - Wire format, registry, and interoperability analysis

### Enhanced Sections

5. **Wire Format FROZEN Statement**
   - Added explicit "WIRE FORMAT FROZEN" banner in Section 5.1
   - Clarifies no changes permitted in v0x01

6. **Cross-References**
   - Added Section 1.2 introducing the spec family
   - Updated Sophia references from draft-02 to draft-03
   - Updated Wotan references from draft-02 to draft-03
   - Added [SOPHIA] and [WOTAN] references throughout

### Registry Updates

7. **TLV Type Registry**
   - Added UNHEADED_METRIC_V1 (0x2A, optional, 52 bytes)

---

## Sophia draft-03 (from draft-02)

**File:** `draft-bellis-unheaded-sophia-dictionary-03.md`

### New Sections

1. **Sub-Dictionary Type System (Section 3.2)**
   - Four entry types: LEAF, BRANCH, COMPOSITE, ALIAS
   - Nested sub-dictionary structure (sophia_typed_sub_entry, 84 bytes)
   - Nested lookup chain (multi-level BPF hash traversal)
   - Maximum nesting depth: 8 levels
   - Circular reference detection via visited bitmask
   - Use cases: service topology, policy hierarchies, geographic routing, tenant isolation
   - CBOR encoding for nested entries
   - BPF map representation with namespace partitioning (0-63 top-level, 64-191 nested)

2. **QPACK Compression Headers (Section 5)**
   - Adaptation of RFC 9204 (QPACK) for Sophia dictionary entries
   - Static table: 24 pre-defined entries for common field names/values
   - Dynamic table: per-connection, 4096-byte capacity
   - Wire format: compression flags + entry count + encoded fields
   - Encoding rules for static/dynamic references and Huffman coding
   - Compression ratios: 3:1 to 5:1 for metadata, 1.1:1 for PQC keys
   - Decompression limits: 1 MB max, 10 ms timeout
   - Backward compatible: raw CBOR (draft-02) entries still valid

### New IANA Registries

3. **Sophia Sub-Dictionary Type Registry**
   - Codes 0x00-0xFF for sub-dictionary entry types

4. **Sophia QPACK Static Table Registry**
   - 24 entries (indices 0-23) for common dictionary fields

### Security Enhancements

5. **Nested Dictionary Security**
   - Depth limit enforcement (8 levels)
   - Circular reference prevention
   - Namespace partitioning enforcement

6. **QPACK Decompression Security**
   - Decompression bomb mitigation
   - Dynamic table poisoning prevention

### Updated References

7. **Cross-References**
   - Foundation reference updated from draft-04 to draft-06
   - Wotan reference updated from draft-01 to draft-03
   - Added Section 1.1 (spec family structure)

---

## Wotan draft-03 (from draft-02)

**File:** `draft-bellis-unheaded-wotan-memory-03.md`

### New Sections

1. **Error Code Taxonomy (Section 3)**
   - 5 severity levels: INFO, WARNING, ERROR, CRITICAL, FATAL
   - 32-bit structured error code format: [severity:3][origin:5][category:8][detail:8]
   - 16 origin codes covering all Wotan/Sophia/Shield subsystems
   - 12 error category codes (access control, bounds, resource, integrity, etc.)

2. **Helper Return Code Mapping (Section 3.3)**
   - Maps standard errno codes to structured error codes
   - Auxiliary error detail via per-CPU BPF array map (wotan_last_error)
   - Code examples for reading structured errors in BPF programs

3. **Common Error Codes (Section 3.4)**
   - L1 cache errors (6 codes)
   - L2 ring buffer errors (5 codes)
   - L3 WAL errors (5 codes)
   - gRPC streaming errors (5 codes)
   - Sophia lookup errors (6 codes)

4. **Error Recovery Procedures (Section 3.5)**
   - Recovery by severity level (INFO through FATAL)
   - Automatic recovery state machine (HEALTHY -> DEGRADED -> RECOVERING -> FAILED)
   - Recovery metrics (6 metric families)
   - Cross-subsystem recovery dependency graph

### New IANA Registries

5. **Wotan Error Origin Registry**
   - Codes 0x00-0x1F for error origin identification

6. **Wotan Error Category Registry**
   - Codes 0x00-0xFF for error category classification

### Security Enhancements

7. **Error Code Information Leakage**
   - Mitigations for structured error codes revealing architecture details

### Updated References

8. **Cross-References**
   - Foundation reference updated from draft-04 to draft-06
   - Sophia reference updated from draft-01 to draft-03
   - Added Section 1.1 (spec family structure)

### Retained from draft-02

All 8 normative security patches (W1-W8) are fully retained:
- W1: Seqno monotonicity validation
- W2: Composite L1 cache key
- W3: CAS alignment enforcement
- W4: HMAC-SHA256 for WAL entries
- W5: Exclusive WAL compaction lock
- W6: Per-program cache-miss rate limiting
- W7: SETTINGS exchange
- W8: GOAWAY frame specification

---

## Supporting Documents Created

| File | Description |
|------|-------------|
| `CROSS-REFERENCE-MATRIX-S77.md` | Complete cross-reference table between all 3 specs |
| `CHANGELOG-S77.md` | This changelog |

---

## Wire Format Status

**FROZEN at v0x01 (20 bytes).** No changes in any of the three new drafts. The UNHEADED_METRIC_V1 extension (Foundation draft-06) is carried as a separate HbH option TLV and does not modify the Monad register file layout.
