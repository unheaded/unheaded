# IANA Error Code Registry

**Last Updated**: February 27, 2026 (S72 Phase 4)
**Status**: RFC v1.0-draft — aligned with draft-bellis-unheaded-protocol-foundation-05

This document specifies the authoritative error code registry for the Unheaded protocol. Error codes are used in packet headers, control messages, and diagnostic logs to communicate failure modes and conditions. All 13 normative codes (0x00-0x0C) are formalized and cross-referenced with protocol patches M1-M8.

---

## Section 1: Error Code Registry

### Overview

The Unheaded protocol defines 13 normative error codes (0x00-0x0C) plus reserved and private-use ranges. All codes are 8-bit unsigned integers.

### Error Code Table

| Code | Level | Name | Description | Recommended Action |
|------|-------|------|-------------|-------------------|
| 0x00 | N/A | NO_ERROR | Normal operation, no error | Continue processing |
| 0x01 | Flow | CRC_VALIDATION_FAILED | CRC-32 or HMAC validation failed on packet header or WAL entry | Drop packet, log event, optionally alert operator |
| 0x02 | Domain | VERSION_NOT_SUPPORTED | Packet version field does not match implementation version | Drop packet, no response (silence is correct) |
| 0x03 | Flow | FLOW_LABEL_INVALID | Flow label value is out of range or not allocated | Drop packet, return error response (optional) |
| 0x04 | Flow | ARITHMETIC_OVERFLOW | Arithmetic operation in bytecode or helper overflowed (e.g., branch offset, address calculation) | Reject operation, return error to program, do not crash |
| 0x05 | Flow | WOTAN_HELPER_BOUNDS_CHECK_FAILED | Wotan helper (`bpf_wotan_read`, `bpf_wotan_write`) detected out-of-bounds access | Return error from helper, do not access memory |
| 0x06 | Domain | MULTIPLE_HBH_HEADERS | Packet contains more than one HbH (Hop-by-Hop) header | Drop packet immediately, no error response |
| 0x07 | Domain | UNKNOWN_CRITICAL_TLV | Unknown TLV type with critical bit set (C=1) | Drop packet, return error code in response (optional) |
| 0x08 | Domain | WAL_SEQNO_DISCONTINUITY | WAL (Write-Ahead Log) entry sequence numbers have gap (missing entries) | Log alert, replay from last known good seqno, may lose data |
| 0x09 | System | INSUFFICIENT_BUFFER_SPACE | Dictionary full (128 entries or 1MB limit exceeded), or global dictionary at 100MB limit | Reject new entry, return error to client |
| 0x0A | Flow | FLOW_STATE_CORRUPTION | Flow state is inconsistent (e.g., WAL seqno mismatch, L1 cache coherency violation) | Drop flow, close connection, log critical alert |
| 0x0B | Domain | TLS_HANDSHAKE_FAILURE | TLS 1.3 handshake failed (invalid certificate, cipher mismatch, etc.) | Close connection, log error, do not retry immediately |
| 0x0C | Domain | QUIC_VERSION_MISMATCH | QUIC version in packet does not match negotiated version | Drop packet, close connection (HTTP/3 integration) |
| 0x0D | N/A | RESERVED | Reserved for future use | Do not use in current implementations |
| 0x0E-0x1E | N/A | UNALLOCATED | Unallocated (IANA expert review required for allocation) | Reserved for future expansion |
| 0x1F | N/A | GREASING_0 | Greasing/testing value (see Section 3) | Implementations MUST accept and ignore gracefully |
| 0x20-0xFE | N/A | PRIVATE_USE | Private/experimental use (not standardized) | Implementation-defined behavior |
| 0xFF | N/A | GREASING_1 | Greasing/testing value (see Section 3) | Implementations MUST accept and ignore gracefully |

### Error Level Hierarchy

Errors are categorized by scope:

- **Flow**: Affects single flow (identified by flow_label). Close flow, continue other flows.
- **Domain**: Affects all flows on same node (connection). Close connection, may reconnect.
- **System**: Affects entire Unheaded protocol instance. May require restart or resource reclamation.

### Error Code Semantics

#### 0x00: NO_ERROR
No error condition. Used in normal packet headers (error_code field is 0x00). No action required; packet is valid.

#### 0x01: CRC_VALIDATION_FAILED
Integrity check (CRC-32 or HMAC-SHA256) failed on packet header or WAL entry. Indicates either:
- Accidental bit flip in transit (network corruption)
- Hardware memory error (bit rot in storage)
- Intentional tampering (attacker modified packet)

**Recommended Action:**
- Drop packet immediately
- Log event: "CRC validation failed on packet from [source]"
- If repeated failures from same source: alert operator, consider blocking source
- Increment metric: `crc_failures_total`

#### 0x02: VERSION_NOT_SUPPORTED
Monad Foundation RFC version field is not 1 (current version). Indicates:
- Sender uses newer/older Monad RFC version
- No fallback or compatibility shim available
- Implementations MUST drop packet without responding

**Recommended Action:**
- Drop packet silently (no error response)
- Log event (optional): "Version mismatch: expected 1, got X"
- Do NOT attempt version negotiation or downgrade
- Do NOT send error response (creates feedback loop if peer also rejects)

#### 0x03: FLOW_LABEL_INVALID
Flow label field contains invalid value. Indicates:
- Flow label is 0x00000 (reserved, invalid)
- Flow label exceeds 0xFFFFF (20-bit overflow)
- Flow label is not allocated to requesting program

**Recommended Action:**
- Drop packet
- Log event: "Invalid flow label: 0x[hex]"
- Optionally send error response to sender
- Reject new flow creation with this label

#### 0x04: ARITHMETIC_OVERFLOW
Arithmetic operation (in bytecode or BPF helper) overflowed. Examples:
- Branch offset calculation: offset + PC overflows i32 range
- Address computation: base_address + length overflows memory space
- Dictionary size addition: current_size + new_entry_size overflows 1MB limit

**Recommended Action:**
- Do NOT crash or raise exception
- Return error code to calling program or helper
- Program must check return value and handle error
- Log event: "Arithmetic overflow in [operation]"
- Do NOT execute operation if overflow detected

#### 0x05: WOTAN_HELPER_BOUNDS_CHECK_FAILED
Wotan helper (`bpf_wotan_read` or `bpf_wotan_write`) detected out-of-bounds access. Indicates:
- Offset parameter is >= 64 (exceeds 64-byte cache line)
- Key parameter does not correspond to allocated cache line
- Helper cannot proceed safely

**Recommended Action:**
- Return error from helper (do not access memory)
- `bpf_wotan_read()` returns 0xFFFFFFFFFFFFFFFF (error sentinel)
- `bpf_wotan_write()` returns -1 (BPF_WOTAN_ERROR)
- Program must check return value
- Log event: "Wotan helper bounds check failed: offset=X, key=Y"

#### 0x06: MULTIPLE_HBH_HEADERS
Packet contains more than one Hop-by-Hop (HbH) header. Indicates:
- Attacker crafted packet with multiple HbH options (header smuggling attack)
- Parser could be confused (process first or all headers?)

**Recommended Action:**
- Drop packet immediately
- Do NOT respond with error (no GOAWAY, no error code)
- Log event (optional): "Multiple HbH headers detected"
- Increment metric: `invalid_packets_total`

#### 0x07: UNKNOWN_CRITICAL_TLV
Received TLV (Type-Length-Value) extension with unknown type and critical bit set (C=1). Indicates:
- Sender used newer protocol extension (not understood by receiver)
- Cannot safely process packet (critical extension required)

**Recommended Action:**
- Drop packet
- Log event: "Unknown critical TLV type: 0x[hex]"
- Optionally send GOAWAY with error code 0x07
- Close connection (domain-level error)
- Optionally alert operator: "Protocol version mismatch detected"

#### 0x08: WAL_SEQNO_DISCONTINUITY
Write-Ahead Log entry sequence numbers have a gap (missing entries). Indicates:
- WAL compaction lost entries (data loss)
- Power failure during WAL write (corruption)
- Out-of-order entries (compaction reordered WAL)

**Recommended Action:**
- Log critical alert: "WAL seqno gap: expected N, got M (lost X entries)"
- Replay WAL up to last known good seqno
- Data loss is likely (inform application)
- Increment metric: `wal_corruption_events_total`
- Consider notifying operator (data integrity compromised)

#### 0x09: INSUFFICIENT_BUFFER_SPACE
Buffer space exhausted. Examples:
- Dictionary full: 128 entries per-flow limit reached, or 1MB per-flow limit reached
- Global dictionary at 100MB limit
- BPF memory quota exceeded (1MB per-program limit)

**Recommended Action:**
- Reject new entry/allocation
- Return error to requestor
- Log event: "Buffer space exhausted: [resource]"
- Provide metrics: current_size, limit, available
- Application may retry after freeing resources (e.g., closing old flows)

#### 0x0A: FLOW_STATE_CORRUPTION
Flow state is corrupted or inconsistent. Examples:
- WAL seqno mismatch: expected monotonic seqno but observed gap
- L1 cache coherency violation: read stale data from cache
- Flow metadata inconsistent (version mismatch, CRC failure)

**Recommended Action:**
- Log critical alert: "Flow state corruption detected: [flow_id]"
- Close flow immediately
- Increment metric: `flow_corruption_events_total`
- Do NOT attempt recovery (corruption is severe)
- Alert operator (may indicate hardware issue or attack)

#### 0x0B: TLS_HANDSHAKE_FAILURE
TLS 1.3 handshake failed. Examples:
- Invalid server certificate (signature verification failed)
- Cipher suite mismatch (client and server disagree)
- Incompatible TLS version (client wants v1.2, server only supports v1.3)

**Recommended Action:**
- Close connection
- Log event: "TLS handshake failed: [reason]"
- Do NOT retry immediately (exponential backoff if retrying)
- Optionally alert operator (possible misconfiguration or attack)

#### 0x0C: QUIC_VERSION_MISMATCH
QUIC version (HTTP/3 integration) does not match negotiated version. Indicates:
- QUIC version negotiation failed
- Packet uses wrong QUIC version (no migration allowed)

**Recommended Action:**
- Drop packet
- Log event: "QUIC version mismatch: expected X, got Y"
- Close connection (domain-level error)
- Do NOT send QUIC version negotiation response (prevents amplification attacks)

---

## Section 2: Allocation Policy and Guidelines

### IANA Allocation Procedure

New error codes (0x0E-0x1E) are allocated via IETF IANA registry using **Expert Review** process:

1. **Request Submission:**
   - RFC author submits request to IANA
   - Provide proposed error code(s), name, description, recommended action
   - Designate expert reviewer (IETF working group chair or senior expert)

2. **Expert Review:**
   - Reviewer examines request for:
     - Clarity of description (unambiguous semantics)
     - Orthogonality to existing codes (no overlap)
     - Practical applicability (not theoretical)
   - Reviewer may request revisions
   - Approval timeline: 2-4 weeks

3. **Allocation:**
   - IANA updates error code registry
   - RFC moves to draft, then RFC
   - Code becomes normative (implementations MUST support)

### Reservation Policy

Ranges are reserved as follows:

- **0x00-0x0D**: Normative (MUST support all codes)
- **0x0E-0x1E**: Unallocated (available for allocation via expert review)
- **0x1F, 0xFF**: Greasing/testing (reserved, see Section 3)
- **0x20-0xFE**: Private use (implementation-defined, not standardized)

### Change Control

- **0x00-0x0D**: Standards action (RFC required to add or remove codes)
- **0x0E-0x1E**: Expert review (expert approval required)
- **0x1F, 0xFF**: Fixed (cannot be reassigned; reserved for testing)
- **0x20-0xFE**: Uncontrolled (implementations may use freely)

---

## Section 3: Testing and Greasing

### Greasing Values (0x1F, 0xFF)

Greasing values are reserved for testing and connection probing. Implementations MUST:

1. **Accept greasing values gracefully:**
   - Parse packet with error_code = 0x1F or 0xFF
   - Treat as "unknown error" (proceed with caution)
   - Log event (optional): "Received greasing error code"

2. **Do NOT interpret greasing values normatively:**
   - Greasing codes have no defined semantics
   - Do NOT trigger error recovery, connection closure, or alerts
   - Greasing is for network fingerprinting detection (see RFC 8701)

3. **Emit greasing values occasionally:**
   - Implementations MAY send error_code = 0x1F or 0xFF with 0.1% probability
   - Greasing packets look like error conditions but are benign
   - Helps detect middleboxes that drop "error" packets

### Pattern-Based Greasing

Additional greasing pattern: 0x1F * N + 0x1F (for N = 0, 1, 2, ...)

- 0x1F (1F * 1 + 1F = 1F): Valid greasing code
- 0x3E (1F * 2 + 1E... wait, pattern is 1F*N + 21): Actually 0x40 (1F * 2 + 1F = 40... let me recalculate)

**Correct pattern:** For spacing greasing codes across 8-bit range:
- 0x1F: Greasing value #0
- 0x3E: (0x1F * 2) = reserved for future greasing #1
- 0x5D: (0x1F * 3) = reserved for future greasing #2
- 0x7C: (0x1F * 4) = reserved for future greasing #3

(These are calculated as 0x1F, then incrementing by 0x1F: 0x1F, 0x3E, 0x5D, 0x7C)

Usage: When probing for protocol support, test with both standard codes and greasing values. If peer accepts greasing value without error, it implements proper code handling.

---

## Section 4: Private-Use Range (0x20-0xFE)

Codes in range 0x20-0xFE are reserved for private/experimental use. Implementations may:

- Define custom error codes for proprietary extensions
- Use for vendor-specific error reporting
- Not register with IANA (no approval required)

**Constraint:** Do not send private-use codes to peers unless:
- Peer has explicitly negotiated support for private code
- Documented in private protocol extension
- Graceful degradation if peer doesn't understand

---

## Section 5: Implementation Guidance

### Sending Error Codes

1. **Set error_code in Monad header:**
   ```
   struct MonadHeader {
     ...
     error_code: u8,  ; 0x00 for success, other values for errors
     ...
   }
   ```

2. **Choose appropriate code:**
   - Use most specific code (e.g., 0x05 for Wotan bounds check, not generic 0x0A)
   - Include error_code in all responses (even if no data payload)

3. **Log and alert:**
   - Error code > 0x00: log event with level INFO or WARN
   - Error code in {0x08, 0x0A}: log level CRIT (data loss or corruption)
   - Optionally send alert to monitoring system

### Receiving Error Codes

1. **Check error_code immediately:**
   ```
   pkt = receive_packet()
   if pkt.error_code != 0x00:
     handle_error(pkt.error_code)
     return
   ```

2. **Handle by level:**
   - Flow level (0x01-0x05, 0x09): close flow, continue other flows
   - Domain level (0x02, 0x03, 0x06, 0x07, 0x08, 0x0B, 0x0C): close connection
   - System level (0x0A): restart may be needed

3. **Metrics and monitoring:**
   - Track error code distribution: `errors_by_code[code]++`
   - Alert on sustained error rate (>1% errors)
   - Correlate errors with network conditions

### Testing Error Codes

1. **Unit tests:**
   - For each error code, inject packet with that code
   - Verify implementation handles gracefully (no crash)
   - Verify appropriate action is taken (log, close, etc.)

2. **Integration tests:**
   - Simulate network corruption (set wrong CRC -> error 0x01)
   - Simulate version mismatch (set wrong version -> error 0x02)
   - Simulate dictionary overflow (reject entry -> error 0x09)

3. **Fuzz testing:**
   - Send error codes 0x00-0xFF (all values)
   - Verify graceful handling (no crash on unknown codes)
   - Verify metrics are updated correctly

---

## Section 6: Examples

### Example 1: CRC Failure

```
Packet received with CRC-32 validation failure:

Action (Receiver):
  1. Detect CRC mismatch during header parsing
  2. Set error_code = 0x01 in response (if sending error packet)
  3. Drop packet (do not process further)
  4. Log: "error_code=0x01 (CRC_VALIDATION_FAILED)"
  5. Metrics: crc_failures_total += 1
  6. If crc_failures_total > 100 in last minute: alert operator
```

### Example 2: Flow Label Invalid

```
Packet with invalid flow_label:

Action (Receiver):
  1. Parse Monad header
  2. Extract flow_label (20 bits)
  3. If flow_label not in allocated set: error
  4. Set error_code = 0x03 in response
  5. Drop packet
  6. Log: "error_code=0x03 (FLOW_LABEL_INVALID): label=0x[hex]"
```

### Example 3: Dictionary Full

```
Application requests new dictionary entry but per-flow limit (128) exceeded:

Action (Wotan implementation):
  1. Check: current_entries >= 128? Yes.
  2. Reject allocation
  3. Return error_code = 0x09 to application
  4. Application receives error, can retry after evicting old entries
  5. Log: "error_code=0x09 (INSUFFICIENT_BUFFER_SPACE): per-flow limit"
```

### Example 4: WAL Corruption

```
WAL replay detects seqno gap:

Action (WAL reader):
  1. Read entries in order: seqno 0, 1, 2, ... 99, 101 (gap at 100)
  2. Seqno mismatch: expected 100, got 101
  3. Set error_code = 0x08
  4. Replay stops at last good seqno (99)
  5. Log (CRIT): "error_code=0x08 (WAL_SEQNO_DISCONTINUITY): gap at seqno 100"
  6. Metrics: wal_corruption_events_total += 1
  7. Alert operator: "Data loss detected in WAL"
```

---

## Section 7: Cross-Reference with RFC Patches

Error codes are integrated with security patches from S21 assessment:

| Error Code | RFC Patch | Security Finding | Reason |
|------------|-----------|------------------|--------|
| 0x01 | M1 (Monad CRC) | X2 (CRC scope) | Extended CRC coverage, detects tampering |
| 0x02 | M5 (Version field) | X4 (Error handling) | Version mismatch drops immediately |
| 0x03 | M4 (Wotan bounds) | D1, LICH-007 | Flow label validation |
| 0x04 | M1, M4 | D6 (Integer overflow) | Arithmetic operation errors |
| 0x05 | M4 (Wotan bounds) | D1, LICH-007 | Wotan helper bounds checking |
| 0x06 | M2 (Multiple HbH) | X3 (TLV parsing) | Header smuggling prevention |
| 0x07 | M6 (TLV extension) | X3 (Critical TLV) | Unknown critical TLV rejection |
| 0x08 | W1 (WAL seqno) | LICH-010 | WAL integrity validation |
| 0x09 | S2, S7 (Dictionary limits) | Dark Grimoire Section 4 | Resource exhaustion prevention |
| 0x0A | Dark Grimoire Section 4 | Concurrency | Flow state corruption detection |
| 0x0B | HTTP/3 integration | Cross-pollination | TLS handshake failure |
| 0x0C | W7 (SETTINGS) | X2 | QUIC version negotiation |

---

## Appendix: IANA Registry Request Template

To request allocation of new error codes (0x0E-0x1E), submit using this template:

```
Title: Allocation of Error Code 0x0E for [NAME]

Requested Code: 0x0E

Error Name: [ERROR_NAME]

Description:
[1-2 sentence description of when this error condition occurs]

Recommended Action:
[How implementations SHOULD respond to this error]

Level:
[Flow / Domain / System]

Use Case:
[Why is this error code needed? What scenarios trigger it?]

Deployment Impact:
[Will existing implementations be affected by this allocation?]

Reference:
[RFC section or external document describing this condition]
```

Submit to IANA registry contact for Unheaded protocol (maintained by IETF working group).

