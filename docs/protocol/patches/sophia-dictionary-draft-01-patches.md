# RFC Patches: Sophia Dictionary Draft-01

This document specifies normative corrections and security enhancements to the Sophia Dictionary RFC. All patches are presented as replacement text for specific sections.

---

## Patch S1: Add Provisioning Node Authentication (ML-DSA-65 Signature on CBOR)

### Issue
Current draft: Dictionary entries are transmitted as unsigned CBOR; no authentication mechanism.

**Problem:** Attacker can inject false dictionary entries without cryptographic proof of origin. Dictionary poisoning attacks enable cache invalidation, denial of service, or cross-program data injection.

**S21 Finding:** Dark Grimoire Section 2 (Cross-Document Consistency Attacks)

### Proposed Fix

**Add new Section 3.2 "Provisioning Node Authentication":**

```
3.2. Provisioning Node Authentication

Sophia dictionary entries originating from a provisioning node MUST be 
digitally signed using ML-DSA-65 (Module-Lattice-Based Digital Signature 
Algorithm, standardized by NIST FIPS 204).

3.2.1. Signature Format

Dictionary entries are packaged as:

  message DictionaryEntry {
    cbor_payload: bytes          # CBOR-encoded dictionary entry
    signature: bytes             # ML-DSA-65 signature (3366 bytes)
    public_key: bytes            # ML-DSA-65 public key (1952 bytes)
  }

The signature is computed over the CBOR payload (bytes only, not including 
signature or public key fields).

  signature = ML-DSA.Sign(
    signing_key = provisioning_node_private_key,
    message = cbor_payload
  )

3.2.2. Verification Procedure

On receiving a dictionary entry:

  1. Extract cbor_payload, signature, public_key from message
  2. Verify signature:
     is_valid = ML-DSA.Verify(
       public_key = public_key,
       message = cbor_payload,
       signature = signature
     )
  3. If is_valid:
     - Check if public_key is in provisioning node whitelist
     - If yes: accept entry, add to local dictionary
     - If no: reject entry (unknown provisioning node)
  4. If not is_valid:
     - Reject entry, log security event
     - Do NOT add to dictionary

3.2.3. Whitelist Management

Implementations MUST maintain a whitelist of authorized provisioning node 
public keys. This whitelist is configured during deployment (out-of-band 
mechanism, e.g., configuration file, environment variable, or secure boot).

Whitelist changes (adding/removing keys) require administrative action. 
No dynamic discovery of provisioning nodes is permitted.

3.2.4. Rationale

ML-DSA-65 is selected because:
- Provides 128 bits of security (quantum-resistant)
- NIST-standardized (FIPS 204), stable specification
- Practical signature size (3366 bytes) acceptable for network transmission
- No patented algorithms (contrasts with ECDSA licensing)

Digital signatures on dictionary entries ensure:
- Authenticity: only authorized provisioning nodes can create entries
- Non-repudiation: provisioning node cannot deny creating entry
- Tamper detection: any modification to CBOR payload invalidates signature
```

**Justification:**
- Defends against dictionary poisoning attacks
- Ensures only trusted provisioning nodes contribute to dictionary
- Aligns with Monad Foundation (M1: integrity protection everywhere)

---

## Patch S2: Explicit Reject at Modular Boundary 128

### Issue
Current draft: Dictionary size limits are informal; no explicit modular arithmetic boundary.

**Problem:** Dictionary can grow unbounded, exhausting memory. Lack of explicit boundary 
creates implementation divergence (some cap at 1MB, others at 100MB).

**S21 Finding:** Dark Grimoire Section 4 (Computational Completeness - DoS via resource exhaustion)

### Proposed Fix

**Add new Section 2.3 "Dictionary Size Constraints":**

```
2.3. Dictionary Size Constraints

Dictionary size is bounded by modular arithmetic. Specifically, the 
dictionary capacity is 2^7 = 128 entries maximum (per flow).

2.3.1. Per-Flow Dictionary Capacity

Each flow (identified by Monad flow_label) maintains its own dictionary. 
The per-flow dictionary is limited to 128 distinct entries.

When a new entry would be added (total entries + 1 > 128):
  - Reject the new entry
  - Return error code 0x09 (Insufficient buffer space)
  - Do NOT evict existing entries (FIFO, LRU, or other replacement policy)
  - Do NOT add entry to dictionary

This is a hard limit, not a soft limit. If dictionary is full, new entries 
are rejected until existing entries are explicitly removed (out-of-band 
mechanism, e.g., connection reset, flow termination).

2.3.2. Global Dictionary Capacity

System-wide (all flows), dictionary capacity is 100 MB (104,857,600 bytes total).

Per-flow allocation: 100 MB / max_flows.

If max_flows = 1,000,000, per-flow budget ≈ 100 bytes per dictionary entry 
(assuming 128 entries per flow = 12,800 bytes per flow).

Implementations MUST track global dictionary size. When global size would 
exceed 100 MB, reject new entries (same as per-flow limit exceeded).

2.3.3. Modular Boundary Rationale

The boundary 128 = 2^7 is a modular limit, chosen to:
- Prevent unbounded dictionary growth (hard limit)
- Fit within 7-bit index in CBOR compact encoding
- Align with cache line sizes (64-128 bytes typical)
- Simplify implementation (bit-width matching)

The 100 MB global limit is a practical safeguard:
- Typical system memory: 1 GB available for Unheaded
- Reserve 10% (100 MB) for dictionaries
- Remaining 900 MB for other protocol state (flows, caches, etc.)

2.3.4. Error Handling

When dictionary is full:

  if (per_flow_entries >= 128) OR (global_size >= 100_MB):
    return_error(0x09)  # Insufficient buffer space
    drop_new_entry()
  else:
    add_entry_to_dictionary()
```

**Justification:**
- Prevents denial of service via unbounded dictionary growth
- Explicit boundary eliminates implementation divergence
- Aligns with computational completeness attack surface (Dark Grimoire Section 4)

---

## Patch S3: CDDL Schema Normative (not Informative)

### Issue
Current draft: CDDL schema is marked "informative"; implementations may deviate.

**Problem:** Without normative CDDL, implementations diverge in CBOR encoding. 
Divergence enables parser confusion attacks (X3).

**S21 Finding:** X3 (TLV Extension Parsing Rules Divergence)

### Proposed Fix

**Change CDDL section from "Informative" to "Normative":**

```
Section 5: CDDL Schema (NORMATIVE)

The Sophia Dictionary CBOR encoding is formally specified using CDDL 
(Concise Data Definition Language, RFC 8610).

[Full CDDL schema here - example:]

DictionaryEntry = {
  id: uint8,           ; unique ID within flow (0-127)
  key: text,           ; lookup key (UTF-8 string, max 256 chars)
  value: bytes,        ; value (max 1 MB per entry)
  timestamp: uint64,   ; creation time (epoch seconds)
  ttl: uint32,         ; time-to-live (seconds)
  ? metadata: {
    ? source: text,
    ? version: uint8,
  }
}

Dictionary = [* DictionaryEntry]

All implementations MUST conform to this CDDL schema. CBOR streams that 
violate the schema MUST be rejected with error 0x07 (Unknown critical TLV).

CDDL Schema Compliance Test (MUST PASS):
  1. Encode DictionaryEntry per schema
  2. Decode on reference implementation
  3. Verify decoded value matches original
  4. Reject any CBOR that violates schema
```

**Justification:**
- Prevents parser divergence via ambiguous CBOR encoding
- Enforces strict schema compliance across implementations
- Aligns with Monad RFC (M6: TLV extension mechanism, critical bit semantics)

---

## Patch S4: "SHOULD be Signed" -> "MUST be Signed"

### Issue
Current draft: "Dictionary entries SHOULD be signed by provisioning node"

**Problem:** "SHOULD" is permissive; some implementations omit signatures, 
creating divergence. Attacker can inject unsigned entries on lenient implementation.

**S21 Finding:** X2, X3 (Cross-document divergence), Dark Grimoire D6

### Proposed Fix

**Replace all "SHOULD be signed" with "MUST be signed":**

```
All Dictionary entries originating from a provisioning node MUST be signed 
using ML-DSA-65 (see Section 3.2).

Dictionary entries originating from local generation (e.g., eBPF program 
creates entry) are NOT required to be signed (they are generated in-process, 
not transmitted).

Implementations MUST verify signatures before accepting dictionary entries 
from remote provisioning nodes. If signature is missing or invalid, reject 
entry immediately (error 0x07: Unknown critical TLV or equivalent).

Implementation Rationale:
- Local entries are trusted (same trust boundary as eBPF verifier)
- Remote entries MUST be authenticated (untrusted network)
- Divergence between implementations on signing requirement enables attacks
- By mandating "MUST", all implementations behave identically
```

**Justification:**
- Eliminates implementation divergence on signing requirement
- Defends against dictionary poisoning attacks
- Aligns with security posture of Dark Grimoire (assume malicious network)

---

## Patch S5: BPF Memory Quota Enforcement

### Issue
Current draft: No explicit memory quota for eBPF programs accessing dictionary.

**Problem:** eBPF program can allocate unbounded temporary memory during 
dictionary operations, exhausting kernel memory (denial of service).

**S21 Finding:** Dark Grimoire Section 4 (Computational Completeness)

### Proposed Fix

**Add new Section 4.1 "Memory Quota Enforcement":**

```
4.1. Memory Quota Enforcement

Each eBPF program has a memory quota for dictionary operations:
- Per-program limit: 1 MB of temporary memory
- Per-operation limit: 100 KB per dictionary lookup or update

4.1.1. Quota Tracking

The BPF verifier (kernel) tracks memory allocation for each eBPF program:

  memory_used = sum(all_temporary_allocations_by_program)
  if memory_used > 1_MB:
    reject_new_allocation()
    return error to program (memory limit exceeded)

4.1.2. Quota Reset

Memory quota resets when:
- eBPF program completes (returns from main function)
- Explicit bpf_mem_reset() call in program
- 60 second timeout (quota automatically reset)

4.1.3. Quota Enforcement in BPF Verifier

The BPF verifier MUST insert memory check instructions into all 
dictionary operation calls:

  Before: result = bpf_dict_lookup(key)
  After:  
    check_mem_quota(1_MB)  # Inserted by verifier
    if quota_exceeded:
      return_error()
    result = bpf_dict_lookup(key)

4.1.4. Rationale

Memory quota prevents:
- Unbounded temporary allocation during dictionary operations
- Kernel memory exhaustion (kernel OOM killer triggered)
- Denial of service (all flows starved of memory)

Limits are conservative:
- 1 MB per program: sufficient for typical dictionaries (128 entries × 8KB = 1 MB)
- 100 KB per operation: sufficient for single lookup with inline decompression

Implementation burden:
- Kernel already tracks memory in BPF verifier
- Quota enforcement is 1-2 additional instructions per dictionary call
```

**Justification:**
- Prevents out-of-memory denial of service attacks
- Kernel-enforced quota (not application-layer soft limit)
- Aligns with BPF security model (verifier hardening)

---

## Patch S6: QPACK-Style Encoder/Decoder Stream for State Sync

### Issue
Current draft: Dictionary state is point-in-time snapshot; no incremental sync mechanism.

**Problem:** When flows migrate (e.g., connection migration in QUIC), 
dictionary state may be lost or inconsistent. No standard mechanism to 
synchronize dictionary updates across endpoints.

**S21 Finding:** Dark Grimoire Section 6 (HTTP/3 Cross-Pollination, QPACK attacks)

### Proposed Fix

**Add new Section 6 "Dictionary State Synchronization":**

```
6. Dictionary State Synchronization (QPACK-style)

Dictionary state can be synchronized between endpoints using a QPACK-style 
encoder/decoder stream (similar to QPACK dynamic table updates in HTTP/3 RFC 9204).

6.1. Stream Format

Dictionary updates are encoded as:

  DictionaryUpdate = {
    operation: uint8,     ; 0=add, 1=remove, 2=replace
    id: uint8,            ; entry ID (0-127)
    key: text,            ; lookup key
    value: bytes,         ; new value (for add/replace)
    expiration: uint64,   ; absolute time when entry expires
  }

Updates are serialized as CBOR and transmitted on a control stream 
(separate from data packets).

6.2. Encoder (Sender)

When dictionary is modified locally:

  1. Create DictionaryUpdate message
  2. Serialize to CBOR
  3. Send on dictionary state stream (TLV type 0x25)
  4. Receiver processes update asynchronously

6.3. Decoder (Receiver)

When dictionary update is received:

  1. Validate signature (if remote update)
  2. Deserialize CBOR
  3. Apply operation (add/remove/replace)
  4. Acknowledge receipt (optional, for reliability)

6.4. Reliability

Updates are sent on a control stream with retransmission. If a DictionaryUpdate 
is not acknowledged within 1 second, resend (TCP-style reliability).

6.5. Rationale

QPACK-style streaming enables:
- Incremental synchronization (not full snapshot)
- Lower latency (updates in-flight as changes occur)
- Connection migration support (dictionary state preserved)
- Alignment with HTTP/3 (similar mechanism already proven in practice)

Unlike QPACK, Sophia dictionary synchronization:
- Is optional (implementations MAY choose not to sync)
- Uses authenticated updates (signed by provisioning node)
- Has bounded state size (128 entries per flow, 100 MB global)
```

**Justification:**
- Enables dictionary state consistency across connection migration
- Aligns with HTTP/3 integration (Dark Grimoire Section 6)
- Reduces latency and bandwidth for dictionary operations

---

## Patch S7: Dictionary Size Limits (Per-Flow 1MB, Global 100MB)

### Issue
Current draft: Dictionary size limits are informal (or missing).

**Problem:** Duplicate of S2, but with specific byte limits (not just entry count).

### Proposed Fix

**Refine Section 2.3 (from S2) with byte limits:**

```
2.3. Dictionary Size Constraints (Refined)

Dictionary capacity is bounded by both entry count and total bytes.

2.3.1. Per-Flow Capacity

Each flow dictionary is limited to:
- Maximum 128 entries (MUST)
- Maximum 1 MB total size (MUST)

Total size = sum(CBOR-encoded entry size, including length headers)

When adding entry would exceed either limit:
  - Reject new entry
  - Return error 0x09 (Insufficient buffer space)

Example:
  - Entry 127 exists, uses 5 KB
  - New entry would be 10 KB
  - Per-flow total: 127 entries, 995 KB
  - Adding entry: 128 entries, 1005 KB (exceeds 1 MB)
  - Result: REJECT

2.3.2. Global Capacity

System-wide dictionary total across all flows MUST NOT exceed 100 MB.

When global total would exceed 100 MB:
  - Reject new entries for all flows
  - Return error 0x09 to all requestors
  - Continue serving existing entries
  - Free memory by closing least-recently-used flows

Global limit is enforced per-node (not per-flow-pair or per-connection).
Multiple connections on same node share the 100 MB global budget.

2.3.3. Implementation

Dictionary sizes are tracked by:
- Per-flow: atomic counter (updated on add/remove)
- Global: shared atomic counter (updated on add/remove)

Checks are performed before allocation:

  if flow_size + entry_size > 1_MB:
    return_error(0x09)
  if global_size + entry_size > 100_MB:
    return_error(0x09)
  allocate_and_add(entry)
```

**Justification:**
- Concrete byte limits (easier to implement and verify)
- Prevents resource exhaustion on both per-flow and global scale
- Aligns with Dark Grimoire Section 4 (DoS prevention)

---

## Patch S8: Compression Guard Flags

### Issue
Current draft: Compression algorithm selection is implicit; no explicit flags.

**Problem:** Without explicit compression selection, implementations may 
diverge (some use gzip, others use zstd, others use no compression).

**S21 Finding:** X3 (TLV parsing rules divergence)

### Proposed Fix

**Add new Section 5.1 "Compression Guard Flags":**

```
5.1. Compression Guard Flags

Dictionary entries MAY be compressed. Compression algorithm is selected 
via explicit flags in the entry header.

5.1.1. Compression Flag Format

  DictionaryEntry = {
    id: uint8,
    key: text,
    value: bytes,
    timestamp: uint64,
    ttl: uint32,
    ? compression: {
      ? algorithm: uint8,    ; 0=none, 1=gzip, 2=zstd
      ? compressed_size: uint16,
    }
  }

5.1.2. Supported Algorithms

  0: No compression (raw bytes)
  1: gzip (RFC 1952, for text payloads)
  2: Zstandard (RFC 8878, for binary payloads)

5.1.3. Compression Selection

Implementations MUST support at least:
- Algorithm 0 (no compression) - REQUIRED
- Algorithm 2 (zstd) - RECOMMENDED for binary, MAY implement for text

Algorithm 1 (gzip) is OPTIONAL but RECOMMENDED for text-heavy entries.

5.1.4. Decompression

On retrieving a dictionary entry:

  if entry.compression.algorithm == 0:
    return entry.value as-is
  elif entry.compression.algorithm == 1:
    return gzip.decompress(entry.value, max_size=1_MB)
  elif entry.compression.algorithm == 2:
    return zstd.decompress(entry.value, max_size=1_MB)
  else:
    return_error(0x07)  # Unknown compression algorithm (treat as critical TLV error)

Decompression MUST validate:
- Decompressed size does not exceed 1 MB
- Decompression completes within 10 ms (timeout prevents DoS)
- All bytes are consumed (no trailing data)

5.1.5. Guard Flag Rationale

Explicit compression flags prevent:
- Parser confusion (decoder doesn't guess compression)
- Compression bomb attacks (size limit prevents decompression bomb)
- Performance variability (timeout prevents resource exhaustion)

Implementation must respect flags; silent guessing is not permitted.
```

**Justification:**
- Explicit compression selection prevents parser divergence (X3)
- Guard flags (size limit, timeout) prevent compression bomb DoS
- Aligns with HTTP/3 compression (Section 6 patch: QPACK)

---

## Summary

All Sophia Dictionary patches:

| Patch | Issue | Finding | Impact |
|-------|-------|---------|--------|
| S1 | No authentication | Dark Grimoire D1 | Adds ML-DSA-65 signing |
| S2 | Unbounded dictionary | Dark Grimoire Section 4 | 128 entries, 1MB/100MB limits |
| S3 | CDDL informative | X3 | Makes CDDL normative |
| S4 | SHOULD vs MUST sign | X2, X3 | Mandates signing |
| S5 | No memory quota | Dark Grimoire Section 4 | 1MB per-program quota |
| S6 | No state sync | Dark Grimoire Section 6 | QPACK-style streaming |
| S7 | Informal size limits | Dark Grimoire Section 4 | Per-flow 1MB, global 100MB |
| S8 | No compression spec | X3 | Explicit compression guard flags |

