# RFC Patches: Monad Foundation Draft-04

This document specifies normative corrections and security enhancements to Monad Foundation RFC (assumed baseline: draft-03). All patches are presented as replacement text for specific sections, with justification from S21 assessment.

---

## Patch M1: Extend CRC Coverage to All 20 Header Bytes (or Replace with HMAC)

### Issue
Current draft: "CRC-32 covers first 8 bytes (magic + version + length fields)"

**Problem:** CRC covers only header metadata, not the header flags or flow label fields. An attacker can modify flags or flow_label without CRC detection.

**S21 Finding:** X2 (CRC scope mismatch between Monad and Wotan RFCs)

### Proposed Fix (Option A: Extend CRC)

**Replace Section 3.1 "Header Format and CRC":**

```
The Monad header is 20 bytes (fixed). The header format is:

  0                   1                   2                   3
  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |Version|  Flags  |    Flow Label (20 bits, part 1)         |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |    Flow Label (continued, part 2, 16 bits)  |  Reserved (8) |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |                  Packet Length (4 bytes)                      |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |                    CRC-32 (4 bytes)                           |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |              Sequence Number (4 bytes, if Flag set)           |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

The CRC-32 is computed over bytes 0-19 (all 20 header bytes), with the 
CRC field (bytes 12-15) zeroed during computation (replaced with 0x00000000).

Specifically:
  crc_value = crc32(header[0:20] with header[12:16] = 0x00000000)
  Store crc_value at header[12:16]

This ensures all header fields (version, flags, flow label, length, reserved, 
and optional sequence number) are integrity-protected.
```

**Justification:**
- Ensures integrity of flow_label field (20 bits), preventing collision-based attacks (LICH-009)
- Ensures integrity of flags field, preventing feature downgrade or capability bypass
- Provides protection against accidental corruption, not just malicious modification (CRC only, not HMAC)

### Proposed Fix (Option B: Replace with HMAC-SHA256)

**Alternative (stronger):**

```
Replace CRC-32 with HMAC-SHA256 for packets originating from provisioning node 
(authenticated sources). For untrusted sources, use CRC-32 for lighter-weight 
checking during network processing, then validate HMAC if further processing required.

HMAC-SHA256(key=session_key, message=header[0:20] || payload[0:payload_len])

This provides authenticity guarantees in addition to integrity, preventing replay 
and spoofing attacks.
```

**Justification:**
- Stronger integrity guarantee (256-bit MAC vs 32-bit CRC)
- Prevents forgery (CRC can be computed by anyone; HMAC requires shared key)
- Aligns with Sophia Dictionary (Section S2) which mandates HMAC for authenticated operations
- Defends against X2 (cross-document integrity checking divergence)

**Recommendation:** Implement Option A (extend CRC to 20 bytes) for compatibility with 
existing implementations. Document Option B as a future enhancement (e.g., draft-05).

---

## Patch M2: Mandate "MUST NOT Contain Multiple HbH Headers"

### Issue
Current draft: Silent on HbH (Hop-by-Hop) headers; no restriction on multiple headers

**Problem:** Attacker embeds multiple HbH headers with conflicting instructions, causing 
parser confusion and header smuggling attacks.

**S21 Finding:** X3 (TLV extension parsing rules divergence)

### Proposed Fix

**Add to Section 4.2 "Option Processing":**

```
4.2.1. Hop-by-Hop (HbH) Options

HbH options appear immediately after the Monad header, before Sophia dictionary 
or Wotan metadata. HbH options apply to this hop (intermediate node) and MUST be 
removed before forwarding.

CRITICAL: A Monad packet MUST NOT contain more than one HbH option. If multiple 
HbH options are present, the packet MUST be dropped immediately. No fallback parsing, 
no error code generation—immediate silent drop.

Rationale: Multiple HbH headers create parsing ambiguity. Some implementations may 
process first HbH header; others may process all HbH headers. This divergence enables 
header smuggling attacks (X3). Mandating exactly zero or one HbH header eliminates 
this ambiguity.

Recommendation: Packet generators SHOULD generate zero HbH headers for standard use cases. 
HbH headers are reserved for debugging and network path inspection.
```

**Processing Rule (formal):**

```
if count(HbH_headers_in_packet) > 1:
  drop_packet()  # No error code, no response
  return
```

**Test Case:**
- Packet with HbH header count = 0: PASS
- Packet with HbH header count = 1: PASS (process HbH, continue)
- Packet with HbH header count > 1: DROP (no error response)

---

## Patch M3: Mandate Kernel 5.17+ (not 5.15)

### Issue
Current draft: "Recommend Linux kernel 5.15+ for BPF eBPF program support"

**Problem:** Kernel 5.15 lacks several critical BPF features:
- Unreliable memory barriers (no full wmb/rmb semantics)
- Missing CAS (Compare-And-Swap) atomic operations
- No ringbuf exclusive reservation (bpf_ringbuf_reserve)
- Incomplete verifier bounds checking

Implementations on kernel 5.15 are vulnerable to LICH-008 (L1 cache race conditions) 
and LICH-010 (WAL atomicity bugs).

**S21 Finding:** D4 (Load-Store Unit Race Conditions), LICH-008, LICH-010

### Proposed Fix

**Replace Section 2.1 "System Requirements":**

```
2.1. System Requirements

Unheaded protocol implementation MUST run on Linux kernel 5.17 or later.

Kernel 5.17 (released March 2022) introduced:
- Full memory barrier semantics (wmb, rmb, mb) in BPF programs
- CAS (Compare-And-Swap) atomic operations (__sync_compare_and_swap)
- Ringbuf exclusive reservation mode (bpf_ringbuf_reserve with 
  BPF_RB_EXCLUSIVE_RING flag)
- Enhanced BPF verifier with stack bounds checking
- Support for 64-bit atomic operations on all platforms

Kernel 5.15 or earlier MUST NOT be used for production deployments. Older 
kernels lack these features and are vulnerable to data races, memory corruption, 
and privilege escalation attacks (see Dark Grimoire Section 5).

**Rationale:** Older kernel versions have incomplete BPF infrastructure. 
Mandating 5.17+ ensures all implementations have access to critical synchronization 
primitives and security hardening in the BPF verifier.
```

**Impact:**
- Forces deprecated deployments to upgrade
- Aligns with Linux LTS kernel support (5.15 reaches EOL in 2023)
- Enables LICH-008 and LICH-010 test requirements (use newer BPF atomic ops)

---

## Patch M4: Add Wotan Helper Bounds Checking Specification

### Issue
Current draft: Wotan helpers (`bpf_wotan_read`, `bpf_wotan_write`) documented informally; 
bounds checking left to implementation.

**Problem:** Each implementation defines bounds checking differently, leading to divergent 
security posture. Some implementations silently clamp addresses; others drop packets; 
still others crash.

**S21 Finding:** D1 (Instruction Decoding Vulnerabilities), LICH-007

### Proposed Fix

**Add new Section 5.2 "Wotan Helper Bounds Checking":**

```
5.2. Wotan Helper Bounds Checking

Wotan helpers allow eBPF programs to read/write to L1 cache and WAL structures. 
All accesses MUST be bounds-checked to prevent out-of-bounds memory access.

5.2.1. bpf_wotan_read(key: u64, offset: u16) -> u64

Parameters:
  - key: 64-bit cache key (flow_label + src/dst hash, see Wotan Memory RFC)
  - offset: byte offset within cache line (0-63)

Return value: 64-bit value read from cache, or BPF_WOTAN_ERROR on error

Bounds checking:
  - Offset MUST be in range [0, 63]. If offset > 63 or offset+8 > 64, 
    return BPF_WOTAN_ERROR (value 0xFFFFFFFFFFFFFFFF).
  - Key MUST correspond to a valid cache line for current flow. If key 
    not in cache, return 0 (zero-fill).

Atomicity:
  - Read is atomic: 64-bit value is read with acquire semantics (no 
    reordering of subsequent operations before this read completes).

5.2.2. bpf_wotan_write(key: u64, offset: u16, value: u64) -> i32

Parameters:
  - key: 64-bit cache key
  - offset: byte offset within cache line (0-63)
  - value: 64-bit value to write

Return value: 0 on success, BPF_WOTAN_ERROR (-1) on error

Bounds checking:
  - Offset MUST be in range [0, 56] (to ensure write does not exceed 
    64-byte cache line: offset + 8 <= 64).
  - If offset > 56, return -1 (BPF_WOTAN_ERROR).
  - Key MUST correspond to valid cache line. If not, allocate new cache 
    line (if space available) or evict LRU entry.

Atomicity:
  - Write is atomic: 64-bit value is written with release semantics 
    (all prior operations complete before write).
  - Cache line is invalidated after write (subsequent reads see new value, 
    not stale cached value).

Memory Ordering:
  - bpf_wotan_read() provides acquire semantics (LoadLoad + LoadStore barriers)
  - bpf_wotan_write() provides release semantics (StoreStore + LoadStore barriers)
  - This provides sequential consistency for data race detection (TSan).

Error Codes:
  - BPF_WOTAN_ERROR = -1 (or 0xFFFFFFFFFFFFFFFF for read return)
  - Do not use exception/signal; return error code in return value

5.2.3. Test Requirements

LICH-008 must verify:
  - Out-of-bounds offset raises error (no crash, no silent clamp)
  - Concurrent access with different keys does not race
  - Concurrent access with same key is exclusive or uses CAS
  - Memory barriers prevent stale reads/writes
```

**Justification:**
- Standardizes bounds checking across all implementations
- Defines error behavior (no crashes, no undefined behavior)
- Specifies memory ordering (prevents LICH-008 race conditions)
- Enables LICH-008 and D4 testing

---

## Patch M5: Strengthen Version Field Behavior - "MUST Drop Immediately, NO Fallback"

### Issue
Current draft: "Implementations MAY reject unknown versions or fall back to compatible version"

**Problem:** Differing behavior creates X4 (error handling divergence). Some implementations 
reject version N; others parse with version N-1 compatibility shim. Leads to parser 
divergence and security feature bypass.

**S21 Finding:** X4 (Error Handling and Fallback Divergence)

### Proposed Fix

**Replace Section 3.2 "Version Field":**

```
3.2. Version Field

The 4-bit version field encodes the Monad Foundation RFC version. Current version is 1.

Version Field Semantics:
  - Version 1: Draft-04 (this RFC)
  - Version 0: Reserved (must not be used)
  - Version 2+: Future versions (currently undefined)

Version Checking (NORMATIVE):

On receiving a packet with version field V:

  if V != 1:
    DROP_IMMEDIATELY()
    # No error code, no response, no fallback
    # Do not process packet further
    # Logging is optional (implementation choice)
  else:
    continue_processing()

CRITICAL REQUIREMENT: Implementations MUST NOT implement version 
negotiation, fallback, or downgrade. A version mismatch causes immediate 
packet rejection, period.

Rationale: Version fallback creates security confusion (X4). By mandating 
strict version checking with no fallback, all implementations behave 
identically: unsupported version always means drop. This eliminates parser 
divergence attacks.

Future RFC Versions:

When RFC advances to draft-05 (version 2), new version MUST be supported 
explicitly. No automatic fallback from v2 to v1. Deployments must upgrade 
both sender and receiver.

Corollary: Two deployments using different Monad versions cannot 
interoperate. This is acceptable; version upgrade is planned maintenance, 
not a surprise compatibility requirement.
```

**Test Case:**
```
# Test 1: Version 1 packet -> PROCESS
send_packet(version=1, valid_headers) -> expect processing

# Test 2: Version 0 packet -> DROP
send_packet(version=0, valid_headers) -> expect drop, no response

# Test 3: Version 2 packet -> DROP
send_packet(version=2, valid_headers) -> expect drop, no response

# Expected: All implementations reject version != 1 identically
```

---

## Patch M6: Add TLV Extension Mechanism Section

### Issue
Current draft: No formal TLV extension mechanism; only Sophia and Wotan RFCs 
define ad-hoc extensions.

**Problem:** Leads to X3 (TLV parsing rules divergence). No unified mechanism 
for defining new TLV types, critical bits, or handling of unknown types.

**S21 Finding:** X3 (TLV Extension Parsing Rules Divergence)

### Proposed Fix

**Add new Section 6 "TLV Extension Mechanism":**

```
6. TLV (Type-Length-Value) Extension Mechanism

TLV extensions allow Monad packets to carry optional metadata from Sophia, 
Wotan, and future protocol extensions.

6.1. TLV Container Format

TLV options are appended after the 20-byte Monad header, before the 
Sophia dictionary or payload. Format:

  0                   1                   2                   3
  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |Type|C|  Length  |                 Value...                    |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

  Type: 7 bits (0-127), identifies TLV type
  C: 1 bit, critical bit (1 = must understand, 0 = can ignore)
  Length: 8 bits, length of value in bytes (0-255)
  Value: variable-length value

6.2. TLV Type Registry

Allocated TLV types (with critical bit 0, i.e., type = base_type):

  Base Type 0x00-0x1F: Reserved for Monad Foundation (future use)
  Base Type 0x20-0x3F: Sophia Dictionary extensions
  Base Type 0x40-0x5F: Wotan Memory extensions
  Base Type 0x60-0x7F: Future protocol extensions

Critical Bit:
  Type_with_critical = base_type (C=0, can ignore) 
               or base_type | 0x80 (C=1, must understand)

Example:
  - Type 0x20 (C=0): Sophia TLV, optional (can skip if not understood)
  - Type 0xA0 (C=1): Sophia TLV, critical (must understand or drop packet)

6.3. Unknown TLV Handling

When processing TLVs in a packet:

  for tlv in packet.tlvs:
    if tlv.type not in KNOWN_TYPES:
      if tlv.critical_bit == 1:
        DROP_PACKET()  # Critical TLV not understood
      else:
        SKIP_TLV()     # Optional TLV, skip and continue
    else:
      PROCESS_TLV(tlv)

This ensures:
  - Critical extensions cannot be silently ignored
  - Optional extensions degrade gracefully
  - No parser divergence on unknown types

6.4. Extension Registration Process

To register a new TLV type:

  1. RFC author allocates type N from appropriate range (0x00-0x7F)
  2. Document interpretation: critical vs. optional
  3. Specify value format (length constraints, field definitions)
  4. Update Monad RFC with allocation table entry
  5. IETF consensus required for allocations

New TLV types MUST NOT overlap with existing allocations.
```

**Justification:**
- Centralizes TLV mechanism in Monad RFC (authoritative source)
- Defines critical bit semantics (prevents X3 parsing divergence)
- Establishes allocation ranges (prevents overlaps)
- Provides clear registration process (prevents squatting)

---

## Patch M7: Add Error Code Field to Wire Format

### Issue
Current draft: No error code field in packet; error codes only used in 
control messages (informal).

**Problem:** When dropping packets due to validation failures, implementations 
have no standard way to communicate error reason. This hampers debugging and 
logging.

**S21 Finding:** Section 3 (IANA Registry Attacks), X4 (Error Handling Divergence)

### Proposed Fix

**Modify Section 3.1 "Header Format":**

```
Add optional error code field to Monad header:

  0                   1                   2                   3
  0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1 2 3 4 5 6 7 8 9 0 1
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |Version|  Flags  |    Flow Label (20 bits, part 1)         |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |    Flow Label (continued, part 2, 16 bits)  | Err Code (8) |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |                  Packet Length (4 bytes)                      |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |                    CRC-32 (4 bytes)                           |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+
 |              Sequence Number (4 bytes, if Flag set)           |
 +-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+-+

Error Code (8 bits):
  0x00: No error (data packet)
  0x01: CRC validation failed
  0x02: Version not supported
  0x03: Flow label invalid
  0x04: Arithmetic operation overflow
  0x05: Wotan helper bounds check failed
  0x06: HbH option count > 1 (multiple HbH headers)
  0x07: Unknown critical TLV
  0x08: WAL seqno discontinuity detected
  0x09: Insufficient buffer space
  0x0A: Flow state corruption
  0x0B: TLS handshake failure
  0x0C: QUIC version mismatch
  0x0D: Reserved
  0x0E-0x1E: Unallocated (IANA)
  0x1F-0xFF: Private use (testing/greasing)

Error codes are set by the sender if a validation failure is detected. 
Receivers MAY use error codes for logging and diagnostics.

Note: Error codes are not used for recovery (no retry logic based on code).
```

**Justification:**
- Provides standardized error reporting (aligns with IANA registry)
- Improves observability and debugging
- Prevents X4 (error handling divergence) by defining codes normatively

---

## Patch M8: Add Ring Path Counter Field

### Issue
Current draft: Ring buffer path tracking is informal; no standard counter 
in packet format.

**Problem:** Implementations cannot standardly track which ring path a packet 
traversed, making flow analysis and traffic engineering difficult.

**S21 Finding:** Cross-Document Consistency (Wotan WAL seqno ordering)

### Proposed Fix

**Add optional ring path counter to TLV extensions (Patch M6):**

```
New TLV Type: 0x01 (Monad Foundation, optional)
Name: Ring Path Counter
Format:
  Type: 0x01
  Critical: 0 (optional)
  Length: 4 bytes
  Value: 32-bit counter (path traversal count)

Semantics:
  Counter increments by 1 each time packet traverses a ring node 
  (Wotan L1 cache, WAL, etc.). Initial value: 0.

  Implementations MAY ignore this TLV if not tracking ring paths. 
  Implementations tracking paths MUST increment counter at each ring node.

  Maximum counter value: 2^32 - 1. If counter would exceed this, 
  implementation SHOULD drop packet (prevent integer overflow).

Use Cases:
  - Detect loops (counter > N indicates packet looping, drop immediately)
  - Traffic engineering (routing decisions based on path depth)
  - Performance analysis (latency vs. ring path count correlation)
```

**Justification:**
- Enables ring path tracking without changing core header format
- Provides mechanism for loop detection
- Aligns with Wotan Memory RFC (Section 4: ring path ordering)

---

## Summary

All patches address security findings from S21 assessment:

| Patch | Issue | S21 Finding | Impact |
|-------|-------|-------------|--------|
| M1 | CRC scope mismatch | X2 | Extends integrity check to all header fields |
| M2 | Multiple HbH headers | X3 | Eliminates header smuggling vector |
| M3 | Weak kernel version requirement | LICH-008, LICH-010 | Forces deployment on secure BPF kernel |
| M4 | Informal Wotan helper bounds | D1, LICH-007 | Standardizes bounds checking across implementations |
| M5 | Version fallback enables downgrade | X4 | Eliminates version confusion attacks |
| M6 | No formal TLV mechanism | X3 | Centralizes extension mechanism, prevents divergence |
| M7 | No standard error codes | IANA Registry, X4 | Provides observability and diagnostics |
| M8 | No ring path tracking | Wotan integration | Enables loop detection and traffic analysis |

