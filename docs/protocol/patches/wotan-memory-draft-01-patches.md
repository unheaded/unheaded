# RFC Patches: Wotan Memory Draft-01

This document specifies normative corrections and security enhancements to the Wotan Memory RFC. All patches address findings from S21 assessment.

---

## Patch W1: Add Seqno to Ring Buffer Entries + Monotonicity Validation

### Issue
Current draft: Ring buffer entries do not carry sequence numbers; ordering is implicit via write order.

**Problem:** Without explicit seqnos, out-of-order entries cannot be detected. 
If WAL compaction reorders entries, or if packets arrive out-of-order, monotonicity 
is violated silently. LICH-010 detects this as data loss.

**S21 Finding:** LICH-010 (WAL Integrity / Compaction Race Testing), D4 (Load-Store Unit Race Conditions)

### Proposed Fix

**Modify Section 3.2 "Ring Buffer Format":**

```
3.2. Ring Buffer Format

Each ring buffer entry (WAL record) MUST contain a 64-bit sequence number (seqno):

  struct RingEntry {
    seqno: u64,           ; monotonically increasing sequence number
    timestamp: u64,       ; creation timestamp (Unix epoch, nanoseconds)
    data_len: u16,        ; length of data payload
    data: [u8; data_len], ; payload (variable-length)
    crc32: u32,           ; CRC-32 checksum (covers seqno + timestamp + data_len + data)
  }

3.2.1. Seqno Semantics

The seqno field is initialized to 0 for the first entry and increments by 1 
for each subsequent entry. Seqno is assigned by the WAL writer at time of entry 
creation.

  entry_N.seqno = entry_(N-1).seqno + 1
  entry_0.seqno = 0

3.2.2. Monotonicity Validation

On reading ring buffer entries, implementations MUST validate monotonicity:

  last_seqno = -1
  for entry in ring_entries:
    if entry.seqno != last_seqno + 1:
      # Seqno gap detected
      raise SeqnoDiscontinuityError()
    last_seqno = entry.seqno
    validate_crc32(entry)

If seqno discontinuity is detected:
  - WAL is corrupted (compaction lost entries, or power failure during write)
  - Replay to last known good seqno
  - Log alert: "WAL seqno gap: expected N, got M"
  - Return error 0x08 (WAL seqno discontinuity detected)

3.2.3. CRC32 Computation

CRC-32 is computed over: [seqno (8 bytes) | timestamp (8 bytes) | data_len (2 bytes) | data]

This ensures all ring entry fields are integrity-protected (unlike earlier 
Monad CRC which covered only header).

3.2.4. Rationale

Explicit seqno enables:
- Corruption detection (missing entries detected via gap)
- Replay idempotency (program can skip duplicate seqnos if retransmitted)
- Recovery point calculation (restart from seqno N instead of last written offset)
- Concurrency validation (detect out-of-order compaction)

Aligns with LICH-010 success criteria: "WAL seqno monotonicity guaranteed for all test runs"
```

**Justification:**
- Detects data loss via seqno gaps
- Enables crash-recovery validation
- Prevents silent corruption from out-of-order entries

---

## Patch W2: Extend L1 Key to Composite (Flow + Src/Dst Hash)

### Issue
Current draft: L1 cache key is flow_label (20 bits) only.

**Problem:** 20-bit key provides only 1M distinct values. Birthday attack (LICH-009) 
forces collisions, enabling cache coherency violations and flow isolation bypass.

**S21 Finding:** LICH-009 (Cross-Flow Composite Key Collision Testing), D5 (Privilege Escalation)

### Proposed Fix

**Modify Section 4.1 "L1 Cache Key Derivation":**

```
4.1. L1 Cache Key Derivation

The L1 cache key is a composite of flow_label and src/dst tuple hash:

  L1_key = (flow_label << 44) | (tuple_hash & 0xFFFFFFFFFFF)
  # 20 bits (flow_label) + 44 bits (tuple_hash) = 64 bits

Where:
  - flow_label: 20-bit value from Monad header
  - tuple_hash: 44-bit hash of (src_ip, dst_ip, src_port, dst_port)

4.1.1. Tuple Hash Computation

The tuple hash is computed as:

  tuple_hash = (
    siphash2_4(
      key=flow_label || <16 random bytes per connection>,
      data=src_ip || dst_ip || src_port || dst_port
    ) & 0xFFFFFFFFFFF  # Mask to 44 bits
  )

SipHash-2-4 is selected for:
- Speed (competitive with 32-bit hash on CQARMA)
- Collision resistance (cryptographic quality for 44-bit output)
- Simplicity (no modular multiplication or complex state)
- Availability (widely implemented in kernel)

4.1.2. Entropy Analysis

Composite key entropy:
- flow_label entropy: 20 bits (2^20 = 1M distinct values)
- tuple_hash entropy: 44 bits (2^44 = 17T distinct values)
- Combined entropy: 64 bits (2^64 distinct values, full 64-bit space)

Birthday attack probability for collision:
- Single 20-bit key: 707 collisions expected for 2^20 samples (LICH-009)
- Composite 64-bit key: 2^32 collisions expected for 2^64 samples (negligible)

L1 cache size: assume 16MB, 64-byte lines = 262K cache lines
With composite key: collision probability = (262K)^2 / 2^64 ≈ 10^-12 (negligible)

4.1.3. Backward Compatibility

Deployments using old 20-bit keys:
- Create new cache lines for composite keys
- Old 20-bit entries remain in cache (not evicted)
- Implement version check: if old and new keys hash to same L1 line, use new key

Recommendation: Deprecate 20-bit keys in draft-02; require composite keys.

4.1.4. Rationale

Composite key extends entropy from 20 to 64 bits, eliminating LICH-009 
collision attack vector. Defending against birthday attack on 20 bits is 
impractical; defending against 64 bits is infeasible.

Aligns with Dark Grimoire LICH-009 success criteria: "Extension to composite key 
(flow + src/dst hash) evaluated for entropy improvement"
```

**Justification:**
- Eliminates birthday attack vector (LICH-009)
- Prevents flow isolation bypass via collisions
- Maintains backward compatibility (soft transition)

---

## Patch W3: Mandate CAS Alignment in BPF Verifier

### Issue
Current draft: CAS (Compare-And-Swap) operations are documented informally; 
no alignment requirement.

**Problem:** Unaligned CAS instructions may not be atomic on all CPU architectures. 
This enables race conditions and lost updates (D4, LICH-008).

**S21 Finding:** D4 (Load-Store Unit Race Conditions), LICH-008 (L1 Cache Race Condition Fuzzing)

### Proposed Fix

**Add new Section 5.1 "CAS Alignment Requirements":**

```
5.1. CAS Alignment Requirements

All Compare-And-Swap (CAS) operations on Wotan L1 cache or WAL state MUST 
be 8-byte aligned.

5.1.1. BPF Verifier Enforcement

The BPF verifier MUST inject alignment checks before every CAS operation:

  // Source code (eBPF)
  atomic_cmpxchg(&ptr, expected, new_value);

  // After verification pass (pseudo-code)
  if ((ptr & 0x7) != 0:  // Check 8-byte alignment
    reject_program()     // Program fails verification

Programs failing alignment checks are rejected during load (not at runtime).

5.1.2. CAS Semantics

After alignment verification, CAS operation provides:
- Atomicity: operation is atomic with respect to all other CAS operations on same cache line
- Sequential consistency: full memory ordering (LoadLoad + LoadStore + StoreLoad + StoreStore barriers)
- Return value: original value before CAS (program checks return to verify success)

  uint64_t original = atomic_cmpxchg(&ptr, expected, new_value);
  if (original == expected):
    # CAS succeeded, ptr now contains new_value
  else:
    # CAS failed, ptr still contains original (stale value)

5.1.3. Implementation on Different Architectures

x86-64:
  - LOCK CMPXCHG instruction is always atomic (even if misaligned, though not recommended)
  - Alignment check is software-only (BPF verifier rejects misaligned programs)

ARM64:
  - LDAXR/STLXR (exclusive load/store) require 8-byte alignment (hardware enforced)
  - Misaligned operations cause illegal instruction exception (crashes)
  - BPF verifier check prevents this

RISC-V:
  - AMO.SWAPD requires 8-byte alignment (hardware enforced)
  - Misaligned operations cause access fault (crashes)
  - BPF verifier check prevents this

5.1.4. Rationale

Alignment check prevents architecture-dependent crashes:
- Without check: program crashes on ARM/RISC-V if developer misaligns access
- With check: program rejected at load time (fail-fast, not fail-slow)
- Enables portable eBPF code across x86/ARM/RISC-V architectures

Aligns with LICH-008 success criteria: "Atomic operation misuse identified"
```

**Justification:**
- Prevents architecture-dependent crashes
- Enforces atomicity across all CPU architectures
- Detects misaligned accesses at program load time (not runtime)

---

## Patch W4: Add HMAC-SHA256 to WAL Entries

### Issue
Current draft: WAL entries use CRC-32 only (covers accidental corruption, not intentional attacks).

**Problem:** CRC-32 can be recomputed by anyone (no key). Does not prevent tampering 
by attacker with network access. LICH-010 requires tampering detection.

**S21 Finding:** LICH-010 (WAL Integrity / Compaction Race Testing), M1 (Monad CRC coverage)

### Proposed Fix

**Modify Section 3.2 "Ring Buffer Format" (extends Patch W1):**

```
3.2.5. HMAC Authentication (Optional, Recommended)

In addition to CRC-32, WAL entries MAY be authenticated with HMAC-SHA256:

  struct RingEntry {
    seqno: u64,
    timestamp: u64,
    data_len: u16,
    data: [u8; data_len],
    crc32: u32,          ; covers accidental corruption
    ? hmac: [u8; 32],    ; HMAC-SHA256 (covers tampering)
  }

3.2.6. HMAC Computation

HMAC-SHA256 is computed over all fields (seqno through CRC32):

  hmac = HMAC_SHA256(
    key = session_key,
    message = seqno || timestamp || data_len || data || crc32
  )

Session key is derived during TLS handshake (per RFC 9001):
  session_key = PRK-Expand(session_secret, label="unheaded-wal", length=32)

3.2.7. Verification Procedure

On reading WAL entry:

  1. Verify CRC-32 (corruption check)
  2. If HMAC field present:
     - Recompute HMAC using session_key
     - Compare computed_hmac == stored_hmac
     - If mismatch: reject entry, log security alert
  3. If HMAC field absent:
     - Entry is unauthenticated (from old implementation)
     - Accept entry (backward compatible)

3.2.8. Rationale

CRC + HMAC provides defense-in-depth:
- CRC detects accidental bit flips (common in storage)
- HMAC detects intentional tampering (defense against attacker)
- Session key ensures HMAC is flow-specific (replay-resistant)

HMAC is optional to maintain backward compatibility with draft-00 implementations.
Recommend "SHOULD use HMAC" -> "MUST use HMAC" in draft-02.

Aligns with LICH-010 success criteria: "All WAL records verified with HMAC-SHA256"
```

**Justification:**
- Detects tampering in WAL entries
- Provides authenticity in addition to integrity
- Defense-in-depth (CRC + HMAC)

---

## Patch W5: Specify Exclusive Lock During WAL Compaction

### Issue
Current draft: WAL compaction is documented informally; no explicit locking mechanism.

**Problem:** Multiple threads can initiate compaction concurrently, causing double-free, 
use-after-free, or data corruption. LICH-010 detects race conditions during compaction.

**S21 Finding:** LICH-010 (WAL Integrity / Compaction Race Testing), D4 (Load-Store Unit Race Conditions)

### Proposed Fix

**Add new Section 3.3 "WAL Compaction Locking":**

```
3.3. WAL Compaction Locking

WAL compaction MUST be protected by exclusive lock. Only one compaction 
operation can proceed at a time.

3.3.1. Compaction Lock Semantics

  compaction_lock = Mutex()  ; per-WAL instance

  void compact_wal():
    with compaction_lock.acquire():
      # Critical section: only one thread executes here
      validate_segments()
      merge_small_segments()
      rewrite_index()
      fsync()  # Ensure durability
      # On lock release, other waiters proceed

3.3.2. Lock Acquisition

Before beginning compaction:

  if not compaction_lock.try_lock(timeout=10s):
    # Another compaction in progress
    log("Compaction already in progress, retrying in 5s")
    sleep(5)
    return  # Retry later, not now

  # Acquired lock, proceed with compaction
  try:
    compact_wal()
  finally:
    compaction_lock.release()

3.3.3. Concurrent Writes During Compaction

While compaction is active (lock held), concurrent writes from other threads:
- Acquire write_lock (separate from compaction_lock)
- Append to current segment (not the segment being compacted)
- Release write_lock
- Proceed independently of compaction

Write lock and compaction lock are held separately, enabling reads and 
writes to continue during compaction (different segments).

3.3.4. Compaction Completion Synchronization

After compaction completes:

  compaction_lock.release()
  # Broadcast: new compacted state available
  wake_all_waiters()

Readers can now observe compacted state:
  - Index rebuilt (faster seeks)
  - Dead segments deleted (freed space)
  - Monotonicity re-verified

3.3.5. Atomicity Guarantee

Compaction as a whole is atomic:
- Either compaction completes successfully (all segments merged)
- Or compaction fails and rolls back (all segments unchanged)

No partial completion: if compaction crashes mid-process, recovery 
discards compaction state and restarts from original segments.

3.3.6. Rationale

Exclusive lock prevents:
- Double-free (two threads freeing same segment)
- Use-after-free (reader accessing segment being compacted)
- Race condition (concurrent modifications during merge)
- Index corruption (two threads rewriting index simultaneously)

Aligns with LICH-010 success criteria: "Compaction exclusivity enforced: 
only one compaction at a time"
```

**Justification:**
- Prevents race conditions during compaction
- Ensures atomicity of compaction operation
- Protects against data corruption from concurrent access

---

## Patch W6: Per-Program Cache-Miss Rate Limiting

### Issue
Current draft: No rate limiting on cache misses; program can trigger repeated cache misses (DoS).

**Problem:** Malicious eBPF program can intentionally cause high cache-miss rate, 
exhausting bandwidth to slow storage (memory-to-L3 cache traffic). This is a 
computational completeness DoS attack (Dark Grimoire Section 4).

**S21 Finding:** Dark Grimoire Section 4 (Computational Completeness DoS)

### Proposed Fix

**Add new Section 5.2 "Cache-Miss Rate Limiting":**

```
5.2. Per-Program Cache-Miss Rate Limiting

Each eBPF program has a per-second cache-miss budget. Programs exceeding 
this budget incur penalty (throttling or program rejection).

5.2.1. Rate Limit Thresholds

Cache-miss budget per program:
  - Limit: 10,000 misses per second (10K cache misses / second)
  - Measurement window: 1 second (rolling window)
  - Action on excess: throttle reads, delay program execution by 100 ms

5.2.2. Cache-Miss Counting

On each L1 cache read:

  if key in L1_cache:
    hit_count[program_id]++
    cache_hit = true
  else:
    miss_count[program_id]++
    cache_miss = true
    # Fetch from L3/main memory
    if miss_count[program_id] > MISS_BUDGET_PER_SECOND:
      apply_throttle(program_id, delay=100ms)

5.2.3. Throttle Mechanism

When miss rate exceeds budget:
  1. Delay program execution: sleep(100 ms)
  2. Recount misses after delay
  3. If still over budget, increase delay to 200 ms
  4. Max delay: 5 seconds (after which program is forcibly terminated)

5.2.4. Metrics and Monitoring

Export metrics:
  - cache_hits_per_sec[program_id]
  - cache_misses_per_sec[program_id]
  - cache_miss_rate[program_id] = misses / (hits + misses)
  - throttle_events[program_id]

Alert on:
  - Cache miss rate > 50% (program has poor locality)
  - Sustained throttling > 10 seconds (DoS attack likely)

5.2.5. Rationale

Cache-miss rate limiting prevents:
- Bandwidth exhaustion (repeated cache misses saturate bus)
- Denial of service (one program starves others via cache traffic)
- CPU stalls (cache miss latency accumulates)

Limits are conservative:
- 10K misses/sec: typical program has <100 misses/sec
- 100 ms throttle: penalizes pathological programs without affecting normal ones

Aligns with Dark Grimoire Section 4: "Per-program cache-miss rate limiting"
```

**Justification:**
- Prevents DoS via cache-miss exhaustion
- Protects fairness (prevents one program starving others)
- Conservative limits don't affect normal programs

---

## Patch W7: SETTINGS Exchange via Control Topic

### Issue
Current draft: No standard mechanism to negotiate Wotan parameters (cache size, WAL segment size, etc.).

**Problem:** Implementations hardcode parameters, preventing interoperability. If receiver 
expects 4MB cache but sender assumes 16MB, behavior diverges (X2: cross-document divergence).

**S21 Finding:** X2 (Cross-Document Consistency Attacks)

### Proposed Fix

**Add new Section 6.1 "Wotan SETTINGS Exchange":**

```
6.1. Wotan SETTINGS Exchange

Before establishing flow, sender and receiver exchange Wotan configuration 
parameters via SETTINGS frame (similar to HTTP/2 SETTINGS).

6.1.1. SETTINGS Frame Format

  struct WotanSettings {
    magic: u32 = 0xDEADBEEF,
    num_settings: u8,
    settings: [Setting; num_settings],
  }

  struct Setting {
    id: u16,     ; parameter ID (0x00-0xFF)
    value: u32,  ; parameter value
  }

Defined settings:
  0x00: L1_CACHE_SIZE (bytes, default 16MB, range 1MB-256MB)
  0x01: L1_CACHE_LINE_SIZE (bytes, default 64, fixed at 64)
  0x02: WAL_SEGMENT_SIZE (bytes, default 4MB, range 1MB-64MB)
  0x03: WAL_RETENTION_TIME (seconds, default 3600, range 60-86400)
  0x04: MAX_FLOW_ENTRIES (entries, default 1M, range 10K-10M)
  0x05: CAS_ALIGNMENT (bytes, fixed at 8, non-negotiable)

6.1.2. SETTINGS Exchange Procedure

On connection establishment:
  1. Sender transmits SETTINGS frame with local parameters
  2. Receiver reads SETTINGS frame
  3. Receiver extracts parameters: cache_size, wal_segment_size, etc.
  4. Receiver validates parameter ranges (reject if out of range)
  5. Receiver replies with ACK (empty SETTINGS frame, same magic)
  6. Both sides proceed with flow using negotiated parameters

If receiver rejects parameter (out of range):
  - Send SETTINGS frame with "error" indicator
  - Close connection (protocol error)
  - Log event: "SETTINGS negotiation failed: parameter X out of range"

6.1.3. Parameter Validation

Each setting has min/max bounds:

  setting_bounds = {
    L1_CACHE_SIZE: (1_MB, 256_MB),
    WAL_SEGMENT_SIZE: (1_MB, 64_MB),
    WAL_RETENTION_TIME: (60, 86400),
    MAX_FLOW_ENTRIES: (10_000, 10_000_000),
  }

  if setting.value < min || setting.value > max:
    send_error_response()
    close_connection()

6.1.4. Rationale

SETTINGS exchange enables:
- Parameter negotiation (sender and receiver agree on configuration)
- Interoperability (implementations can use different defaults)
- Extensibility (new settings added in future RFCs)
- Failure detection (mismatched parameters detected early)

Aligns with HTTP/2 RFC 7540, HTTP/3 RFC 9114 (both use SETTINGS exchange).

Prevents X2 (cross-document divergence): parameters are explicitly negotiated, 
not silently assumed.

6.1.5. Transport

SETTINGS frame is transmitted as:
  - TLV type 0x40 (Wotan Memory, critical)
  - Payload: WotanSettings struct (CBOR-encoded)
  - Sent during TLS handshake or early in connection setup
```

**Justification:**
- Enables parameter negotiation (prevents X2)
- Follows HTTP/2 and HTTP/3 patterns (familiar to implementers)
- Detects configuration mismatches early (fail-fast)

---

## Patch W8: GOAWAY Frame Specification

### Issue
Current draft: No standard mechanism to gracefully terminate flow (connection).

**Problem:** Implementations close connections abruptly, losing in-flight packets. 
No graceful shutdown sequence, leading to data loss and incomplete WAL writes.

**S21 Finding:** LICH-010 (WAL Integrity: "Crash-and-recover scenarios all validate correctly")

### Proposed Fix

**Add new Section 6.2 "GOAWAY Frame":**

```
6.2. GOAWAY Frame (Graceful Flow Termination)

GOAWAY frame signals graceful termination of flow. Receiver can finish 
in-flight operations before connection closes.

6.2.1. GOAWAY Frame Format

  struct GoAwayFrame {
    magic: u32 = 0xC0FFEE00,
    reason: u16,           ; termination reason (0x00-0xFF)
    last_flow_id: u32,     ; last flow ID processed by sender
    debug_data: [u8; ...], ; variable-length debug info (optional)
  }

Reason codes:
  0x00: No error (normal termination)
  0x01: Protocol error (invalid packet)
  0x02: Flow ID exhaustion (too many flows)
  0x03: WAL compaction in progress (pause new writes)
  0x04: Server shutdown (graceful restart)
  0x05: Resource limits (memory/CPU exhausted)
  0x06: Timeout (no activity for 30s)
  0x07: Explicit client request

6.2.2. Sending GOAWAY

Before closing connection:

  1. Send GOAWAY frame (TLV type 0x41, critical)
  2. Set reason code (e.g., 0x00 for normal termination)
  3. Set last_flow_id to highest processed flow
  4. Wait up to 5 seconds for receiver to finish in-flight operations
  5. Close TCP connection (both sides)

6.2.3. Receiving GOAWAY

On receiving GOAWAY frame:

  1. Extract reason_code and last_flow_id
  2. Log event: "GOAWAY received, reason=X, last_flow_id=Y"
  3. Stop initiating new flows
  4. Allow in-flight operations to complete (up to 5s timeout)
  5. Flush WAL (fsync all pending writes)
  6. Close TCP connection
  7. Reconnect if needed (new connection, new flow IDs)

6.2.4. Graceful Shutdown Guarantee

GOAWAY frame ensures:
- In-flight packets are processed before close
- WAL is flushed to disk (durable)
- Flow state is consistent (no lost updates)
- Replay-safe (all seqnos are durable)

Procedure:
  1. Send GOAWAY(last_flow_id=123)
  2. Receiver processes flows 0-123 to completion
  3. Receiver sends ACK (implicit, via TCP FIN)
  4. Sender closes connection
  5. Both sides are in consistent state

6.2.5. Timeout Handling

If receiver doesn't close within 5 seconds:
  - Sender forcibly closes TCP connection (timeout)
  - Any in-flight writes are rolled back (not durable)
  - Log: "GOAWAY timeout, forcing close"

5-second timeout balances graceful shutdown vs. hung connection detection.

6.2.6. Rationale

GOAWAY frame prevents:
- Abrupt connection closes (causes data loss)
- In-flight packet loss (receiver not ready)
- WAL inconsistency (flush on close)
- Incomplete compaction (finish in-progress operations)

Aligns with LICH-010 success criteria: "Crash-and-recover scenarios all 
validate correctly" (GOAWAY ensures clean shutdown, reducing recovery burden).
```

**Justification:**
- Graceful shutdown (prevents data loss)
- Flushes WAL before close (ensures durability)
- Allows in-flight operations to complete (consistency)
- Familiar pattern from HTTP/2 GOAWAY

---

## Summary

All Wotan Memory patches:

| Patch | Issue | Finding | Impact |
|-------|-------|---------|--------|
| W1 | No seqno in WAL | LICH-010 | Adds monotonicity validation |
| W2 | 20-bit key collision | LICH-009 | Extends to 64-bit composite key |
| W3 | No CAS alignment check | D4, LICH-008 | BPF verifier enforces alignment |
| W4 | CRC-32 only (no HMAC) | LICH-010 | Adds HMAC-SHA256 authentication |
| W5 | No compaction locking | D4, LICH-010 | Exclusive mutex during compaction |
| W6 | No cache-miss limiting | Dark Grimoire Section 4 | Rate limiting per program |
| W7 | No parameter negotiation | X2 | SETTINGS frame exchange |
| W8 | No graceful shutdown | LICH-010 | GOAWAY frame specification |

