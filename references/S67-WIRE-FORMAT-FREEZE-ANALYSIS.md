# S67: Wire Format Freeze Analysis
## Date: 2026-02-27 | Author: RFC Editor + Architect + Scientist
## Status: ANALYSIS COMPLETE — DECISION REQUIRED

---

## 1. FLAGS COLLISION ANALYSIS

### Current Flags Byte (offset 0x07, 8 bits)

```
 Bit 7   6   5   4   3   2   1   0
    +---+---+---+---+---+---+---+---+
    | C | Y | T | E | S | M |CUST| R |
    +---+---+---+---+---+---+---+---+

C    (0x80): CHAOS — Chaos injection active (Yaldabaoth)
Y    (0x40): CANARY — Canary deployment path
T    (0x20): TRACED — Full trace active
E    (0x10): ENCRYPT — Payload encrypted (intra-Kingdom TLS)
S    (0x08): SAMPLED — Statistically sampled
M    (0x04): MIRROR — Mirror copy
CUST (0x02): CUSTOM — Scratch fields carry exponent-encoded values
R    (0x01): RESERVED — MUST be zero
```

### External Collision Risk: **NONE**

The flags byte lives INSIDE the Monad option TLV payload (offset 0x07 within the
option data). It is NOT a shared field. No external RFC can collide with our flags
unless they claim our same HbH option type (0x3E), which IANA registration prevents.

**RFC 9927 Lesson Applied**: The EARO C-Flag collision occurred because two RFCs
(8928 and 8685) assigned different semantics to the SAME bit in a SHARED header
field. Our flags byte is option-private. Lesson learned, crisis avoided.

### Internal Extensibility Risk: **HIGH**

- 8/8 bits allocated (including RSVD)
- RSVD (bit 0) provides exactly ONE future flag slot
- After that: wire format change required for more flags

### Recommendation: FREEZE AS-IS

Rationale:
- 7 active flags + 1 reserved is sufficient for Alpha/Beta
- The Extended Register Option (MONAD-EXT-REG) provides overflow capacity
- If we MUST expand later, RSVD bit 0 can be repurposed via spec update
- A 16-bit flags expansion (breaking change) is NOT justified at this stage

---

## 2. BREAKING CHANGE CANDIDATES — IMPACT MATRIX

Deployment count = 0 (no production packets on wire yet). This is the LAST window
for zero-cost breaking changes. After bare metal deployment, every change has
migration cost.

### Candidate 1: Flags Byte Expansion (8 → 16 bits)

| Criterion | Score |
|-----------|-------|
| **Benefit** | Future-proofs flag space (8 more bits) |
| **Cost** | +1 byte to Monad (20 → 21 bytes, breaks 8-byte alignment) |
| **Code Impact** | Every struct, parser, serializer, BPF program, test |
| **Spec Impact** | Wire format diagram, checksum range, all offset tables |
| **Risk** | HIGH — alignment break cascades through entire stack |
| **Verdict** | **REJECT** — Extended Register Option handles overflow |

### Candidate 2: CRC-16 → CRC-32 Upgrade

| Criterion | Score |
|-----------|-------|
| **Benefit** | Stronger error detection (missed error rate: 2^-32 vs 2^-16) |
| **Cost** | +2 bytes to Monad (20 → 22 bytes) |
| **Code Impact** | All checksum compute/verify functions |
| **Spec Impact** | Checksum section, wire format, all offset tables |
| **Risk** | MEDIUM — CRC-16 is adequate for 18-byte payload |
| **Verdict** | **REJECT** — CRC-16/CCITT is sufficient for 18 bytes. For 18 bytes, CRC-16 has undetected error probability of ~1.5×10^-5 which is adequate for a non-security checksum. Cryptographic integrity is handled separately. |

### Candidate 3: trace_id Field (reintroduce dedicated field)

| Criterion | Score |
|-----------|-------|
| **Benefit** | Dedicated 32-bit trace correlation (vs borrowing IPv6 Flow Label) |
| **Cost** | +4 bytes to Monad (20 → 24 bytes) |
| **Code Impact** | Major — new field in all structs, BPF programs |
| **Spec Impact** | Wire format, offset tables, checksum range |
| **Risk** | HIGH — 4 bytes is expensive in a 20-byte register |
| **Verdict** | **REJECT** — IPv6 Flow Label (20-bit) provides sufficient trace correlation within a Limited Domain. 2^20 = ~1M concurrent flows. If more needed, Extended Register Option. |

### Candidate 4: Reserved Field Repurpose (offset 0x10-0x11)

| Criterion | Score |
|-----------|-------|
| **Benefit** | 16 bits of currently wasted space could carry useful data |
| **Cost** | 0 bytes (no size change) |
| **Code Impact** | Minimal — field rename/redefine |
| **Spec Impact** | Reserved field section only |
| **Risk** | LOW — but removes future flexibility |
| **Verdict** | **DEFER** — Keep reserved for now. Can be repurposed in draft-06 without wire format break (reserved fields are zero-initialized, so old parsers ignore non-zero values). |

NOTE: Looking at the actual spec, scratch[0-3] at 0x0E-0x11 + checksum at 0x12-0x13
are the final fields. The "reserved" mentioned in early drafts has been replaced by
src_prefix_lo and dst_prefix_lo (0x0C-0x0D). All 20 bytes are now allocated.

### Candidate 5: Optional Crypto-ID Field (variable length)

| Criterion | Score |
|-----------|-------|
| **Benefit** | PQC fingerprint directly in Monad (quantum-resistant per-packet auth) |
| **Cost** | Variable — violates fixed-size register principle |
| **Code Impact** | EXTREME — variable-length Monad breaks all BPF map assumptions |
| **Spec Impact** | Fundamental architecture change |
| **Risk** | CRITICAL — violates Monad design principle (fixed 20 bytes) |
| **Verdict** | **REJECT** — PQC identity binding is handled via Sophia lookup (exponent → fingerprint), not inline. This was a settled architectural decision. |

### Candidate 6: Exponent Encoding Revision

| Criterion | Score |
|-----------|-------|
| **Benefit** | Could improve precision or range of exponent-encoded fields |
| **Cost** | 0 bytes (encoding change, not wire format change) |
| **Code Impact** | All Sophia encode/decode functions |
| **Spec Impact** | Section 6 (Exponent Encoding) |
| **Risk** | MEDIUM — silent data corruption if old/new encoders mixed |
| **Verdict** | **FREEZE AS-IS** — Current encoding (base^exponent * multiplier) is adequate. Any revision creates mixed-version risk with no clear benefit. |

### Candidate 7: Version Field Reduction (8 → 4 bits)

| Criterion | Score |
|-----------|-------|
| **Benefit** | Frees 4 bits for future use (header flags, sub-version, etc.) |
| **Cost** | Reduces version space from 256 to 16 |
| **Code Impact** | Version check logic, all parsers |
| **Spec Impact** | Wire format, version field section |
| **Risk** | LOW-MEDIUM — 16 versions is plenty (IPv6 has version=6, never changed) |
| **Verdict** | **REJECT for now** — 8-bit version with current value 0x01 is clean. Splitting a byte into 4+4 adds parsing complexity for speculative benefit. If needed, version 0x02 can redefine the version byte format. |

---

## 3. FREEZE DECISION SUMMARY

### ACCEPTED CHANGES: **NONE**

All 7 candidates analyzed. None justify a wire format break at this stage.

### RATIONALE

The 20-byte Monad wire format as specified in draft-05 is:

1. **Compact**: 20 bytes fits in a single cache line alongside the HbH header
2. **Aligned**: 20 bytes = 5 × 32-bit words, clean for BPF register operations
3. **Complete**: All fields have clear semantics (7 raw + 6 exponent + 4 scratch + 1 checksum)
4. **Extensible**: Extended Register Option provides overflow without breaking base format
5. **Versioned**: Version 0x01 allows future wire format changes via version bump
6. **Protected**: CRC-16 adequate for 18-byte error detection

### WIRE FORMAT: FROZEN

```
The Monad wire format as specified in draft-bellis-unheaded-protocol-foundation-05
is hereby FROZEN at version 0x01.

No breaking changes will be made to:
- Field offsets (0x00 through 0x13)
- Field sizes (all 20 bytes allocated)
- Flags bit assignments (C|Y|T|E|S|M|CUST|R)
- Checksum algorithm (CRC-16/CCITT-FALSE)
- Exponent encoding semantics

Future extensions MUST use:
- The Extended Register Option (MONAD-EXT-REG)
- New Sophia dictionary entries
- Reserved bit 0 (one-time repurpose allowed)
- Version field bump (0x02+) for incompatible changes
```

---

## 4. IANA REGISTRIES — STATUS

### Immediate Registration Required (5 registries for foundation spec)

| Registry | Range | Policy | Status |
|----------|-------|--------|--------|
| IPv6 HbH Option Type | 0x3E | Standards Action | **DRAFT NEEDED** |
| Monad Version | 0x00-0xFF | Standards Action | **DRAFT NEEDED** |
| Monad Flags Bitfield | bits 0-7 | Specification Required | **DRAFT NEEDED** |
| Flow Action | 0x00-0xFF | Expert Review | **DRAFT NEEDED** |
| Kingdom Mode | 0x00-0x03 | Standards Action | **DRAFT NEEDED** |

### Deferred (companion spec registries)

| Registry | Spec | Status |
|----------|------|--------|
| Sophia Sub-Dictionary Types | sophia-dictionary I-D | Deferred to sophia-03 |
| Anamnesis Event Types | foundation + sophia | Deferred to sophia-03 |
| QoS Class | foundation + sophia | Deferred to sophia-03 |
| Circuit State | foundation + sophia | Deferred to sophia-03 |
| Wotan Error Codes | wotan-memory I-D | Deferred to wotan-03 |

---

## 5. IPR STATUS

### RFC 8928 (Address Protected Neighbor Discovery)
- **IPR Search**: No IETF IPR disclosures found for RFC 8928
- **Patent Risk**: LOW — AP-ND addresses IPv6 neighbor discovery, not HbH options
- **Our Overlap**: None. We don't implement neighbor discovery extensions.

### RFC 9927 (EARO C-Flag Correction)
- **IPR Search**: No IETF IPR disclosures found for RFC 9927
- **Patent Risk**: NONE — RFC 9927 is a flag reassignment correction, not new IP
- **Our Overlap**: LESSON ONLY — we learned to register flags before deployment
- **Lesson Applied**: Our flags byte is option-private, not shared. No collision possible.

### Unheaded Protocol
- **IPR Status**: No third-party IPR claims
- **Author**: Stevie Bellis (sole author)
- **License**: To be determined (Barrister domain)

---

## 6. NEXT STEPS

1. **DRAFT IANA Considerations section** for foundation spec (5 registries)
2. **Integrate into draft-06** (or update draft-05 in-place)
3. **Documentation ripple**: Update wiki, protocol summary, CLAUDE.md
4. **Bare metal deployment**: Wire format is now safe to deploy
5. **Future spec work**: sophia-03 and wotan-03 add remaining 5 registries

---

*Wire Format Freeze Analysis — Forged 2026-02-27*
*7 candidates analyzed. 0 accepted. Wire format FROZEN at v0x01.*
*"The C-Flag fell because nobody registered it. Monad will not fall the same way."*
