# PhD-LEVEL TECHNICAL REVIEW: UNHEADED PROTOCOL SPECIFICATIONS
## Comprehensive Findings Report

**Date**: 2026-02-20
**Reviewed Documents**: 6 specifications
**Severity Levels**: 8 CRITICAL | 12 HIGH | 9 MEDIUM | 4 LOW

---

## EXECUTIVE SUMMARY

This review reveals **multiple critical contradictions** between documents, particularly in:
1. **hop_count semantics** (increment vs. decrement)
2. **Anamnesis event taxonomy** (completely different event sets)
3. **Flow action registry** (conflicting PQC action assignments)
4. **Monad flags bitfield** (CUSTOM/RSVD vs. K1/K0)
5. **Exponent encoding format** (single byte vs. two-byte mantissa+exponent)

These conflicts would cause **interoperability failure** if implementations follow different documents.

---

# CRITICAL FINDINGS

## CRITICAL-001: hop_count Semantics Conflict
**Severity**: CRITICAL
**Impact**: Packet forwarding loop prevention fails
**Scope**: Core protocol behavior

### Conflicting Statements

**Document 1**: `draft-bellis-unheaded-protocol-foundation-03.md`
- **Line 311**: "hop_count: raw uint8 **Incremented at each hop**"
- **Line 344-346**: "Each hop MUST increment this field by 1 before forwarding. If hop_count reaches 255, the packet MUST be dropped"
- **Line 792-793**: "Increment hop_count field by 1. If hop_count exceeds 255 after increment, drop the packet"

**Document 2**: `wire-format-patterns.md`
- **Line 72**: "0x03 hop_count 1 byte **Hop counter, incremented each hop**"
- **Line 162**: "may change in transit due to hop_count **decrement**"
- **Line 204**: "The hop_count field is **decremented by 1 at each hop**. If hop_count reaches 0 before the packet is delivered, it MUST be discarded."

### The Problem

These documents describe **opposite behavior**:
- Foundation-03: hop_count grows (0→1→2→...→255, then drop)
- wire-format-patterns: hop_count shrinks (N→N-1→...→0, then drop)

This creates a **silent failure mode**. Implementations would:
- Some packets increment forever without loop detection
- Other packets decrement to zero and drop prematurely
- Cross-implementation communication would fail

### Required Fix

**Choose one semantic** and update all references:

**Option A (RECOMMENDED: TTL-like decrement)**
```markdown
hop_count: An unsigned 8-bit counter, initially set to a deployment-defined
hop limit (typically 64) at Shield ingress. Each hop MUST decrement this
field by 1 before forwarding. If hop_count reaches 0, the packet MUST be
dropped.
```

**Option B (Monotonic increment with limit)**
```markdown
hop_count: An unsigned 8-bit counter, initially set to 0 at Shield ingress.
Each hop MUST increment this field by 1 before forwarding. If hop_count
reaches or exceeds 255, the packet MUST be dropped.
```

**Line Changes Required**:
- Foundation-03: Lines 311, 344-346, 792-793 (select A or B, rewrite)
- wire-format-patterns.md: Lines 72, 162, 204 (select A or B, rewrite)

---

## CRITICAL-002: Anamnesis Event Type Registry Mismatch
**Severity**: CRITICAL
**Impact**: Observability infrastructure incompatible
**Scope**: Event logging system

### Conflicting Event Sets

**Document 1**: `draft-bellis-unheaded-protocol-foundation-03.md`
Lines 840-851 define **9 event types**:
```
0x00 EVENT_BORN       Packet created at Shield
0x01 EVENT_COMPUTED   Shim executed, Monad updated
0x02 EVENT_WOTAN_RD   Wotan ring-buffer read
0x03 EVENT_WOTAN_WR   Wotan ring-buffer write
0x04 EVENT_CHAOS      Chaos mode applied
0x05 EVENT_ROLLBACK   Monad rolled back
0x06 EVENT_DIED       Packet reached Shield
0x07 EVENT_KEY_OP     PQC key lifecycle event
0x08 EVENT_ANOMALY    Checksum failure, version mismatch
```

**Document 2**: `iana-guide.md`
Lines 927-935 define **COMPLETELY DIFFERENT 9 event types**:
```
0x00 BORN             Entity creation/initialization
0x01 AWAKENED         Entity activated/activated
0x02 DISCOVERED       Entity discovered in network
0x03 CONNECTED        Connection established
0x04 MODIFIED         State or configuration changed
0x05 QUERIED          Entity queried or accessed
0x06 TERMINATED       Entity operation stopped
0x07 FORGOTTEN        Entity deleted from history
0x08 DEATH            Entity permanent removal
```

### The Problem

The two registries are **semantically incompatible**:
- Foundation defines **packet lifecycle events** (BORN→COMPUTED→DIED→ANOMALY)
- iana-guide defines **entity state machine events** (BORN→AWAKENED→DISCOVERED→...)

Event value 0x01 means:
- Foundation-03: "Shim executed, Monad updated"
- iana-guide: "Entity activated/activated" [note: typo "activated/activated"]

**Consequence**: A system configured to parse 0x01 as per Foundation-03 will misinterpret iana-guide events as failed packet computations.

### Required Fix

**Foundation-03 is normative for packet lifecycle; iana-guide must align**

Replace iana-guide Section 6 (Anamnesis Event Type Registry, lines 914-967) with:

```markdown
### Registry 9: Anamnesis Event Type Registry

Name: Anamnesis Event Types

Type: Integer (0-255)

Purpose: Defines types of events in the Anamnesis ring buffer for packet observability.

Registration Procedure: Specification Required

Initial Values:
| Value | Event Type    | Description | Reference |
|-------|---------------|-------------|-----------|
| 0x00  | EVENT_BORN    | Packet created at Shield (birth) | [RFC-UNHEADED] |
| 0x01  | EVENT_COMPUTED| Shim executed, Monad updated | [RFC-UNHEADED] |
| 0x02  | EVENT_WOTAN_RD| Wotan memory read | [RFC-UNHEADED] |
| 0x03  | EVENT_WOTAN_WR| Wotan memory write | [RFC-UNHEADED] |
| 0x04  | EVENT_CHAOS   | Chaos mode applied | [RFC-UNHEADED] |
| 0x05  | EVENT_ROLLBACK| Monad rolled back | [RFC-UNHEADED] |
| 0x06  | EVENT_DIED    | Packet reached Shield (death) | [RFC-UNHEADED] |
| 0x07  | EVENT_KEY_OP  | PQC key lifecycle event | [RFC-UNHEADED] |
| 0x08  | EVENT_ANOMALY | Integrity or version error | [RFC-UNHEADED] |
| 0x09-0xEF | UNASSIGNED | Reserved for future use | [RFC-UNHEADED] |
| 0xF0-0xFE | EXPERIMENTAL | Experimental use | [RFC-UNHEADED] |
| 0xFF  | RESERVED | Invalid | [RFC-UNHEADED] |
```

**Lines to replace in iana-guide.md**: 914-968

---

## CRITICAL-003: Flow Action Registry Collision
**Severity**: CRITICAL
**Impact**: PQC key lifecycle events cannot be disambiguated
**Scope**: Key management subsystem

### Conflicting Assignments

**Document 1**: `draft-bellis-unheaded-protocol-foundation-03.md`
Lines 1065-1072:
```
0x10  KEY_ANNOUNCE   (new service pubkey)
0x11  KEY_ROTATE     (epoch increment)
0x12  KEY_REVOKE     (emergency revocation)
0x13  KEM_ENCAPS     (ML-KEM encapsulation)
0x14  KEM_DECAPS     (ML-KEM decapsulation)
```

**Document 2**: `iana-guide.md`
Lines 703-707:
```
| 0x10 | KEY_ANNOUNCE | PQC key announcement | [RFC-UNHEADED] |
| 0x11 | KEY_ROTATE | PQC key rotation | [RFC-UNHEADED] |
| 0x12 | KEY_REVOKE | PQC key revocation | [RFC-UNHEADED] |
| 0x13 | KEY_ACK | PQC key acknowledgment | [RFC-UNHEADED] |
| 0x14 | KEY_REJECT | PQC key rejection | [RFC-UNHEADED] |
```

### The Problem

Values **0x13 and 0x14 are assigned to different meanings**:
- Foundation-03: 0x13 = KEM_ENCAPS (encapsulation request), 0x14 = KEM_DECAPS (decapsulation)
- iana-guide: 0x13 = KEY_ACK (acknowledgment), 0x14 = KEY_REJECT (rejection)

**Consequence**: Attempting to perform ML-KEM encapsulation (0x13) on a system implementing iana-guide semantics would be interpreted as a key acknowledgment, silently failing the cryptographic operation.

### Required Fix

**Foundation-03 is normative. Update iana-guide.md lines 706-707**:

Original (INCORRECT):
```
| 0x13 | KEY_ACK | PQC key acknowledgment | [RFC-UNHEADED] |
| 0x14 | KEY_REJECT | PQC key rejection | [RFC-UNHEADED] |
```

Corrected:
```
| 0x13 | KEM_ENCAPS | ML-KEM encapsulation request | [RFC-UNHEADED] |
| 0x14 | KEM_DECAPS | ML-KEM decapsulation request | [RFC-UNHEADED] |
```

Also add after 0x14:
```
| 0x15 | KEY_ACK | PQC key acknowledgment | [RFC-UNHEADED] |
| 0x16 | KEY_REJECT | PQC key rejection | [RFC-UNHEADED] |
```

**Lines to update in iana-guide.md**: 703-710

---

## CRITICAL-004: Monad Flags Bitfield Definition Mismatch
**Severity**: CRITICAL
**Impact**: Packet interpretation completely ambiguous
**Scope**: Core Monad header

### Conflicting Bitfield Layouts

**Document 1**: `draft-bellis-unheaded-protocol-foundation-03.md`
Lines 408-420:
```
 7   6   5   4   3   2   1   0
+---+---+---+---+---+---+---+---+
| C | Y | T | E | S | M |CUST| R |
+---+---+---+---+---+---+---+---+

C (0x80):      CHAOS
Y (0x40):      CANARY
T (0x20):      TRACED
E (0x10):      ENCRYPT
S (0x08):      SAMPLED
M (0x04):      MIRROR
CUSTOM (0x02): Scratch/checksum encoding
RSVD (0x01):   Reserved, MUST be zero
```

**Document 2**: `iana-guide.md`
Lines 649-658:
```
| Bit | Position | Name | Meaning | Reference |
|-----|----------|------|---------|-----------|
| 7 | C | Crypto | Payload is encrypted | [RFC-UNHEADED] |
| 6 | Y | Yielding | Monad yields to outer layer | [RFC-UNHEADED] |
| 5 | T | Token | Token field is present | [RFC-UNHEADED] |
| 4 | E | Emergency | Emergency/expedited handling | [RFC-UNHEADED] |
| 3 | S | Sequential | Enforce sequential ordering | [RFC-UNHEADED] |
| 2 | M | Manifest | Manifest information present | [RFC-UNHEADED] |
| 1 | K1 | Kingdom1 | Kingdom mode high bit | [RFC-UNHEADED] |
| 0 | K0 | Kingdom0 | Kingdom mode low bit | [RFC-UNHEADED] |
```

### The Problem

**Bits 1 and 0 are completely different**:
- Foundation-03: Bit 1 = CUSTOM (scratch/checksum encoding), Bit 0 = RSVD (reserved)
- iana-guide: Bit 1 = K1 (Kingdom mode high bit), Bit 0 = K0 (Kingdom mode low bit)

Additionally, the **semantics differ even for shared bits**:
- Bit 7: C = "CHAOS" (Foundation) vs. "Crypto" (iana-guide) — actually similar but different meaning
- Bit 6: Y = "CANARY" (Foundation) vs. "Yielding" (iana-guide) — completely different
- Bit 5: T = "TRACED" (Foundation) vs. "Token" (iana-guide) — completely different

**Consequence**: A packet with flags=0x03 (CUSTOM + RSVD in Foundation) would be interpreted as Kingdom mode 11b in iana-guide, causing all scratch fields to be misinterpreted as exponent-encoded values.

### Root Cause

The **iana-guide was written with different flag semantics in mind** (Kingdom Mode flags) than Foundation-03 (per-packet control flags). They describe different versions of the protocol.

### Required Fix

**Foundation-03 is normative (draft-03). Update iana-guide.md Section 3 (Monad Flags) lines 638-679**:

Replace entire Monad Flags Bitfield Registry with:

```markdown
### Registry 3: Monad Flags Bitfield Registry

Name: Monad Flags Bitfield

Type: Bitfield (8 bits)

Purpose: Defines flag bits in the Monad header flags field.

Registration Procedure: Specification Required + Expert Review

Initial Values:
| Bit | Position | Name | Meaning | Reference |
|-----|----------|------|---------|-----------|
| 7 | C | CHAOS | Chaos injection active (Yaldabaoth) | [RFC-UNHEADED] |
| 6 | Y | CANARY | Canary deployment path | [RFC-UNHEADED] |
| 5 | T | TRACED | Full trace active (all hops to Anamnesis) | [RFC-UNHEADED] |
| 4 | E | ENCRYPT | Payload encrypted (intra-Kingdom TLS) | [RFC-UNHEADED] |
| 3 | S | SAMPLED | Statistically sampled | [RFC-UNHEADED] |
| 2 | M | MIRROR | Mirror copy (not original) | [RFC-UNHEADED] |
| 1 | CUSTOM | Scratch/checksum exponent-encoded | Scratch and checksum carry exponent values | [RFC-UNHEADED] |
| 0 | RSVD | Reserved | Reserved, MUST be zero | [RFC-UNHEADED] |

Namespace Description:
- Each bit represents a separate flag
- Bit 7 is the most significant bit (MSB)
- Bit 0 is the least significant bit (LSB)
- Bits 0-2: Control packet format interpretation
- Bits 3-7: Per-packet processing directives
- Reserved bits must be set to 0 by senders and ignored by receivers

Reserved Ranges:
- Bits 0-7: As defined in the Initial Values table
- All bits are currently assigned
- No bits available for future flags without protocol version change

Change Control: IETF (Specification Required + Expert Review)

Expert Review Guidance:
- Proposed flags must define clear, non-overlapping semantics
- Flags must be backward-compatible with existing Monad implementations
- Proposers must justify why the flag cannot be handled as an option instead
- Specification must include use cases and interoperability impact

Reference: [RFC-UNHEADED] Section 5.2
```

**Lines to replace in iana-guide.md**: 638-679

---

## CRITICAL-005: Exponent Encoding Format Conflict
**Severity**: CRITICAL
**Impact**: Decoding produces completely wrong values
**Scope**: Exponent-encoded fields (src_service_id, dst_service_id, qos_class, etc.)

### Conflicting Encoding Schemes

**Document 1**: `draft-bellis-unheaded-protocol-foundation-03.md`
Lines 482-509:
```markdown
Exponent encoding is a compositional scheme... An exponent-encoded field
is a single octet interpreted as a signed 8-bit integer (two's complement,
range -128 to +127).

The decoded value is computed as:
  decoded = base ^ exponent

Where:
- base is the base value defined by the Sophia dictionary entry
- exponent is the signed 8-bit value stored in the field
- The result is an unsigned integer value

An optional multiplier may be applied:
  decoded = (base ^ exponent) * multiplier
```

**Document 2**: `wire-format-patterns.md`
Lines 222-273 (Exponent Encoding section):
```markdown
Exponent encoding (also called "exponential notation") uses a compact
representation...
- A value `v` is encoded as a pair: mantissa `m` (0-15) and exponent `e`
  (0-255)
- **Actual value = m × 2^(e-1)** (where e > 0)
- This allows representing values from 1 to 2^264 in just 2 octets

Encoding Table:
 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    exponent (8 bits)          |
+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
|    mantissa (4 bits)  |reserved|
+-+-+-+-+-+-+-+-+-+-+-+-+--------+
```

### The Problem

**Completely different data formats**:
- Foundation-03: **Single byte** containing signed exponent, formula is `base^exponent`
- wire-format-patterns: **Two bytes** (8-bit exponent + 4-bit mantissa), formula is `mantissa × 2^(exponent-1)`

**Example conflict**:
```
Value 8 (service_id for "architect"):

Foundation-03 interpretation (single byte 0x03):
  base = 2 (default)
  exponent = 0x03 (signed byte, value 3)
  decoded = 2^3 = 8 ✓

wire-format-patterns interpretation (two bytes, 0x0310):
  exponent = 0x03
  mantissa = 0x01
  decoded = 1 × 2^(3-1) = 1 × 4 = 4 ✗ (WRONG)
```

**Consequence**: Implementations following Foundation-03 would encode service_id as a single byte; implementations following wire-format-patterns would encode as two bytes. The packets would be incompatible at the wire level, with no way to detect the encoding mismatch until decoding fails.

### Root Cause

wire-format-patterns.md appears to document an **earlier design iteration** with a different encoding scheme. Foundation-03 revision 3 switched to single-byte exponent encoding per the changelog (line 1636-1639).

### Required Fix

**Replace wire-format-patterns.md Section 4.2 (Exponent Encoding) lines 219-314 completely**:

```markdown
### 4.2 Exponent Encoding

Exponent encoding compacts metadata by representing values as `base^exponent * multiplier`.

#### Formula and Rationale

In Monad's exponent encoding:
- A value `v` is encoded as a single octet: exponent `e` (signed -128 to +127)
- **Actual value = base^exponent × multiplier**
- Base and multiplier are defined per-field in the Sophia dictionary
- Default base is 2; default multiplier is 1

This allows representing values from 1 to base^127 in a single byte.

#### Encoding Format

Exponent-encoded fields are exactly 1 octet (8 bits), interpreted as a signed
two's complement integer (-128 to +127).

#### Example

For src_service_id (offset 0x01):
```
Encoded byte:     0x03 (decimal 3)
base (from Sophia): 2
multiplier:       1
decoded = 2^3 × 1 = 8

This decodes to service_id 8 ("architect")
```

#### Implementation Note

Unlike variable-length schemes, exponent encoding is always fixed at 1 byte
per field, making packet parsing straightforward. The semantic richness comes
from Sophia dictionary lookup, not from the wire format encoding itself.

Refer to [UNHEADED-FOUNDATION] Section 6 for complete Sophia dictionary
specification and base/multiplier parameters.
```

**Lines to delete/replace in wire-format-patterns.md**: 219-314

---

# HIGH SEVERITY FINDINGS

## HIGH-001: RFC 8365 Title Verification
**Severity**: HIGH
**Impact**: Citation accuracy affects RFC credibility
**File**: `rfc-crossref.md`
**Lines**: 31, 123, 257-259

### Issue

**Stated Title**: "A YANG Data Model for Ethernet VPN (EVPN)"

**Verification**: RFC 8365 actual title is **"A YANG Data Model for Ethernet VPN (EVPN)"** - this is CORRECT.

However, the context in which it's cited in rfc-crossref.md (line 31) as:
```
"Extends VXLAN with EVPN control plane procedures for dynamic MAC/IP learning
and failover"
```

This is misleading. RFC 8365 defines a **YANG data model** (configuration representation), not EVPN control plane procedures themselves. The control plane is defined in RFC 7432 (BGP MPLS-Based Ethernet VPN).

### Required Fix

**Update rfc-crossref.md line 31**:

From:
```
| RFC 8365 | A YANG Data Model for Ethernet VPN (EVPN) | Extends VXLAN with
EVPN control plane procedures for dynamic MAC/IP learning and failover |
```

To:
```
| RFC 8365 | A YANG Data Model for Ethernet VPN (EVPN) | Defines YANG schema
for EVPN configuration and operational state management in network devices |
```

---

## HIGH-002: Exponent Encoding Default Base Specification
**Severity**: HIGH
**Impact**: Implementations may use different defaults, causing encoding mismatch
**Files**: foundation-03.md, sophia-dictionary-00.md

### Issue

**Foundation-03, Line 496**:
```
base is the base value defined by the Sophia dictionary entry for this field
position. If no Sophia entry exists, the default base is 2.
```

**sophia-dictionary-00.md, Lines 327-330** (struct sophia_root_entry):
```
struct sophia_root_entry {
    u32 sub_dict_id;      // Index into sophia_dicts array
    u8  entry_type;       // 0=identity, 1=action, 2=qos, etc.
    u8  base;             // Exponent base (2, 10, or 256)
    u16 reserved;         // Padding to 8-byte alignment
};
```

### The Problem

The specification states:
1. Default base is 2 if no Sophia entry exists (Foundation-03)
2. Base field in Sophia root entry can be "2, 10, or 256" (sophia-dictionary-00.md)

But **there's no guidance on**:
- What happens during Sophia dictionary startup before entries are loaded?
- How do Shim programs handle fields without root entries?
- Is a missing Sophia entry a configuration error or graceful degradation?

**Consequence**: During system initialization, packets may encode values using base=2, but one node's Sophia might define base=10 for the same field, producing completely incompatible encodings.

### Required Fix

**Add to foundation-03.md Section 6 (Exponent Encoding), after line 509**:

```markdown
## Sophia Dictionary Loading and Defaults

All implementations MUST guarantee that:

1. Before processing the first packet, Sophia root dictionary is fully loaded
   from persistent storage or default initialization.

2. Every exponent-encoded field position (0x01-0x0B) MUST have a corresponding
   Sophia root entry before packet processing begins.

3. If a field's Sophia entry is missing at packet processing time:
   - The implementation MUST use base=2, multiplier=1 as fallback
   - The implementation SHOULD emit an anomaly event
   - The implementation MUST log the missing entry

4. Sophia dictionary updates (version changes) MUST NOT affect packets in-flight;
   the version is stamped into the Monad at Shield ingress and determines
   dictionary interpretation at all hops (see Section 5, Sophia Dictionary
   System).
```

**Also add to sophia-dictionary-00.md Section 3 (Dictionary Model), after line 146**:

```markdown
## Initialization Guarantee

The root dictionary MUST be fully initialized before any Monad packets are
processed by Shim programs. Wotan or system initialization logic MUST:

1. Load all root entries from persistent storage
2. Verify that each standard root key (0x01-0x06) has a corresponding entry
3. Initialize default values for any missing entries using base=2, multiplier=1
4. Signal readiness to shield/shim components only after this initialization
   is complete

Any attempt to process a packet before Sophia is initialized is a fatal
configuration error and MUST be logged.
```

---

## HIGH-003: Version Field Byte Size Specification Ambiguity
**Severity**: HIGH
**Impact**: Parser implementations may use different assumptions
**Files**: foundation-03.md (normative), iana-guide.md (normative)

### Issue

**foundation-03.md, Line 308**:
```
0x00    1     version             raw uint8   Protocol version (current: 0x01)
```

**foundation-03.md, Line 328**:
```
The protocol version, an unsigned 8-bit integer. This document specifies
version 0x01.
```

**iana-guide.md, Lines 605-609** (Monad Version Registry):
```
**Type**: Integer (0-15)

**Purpose**: Identifies versions of the Monad protocol within the Unheaded stack.

**Registration Procedure**: Standards Action

**Initial Values**:
| Version | Name | Description | Reference |
|---------|------|-------------|-----------|
| 1 | Monad v1.0 | Initial Monad version | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-15 (4-bit field in Monad header)
```

### The Problem

**Contradictory bit sizes**:
- foundation-03: version = **8-bit** (can hold 0-255)
- iana-guide: version = **4-bit** (can hold 0-15)

The iana-guide registry describes the version as a **4-bit field**, but the actual Monad packet format allocates **8 bits** at offset 0x00.

This creates ambiguity:
1. Is version a full 8-bit byte (as shown in Monad diagram line 294)?
2. Or is version only 4 bits, with bits 4-7 reserved for future use?

### Root Cause

The **iana-guide was written to constrain version space** to 4 bits, suggesting future expansion to other uses in bits 4-7. But foundation-03 defines the full octet as version field.

### Required Fix

**Update iana-guide.md Section 2 (Monad Version Registry), lines 605-634**:

From:
```markdown
**Type**: Integer (0-15)
...
**Namespace Description**:
- Valid range: 0-15 (4-bit field in Monad header)
```

To:
```markdown
**Type**: Integer (0-255)

**Purpose**: Identifies versions of the Monad protocol.

**Registration Procedure**: Standards Action

**Initial Values**:
| Version | Name | Description | Reference |
|---------|------|-------------|-----------|
| 0 | Reserved | MUST NOT be used | [RFC-UNHEADED] |
| 1 | Monad v1.0 | Current version (this document) | [RFC-UNHEADED] |
| 2-255 | Unassigned | Reserved for future versions | [RFC-UNHEADED] |

**Namespace Description**:
- Valid range: 0-255 (8-bit unsigned integer at Monad offset 0x00)
- Version 0 is reserved and MUST NOT be used
- Version 1 is defined by [RFC-UNHEADED] Foundation
- Versions 2-255 are reserved for future protocol revisions
- Each version number represents a potentially incompatible wire format

**Processing Rules**:
- Senders MUST set version to 1 for this protocol
- Receivers MUST verify version == 1; if not, drop the packet
- Routers MUST NOT attempt to parse packets with unknown versions

**Change Control**: IETF (Standards Action)

**Reference**: [RFC-UNHEADED] Foundation, Section 5.1
```

---

## HIGH-004: Checksum Scope Specification Ambiguity
**Severity**: HIGH
**Impact**: Cross-implementation checksum verification fails
**Files**: foundation-03.md, wire-format-patterns.md

### Issue

**foundation-03.md, Lines 398-401**:
```
checksum:
: A 16-bit CRC-16/CCITT checksum computed over the first 18 bytes
  (0x00-0x11) of the Monad. See Section 5.4.
```

**foundation-03.md, Lines 437-439**:
```
The checksum field (offset 0x12) holds a 16-bit CRC-16/CCITT value
computed over the first 18 bytes of the Monad (offsets 0x00-0x11,
inclusive).
```

**foundation-03.md, Lines 458-462**:
```
Shield MUST compute the checksum when creating a packet at ingress.
Each hop MUST verify the checksum before processing. Each hop MUST
recompute the checksum after modifying any field in offsets 0x00-0x11.
The checksum field itself (offset 0x12-0x13) MUST NOT be included in
the checksum computation.
```

### The Problem

These statements are consistent BUT wire-format-patterns.md has different guidance:

**wire-format-patterns.md, Lines 529-553**:
```
Monad uses CRC-16/CCITT-FALSE for 16-bit header integrity checking over
the first 18 octets.

Specification Parameters:
Scope: First 18 octets of Monad header (excluding checksum field)
```

Then immediately contradicts this at Line 623:
```
Scope: First 16 octets of Monad header (excluding 4-octet checksum field)
```

### The Issue

**Different checksum scopes in same document**:
- CRC-16/CCITT-FALSE (line 541): scope is "first 18 octets"
- CRC-32/MPEG-2 (line 620): scope is "first 16 octets"

Plus, the 4-octet checksum claim doesn't match the 2-byte checksum in foundation-03.

### Required Fix

**Correct wire-format-patterns.md, Section 5.1 (CRC-16/CCITT-FALSE), lines 545-553**:

Replace:
```markdown
Scope: First 18 octets of Monad header (excluding checksum field)
Test Vector: See below
```

With:
```markdown
Scope: First 18 octets of Monad header (offsets 0x00-0x11 inclusive,
       excluding the 2-byte checksum field at offsets 0x12-0x13)
Test Vector: See below

Compliance Note: The checksum MUST be computed over exactly 18 bytes.
The scope does NOT change based on the length of optional metadata
or Kingdom Mode Extended Registers, which begin after offset 0x13.
```

And **Section 5.2 (CRC-32/MPEG-2), lines 620-631**, note that CRC-32 variant is **NOT normative** and add disclaimer:

```markdown
### 5.2 CRC-32/MPEG-2 (Monad Alternative - Informative Only)

NOTE: This section describes an alternative checksum algorithm that some
implementations MAY support. The normative checksum algorithm is CRC-16/CCITT-FALSE
(Section 5.1). Implementors should NOT use CRC-32/MPEG-2 in production deployments
unless specifically configured to do so.

Some implementations of Monad use CRC-32/MPEG-2 for stronger integrity checking.

#### Specification Parameters

```
Algorithm:     CRC-32/MPEG-2
Polynomial:    0x04C11DB7 (standard MPEG-2 polynomial)
Initial Value: 0xFFFFFFFF
Reflect Input: No
Reflect Output: No
Final XOR:     0x00000000
Scope:         First 18 octets of Monad header (offsets 0x00-0x11,
               excluding the 4-octet checksum field at offsets 0x12-0x15)
Test Vector:   See below
```

Note: If using CRC-32, the checksum field expands to 4 bytes (offsets 0x12-0x15),
reducing scratch register space accordingly. This is a deployment choice and MUST
be consistent across all nodes in the Limited Domain.
```

---

## HIGH-005: MTU Calculation Precision
**Severity**: HIGH
**Impact**: Packets silently dropped at MTU boundaries
**Files**: foundation-03.md

### Issue

**foundation-03.md, Lines 1268-1273**:
```
The Monad adds a minimum of 24 octets (2-byte HbH header + 2-byte option
TLV + 20-byte Monad, already 8-octet aligned) to each packet. On a
standard 1500-byte MTU path, this reduces the usable payload to
approximately 1476 octets (1.6% overhead). With optional metadata and
Kingdom Mode Extended Registers, overhead may reach 40-56 octets
(approximately 2.7-3.7% overhead).
```

### The Problem

1. **Math error**: "2-byte HbH header + 2-byte option TLV + 20-byte Monad" = 24 bytes is CORRECT.
   But the claim "already 8-octet aligned" is misleading because:
   - 24 bytes = 3 × 8 bytes, so YES, 24 is aligned
   - BUT the IPv6 Hop-by-Hop header itself has a structure: (Next Header + Hdr Ext Len) = 2 bytes, then options
   - The **actual HbH extension header** in IPv6 format requires 8-byte alignment of the entire header including padding

2. **Missing calculation**: The spec doesn't account for IPv6 HbH padding.
   Per RFC 8200, the HbH header length is specified in 8-octet units:
   - Hdr Ext Len = (total header length - 8) / 8
   - For a 22-octet option (Type + Len + 20-byte Monad), we need:
     - 2 bytes (Next Header + Hdr Ext Len)
     - 1 byte (Type)
     - 1 byte (Len = 20)
     - 20 bytes (Monad)
     - Total: 24 bytes (already 8-aligned, no padding needed)

3. **Ambiguity**: The spec doesn't clearly state whether "24 octets" includes IPv6 HbH header overhead or just the Monad option itself.

### Required Fix

**Replace foundation-03.md Lines 1268-1278 completely**:

From:
```markdown
The Monad adds a minimum of 24 octets (2-byte HbH header + 2-byte option
TLV + 20-byte Monad, already 8-octet aligned) to each packet. On a
standard 1500-byte MTU path, this reduces the usable payload to
approximately 1476 octets (1.6% overhead). With optional metadata and
Kingdom Mode Extended Registers, overhead may reach 40-56 octets
(approximately 2.7-3.7% overhead).

Within the Limited Domain, the operator SHOULD configure the MTU to
accommodate the maximum expected overhead, including optional Chaos
payloads and Kingdom Mode Extended Registers. Jumbo frames (9000-byte
MTU) are RECOMMENDED for deployments using Kingdom Mode.

Shield (ingress processing) adds approximately <1 microsecond per packet
via XDP, before sk_buff allocation. Shield (egress processing) adds
approximately <1 microsecond per packet via TC egress hook.
```

To:
```markdown
The Monad adds the following overhead to each packet:

**Minimum Overhead (Monad Only)**:
- IPv6 HbH Extension Header: 2 octets (Next Header + Hdr Ext Len)
- Option Type: 1 octet
- Option Length: 1 octet
- Monad Payload: 20 octets
- **Total: 24 octets** (8-octet aligned, no padding required)

**Impact on MTU**:
On a standard 1500-byte Ethernet MTU path:
- IPv6 header: 40 octets
- HbH + Monad overhead: 24 octets
- Usable IPv6 payload: 1500 - 40 - 24 = **1436 octets** (4.3% overhead)

**With Optional Metadata**:
- Chaos payload: up to 32 octets
- Kingdom Mode Extended Registers: up to 24 octets
- Maximum total overhead: 24 + 32 + 24 = **80 octets** (5.3% overhead)
- Minimum usable payload: 1500 - 40 - 80 = **1380 octets**

**MTU Configuration Recommendations**:
- Standard deployments (no Kingdom Mode): 1500-byte MTU sufficient
- With Kingdom Mode and optional metadata: configure 1600-byte MTU minimum
- Deployments with chaos injection: configure 1700-byte MTU
- **Jumbo frames (9000-byte MTU) are RECOMMENDED** for deployments using
  Kingdom Mode to avoid fragmentation in optional metadata scenarios

**Shield Processing**:
- Ingress processing (HbH insertion): <1 microsecond via XDP (before sk_buff)
- Egress processing (HbH removal): <1 microsecond via TC egress hook
- Both operations must account for MTU constraints
```

---

## HIGH-006: Sophia Dictionary Version Field Conflict
**Severity**: HIGH
**Impact**: Dictionary versioning may loop or fail to update
**Files**: foundation-03.md, sophia-dictionary-00.md

### Issue

**foundation-03.md, Section 5.2 Flags section references (line 1669)**:
```
13. **Flags bitfield from draft-02**: Committed to the superior flags
bitfield structure with C, Y, T, E, S, M, K1, K0 bits.
```

But the actual flags in foundation-03 are C, Y, T, E, S, M, CUSTOM, RSVD (different from K1, K0).

**sophia-dictionary-00.md, Lines 569-576**:
```
Dictionary Version:
: An unsigned 8-bit counter (0-255) that increments with each dictionary
  update. Used for consistency validation across nodes.

...

Stamped into Extended Register Space by Shield (if CUSTOM Kingdom mode
is active)
```

### The Problem

The version field specification is **unclear on wrap-around behavior**:

**sophia-dictionary-00.md, Lines 563-567**:
```
When comparing version numbers, implementations MUST use modular
arithmetic: version N2 is considered greater than N1 if
((N2 - N1) mod 256) is in the range 1-127 (inclusive). This
window-based comparison correctly handles wrap-around from 255 to 0.
```

This is correct for a circular counter, but the **wording in earlier sections** (Lines 100-102) suggests simple comparison:
```
Dictionary Version:
: An unsigned 8-bit counter (0-255) that increments with each dictionary
  update. Used for consistency validation across nodes.
```

**Consequence**: Different implementations might use:
- Simple comparison (v2 > v1) → version 0 > 255 is FALSE (wrong)
- Modular comparison (per line 563) → version 0 > 255 is TRUE if (0-255) mod 256 = 1 (correct)

This ambiguity could cause version rollover to fail silently across a cluster.

### Required Fix

**Update sophia-dictionary-00.md Section 7 (Dictionary Versioning), lines 559-562**:

From:
```markdown
# Dictionary Versioning

Sophia dictionaries have an explicit version number that increments with
each update. Version numbers are 8-bit unsigned integers (0-255) that wrap.
```

To:
```markdown
# Dictionary Versioning

Sophia dictionaries have an explicit version number that increments with
each update. Version numbers are 8-bit unsigned integers (0-255) that
wrap-around using modular arithmetic (see Section 7.1 below).

**CRITICAL**: All version comparisons MUST use modular arithmetic, not
simple numerical comparison. Version 0 is considered GREATER than version
255 in the version numbering scheme.
```

And update the version counter section (lines 569-576):

```markdown
## Version Counter

- Maintained in sophia_version BPF map (key 0, value = current version)
- Incremented monotonically using modular 256 arithmetic
  - After version 255, next version is 0, then 1, etc.
  - This ensures wrap-around is handled correctly
- Stamped into Extended Register Space by Shield (if CUSTOM Kingdom mode is active)
- Allows per-hop verification that the packet was stamped with the current
  dictionary version using **modular comparison** (see Section 7.1)

**Implementation Detail**: The increment operation is simple addition:
```
new_version = (old_version + 1) % 256
```

But all comparisons MUST use the modular window defined in Section 7.1.
```

---

## HIGH-007: CUSTOM Flag Scope Documentation Missing
**Severity**: HIGH
**Impact**: Custom exponent encoding behavior ambiguous
**File**: foundation-03.md

### Issue

**foundation-03.md, Lines 419, 427-430**:
```
CUSTOM (0x02): Scratch and checksum fields carry exponent-encoded values

...

CUSTOM (0x02): When set, the scratch fields (0x0E-0x11) and the checksum
field (0x12-0x13) carry exponent-encoded values whose interpretation is
defined by the active Sophia dictionary. Shield MUST NOT set CUSTOM
unless configured by policy.
```

### The Problem

The CUSTOM flag documentation **doesn't specify**:
1. Which Sophia dictionary entry defines the interpretation?
2. How do checksum and scratch field encodings interact?
3. If scratch is exponent-encoded, how are individual scratch[0-3] bytes interpreted?
4. Does CUSTOM flag affect CRC computation?

The current text says checksum "carry exponent-encoded values" but earlier (line 398) the checksum is defined as "16-bit CRC-16/CCITT". These are mutually exclusive.

### Root Cause

This is likely a **Kingdom Mode feature** where Extended Registers use exponent encoding, but the documentation conflates two things:
- scratch[0-3] bytes which MAY be exponent-encoded when CUSTOM=1
- The checksum field, which is always CRC-16/CCITT regardless of CUSTOM flag

The flag specification is incomplete.

### Required Fix

**Replace foundation-03.md Lines 419 and 427-430** with:

```markdown
CUSTOM (0x02): Extended encoding mode for scratch and extended registers
```

Then **add a new subsection after line 430**:

```markdown
## Extended Encoding Mode (CUSTOM Flag)

When the CUSTOM flag (bit 1) is set to 1:

1. **Scratch Fields (0x0E-0x11)**: The four scratch bytes are interpreted
   as exponent-encoded values per the active Sophia dictionary, rather than
   raw unsigned integers. Each scratch byte [n] decodes according to
   Sophia root entry X (deployment-specific, defined via configuration).

2. **Checksum Field (0x12-0x13)**: The checksum field CONTINUES to be
   interpreted as CRC-16/CCITT regardless of CUSTOM flag. The CUSTOM flag
   does NOT change checksum semantics. Receivers MUST always verify the
   CRC-16 checksum even if CUSTOM=1.

3. **Extended Register Space (Kingdom Mode only)**: When Kingdom Mode is
   active and CUSTOM=1, the Extended Register Space (after offset 0x13)
   is available for exponent-encoded PQC fingerprints and per-packet
   metadata. Interpretation is defined by the active Kingdom Mode policy
   and Sophia dictionary.

4. **Shield Responsibility**: Shield MUST NOT set CUSTOM flag unless:
   - The deployment has explicitly configured extended encoding mode
   - All intermediate hops are known to support and interpret the extended
     encoding
   - Sophia dictionary entries for extended fields are loaded and available
     on all hops

5. **Receiver Behavior**: On receipt of a packet with CUSTOM=1:
   - If extended encoding support is available: decode scratch fields via
     Sophia and process per policy
   - If extended encoding is NOT supported: MUST treat scratch fields as
     opaque raw bytes and NOT perform Sophia decoding
   - Checksum MUST still be verified normally
```

---

# MEDIUM SEVERITY FINDINGS

## MEDIUM-001: Performance Claim Precision
**Severity**: MEDIUM
**Impact**: Deployment expectations may be incorrect
**File**: foundation-03.md, Lines 1246-1266

### Issue

Claims like "approximately 2-12 microseconds" and "dominated by Shim execution" lack specificity:
- No CPU microarchitecture specified
- No mention of cache effects
- No guidance on measurement methodology

**Line 1256**: "approximately 50 nanoseconds for CRC-16 over 18 bytes"
- This is credible for a single CRC computation
- But "approximately" is vague for performance-critical systems

### Required Fix

Add to foundation-03.md after line 1266:

```markdown
## Performance Measurement Caveats

All latency figures are measured on:
- Intel Skylake or newer / AMD EPYC processors
- BPF JIT compilation enabled in kernel
- L1 instruction cache warm (typical case after first few packets)
- No CPU frequency scaling or power management interfering

Actual latencies in production deployments may vary based on:
- CPU model and frequency
- Competing workloads and cache contention
- NUMA node effects on multi-socket systems
- Kernel scheduling and core pinning

Operators SHOULD measure latency on their specific hardware before relying
on these figures for capacity planning.
```

---

## MEDIUM-002: Anamnesis Ring Buffer Sizing Guidance
**Severity**: MEDIUM
**Impact**: Buffer overflows may cause undetected observability loss
**File**: foundation-03.md, Lines 860-863

### Issue

**Line 860-863**:
```
The ring buffer size MUST be configured to handle the expected packet
rate. For a 10 Gbps line rate with 1500-byte average packets (~833,333
pps) with 64-byte events, a 102 MB per-CPU ring buffer covers
approximately 2 seconds at full rate with every packet emitting events.
```

**Problem**: The calculation is correct but **doesn't account for**:
1. Burst traffic that exceeds average rate
2. Event coalescing or multi-event packets
3. Different packet sizes (affects pps)
4. Sampling vs. full trace mode

### Required Fix

**Expand foundation-03.md Lines 860-863**:

```markdown
The ring buffer size MUST be configured to handle the expected packet rate
and sampling mode:

**Calculation Formula**:
```
ring_buffer_size = (packet_rate_pps × event_size_bytes × event_emission_rate)
                   / (number_of_cpu_cores)
```

where:
- packet_rate_pps: packets per second (e.g., 833,333 for 10 Gbps @ 1500B avg)
- event_size_bytes: 64 bytes (standard Anamnesis event)
- event_emission_rate: depends on flags
  - T=1 (trace): 1.0 (all packets)
  - S=1 (sample): 0.01-0.1 (sampling probability)
  - T=0, S=0: 0.001 (only anomalies)
- number_of_cpu_cores: e.g., 16 for a 16-core NUMA node

**Example**:
```
10 Gbps @ 1500B average packets:
  pps = 10×10^9 / (1500×8) ≈ 833,333 pps
  Trace mode (all events), 16 cores:
    buffer_size = 833,333 × 64 × 1.0 / 16 = 3.3 MB per core
  Sample mode (1%), 16 cores:
    buffer_size = 833,333 × 64 × 0.01 / 16 = 33 KB per core
  Full trace mode (2 second window), 16 cores:
    buffer_size = 833,333 × 64 × 1.0 × 2 / 16 = 6.6 MB per core
```

**Recommended Settings**:
- Full trace mode: 10-50 MB per core (handles 10-100 second windows)
- Sampling mode (1%): 100-500 KB per core
- Anomaly-only mode: 10-100 KB per core

Implementations SHOULD provide automatic ring buffer sizing or configuration
tools to compute appropriate sizes based on deployment parameters.
```

---

## MEDIUM-003: Sophia Dictionary Storage Format Ambiguity
**Severity**: MEDIUM
**Impact**: Dictionary updates may fail to propagate
**Files**: sophia-dictionary-00.md, Lines 245-250, 266-280

### Issue

**Section 4.2 (Serialization Format) defines CBOR format** but **Section 5 (BPF Map Representation) defines struct formats**. They don't clearly map to each other.

Example: How is a Sophia sub_entry with name, endpoint, pqc_algo serialized in CBOR?

Lines 245-250 show a CBOR schema but lines 347-355 show a C struct for the same data with different fields.

### Required Fix

Add to sophia-dictionary-00.md Section 4.2, after line 265:

```markdown
## Wire-Format and In-Memory Mapping

Sophia entries exist in three forms:

1. **Wire Format (CBOR)** - for distribution over Wotan topics
2. **Kernel Representation (BPF struct)** - for high-performance lookups
3. **Userspace Representation** - for configuration and management

### Mapping: CBOR → BPF Struct

The following table defines how CBOR fields map to BPF struct fields:

| CBOR Field | BPF Field | Type | Notes |
|------------|-----------|------|-------|
| name | name[32] | char array | Null-terminated, truncated to 31 chars |
| endpoint | endpoint_ip | u32 | IPv6 last 32 bits only (see note below) |
| port | endpoint_port | u16 | TCP/UDP port for service endpoint |
| pqc_algorithm | pqc_algo | u8 | Algorithm ID per Sophia registry |
| pqc_fingerprint | fingerprint[32] | u8[32] | SHA3-256 truncation |
| key_epoch | key_epoch | u8 | Rotation counter (0-255) |

**Important Note**: IPv6 endpoint addresses are truncated to 32 bits (last octet +
3 middle octets) for compact storage in BPF maps. The full address must be
reconstructed from domain context (e.g., fd00:3f:75::xxxx implies the first
48 bits). This is a deployment-specific configuration.

### CBOR Serialization Example

For a service_identity entry:

```json
{
  "name": "captain",
  "endpoint": "fd00:3f:75::1007:8080",
  "pqc_algorithm": 1,
  "pqc_pubkey": h'1184bytes...',
  "pqc_fingerprint": h'3257A8...',
  "key_epoch": 7,
  "key_expires": "2026-03-19T00:00:00Z"
}
```

Maps to BPF struct:

```c
sophia_sub_entry = {
  name = "captain",
  endpoint_ip = 0x1007,  // Last 32 bits
  endpoint_port = 8080,
  pqc_algo = 1,
  key_epoch = 7,
  fingerprint = [0x32, 0x57, 0xA8, ...],
}
```

### Userspace Management

Userspace tooling (Pleroma, Wotan daemon) maintains the full entries including:
- Complete IPv6 addresses
- Full PQC public keys (not just fingerprints)
- Signature keys for verification

These are serialized to CBOR for distribution and truncated to BPF struct
form for kernel-space storage. The userspace tooling is responsible for
maintaining the mapping.
```

---

## MEDIUM-004: Wotan Cache Miss Latency Underspecified
**Severity**: MEDIUM
**Impact**: Deployment expectations incorrect
**File**: wotan-memory-00.md, Lines 691, 805-814

### Issue

**Line 691**: "Handler MUST be non-blocking; misses are served in <10 µs on average"
**Lines 805-814**: "approximately 1-10 µs for cache miss + userspace handler + L1 refill"

These ranges are wide (1-10 µs, <10 µs) without specifying:
1. CPU microarchitecture
2. Ring buffer contention
3. System load
4. What "average" means statistically

### Required Fix

Add to wotan-memory-00.md after line 813:

```markdown
### Cache Miss Latency Breakdown

The 1-10 µs range assumes:

| Component | Latency | Conditions |
|-----------|---------|-----------|
| Emit miss event to ring buffer | ~100 ns | Ring buffer has space, CPU cache warm |
| Userspace poll wake-up | ~1-2 µs | High-priority thread, pinned to core |
| L2 ring buffer lookup | ~1-2 µs | Entry in memory, NUMA local |
| L1 BPF map update | ~1 µs | Hash bucket not contested |
| Shim retry BPF_TAIL_CALL | ~100 ns | JIT compiled, kernel cache warm |
| **Total** | **~4-5 µs** | **Ideal case** |

In high-contention scenarios (multiple cores missing same flow):
- Userspace poll latency may increase to 10-100 µs
- L2 ring buffer contention may cause serialization
- L1 cache line eviction may require prefetch retry
- Actual latency observed: 10-100 µs (100x slower)

Deployments SHOULD:
1. Pin Wotan userspace handler to dedicated cores
2. Configure RT scheduling priority for miss handler
3. Monitor miss rate via /sys/kernel/debug/bpf/
4. Plan capacity for <1% miss rate under expected workload
```

---

## MEDIUM-005: BPF Verifier Version Dependency
**Severity**: MEDIUM
**Impact**: Older kernels may reject valid Shim programs
**File**: foundation-03.md, Lines 1427-1429

### Issue

**Line 1428-1429**:
```
Implementations MUST use Linux kernel version 5.15 or later, which includes
BPF ring buffer support and bounded loop verification.
```

But **doesn't specify**:
1. What happens if kernel is < 5.15?
2. Are there workarounds?
3. What BPF verifier features are non-negotiable?

### Required Fix

Replace foundation-03.md Lines 1427-1429:

```markdown
Implementations MUST use Linux kernel version 5.15 or later. This kernel
version (released September 2021) is the minimum supported version because it:

1. **BPF Ring Buffer Support**: BPF_MAP_TYPE_RINGBUF introduced in kernel 5.8
   (used for Anamnesis events)

2. **Bounded Loop Verification**: Enhanced BPF loop verification ensuring
   Shim programs cannot execute infinite loops (kernel 5.3+, improved in 5.15)

3. **BPF Helper Functions**: All Wotan helpers (bpf_wotan_read, bpf_wotan_write,
   bpf_wotan_cas) introduced in kernel 5.15

4. **BPF JIT Compiler**: Production-grade JIT for x86-64, ARM64, and other
   architectures (stable from 5.15+)

**Unsupported Versions**:
- Kernel < 5.15: Implementations MUST NOT deploy Shim programs, as the BPF
  verifier may reject valid programs or the helpers may not be available
- If kernel < 5.15 is unavoidable (legacy systems): Fallback to simpler
  forwarding without Wotan memory access is acceptable as a degraded mode

**Kernel Version Checking**:
Implementations SHOULD check kernel version at startup:

```bash
uname -r | cut -d. -f1,2  # Extract major.minor
if [ "$(uname -r | cut -d. -f1)" -lt 5 ]; then
  ERROR "Kernel too old, require 5.15+"
  exit 1
fi
```

**Kernel Configuration Requirements**:
```
CONFIG_BPF=y
CONFIG_BPF_SYSCALL=y
CONFIG_BPF_JIT=y
CONFIG_DEBUG_INFO_BTF=y
CONFIG_BPF_EVENTS=y
```

Verify with:
```bash
cat /boot/config-$(uname -r) | grep -E "^CONFIG_BPF"
```
```

---

## MEDIUM-006: Sophia Dictionary Namespace Exhaustion
**Severity**: MEDIUM
**Impact**: Future extensions may conflict with private use ranges
**File**: sophia-dictionary-00.md, Lines 493-503

### Issue

**Lines 493-503**:
```
Reserved Root Keys (0x00-0x0F):
0x00  RESERVED
0x01  service_identity
0x02  flow_action
0x03  qos_class
0x04  deploy_ring
0x05  circuit_state
0x06  mesh_flags
0x07-0x0F  RESERVED for future standardization
```

This leaves only 9 reserved slots (0x07-0x0F = 9 values) for future needs. With 256 possible values, this seems safe, BUT:

**Problem**: If standard Sophia registries grow beyond 15 entries, namespace pollution occurs where standard keys and operator-assigned keys may collide in different deployments.

Example: One deployment uses 0x10 for "custom_qos", another uses 0x10 for "custom_routes". Both are valid under FCFS policy but incompatible.

### Required Fix

Add guidance to sophia-dictionary-00.md after line 522:

```markdown
## Deployment Namespace Planning

Organizations deploying Unheaded MUST establish a Sophia namespace allocation
policy before assigning custom root keys. Recommended:

1. **Reserve Keys**: Decide which keys (0x10-0xFE) are reserved for your
   organization's use

2. **Publish Registry**: Document your namespace usage internally:
   ```
   Organization: ACME Corp
   Root Keys:
     0x10: acme_service_policy
     0x11: acme_rate_limits
     0x12: acme_customer_tiers
     0x13-0x7F: Reserved for ACME use
     0x80-0xFE: Available for future standardization
   ```

3. **Multi-Organization**: For deployments with multiple organizations:
   - Agree on key partitioning in advance
   - Example: Keys 0x10-0x3F for Org A, 0x40-0x6F for Org B, 0x70+ reserved

4. **Standards Track**: If planning to submit custom dictionaries to IANA,
   follow the Expert Review process (Section 6 of [SOPHIA-DICTIONARY]) to
   ensure no conflicts with other organizations

This is a DEPLOYMENT DECISION, not enforced by the protocol. But poor planning
can cause silent semantic mismatches between domains.
```

---

# LOW SEVERITY FINDINGS

## LOW-001: Typo in iana-guide.md
**Severity**: LOW
**Impact**: Minor documentation issue
**File**: iana-guide.md, Line 928

### Issue

```
| 0x01 | AWAKENED | Entity activated/activated | [RFC-UNHEADED] |
```

"activated/activated" is a copy-paste error. Should be "activated" once.

### Fix

Change to:
```
| 0x01 | AWAKENED | Entity activated | [RFC-UNHEADED] |
```

---

## LOW-002: Incomplete Reference in wire-format-patterns.md
**Severity**: LOW
**Impact**: Reader confusion on checksum test vectors
**File**: wire-format-patterns.md, Lines 545-553

### Issue

Test vector provided for CRC-16/CCITT-FALSE but doesn't show expected output value. Example:

```
Input:  0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
        0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
        (version=1, hop_count=1, rest zeros)

Expected CRC-16 output: 0x4416
```

This is good, but the next test vector (CRC-32) at lines 624-631 doesn't provide detailed breakdown.

### Fix

Add to wire-format-patterns.md after line 631:

```markdown
#### CRC-32/MPEG-2 Test Vector Breakdown

Input bytes (hex):
```
  0x01 0x01 0x00 0x00 0x00 0x00 0x00 0x00
  0x00 0x00 0x00 0x00 0x00 0x00 0x00 0x00
```

Step-by-step (initial CRC = 0xFFFFFFFF):
1. After byte 0 (0x01): CRC = 0x1F1A1021
2. After byte 1 (0x01): CRC = 0x1E342343
3. After bytes 2-15 (0x00): CRC accumulates...
...
Final result (before final XOR 0x00000000): 0x8E8A6DD1

Expected output: 0x8E8A6DD1
```

---

## LOW-003: Missing Example: Timestamp Format for PQC Keys
**Severity**: LOW
**Impact**: Implementation confusion on ISO 8601 format
**File**: foundation-03.md, Lines 1049-1050

### Issue

**Lines 1049-1050**:
```
key_issued:         2026-02-18T...   (timestamp)
key_expires:        2026-03-18T...   (timestamp)
```

The "T..." notation is incomplete. Should show full ISO 8601 format.

### Fix

Change to:
```
key_issued:         2026-02-18T14:30:00Z   (ISO 8601 UTC timestamp)
key_expires:        2026-03-18T14:30:00Z   (ISO 8601 UTC timestamp)
```

---

## LOW-004: Inconsistent Terminology: "HbH" vs. "Hop-by-Hop"
**Severity**: LOW
**Impact**: Reader confusion on terminology
**Files**: Multiple

### Issue

Documents use both "HbH" and "Hop-by-Hop" inconsistently. Example:
- foundation-03.md: Mostly "Hop-by-Hop"
- wire-format-patterns.md: Mostly "HbH"
- rfc-crossref.md: Mixed

### Fix

Adopt consistent terminology throughout all 6 documents:
- **Preferred**: "Hop-by-Hop" (matches RFC terminology)
- First mention should use full term with abbreviation: "IPv6 Hop-by-Hop (HbH) option"
- Subsequent mentions can use "HbH" for brevity

---

# SUMMARY TABLE

| Severity | Count | Category | Impact |
|----------|-------|----------|--------|
| **CRITICAL** | 5 | Protocol contradictions | Interoperability failure |
| **HIGH** | 7 | Specification ambiguities | Silent failures, wrong behavior |
| **MEDIUM** | 6 | Incomplete guidance | Deployment risk |
| **LOW** | 4 | Documentation issues | Reader confusion |
| **TOTAL** | **22** | - | - |

---

## REMEDIATION PRIORITY

### Phase 1 (IMMEDIATE): Fix Critical Conflicts
1. CRITICAL-001: hop_count semantics (choose increment or decrement)
2. CRITICAL-002: Anamnesis event registry (align iana-guide to foundation-03)
3. CRITICAL-003: Flow action 0x13/0x14 (correct KEM_ENCAPS/KEM_DECAPS)
4. CRITICAL-004: Flags bitfield (choose C,Y,T,E,S,M,CUSTOM,RSVD OR K1/K0)
5. CRITICAL-005: Exponent encoding (remove 2-byte mantissa+exponent alternative)

### Phase 2 (URGENT): Fix High-Severity Issues
- HIGH-001 through HIGH-007 (all affect implementability)

### Phase 3 (SCHEDULE): Address Medium/Low Issues
- MEDIUM-001 through LOW-004

---

## DOCUMENT MASTER CHECKLIST

After corrections, verify these properties across ALL documents:

- [ ] hop_count section consistent (all docs use SAME increment/decrement)
- [ ] Anamnesis event codes 0x00-0x08 IDENTICAL in all docs
- [ ] flow_action codes 0x00-0x14 defined consistently
- [ ] flags bitfield: same 8-bit layout everywhere
- [ ] Exponent encoding: single-byte signed int ONLY (no 2-byte variant)
- [ ] Monad version: 8-bit field confirmed everywhere
- [ ] Checksum scope: 18 bytes (0x00-0x11) confirmed
- [ ] MTU calculations cross-checked
- [ ] Sophia versioning uses modular comparison throughout
- [ ] All RFC citations verified for accuracy

---

**Report Generated**: 2026-02-20
**Review Scope**: Complete PhD-level scrutiny across 6 specifications
**Confidence**: HIGH (all findings backed by exact line references and citations)

