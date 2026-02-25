# Dark Grimoire Addendum - S21 Assessment

This document extends the Black Mage grimoire with attack surfaces discovered during the S21 assessment. It serves as the authoritative taxonomy of Unheaded protocol vulnerabilities and exploitation techniques.

---

## Section 1: Doom-Specific Attack Surface (D1-D6 Findings)

The Doom substrate for Monad bytecode interpretation introduces a specialized attack surface focused on bytecode semantics, execution sandboxing, and memory protection.

### D1: Instruction Decoding Vulnerabilities

**Description:** Flaws in translating MBC bytecode opcodes to native execution, enabling instruction injection or privilege escalation.

**Attack Scenario:**
- Malformed instruction encoding triggers buffer over-read in opcode fetch
- Attacker crafts instruction with register field > 15 (undefined behavior)
- Incorrectly sized instruction parsed as two half-sized instructions
- Branch offset arithmetic overflows, jumping into unrelated code sections

**Impact:** Code execution with Doom sandbox privileges, potential escape to kernel

**Mitigation Checklist:**
- [ ] Verify all 256 MBC opcode implementations match specification exactly
- [ ] Register field validation: must be 0-15, reject higher values
- [ ] Instruction size validation: multi-byte instructions must be fully present before decode
- [ ] Branch offset sanity check: must stay within current code segment
- [ ] Bounds checking on all instruction fetch loops (no speculative execution beyond segment)

**Regression Test:** LICH-007 (MBC Bytecode Instruction Fuzzing)

### D2: Stack Machine Underflow / Overflow

**Description:** Incorrect stack depth tracking in bytecode interpreter allows reading/writing past stack bounds.

**Attack Scenario:**
- Operation pops from empty stack, reading adjacent heap data
- Deep recursion or large local variable allocation exceeds stack limit
- Attacker crafts series of POP instructions to read secret data below stack base
- PUSH instruction writes above stack limit, corrupting heap metadata

**Impact:** Information disclosure (read past bounds), denial of service (stack overflow), memory corruption

**Mitigation Checklist:**
- [ ] Stack depth tracking: increment on PUSH, decrement on POP, validate before each operation
- [ ] Explicit stack size limit (recommend 1MB per bytecode program)
- [ ] Bounds check on all stack access: `stack_ptr >= stack_base && stack_ptr < stack_limit`
- [ ] Reject any instruction sequence with static stack depth analysis violating bounds
- [ ] Initialize stack with canary values to detect over-write

**Regression Test:** LICH-007 (test corpus: stack_overflow/underflow)

### D3: Memory Protection Bypass via Mmap Tricks

**Description:** Bytecode program escapes from memory protection (read-only code, data separation) through mmap manipulation or address space layout attacks.

**Attack Scenario:**
- Doom substrate uses mmap to allocate code/data regions with different permissions
- Attacker finds TOCTOU window between permission check and access
- Bytecode loads code segment address, modifies mmap permissions, executes data section as code
- Timing-based attack forces relocation during bytecode execution, creating stale pointers

**Impact:** Code injection, arbitrary execution within Doom sandbox

**Mitigation Checklist:**
- [ ] Mandate mmap with MAP_FIXED_NOREPLACE or equivalent to prevent relocation
- [ ] Verify all memory regions mapped with explicit permissions (PROT_READ | PROT_EXEC for code, PROT_READ | PROT_WRITE for data)
- [ ] Disallow mprotect() calls on mapped regions after initial setup
- [ ] Use address randomization (ASLR) to reduce address prediction accuracy
- [ ] Document bytecode interpreter sandbox boundary (where privilege transitions occur)

**Regression Test:** LICH-007 (test corpus: mmap_chaos/)

### D4: Load-Store Unit Race Conditions

**Description:** Concurrent bytecode execution threads access shared memory without synchronization, causing TOCTOU violations.

**Attack Scenario:**
- Two bytecode programs run on different cores, both read same cache line
- Program A reads value V, Program B modifies V
- Program A uses stale V, producing incorrect computation or escaping bounds checks
- Missing memory barriers cause reordering: write to permissions buffer occurs after read of sensitive data

**Impact:** Privilege escalation within Doom sandbox (bypass bounds checks), information disclosure

**Mitigation Checklist:**
- [ ] Identify all shared mutable state in Doom interpreter (registers, stack, heap)
- [ ] Add explicit memory barriers (mb(), wmb(), rmb()) around critical sections
- [ ] Use atomic operations for all concurrent accesses (atomics or locks)
- [ ] Document memory ordering guarantee: "Doom interpreter is sequentially consistent"
- [ ] Enforce single-threaded bytecode execution per core, or explicit inter-program synchronization

**Regression Test:** LICH-008 (Wotan L1 Cache Line Race Condition Fuzzing)

### D5: Privilege Escalation within Doom Sandbox

**Description:** Bytecode program elevates privileges from user-space Doom runtime to kernel-accessible operations (e.g., reading raw L1 cache, invoking BPF helpers).

**Attack Scenario:**
- Monad BPF helpers are exposed to bytecode interpreter
- Attacker crafts bytecode that calls `bpf_wotan_read()` with arbitrary cache key
- Helper returns privileged memory (e.g., other program's state or kernel secrets)
- Doom interpreter fails to validate bytecode authorization level for helper access

**Impact:** Information disclosure from kernel memory, cross-program data theft, sandbox escape

**Mitigation Checklist:**
- [ ] Audit all BPF helper calls accessible from Doom bytecode
- [ ] Implement per-program capability model: which helpers each program may invoke
- [ ] Validate capabilities before each helper call; deny if not authorized
- [ ] Log all helper calls for forensic analysis
- [ ] Consider removing direct bytecode access to privileged helpers (invoke only via Monad flow engine)

**Regression Test:** LICH-007 + integration test with bpf_wotan_read/write

### D6: Integer Overflow in Instruction Arithmetic

**Description:** Bytecode operations on large integers (e.g., branch offset calculation, address arithmetic) wrap silently, violating specification.

**Attack Scenario:**
- Branch offset field is i32; bytecode adds 0x7FFFFFFF to jump offset
- Overflow wraps to -2147483648, jumping to negative address (interpreted modulo memory size)
- Attacker crafts sequence of arithmetic operations to leak data via side-channel (execution time of out-of-bounds branches)
- Memory access address computation overflows, wrapping to unintended address range

**Impact:** Incorrect program behavior, side-channel leaks, privilege escalation

**Mitigation Checklist:**
- [ ] Use overflow-checked arithmetic (e.g., Rust's `checked_add()` or C's UBSan)
- [ ] Reject instructions causing overflow; define behavior clearly in spec ("MUST NOT overflow, instruction fails")
- [ ] Test all arithmetic operations with boundary values (i32::MAX, i32::MIN, INT_MAX, INT_MIN)
- [ ] Document integer semantics: wrapping vs. saturating vs. checked behavior for each operation

**Regression Test:** LICH-007 (test corpus: arithmetic_overflow/)

---

## Section 2: Cross-Document Consistency Attacks (X1-X4)

The Unheaded protocol spans multiple RFCs (Monad Foundation, Sophia Dictionary, Wotan Memory). Inconsistencies between documents enable attacks exploiting differing interpretations.

### X1: Version Field Interpretation Divergence

**Description:** Monad Foundation RFC specifies one version field encoding, while Sophia Dictionary or Wotan Memory RFCs interpret it differently.

**Attack Scenario:**
- Monad Foundation RFC 5: version field is "semantic version X.Y.Z in 8 bits"
- Sophia Dictionary RFC 1: version field is "feature bitmap where bit 3 enables new TLV parsing"
- Attacker sends packet with version 0x08 (Monad: v0.0.8; Sophia: feature-3-enabled)
- Monad parser rejects as unsupported; Sophia parser accepts, enables experimental TLV code
- Inconsistency causes handler divergence: version checks pass/fail depending on parsing order

**Impact:** Protocol confusion attack, feature negotiation bypass, specification non-compliance

**Mitigation Checklist:**
- [ ] Define version field semantics in Monad Foundation RFC as normative authority
- [ ] Sophia and Wotan RFCs MUST reference and conform to Monad version spec (no independent interpretation)
- [ ] Create version compatibility matrix: (Monad version, Sophia version, Wotan version) -> allowed combinations
- [ ] Implement strict version checking: reject packets if any RFC version is unsupported
- [ ] Document version evolution path: what changes require new version, what are backward compatible

**Regression Test:** Integration test: create packets with version mismatch, verify consistent rejection

### X2: CRC / Integrity Checking Scope Mismatch

**Description:** Different RFCs specify different fields covered by integrity checks, leading to undetected corruption.

**Attack Scenario:**
- Monad Foundation RFC: "CRC-32 covers bytes [0:8]" (header only)
- Wotan Memory RFC: "CRC-32 covers bytes [0:64]" (header + first data segment)
- Attacker modifies bytes [20:30] (covered by Wotan but not Monad)
- Monad parser checks CRC (bytes [0:8] unchanged), passes
- Wotan parser checks CRC (bytes [20:30] changed), fails
- Divergent security posture: Monad accepts corrupted packet, Wotan rejects it

**Impact:** Packet acceptance divergence, potential for replay attacks, data corruption

**Mitigation Checklist:**
- [ ] Define comprehensive integrity scope: "CRC-32 covers all 20 header bytes of Monad foundation header"
- [ ] Specify in Monad RFC that Sophia TLV payloads MUST be covered by CRC (or explicitly excepted)
- [ ] Consider replacing CRC-32 with HMAC-SHA256 for stronger guarantees (covers authenticity + integrity)
- [ ] Test: forge packet, verify all parsers reject consistently
- [ ] Document any unvalidated fields and why (e.g., hop limit is intentionally unchecked to avoid recomputation)

**Regression Test:** Fuzzing: corrupt random bytes in packet, verify all RFCs reject or all accept (not divergent)

### X3: TLV Extension Parsing Rules Divergence

**Description:** Monad, Sophia, and Wotan RFCs define different rules for handling unknown TLV types, leading to downgrade attacks.

**Attack Scenario:**
- Monad RFC: "MUST ignore unknown TLV types, continue parsing"
- Sophia RFC: "SHOULD reject unknown TLV types, fail parsing"
- Wotan RFC: "MAY skip unknown TLV types if marked as non-critical"
- Attacker sends TLV of type 0x1F with Sophia security policy embedded
- Monad implementation ignores it (MUST clause), processes packet
- Sophia implementation rejects it (SHOULD clause), drops packet
- Parser divergence: security enforced in Sophia but not Monad

**Impact:** Feature bypass (security policy ignored by lenient parser), attack downgrade

**Mitigation Checklist:**
- [ ] Enforce uniform TLV handling rule across all RFCs: "Unknown critical TLVs MUST cause packet rejection"
- [ ] Define "critical" TLV flag in Monad header (bit 7 of type field) as standard
- [ ] Monad RFC: "If TLV type > 0x80, it is critical: MUST reject if unknown"
- [ ] Sophia and Wotan RFCs MUST use only critical TLV types (or explicitly inherit Monad rules)
- [ ] Test: send unknown critical TLV, verify all implementations reject

**Regression Test:** LICH test: inject unknown TLV-0xAA (critical), verify rejection by all parsers

### X4: Error Handling and Fallback Divergence

**Description:** Different RFCs specify different error recovery strategies, enabling bypass of security checks.

**Attack Scenario:**
- Monad RFC: "On version mismatch, MUST drop immediately"
- Sophia RFC: "On version mismatch, fall back to previous-version parsing"
- Wotan RFC: "On version mismatch, apply compatibility shim if available"
- Attacker sends Monad v1.5 (unsupported) which Sophia downgrades to v1.0 (supported by Sophia)
- Monad parser drops packet (no fallback)
- Sophia parser accepts packet (fallback enabled)
- Divergence: packet reaches protocol handler in Sophia but not Monad (inconsistent security policy)

**Impact:** Feature downgrade attacks, bypass of new security requirements, inconsistent behavior

**Mitigation Checklist:**
- [ ] Define authoritative error handling policy in Monad RFC: "Unknown versions MUST cause immediate packet drop. No fallback."
- [ ] Sophia and Wotan RFCs MUST explicitly inherit this: "Do not override Monad version checking; it is final."
- [ ] Remove any fallback or compatibility shim language from RFCs
- [ ] Document rationale: "Fallback creates security confusion; explicit version support prevents surprises"
- [ ] Test: all RFCs must reject vN for N >= current_version

**Regression Test:** Conformance test: unknown version value -> all RFCs reject with same error code

---

## Section 3: IANA Registry Attacks

The Unheaded protocol relies on IANA registries for allocated codepoints (error codes, TLV types, option values). Registry attacks exploit gaps and squatting.

### Option Squatting Attack

**Description:** Attacker pre-allocates option/parameter codepoints in IANA registry before legitimate definitions exist, forcing future legitimate options into unexpected positions.

**Attack Scenario:**
- Monad RFC defines error codes 0x00-0x0D in IANA registry
- Attacker (or rogue RFC author) allocates error code 0x0A as "Reserved for Experimental X"
- Later, legitimate RFC wants to define 0x0A as "Connection Reset"
- IANA registry now has two overlapping definitions; implementations diverge on 0x0A interpretation
- Security-relevant error code collides with reserved experimental code

**Impact:** Error code confusion, misinterpretation of error conditions, security policy bypass

**Mitigation Checklist:**
- [ ] IANA registry entry: mark error codes 0xE0-0xFF as reserved for private/experimental use
- [ ] Pre-allocate critical error codes (0x00-0x0D) with normative RFC status required for allocation
- [ ] Require consensus approval for new error codes (IETF review, not expert review)
- [ ] Publish reserved ranges prominently: "0xE0-0xFF: Private Use, 0xF0-0xFF: Greasing/Testing"
- [ ] Define squatting prevention: "Once allocated, codepoint is reserved; allocation cannot be revoked"

**Regression Test:** Verify IANA registry entries match RFC spec; no gaps or conflicts

### Allocation Gap Exploitation

**Description:** IANA registry leaves gaps in codepoint ranges, enabling ambiguity in implementations (is gap meaningful or oversight?).

**Attack Scenario:**
- Error codes defined: 0x00-0x05, then 0x08-0x0D (gap at 0x06-0x07)
- Implementation A: treats 0x06 as "unknown error, default handling"
- Implementation B: treats 0x06 as "reserved, drop packet"
- Attacker sends error 0x06
- Divergent behavior: A processes error, B drops packet
- Security policy differs: A logs unknown error, B silently fails (no audit trail)

**Impact:** Parser divergence, security policy inconsistency, unspecified behavior

**Mitigation Checklist:**
- [ ] Explicitly allocate all error codes 0x00-0x1F; no gaps allowed
- [ ] Gap code 0x06-0x07: "Reserved (allocated for future expansion, currently undefined)"
- [ ] Greasing/testing codes: 0x1F, 0x3E, 0x5D, 0x7C (pattern: 0x1F*N + 0x1F) reserved for connection probing
- [ ] Implementers MUST handle reserved codes gracefully: "Log and drop packet for undefined code"
- [ ] Test: send all codepoint values 0x00-0xFF, verify consistent behavior for gaps

**Regression Test:** Fuzzing: inject undefined error codes, verify all implementations drop or handle consistently

### TLV Type Allocation Conflicts

**Description:** Sophia Dictionary and Wotan Memory define independent TLV type ranges without coordination, causing allocation conflicts.

**Attack Scenario:**
- Monad Foundation RFC allocates TLV types 0x00-0x3F
- Sophia Dictionary RFC allocates TLV types 0x10-0x2F independently (overlap with Monad 0x10-0x2F)
- Wotan Memory RFC allocates TLV types 0x20-0x4F (overlap with both)
- Packet contains TLV 0x25: Sophia interprets as "Dictionary Flag X", Wotan interprets as "Cache Key Y"
- Parser divergence: payload is misinterpreted depending on RFC processing order

**Impact:** TLV type collision, payload misinterpretation, security feature bypass

**Mitigation Checklist:**
- [ ] Create unified TLV type registry in Monad Foundation RFC
- [ ] Allocate type ranges: 0x00-0x1F (Monad Foundation), 0x20-0x3F (Sophia), 0x40-0x5F (Wotan), 0x60-0x7F (future), 0x80-0xFF (private/experimental)
- [ ] Sophia RFC MUST NOT allocate any TLV types; it extends Monad types only (via nested TLVs or reserved Monad type codes)
- [ ] Wotan RFC MUST allocate from reserved range only, or define no new types
- [ ] Test: verify no two RFCs claim same TLV type code

**Regression Test:** IANA registry audit: scan all RFCs for overlapping allocations

---

## Section 4: Computational Completeness Attack Surface (Section 12 Monad Spec)

Monad Foundation RFC Section 12 defines Turing completeness (universal computation) of the MBC bytecode. This attack surface focuses on algorithmic attacks exploiting unrestricted computation.

### Denial of Service via Infinite Loops

**Description:** Bytecode program contains tight infinite loop that never yields control, starving other programs and causing denial of service.

**Attack Scenario:**
- Attacker submits Monad program with bytecode: `LABEL: JMP LABEL`
- Doom interpreter executes loop indefinitely without CPU yield
- Thread running interpreter hangs; other programs cannot execute
- Resources (CPU time, memory) allocated to single program; system becomes unresponsive

**Impact:** Denial of service, resource exhaustion, system hang

**Mitigation Checklist:**
- [ ] Implement instruction counter: count executed instructions, yield every N instructions (recommend 100K)
- [ ] Yield semantics: "After 100K instructions, yield to scheduler; resume on next time slice"
- [ ] Enforce timeout: program exceeding 10 second wallclock time is forcibly terminated
- [ ] Report resource exhaustion: log which program triggered timeout
- [ ] Test: submit LICH-007 corpus containing tight loops, verify scheduler enforces limits

**Regression Test:** LICH-007 (test corpus: tight_loops/)

### Side-Channel Attacks via Timing

**Description:** Bytecode program behavior (execution time) leaks information about secret data or other programs' state.

**Attack Scenario:**
- Attacker program observes scheduling timers and CPU cycle counters
- Program measures time to execute various bytecode sequences
- If execution time varies based on cache state, infers what data other programs cached
- Measures time to compute hash of secret value, uses timing to brute-force secret (e.g., via Spectre-style L1 cache timing)

**Impact:** Information disclosure (side-channel leaks), cryptographic key recovery, cross-program data theft

**Mitigation Checklist:**
- [ ] Disable CPU cycle counters: bytecode cannot access `rdtsc` or equivalent timing primitives
- [ ] Disable L1 cache profiling: no access to `bpf_perf_event_read()` for cache hit/miss rates
- [ ] Use constant-time operations for cryptographic functions (implement in BPF helpers, not bytecode)
- [ ] Consider CPU frequency pinning: eliminate timing variance from frequency scaling
- [ ] Test: attacker program tries to measure execution time of cryptographic operations, verify timing randomization

**Regression Test:** Timing side-channel fuzzing (advanced): attempt to extract secrets via timing leaks

### Integer Overflow as Denial of Service

**Description:** Bytecode program triggers integer overflow in arithmetic operations, causing incorrect computation or crash.

**Attack Scenario:**
- Attacker submits program with: `ADD 0x7FFFFFFF, 0x00000001` (i32 overflow)
- Interpreter behavior undefined: wraps to 0x80000000 or crashes
- If wraps: subsequent instruction computes wrong value (e.g., memory address)
- If crashes: interpreter terminates; program cannot execute any further

**Impact:** Denial of service (interpreter crash), incorrect program behavior, potential privilege escalation

**Mitigation Checklist:**
- [ ] Define overflow behavior normatively in Monad RFC: "All arithmetic operations MUST use checked arithmetic; overflow causes instruction to fail with error code 0x04"
- [ ] Doom interpreter MUST NOT crash on overflow; instruction must fail gracefully
- [ ] Program must handle error codes returned from failed instructions
- [ ] Test: LICH-007 (corpus: arithmetic_overflow/)

**Regression Test:** LICH-007 (test corpus: overflow_variations/)

---

## Section 5: Concurrency Primitives Audit Checklist

The Unheaded protocol integrates with Linux kernel BPF and memory-mapped I/O. Concurrency bugs are critical security issues.

### Checklist Item: Every `get_ptr_mut()` Call

**Description:** `get_ptr_mut()` is a BPF helper that returns a mutable pointer to kernel memory. Misuse enables arbitrary memory corruption.

**Audit Points:**
- [ ] Bounds check: returned pointer must be validated before dereference
  ```
  ptr = get_ptr_mut(key);
  if (!ptr || ptr < MAP_BASE || ptr >= MAP_BASE + MAP_SIZE) return error;
  *ptr = value;  // Safe
  ```
- [ ] Lifetime guarantee: pointer is valid only during current eBPF program execution; cannot be stored
- [ ] Synchronization: if multiple programs call `get_ptr_mut(key)` on same key, ensure exclusive access (use spinlock)
- [ ] Atomicity: verify that all reads and writes through pointer are atomic operations (use `bpf_probe_read_kernel()` or volatile access)
- [ ] Test: concurrent programs, each calling `get_ptr_mut(same_key)`, verify no data races

**Regression Test:** LICH-008 (concurrent access patterns, TSan instrumentation)

### Checklist Item: Map Hot-Swap / Reallocation

**Description:** BPF map resizes or reallocates, invalidating pointers returned by prior `get_ptr_mut()` calls.

**Audit Points:**
- [ ] Lock during map resizing: exclusive lock held while reallocation in progress
- [ ] Pointer invalidation: all in-flight `get_ptr_mut()` pointers revoked during resize
- [ ] Error handling: programs accessing invalidated pointers receive error (not crash)
- [ ] Test sequence:
  1. Program A calls `get_ptr_mut(key)`, receives pointer P
  2. Map resize triggered (threshold exceeded)
  3. Reallocation begins, P is invalidated
  4. Program A dereferences P -> error (not crash or use-after-free)

**Regression Test:** Stress test: concurrent program with map size oscillation

### Checklist Item: Multi-Packet Flow Operations

**Description:** Single flow (identified by flow_label) spans multiple packets; state must be synchronized across packets.

**Audit Points:**
- [ ] State coherency: modifications to flow state in packet 1 visible to packet 2 (same flow, later arrival)
- [ ] Order guarantee: if packet A arrives before packet B (both same flow), state effects of A applied before B processes
- [ ] Lost updates: if packets A and B arrive concurrently (same flow, different cores), both modifications preserved (no lost writes)
- [ ] Test: Send packets for same flow in parallel, verify state not lost
- [ ] Wotan WAL must log in strict order (seqno guarantee)

**Regression Test:** LICH-010 (WAL integrity / multi-packet ordering)

### Checklist Item: L1 Cache Line Invalidation

**Description:** Wotan L1 cache lines must be invalidated when underlying data changes, or cache will serve stale data.

**Audit Points:**
- [ ] Invalidation on write: every `bpf_wotan_write(key, value)` invalidates all cache lines for that key
- [ ] No stale reads: subsequent `bpf_wotan_read(same_key)` must return latest value, not cached stale value
- [ ] Concurrent invalidation: if invalidation in progress when read occurs, read waits for invalidation to complete
- [ ] Test: write value to key, read immediately, verify new value (not old cached value)
- [ ] Extend key to composite (flow + hash) to prevent collision-based coherency bugs

**Regression Test:** LICH-009 (cache coherency violation testing)

### Checklist Item: WAL Operation Atomicity

**Description:** Write-Ahead Log must maintain strict ordering and atomicity; partial writes must be detectable.

**Audit Points:**
- [ ] Atomic write: WAL record write is atomic at 4KB segment boundary (page-aligned writes)
- [ ] Seqno monotonicity: each record has monotonically increasing seqno (no gaps)
- [ ] Replay idempotency: replaying WAL multiple times produces identical result (not idempotent: problem)
- [ ] Compaction exclusivity: only one compaction in progress at a time (exclusive lock held)
- [ ] CRC validation: WAL entries validated before replay (add HMAC-SHA256 to spec)
- [ ] Test: concurrent writes + compaction, crash simulation, replay verification

**Regression Test:** LICH-010 (WAL integrity / race testing)

---

## Section 6: HTTP/3 Cross-Pollination Attack Matrix

The Unheaded protocol is designed to integrate with HTTP/3 over QUIC. Cross-protocol attacks can bypass security assumptions.

### Attack Class 1: QUIC Version Downgrade

**Description:** Attacker forces protocol to negotiate older, less secure QUIC version.

**Mitigations:**
- [ ] Unheaded MUST specify supported QUIC versions (recommend v1 only, RFC 9000)
- [ ] Reject version negotiation; no fallback to older versions
- [ ] Document: "Version mismatch MUST cause immediate connection termination"

### Attack Class 2: HTTP/3 Stream ID Reuse

**Description:** Attacker reuses expired stream IDs to hijack or replay prior communications.

**Mitigations:**
- [ ] Stream ID tracking: maintain bitmap of previously used IDs, reject reuse
- [ ] Grace period: after stream close, block reuse for at least 30 seconds
- [ ] Test: resubmit request on closed stream ID, verify rejection

### Attack Class 3: Header Compression (QPACK) Attacks

**Description:** QPACK dynamic table can be leveraged for cache-based side-channel attacks or header injection.

**Mitigations:**
- [ ] Limit QPACK dynamic table size (recommend 4KB per connection)
- [ ] Encode sensitive headers (auth tokens) as literal, not via compression
- [ ] Validate decompressed headers match expected format (length, characters)
- [ ] Test: craft QPACK headers with unexpected huffman codes, verify rejection

### Attack Class 4: Connection Migration Hijacking

**Description:** Attacker switches connection ID mid-stream, causing packets to be routed to wrong endpoint.

**Mitigations:**
- [ ] Validate connection ID consistency throughout session
- [ ] Reject packets with new connection ID unless path validation completed
- [ ] Document: "Connection ID changes MUST be validated via handshake"

### Attack Class 5: Path Validation Bypass

**Description:** Attacker exploits QUIC path validation (NAT rebinding) to perform connection hijacking.

**Mitigations:**
- [ ] Require address validation token for every path change
- [ ] Token must include timestamp (prevent replay for >60 seconds)
- [ ] Validate token before processing data frames from new path

### Attack Class 6: Flow Control Attack

**Description:** Attacker manipulates QUIC flow control (MAX_DATA frames) to trigger buffer exhaustion or stall connections.

**Mitigations:**
- [ ] Enforce local flow control limits: cap flow window at 10MB per stream
- [ ] Reject MAX_DATA frames increasing limit beyond cap
- [ ] Monitor for flooding: if >1000 MAX_DATA frames per second, drop connection

### Attack Class 7: 0-RTT Replay

**Description:** 0-RTT early data can be replayed in different context, violating idempotency assumptions.

**Mitigations:**
- [ ] Disable 0-RTT in Unheaded (EARLY_DATA disable)
- [ ] Document: "All requests must complete TLS 1.3 handshake; no early data allowed"
- [ ] Test: verify server rejects 0-RTT requests with ILLEGAL_TLS_CONTENT_TYPE error

### Attack Class 8: Stateless Reset Forgery

**Description:** Attacker forges STATELESS_RESET frame to terminate connection from afar.

**Mitigations:**
- [ ] Stateless reset token derived from unpredictable source (e.g., HMAC of connection ID)
- [ ] Token length ≥128 bits
- [ ] Verify token before closing connection

### Attack Class 9: ACK-based Amplification

**Description:** Attacker sends spoofed initial packet, causing server to send large ACK response (amplification attack).

**Mitigations:**
- [ ] Anti-amplification limit: server response size <= 3x request size during handshake
- [ ] Address validation required before sending unilaterally large packets
- [ ] Implement ACK packet size cap (max 200 bytes)

### Attack Class 10: Packet Number Decryption Oracle

**Description:** Attacker observes packet number field to infer encryption keys or packet loss patterns.

**Mitigations:**
- [ ] Encrypt packet number field (or reserve as application data, not visible to network)
- [ ] Use packet protection from TLS (AEAD, nonce derivation per RFC 9001)
- [ ] Randomize packet number incrementing to avoid timing leaks

### Attack Class 11: Frame Type Confusion

**Description:** Attacker sends frame with invalid type code, causing divergent parser behavior.

**Mitigations:**
- [ ] Define all HTTP/3 frame types normatively; no reserved/undefined frames
- [ ] Unknown frame type: drop frame, continue parsing (RFC 9114 compliant)
- [ ] Test: send frame type 0xFF, verify drop and continue

### Attack Class 12: Setting Pollution

**Description:** Attacker injects HTTP/3 SETTINGS frame with adversarial parameter values.

**Mitigations:**
- [ ] Validate all SETTINGS parameters against reasonable bounds:
  - SETTINGS_MAX_FIELD_SECTION_SIZE: 0x4000-0x1000000 (16KB to 16MB)
  - SETTINGS_QPACK_MAX_TABLE_CAPACITY: 0-2147483648
  - SETTINGS_QPACK_BLOCKED_STREAMS: 0-65535
- [ ] Reject out-of-range values; drop connection
- [ ] Test: send extreme SETTINGS values, verify rejection

### Attack Class 13: Trailers / Pseudo-Header Injection

**Description:** Attacker injects HTTP trailers or pseudo-headers in wrong location, confusing header parsing.

**Mitigations:**
- [ ] Pseudo-headers (`:method`, `:scheme`, `:authority`, `:path`) allowed only in request headers, not trailers
- [ ] Trailers must not contain pseudo-headers (reject if present)
- [ ] Validate pseudo-header presence: `:method` and `:path` REQUIRED for requests
- [ ] Test: send malformed pseudo-headers in trailers, verify rejection

### Attack Class 14: Priority Inversion / Head-of-Line Blocking

**Description:** Low-priority stream stalls entire connection due to HOL blocking in protocol layers.

**Mitigations:**
- [ ] Implement priority tree per RFC 9218 (extensible priorities)
- [ ] Scheduler must respect priority (high-priority streams processed first)
- [ ] Monitor: if single low-priority stream delays high-priority stream by >10ms, apply fairness shaper
- [ ] Test: high-priority request delayed by low-priority request, measure latency

### Attack Class 15: Unexpected Packet Loss Amplification

**Description:** Network path with asymmetric loss causes repeated retransmission of large packets.

**Mitigations:**
- [ ] Implement congestion control (Cubic or BBR per RFC 9002)
- [ ] Limit retransmission count (max 10 retransmits before connection abort)
- [ ] Exponential backoff: first retransmit at 1x RTO, then 2x, 4x, etc.
- [ ] Test: simulate packet loss, verify connection stabilizes

### Attack Class 16: TLS Handshake Amplification

**Description:** Attacker triggers repeated TLS handshake messages, exhausting server resources.

**Mitigations:**
- [ ] Rate-limit handshake restarts: max 1 per 60 seconds per connection
- [ ] Stateless handshake: use stateless resumption tokens to reduce per-connection state
- [ ] Implement handshake timeout: abort if handshake incomplete after 60 seconds

### Attack Class 17: Cross-Protocol Request Smuggling

**Description:** Attacker crafts HTTP request that differs in interpretation between HTTP/3 and HTTP/2, enabling smuggling.

**Mitigations:**
- [ ] Enforce identical interpretation: if supporting both HTTP/2 and HTTP/3, normalize all headers uniformly
- [ ] Test: craft ambiguous request, send via both protocols, verify identical parsing
- [ ] Document: "HTTP/3 header parsing is authoritative; HTTP/2 must match exactly"

---

## Summary and Integration

This Dark Grimoire addendum serves as the canonical attack surface taxonomy for Unheaded protocol reviews. Each vulnerability category maps to specific LICH campaigns (fuzzing, concurrency, cryptanalysis) and RFC patch sections (mitigations, clarifications).

For complete attack surface mapping, cross-reference:
- Doom-Specific (D1-D6) -> LICH-007, RFC patches for Monad Foundation
- Cross-Document (X1-X4) -> RFC patch sections (clarify version handling, CRC scope, TLV rules, error handling)
- Computational Completeness -> LICH-007, RFC patches (instruction counter, timeouts)
- Concurrency Primitives -> LICH-008, LICH-010, Wotan Memory RFC patches
- HTTP/3 Cross-Pollination -> HTTP/3 integration tests (separate from LICH campaigns)

