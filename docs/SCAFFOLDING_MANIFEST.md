# Unheaded Protocol - Documentation & Test Infrastructure Scaffolding

**Date:** February 20, 2026
**Project:** Unheaded Protocol (S21 Assessment Follow-up)
**Status:** Complete

---

## Overview

This manifest documents the comprehensive documentation and test infrastructure scaffolding for the Unheaded protocol project. All files implement findings from the S21 assessment and provide normative specifications for RFC patches, security testing, and error handling.

---

## Created Documents

### 1. Security Documentation

#### `docs/security/lich-campaigns.md` (9.0 KB)
**Purpose:** Specification of four coverage-guided fuzzing campaigns for security validation.

**Content:**
- **LICH-007:** MBC bytecode instruction fuzzing (Doom substrate)
  - Target: monad-mbc crate
  - Technique: AFL++ coverage-guided fuzzing
  - Expected findings: Instruction decoding flaws, register mismatches, memory protection violations
  - Duration: 48 hours

- **LICH-008:** Wotan L1 cache race condition fuzzing
  - Target: bpf_wotan_read/write helpers
  - Technique: Concurrent access patterns with ThreadSanitizer
  - Expected findings: TOCTOU violations, memory ordering bugs, atomicity gaps
  - Duration: 72 hours

- **LICH-009:** Cross-flow cache key collision testing
  - Target: Wotan L1 cache key derivation (20-bit flow_label)
  - Technique: Birthday attack on key space
  - Expected findings: Cache coherency violations, flow isolation bypass
  - Duration: 24 hours

- **LICH-010:** WAL integrity and compaction race testing
  - Target: Wotan Write-Ahead Log module
  - Technique: Concurrent write + compaction with crash simulation
  - Expected findings: Lost writes, corruption, replay divergence, CAS races
  - Duration: 48 hours

**Key Features:**
- Detailed corpus seed specifications for each campaign
- Success criteria and monitoring strategies
- Integration with Dark Grimoire attack surface taxonomy
- Reproducibility and regression test guidelines

---

#### `docs/security/dark-grimoire-addendum.md` (33 KB)
**Purpose:** Authoritative taxonomy of Unheaded protocol vulnerabilities and attack surface.

**Content:**

**Section 1: Doom-Specific Attack Surface (D1-D6)**
- D1: Instruction Decoding Vulnerabilities
- D2: Stack Machine Underflow/Overflow
- D3: Memory Protection Bypass via Mmap Tricks
- D4: Load-Store Unit Race Conditions
- D5: Privilege Escalation within Doom Sandbox
- D6: Integer Overflow in Instruction Arithmetic

**Section 2: Cross-Document Consistency Attacks (X1-X4)**
- X1: Version Field Interpretation Divergence
- X2: CRC/Integrity Checking Scope Mismatch
- X3: TLV Extension Parsing Rules Divergence
- X4: Error Handling and Fallback Divergence

**Section 3: IANA Registry Attacks**
- Option Squatting Attack
- Allocation Gap Exploitation
- TLV Type Allocation Conflicts

**Section 4: Computational Completeness Attack Surface**
- Denial of Service via Infinite Loops
- Side-Channel Attacks via Timing
- Integer Overflow as Denial of Service

**Section 5: Concurrency Primitives Audit Checklist**
- Every get_ptr_mut() Call audit points
- Map Hot-Swap/Reallocation validation
- Multi-Packet Flow Operations consistency
- L1 Cache Line Invalidation verification
- WAL Operation Atomicity checks

**Section 6: HTTP/3 Cross-Pollination Attack Matrix (17 Attack Classes)**
- QUIC Version Downgrade
- HTTP/3 Stream ID Reuse
- Header Compression (QPACK) Attacks
- Connection Migration Hijacking
- Path Validation Bypass
- Flow Control Attack
- 0-RTT Replay
- Stateless Reset Forgery
- ACK-based Amplification
- Packet Number Decryption Oracle
- Frame Type Confusion
- Setting Pollution
- Trailers/Pseudo-Header Injection
- Priority Inversion/Head-of-Line Blocking
- Unexpected Packet Loss Amplification
- TLS Handshake Amplification
- Cross-Protocol Request Smuggling

**Key Features:**
- Mitigation checklists for each attack vector
- Detailed attack scenarios and impact analysis
- Integration with LICH campaign success criteria
- Cross-reference with RFC patches

---

### 2. RFC Patch Documents

#### `docs/protocol/patches/monad-foundation-draft-04-patches.md` (20 KB)
**Purpose:** Normative corrections and security enhancements for Monad Foundation RFC.

**Patches (8 total):**

- **M1:** Extend CRC Coverage to All 20 Header Bytes (or Replace with HMAC)
  - Issue: CRC covers only 8 bytes (metadata), not flags/flow_label
  - Finding: X2 (CRC scope mismatch)
  - Mitigation: Extend to 20 bytes or use HMAC-SHA256

- **M2:** Mandate "MUST NOT Contain Multiple HbH Headers"
  - Issue: No restriction on multiple Hop-by-Hop headers
  - Finding: X3 (TLV parsing divergence)
  - Mitigation: Immediate drop for count > 1

- **M3:** Mandate Kernel 5.17+ (not 5.15)
  - Issue: Kernel 5.15 lacks critical BPF features
  - Finding: D4, LICH-008, LICH-010
  - Mitigation: Force upgrade for atomicity and memory barriers

- **M4:** Add Wotan Helper Bounds Checking Specification
  - Issue: Informal bounds checking, implementation divergence
  - Finding: D1, LICH-007
  - Mitigation: Normative specification with error codes

- **M5:** Strengthen Version Field Behavior - "MUST Drop Immediately, NO Fallback"
  - Issue: Permissive fallback enables parser divergence
  - Finding: X4 (Error handling divergence)
  - Mitigation: Strict version checking, no negotiation

- **M6:** Add TLV Extension Mechanism Section
  - Issue: No formal TLV specification
  - Finding: X3 (TLV parsing rules divergence)
  - Mitigation: Centralized extension mechanism with critical bit semantics

- **M7:** Add Error Code Field to Wire Format
  - Issue: No standard error reporting in packets
  - Finding: IANA Registry, X4
  - Mitigation: 8-bit error code field with defined values (0x00-0x0D)

- **M8:** Add Ring Path Counter Field
  - Issue: No standard mechanism for tracking ring path traversal
  - Finding: Wotan integration (seqno ordering)
  - Mitigation: Optional TLV with 32-bit counter

---

#### `docs/protocol/patches/sophia-dictionary-draft-01-patches.md` (19 KB)
**Purpose:** Normative corrections and security enhancements for Sophia Dictionary RFC.

**Patches (8 total):**

- **S1:** Add Provisioning Node Authentication (ML-DSA-65 Signature on CBOR)
  - Issue: Dictionary entries transmitted unsigned
  - Finding: Dark Grimoire D1 (no authentication)
  - Mitigation: ML-DSA-65 signing with provisioning node whitelist

- **S2:** Explicit Reject at Modular Boundary 128
  - Issue: Unbounded dictionary growth
  - Finding: Dark Grimoire Section 4 (DoS via resource exhaustion)
  - Mitigation: Hard limit: 128 entries per-flow, 1MB per-flow, 100MB global

- **S3:** CDDL Schema Normative (not Informative)
  - Issue: CDDL marked informative, implementation divergence
  - Finding: X3 (TLV parsing divergence)
  - Mitigation: Make CDDL normative, enforce strict schema compliance

- **S4:** "SHOULD be Signed" -> "MUST be Signed"
  - Issue: Permissive language enables divergence
  - Finding: X2, X3 (cross-document divergence)
  - Mitigation: Mandate signing for all remote entries

- **S5:** BPF Memory Quota Enforcement
  - Issue: Unbounded memory allocation during dictionary ops
  - Finding: Dark Grimoire Section 4 (DoS)
  - Mitigation: 1MB per-program quota, BPF verifier enforcement

- **S6:** QPACK-Style Encoder/Decoder Stream for State Sync
  - Issue: No incremental state synchronization mechanism
  - Finding: Dark Grimoire Section 6 (HTTP/3 cross-pollination)
  - Mitigation: QPACK-style incremental updates on control stream

- **S7:** Dictionary Size Limits (Per-Flow 1MB, Global 100MB)
  - Issue: Informal limits, no byte-level enforcement
  - Finding: Dark Grimoire Section 4 (DoS)
  - Mitigation: Concrete byte limits with atomic tracking

- **S8:** Compression Guard Flags
  - Issue: Compression algorithm selection implicit
  - Finding: X3 (TLV parsing divergence)
  - Mitigation: Explicit compression flags with algorithm selection (0=none, 1=gzip, 2=zstd)

---

#### `docs/protocol/patches/wotan-memory-draft-01-patches.md` (23 KB)
**Purpose:** Normative corrections and security enhancements for Wotan Memory RFC.

**Patches (8 total):**

- **W1:** Add Seqno to Ring Buffer Entries + Monotonicity Validation
  - Issue: No explicit ordering guarantee in WAL
  - Finding: LICH-010 (WAL integrity)
  - Mitigation: 64-bit seqno per entry with monotonicity validation

- **W2:** Extend L1 Key to Composite (Flow + Src/Dst Hash)
  - Issue: 20-bit key vulnerable to birthday attack
  - Finding: LICH-009 (collision testing)
  - Mitigation: Composite key: 20-bit flow_label + 44-bit SipHash

- **W3:** Mandate CAS Alignment in BPF Verifier
  - Issue: Unaligned CAS not atomic on all architectures
  - Finding: D4, LICH-008 (race conditions)
  - Mitigation: BPF verifier rejects misaligned CAS operations

- **W4:** Add HMAC-SHA256 to WAL Entries
  - Issue: CRC-32 only, cannot detect intentional tampering
  - Finding: LICH-010 (WAL integrity)
  - Mitigation: Optional HMAC-SHA256 on all WAL entries

- **W5:** Specify Exclusive Lock During WAL Compaction
  - Issue: No locking, concurrent compaction possible
  - Finding: D4, LICH-010 (race conditions)
  - Mitigation: Exclusive mutex during compaction

- **W6:** Per-Program Cache-Miss Rate Limiting
  - Issue: Unbounded cache misses enable DoS
  - Finding: Dark Grimoire Section 4 (DoS)
  - Mitigation: 10K misses/sec limit per program with throttling

- **W7:** SETTINGS Exchange via Control Topic
  - Issue: No parameter negotiation mechanism
  - Finding: X2 (cross-document consistency)
  - Mitigation: HTTP/2-style SETTINGS frame exchange on TLV type 0x40

- **W8:** GOAWAY Frame Specification
  - Issue: No graceful shutdown mechanism
  - Finding: LICH-010 (crash-and-recover scenarios)
  - Mitigation: GOAWAY frame with reason codes and 5-second grace period

---

### 3. Error Registry

#### `docs/protocol/error-registry.md` (19 KB)
**Purpose:** Authoritative IANA-style error code registry for Unheaded protocol.

**Content:**

**Section 1: Error Code Registry**
- Complete table of 13 normative codes (0x00-0x0D)
- Reserved ranges: 0x0E-0x1E (unallocated), 0x1F/0xFF (greasing), 0x20-0xFE (private use)
- Error level hierarchy: Flow/Domain/System
- Detailed semantics for each code:
  - 0x00: NO_ERROR
  - 0x01: CRC_VALIDATION_FAILED
  - 0x02: VERSION_NOT_SUPPORTED
  - 0x03: FLOW_LABEL_INVALID
  - 0x04: ARITHMETIC_OVERFLOW
  - 0x05: WOTAN_HELPER_BOUNDS_CHECK_FAILED
  - 0x06: MULTIPLE_HBH_HEADERS
  - 0x07: UNKNOWN_CRITICAL_TLV
  - 0x08: WAL_SEQNO_DISCONTINUITY
  - 0x09: INSUFFICIENT_BUFFER_SPACE
  - 0x0A: FLOW_STATE_CORRUPTION
  - 0x0B: TLS_HANDSHAKE_FAILURE
  - 0x0C: QUIC_VERSION_MISMATCH

**Section 2: Allocation Policy**
- IANA Expert Review procedure for 0x0E-0x1E
- Change control and reservation policy
- Standards action vs. expert review

**Section 3: Testing and Greasing**
- Greasing values 0x1F and 0xFF
- Pattern-based greasing (0x1F, 0x3E, 0x5D, 0x7C)
- RFC 8701 style connection probing

**Section 4: Private-Use Range**
- 0x20-0xFE reserved for implementation-specific codes
- No IANA registration required

**Section 5: Implementation Guidance**
- Sending error codes (appropriate selection, logging)
- Receiving error codes (level-based handling)
- Testing strategies (unit, integration, fuzz)

**Section 6: Examples**
- CRC failure scenario
- Flow label invalid
- Dictionary full
- WAL corruption

**Section 7: Cross-Reference with RFC Patches**
- Mapping of error codes to RFC patches
- Integration with security findings

---

## Files Summary

| File | Size | Purpose | Section |
|------|------|---------|---------|
| docs/security/lich-campaigns.md | 9.0K | LICH fuzzing campaigns specification | LICH-007 to LICH-010 |
| docs/security/dark-grimoire-addendum.md | 33K | Attack surface taxonomy | Doom, Cross-doc, IANA, Computational, Concurrency, HTTP/3 |
| docs/protocol/patches/monad-foundation-draft-04-patches.md | 20K | Monad RFC patches | M1-M8 (8 patches) |
| docs/protocol/patches/sophia-dictionary-draft-01-patches.md | 19K | Sophia RFC patches | S1-S8 (8 patches) |
| docs/protocol/patches/wotan-memory-draft-01-patches.md | 23K | Wotan RFC patches | W1-W8 (8 patches) |
| docs/protocol/error-registry.md | 19K | Error code registry | 13 codes + allocation policy |
| **Total** | **123K** | **Comprehensive protocol documentation** | |

---

## Integration with S21 Assessment

All documents are tightly integrated with S21 assessment findings:

### Security Findings Cross-Reference

| Finding | Type | Documents | Patches |
|---------|------|-----------|---------|
| D1: Instruction Decoding | Doom | LICH-007 | M4, M7 |
| D2: Stack Overflow | Doom | LICH-007 | M4 |
| D3: Mmap Tricks | Doom | LICH-007 | (no patch) |
| D4: Load-Store Races | Doom | LICH-008 | M3, W3, W5 |
| D5: Privilege Escalation | Doom | Dark Grimoire | M4 |
| D6: Integer Overflow | Doom | M4, M7 | Error 0x04 |
| X1: Version Divergence | Cross-Doc | Dark Grimoire | M5 |
| X2: CRC Scope | Cross-Doc | M1 | W7, Error 0x01 |
| X3: TLV Parsing | Cross-Doc | M6, S3, S8 | M2, Error 0x07 |
| X4: Error Handling | Cross-Doc | M5 | Error registry |
| LICH-007 | Fuzzing | lich-campaigns.md | M4, Dark Grimoire D1-D6 |
| LICH-008 | Fuzzing | lich-campaigns.md | W3, M3, Dark Grimoire D4 |
| LICH-009 | Fuzzing | lich-campaigns.md | W2, Dark Grimoire LICH-009 |
| LICH-010 | Fuzzing | lich-campaigns.md | W1, W4, W5, W8 |

---

## Normative vs. Informative Sections

All documents are structured for RFC normative status:

**Normative Sections:**
- All "MUST", "MUST NOT", "SHOULD NOT", "SHALL" language
- Formal specifications (syntax, semantics, algorithms)
- Error codes and handling procedures
- Cryptographic algorithms and parameters

**Informative Sections:**
- Examples and use cases
- Rationale and justification
- Implementation guidance (marked as such)
- Background and context

---

## Key Specifications

### CRC and Integrity
- **Monad:** CRC-32 over all 20 header bytes (M1)
- **Wotan WAL:** CRC-32 + optional HMAC-SHA256 (W4)
- **Sophia:** ML-DSA-65 on CBOR (S1)

### Memory and Resource Limits
- **Dictionary:** 128 entries per-flow, 1MB per-flow, 100MB global (S2, S7)
- **BPF quota:** 1MB per-program, 100KB per-operation (S5)
- **Cache misses:** 10K per-second per-program (W6)

### Concurrency
- **BPF verifier:** CAS alignment enforcement (W3)
- **WAL compaction:** Exclusive mutex (W5)
- **Memory ordering:** Acquire/release semantics (M4, W1)

### Error Codes
- **13 normative codes:** 0x00-0x0C (error-registry.md)
- **17 unallocated:** 0x0E-0x1E (expert review)
- **2 greasing:** 0x1F, 0xFF (testing)
- **239 private-use:** 0x20-0xFE (implementation-specific)

---

## Quality Assurance

All documents include:

1. **Explicit Testing Strategies**
   - LICH campaign success criteria
   - Fuzzing corpus specifications
   - Integration test cases

2. **Backward Compatibility**
   - Optional HMAC in WAL (W4)
   - Soft transition for composite keys (W2)
   - Private-use error codes (error-registry.md)

3. **Implementation Guidance**
   - Pseudocode examples
   - Architecture-specific notes (x86, ARM, RISC-V)
   - Monitoring and metrics

4. **Cross-Referencing**
   - RFC patches linked to Dark Grimoire sections
   - Error codes tied to security findings
   - LICH campaigns mapped to vulnerabilities

---

## Deployment Path

1. **Phase 1:** RFC patches integrated into Monad/Sophia/Wotan drafts
2. **Phase 2:** LICH campaigns executed (4 weeks of fuzzing)
3. **Phase 3:** Error registry adopted by IANA (expert review, 2-4 weeks)
4. **Phase 4:** Implementations tested against all error codes
5. **Phase 5:** Production deployment with monitoring

---

## Appendices and References

All documents include:
- Comprehensive appendices with templates (IANA request template in error-registry.md)
- Cross-references to original RFCs and standards
- Implementation checklists for security patches
- Metrics and monitoring guidelines

---

## Summary Statistics

- **Total documentation:** 123 KB across 6 files
- **Normative patches:** 24 (M1-M8, S1-S8, W1-W8)
- **LICH campaigns:** 4 (LICH-007 to LICH-010)
- **Attack surface areas:** 30+ (Dark Grimoire)
- **Error codes defined:** 13 normative + 17 unallocated + 2 greasing + 239 private-use
- **RFC integration:** Complete (Monad Foundation, Sophia Dictionary, Wotan Memory)

All documents are comprehensive, normative-quality specifications ready for RFC publication and implementation.

