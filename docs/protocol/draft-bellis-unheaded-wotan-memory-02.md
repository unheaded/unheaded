---
title: "Wotan Memory Protocol for the Unheaded Protocol"
abbrev: "Wotan Memory Protocol"
docname: draft-bellis-unheaded-wotan-memory-02
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
date: 2026-02-27
stand_alone: yes

author:
  - ins: S. Bellis
    name: Stevie Bellis
    org: Unheaded
    email: stevie@bellis.tech
    country: US

normative:
  RFC2119:
  RFC8174:
  RFC9669:
  UNHEADED-FOUNDATION:
    title: "Unheaded: Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-04
  UNHEADED-SOPHIA:
    title: "Sophia Dictionary Format for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-sophia-dictionary-01

informative:
  RFC0768:
  RFC0791:
  RFC0792:
  RFC0793:
  RFC8799:
  RFC9180:
  FIPS203:
    title: "Module-Lattice-Based Key-Encapsulation Mechanism Standard"
    author:
      org: National Institute of Standards and Technology
    date: 2024-08
    target: https://csrc.nist.gov/pubs/fips/203/final

--- abstract

Wotan is the memory and I/O bus for the Unheaded Protocol, providing addressable per-flow storage for BPF programs executing within the Limited Domain [RFC8799].

The Wotan protocol specifies the BPF helper interface for memory access, the address space layout for per-flow data structures, a three-level cache hierarchy (L1 BPF hash maps, L2 per-flow ring buffers, L3 persistent Write-Ahead Log), and the topic-based I/O model for interaction with userspace services.

This memo defines the memory model, helper functions, address space, cache miss protocol, gRPC streaming contracts, triple-role architecture, reliability guarantees, and I/O topic naming conventions for systems implementing the Unheaded Protocol's computational layer. Draft-02 incorporates normative security patches W1-W8 addressing LICH findings and cross-flow isolation attacks.

---

# Introduction

The Unheaded Protocol [UNHEADED-FOUNDATION] specifies a 20-byte register file (the Monad) that travels with every packet through a Limited Domain. BPF programs at each hop read and write the Monad, performing stateless per-packet computation.

Many use cases require state beyond the 20-byte Monad: buffering input, accumulating results, maintaining per-flow state machines, or storing scratch memory for complex algorithms.

Wotan provides this state via a hierarchical memory model:

- L0: Monad (20 bytes, in packet, per-hop latency ~320 ns)
- L1: Per-hop BPF map cache (64-byte cache lines, ~100-200 ns latency)
- L2: Per-flow ring buffer RAM (configurable size, ~1-10 µs latency)
- L3: Write-Ahead Log (persistent storage, ~100 µs-1 ms latency)
- L4: Sophia dictionaries (instruction decode, ~100-200 ns latency)

This memo defines the Wotan memory protocol: the BPF helper interface, address space layout, cache coherency model, and userspace I/O interaction.

# Terminology and Language

The key words "MUST", "MUST NOT", "REQUIRED", "SHALL", "SHALL NOT", "SHOULD", "SHOULD NOT", "RECOMMENDED", "MAY", and "OPTIONAL" in this document are to be interpreted as described in BCP 14 [RFC2119] [RFC8174] when, and only when, they appear in all capitals, as shown here.

The following terms are used:

Flow Label:
: The IPv6 Flow Label field (20 bits) or derived hash used to key per-flow state in Wotan. Maps packets to unique per-flow ring buffers.

Ring Buffer:
: A BPF ring buffer (BPF_MAP_TYPE_RINGBUF) allocated per flow, used as L2 memory (general-purpose RAM) with configurable size via --ring-size.

Cache Line:
: A 64-byte unit of L1 cache (per-hop BPF map), with tag, valid, dirty, and LRU tracking.

Write-Back:
: Transfer of dirty cache lines from L1 (per-hop map) to L2 (ring buffer) for persistence or hand-off.

Memory-Mapped I/O:
: Designated address ranges that publish to or read from Wotan topics (e.g., write to address 0x0000C000 publishes to compute.screen).

# Architecture Overview

## Role in the Unheaded Protocol

Wotan bridges Monad computation to memory and I/O:

- Shim programs (BPF) running at each hop read/write Wotan memory via BPF helpers (bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas).
- Wotan maintains per-flow state keyed by IPv6 Flow Label.
- Wotan interfaces with userspace via ring buffer events and pub/sub topics.
- Wotan implements cache miss handling, prefetching, and Write-Ahead Log management.

## Memory Hierarchy

~~~~~
Level   Name                Size           Latency      Backing
------  --------------------  ---------      ----------   ----------
L0      Monad (packet)      20 bytes       ~ns          wire
L1      Cache (BPF map)     variable       ~100-200ns   per-hop
L2      Ring Buffer (RAM)   configurable   ~1-10µs      per-flow
L3      Write-Ahead Log     disk           ~100µs-1ms   persistent
L4      Sophia dictionaries BPF maps       ~100-200ns   instruction decode
~~~~~

Wotan implements transparent L1→L2 promotion on cache miss, L2→L3 flush on overflow, and L3→L2 recovery on process restart.

## Separation of Compute and Memory

The Monad is transient compute state (stateless by design). Wotan is persistent state machine storage. This separation allows:

- Shim programs to remain stateless with respect to the packet format.
- External state to be accessed in a controlled, measurable manner.
- Cache miss latency to be handled without blocking per-hop logic.
- Memory updates to be tracked in Anamnesis for observability.

# BPF Helper Interface

BPF Shim programs access Wotan memory via three helper functions. All helpers operate on a 32-bit address space keyed by IPv6 Flow Label.

## bpf_wotan_read

Read from Wotan memory.

~~~~~
long bpf_wotan_read(u32 flow_label, u32 addr, void *buf, u32 len);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label (zero-extended to u32), identifying the flow
- addr: 32-bit address within the flow's address space
- buf: pointer to destination buffer (user-controlled)
- len: number of bytes to read (MUST be 1, 2, or 4; others return -EINVAL)

**Returns:**
- On success: number of bytes read (len)
- -ENOENT (-2): flow_label not found; no ring buffer allocated for this flow
- -EFAULT (-14): addr out of bounds for this flow's address space
- -ENOMEM (-12): L1 cache miss; miss event emitted, caller SHOULD stall
- -EACCES (-13): program not authorized for this flow_label
- -EINVAL (-22): len not in {1, 2, 4}

**Semantics:**

The helper performs the following steps:

1. Verify len in {1, 2, 4}. Return -EINVAL otherwise.

2. Check per-program access control: is the calling program authorized to read from this flow_label? Return -EACCES if not.

3. Compute cache line address: `cache_addr = addr & ~0x3F` (64-byte alignment).

4. Look up cache line in L1 BPF map keyed by (flow_label, cache_addr). If found and valid:
   a. Extract data at offset (addr & 0x3F)
   b. Copy len bytes to buf
   c. Return len

5. If cache miss: emit wotan_cache_miss_event to compute.mem.miss ring buffer. Return -ENOMEM (signal to caller to stall).

6. Userspace miss handler reads compute.mem.miss, fetches from L2 ring buffer, populates L1 cache line, and signals resumption.

The helper MUST NOT block; on miss, it returns immediately.

## bpf_wotan_write

Write to Wotan memory.

~~~~~
long bpf_wotan_write(u32 flow_label, u32 addr, const void *buf, u32 len);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label, identifying the flow
- addr: 32-bit address within the flow's address space
- buf: pointer to source buffer (user-controlled)
- len: number of bytes to write (MUST be 1, 2, or 4)

**Returns:**
- On success: number of bytes written (len)
- -ENOENT (-2): flow_label not found
- -EFAULT (-14): addr out of bounds
- -ENOMEM (-12): L1 cache miss (same as read); L2 ring buffer full or missing
- -EACCES (-13): program not authorized for this flow_label
- -EINVAL (-22): len not in {1, 2, 4}

**Semantics:**

1. Verify len in {1, 2, 4}. Return -EINVAL otherwise.

2. Check access control. Return -EACCES if not authorized.

3. Compute cache line address: `cache_addr = addr & ~0x3F`.

4. Look up or allocate cache line in L1 map. If miss: emit cache_miss_event with access_type=1 (write) and return -ENOMEM.

5. On hit: copy len bytes from buf to cache line data at offset (addr & 0x3F). Set dirty bit = 1.

6. Return len.

Writes go only to L1 cache; they are marked dirty for later write-back to L2 by userspace handler or on overflow.

## bpf_wotan_cas

Atomic compare-and-swap on Wotan memory.

~~~~~
long bpf_wotan_cas(u32 flow_label, u32 addr, u32 expected, u32 desired);
~~~~~

**Arguments:**
- flow_label: 20-bit IPv6 Flow Label
- addr: 32-bit address (MUST be 4-byte aligned)
- expected: expected current value (u32)
- desired: value to write if current == expected

**Returns:**
- 0: swap successful (current was == expected, now == desired)
- -EAGAIN (-11): current != expected (swap not performed)
- -ENOENT (-2): flow_label not found
- -EFAULT (-14): addr out of bounds or not 4-byte aligned
- -EACCES (-13): program not authorized for this flow_label

**Semantics:**

1. Verify addr is 4-byte aligned. Return -EFAULT if not.

2. Check access control. Return -EACCES if not authorized.

3. Look up cache line. On miss, return -ENOMEM (same as read).

4. Atomically:
   a. Read current u32 at (addr & 0x3F + offset)
   b. If current == expected: write desired, set dirty bit, return 0
   c. If current != expected: return -EAGAIN

The CAS operation MUST use atomic compare-and-swap semantics (via BPF_XADD or
architecture-specific atomic instructions as defined in [RFC9669] Section 4.3)
to prevent races between concurrent Shims on different CPUs or cores.

### CAS Alignment Enforcement (PATCH W3)

The address MUST be 8-byte aligned (not 4-byte). The BPF verifier MUST enforce
this at program load time by injecting alignment checks. Unaligned CAS operations
cause architecture-dependent failures on ARM64 and RISC-V:

```c
// In Wotan helper before CAS (draft-02):
if ((addr & 0x7) != 0) {  // Check if not 8-byte aligned
    return -EFAULT;  // Return alignment error
}
```

**BPF Verifier Requirement (MANDATORY)**: The BPF verifier MUST reject programs
that perform CAS operations on misaligned addresses. This check MUST be inserted
during the verification pass, before JIT compilation. Programs failing alignment
checks are rejected at load time (not at runtime).

```c
// Verifier pseudo-code (injected before CAS):
if ((base_register & 0x7) != 0 || (immediate_offset & 0x7) != 0:
    reject_program()  // Fail verification
```

**Architecture-Specific Behavior**:

x86-64:
- LOCK CMPXCHG instruction tolerates misalignment (slow, but not fatal)
- Alignment check is software-only (BPF verifier rejects misaligned programs)

ARM64:
- LDAXR/STLXR (exclusive load/store) REQUIRE 8-byte alignment (hardware enforced)
- Misaligned operations cause illegal instruction exception (hardware fault)
- BPF verifier check prevents this fault

RISC-V:
- AMO.SWAPD requires 8-byte alignment (hardware enforced)
- Misaligned operations cause access fault (hardware exception)
- BPF verifier check prevents this fault

### CAS Atomicity Limitations

CAS operation atomicity is ONLY guaranteed for a single 64-bit word at the
specified address. Important limitations:

1. **Single Address Only**: CAS atomically compares and swaps one u64 at one
   address. It does NOT provide transactional semantics across multiple addresses.

2. **Per-Address Atomicity**: If Shim needs to update multiple related fields,
   CAS only guarantees atomicity for one of them. Concurrent accesses from other
   Shims may see intermediate states.

3. **Non-Blocking**: CAS is non-blocking—it returns immediately with success or
   -EAGAIN. It does NOT stall.

4. **Concurrent Flows**: CAS operations on different flows (different keys) do
   not interfere; each flow's CAS is independent.

5. **Alignment Requirement (PATCH W3)**: Address MUST be 8-byte aligned. The BPF
   verifier MUST check this at program load time and reject misaligned programs.
   No runtime fallback; alignment violations are caught at verification (fail-fast).

**Recommended Use Pattern**:
- Use CAS for per-flow state counters (e.g., packet count)
- Do NOT use CAS for multi-field updates expecting atomic visibility
- For complex transactions, use a per-flow spinlock implemented via CAS on a
  separate "lock" field (u64-aligned)
- Ensure base register and offset are both 8-byte aligned before calling bpf_wotan_cas()
- Let the BPF verifier catch alignment mistakes at load time

## Error Handling

Implementations MUST handle all specified error codes. RECOMMENDED error handling:

- -ENOENT: log flow allocation failure, skip memory access, record to Anamnesis anomaly
- -EFAULT: log address violation, skip access, mark packet for dropped-event
- -ENOMEM (cache miss): increment miss counter, optionally stall and retry via BPF_TAIL_CALL
- -EACCES: log authorization failure, drop packet or set flow_action=DROP
- -EINVAL: log invalid parameter, skip access

Programs MUST NOT crash on negative returns; they MUST check return values and branch accordingly.

## Access Control

Wotan enforces per-program, per-flow-label authorization. Each BPF program (identified by its file descriptor or program ID) is associated with a set of allowed flow_labels via Sophia dictionary configuration.

Authorization check pseudocode:

~~~~~
bool is_authorized(program_id, flow_label):
  entry = bpf_map_lookup(&sophia_auth_map, &program_id)
  if !entry: return false
  if entry.allowed_labels == 0: return true  // wildcard (all flows)
  return (entry.allowed_labels & BIT(flow_label)) != 0
~~~~~

Access control rules are defined per deployment and distributed via Sophia. Default: all programs authorized for all flows (wildcard). Operators SHOULD restrict access to enforce least-privilege per BPF program.

# Memory Address Space

## Address Layout

Each flow's per-flow address space is 0x00000000 to 0x00FFFFFF (16 MB):

~~~~~
0x00000000 +------------------+
           |  Data Memory     |  0x00000000-0x0000BFFF (48 KB RAM)
           |  (Wotan ring)    |  General-purpose per-flow state
0x0000BFFF +------------------+
0x0000C000 |  Screen / Output |  0x0000C000-0x0000FFFE (16 KB I/O)
           |  (MMIO region)   |  Writes publish to compute.screen
0x0000FFFE +------------------+
0x0000FFFF |  Input (1 word)  |  0x0000FFFF (4 bytes, read-only)
           |                  |  Reads consume from compute.input
           +------------------+
0x00010000 |  Extended Memory |  0x00010000-0x00FFFFFF (Optional ROM)
           |  (configurable)  |  Dictionary/WAD data, backed by WAL
0x00FFFFFF +------------------+
~~~~~

## Data Memory Region (0x00000000-0x0000BFFF)

Per-flow RAM, allocated from Wotan ring buffer. Size configurable via --ring-size (default: 16 KB).

Used for:
- Stack: top-of-RAM, grows downward
- Heap: allocated upward from 0x00000000
- Scratch: temporary computation state
- Counters: per-flow statistics

Addressing: Linear, byte-addressed. Shims MAY use conventional stack/heap management or flat address space, at operator's discretion.

## I/O Memory Region (0x0000C000-0x0000FFFE)

Memory-mapped I/O. Writes to this region are non-blocking publishes to Wotan topics. Reads from this region fetch from Wotan topics (blocking if no data available, or non-blocking with a timeout).

~~~~~
Address Range     Meaning                Topic Published
-----------       ----------             -----------------
0x0000C000        Screen / Output        compute.screen.{flow_label}
0x0000DFFF        (reserved, TBD)        (future I/O endpoints)
~~~~~

Write to 0x0000C000-0x0000DFFF: data is published to compute.screen.{flow_label}. Each write MUST include:
- 2-byte header: (length in bytes, 1-2 octets)
- data payload (up to 16 KB)

Implementation: Writes to MMIO addresses trigger an event publication (non-blocking ringbuf_output).

## Input Region (0x0000FFFF)

Single 4-byte memory-mapped I/O location. Reads consume one event from
compute.input.{flow_label} topic.

### Read Semantics

- Read returns 4-byte event data if available
- If no event: return 0x00000000 (default), or -EAGAIN if non-blocking mode enabled
- Multiple reads in succession fetch successive events (FIFO)

### Write Behavior

Writes to address 0x0000FFFF are not permitted and MUST be rejected:

- Shim issues bpf_wotan_write() to 0x0000FFFF
- Helper returns -EACCES error code
- Shim MUST handle this error gracefully
- No data is written; operation is silently rejected

This enforces the read-only nature of the input region and prevents Shim
programs from accidentally modifying the input queue.

## Extended Memory (0x00010000-0x00FFFFFF)

Optional: dictionary/WAD (Write-Anywhere Data) region backed by userspace. Used for:
- Read-only program code (ROM)
- Sophia dictionary local copy
- Large data structures (graphs, tables)

Allocation: per-flow, configured at startup. Misses backed by WAL recovery on restart.

## Per-Flow Addressing via Composite Key

Each IPv6 packet carries a 20-bit Flow Label, source and destination addresses,
and a transport protocol identifier. Wotan uses a composite key for per-flow
ring buffer allocation to prevent collisions:

~~~~~
Wotan ring buffer map key:
  struct wotan_rb_key {
    u32 flow_label;       // 20-bit Flow Label from IPv6 header (zero-extended)
    u64 src_ip;           // First 64 bits of source IPv6 address
    u64 dst_ip;           // First 64 bits of destination IPv6 address
    u8  protocol;         // IPv6 Next Header (transport protocol)
    u8  reserved[3];      // Alignment padding
  };
  hash = BLAKE3(wotan_rb_key)  // 256-bit hash used as primary key
~~~~~

This composite key prevents Flow Label collisions. RFC 6437 explicitly states
that Flow Labels are not guaranteed to be unique, and multiple flows can
share the same Flow Label. Using only the Flow Label would cause different
flows to share memory, violating data isolation.

**Rationale**: Two packets with identical Flow Label but different source,
destination, or protocol represent different flows and MUST have separate
ring buffers.

On first access to a new flow (identified by composite key), Wotan MUST
allocate a new ring buffer and associated L1 cache map. Ring buffer is
freed on flow timeout (configurable, typically 30-300 seconds).

# L1 Cache: Per-Hop BPF Maps

## Cache Line Structure and Composite Key

L1 cache is a per-hop BPF hash map (BPF_MAP_TYPE_HASH). Per [RFC 6437], IPv6 Flow
Labels are NOT guaranteed to be unique; multiple distinct flows can share the same
Flow Label. To prevent cross-flow memory access attacks (LICH-009), Wotan extends
the L1 cache key from 20-bit flow_label to a 64-bit composite key:

~~~~~
struct l1_cache_key {
    u64 flow_composite_key;  // 64-bit composite key (see Section 4.2)
    u32 addr;                // cache-line-aligned address (& ~0x3F)
};

struct l1_cache_line {
    u8  data[64];            // 64-byte cache line (8 x u64)
    u32 tag;                 // address tag for mismatch detection
    u8  valid;               // 0=invalid, 1=valid
    u8  dirty;               // 0=clean, 1=dirty (needs write-back)
    u16 lru_counter;         // for LRU eviction ordering
    u8  seqno_valid;         // PATCH W1: seqno monotonicity flag
    u8  _pad[3];             // alignment
};
~~~~~

Cache lines default to 64 bytes (one L3 cache line on x86-64), matching CPU
hardware cache line size for optimal performance on modern systems.

### L1 Cache Composite Key (PATCH W2)

The L1 cache key is computed as a 64-bit composite of flow_label and source/destination
tuple hash, preventing birthday attack collisions (LICH-009):

~~~~~
flow_composite_key = (flow_label << 44) | (tuple_hash & 0xFFFFFFFFFFF)
  # 20 bits (flow_label) + 44 bits (tuple_hash) = 64 bits

Where:
  - flow_label: 20-bit value from Monad header (zero-extended)
  - tuple_hash: 44-bit SipHash-2-4 of (src_ip, dst_ip, src_port, dst_port)

Computation:
  tuple_hash = (
    siphash2_4(
      key = flow_label || <16 random bytes per session>,
      data = src_ip || dst_ip || src_port || dst_port
    ) & 0xFFFFFFFFFFF  // Mask to 44 bits
  )
~~~~~

**Entropy Analysis**: Composite key entropy = 20 + 44 = 64 bits (full u64 space).
Birthday attack collision probability with 262K cache lines (16 MB / 64-byte lines)
and composite key: (262K)^2 / 2^64 ≈ 10^-12 (negligible). Single 20-bit flow_label
key enables 707 expected collisions for 2^20 samples; composite key extends collision
probability to negligible range.

**Backward Compatibility**: Draft-02 requires composite keys. Implementations MUST
NOT accept 20-bit-only keys for new flows; deprecate support in draft-03.

### Cache Line Size Configuration

While 64 bytes is the default, implementations MAY configure cache line size
to match their target hardware:

| Architecture | Typical Cache Line | Default for Wotan |
|--------------|-------------------|------------------|
| x86-64 (Intel/AMD) | 64 bytes | 64 bytes |
| ARM64 (Graviton, Neoverse) | 64 bytes | 64 bytes |
| ARM64 (older designs) | 32 bytes | 64 bytes (compatible) |
| PowerPC | 128 bytes | 64 bytes (compatible, suboptimal) |
| RISC-V | 64 bytes | 64 bytes |

Implementations SHOULD allow operators to override cache line size via
configuration:
```
wotan:
  cache_line_size_bytes: 64  # Default
```

Using the correct cache line size for target hardware can improve L1 cache
hit rate and reduce memory bandwidth usage.

## Prefetch Model

Prefetch is explicit, not automatic. A Shim MAY call bpf_wotan_read on a nearby address to trigger prefetch of that cache line into L1:

~~~~~
// Prefetch next cache line
long ret = bpf_wotan_read(flow_label, next_addr, &dummy, 1);
if (ret == -ENOMEM) {
    // Cache miss; handler will populate
}
~~~~~

Userspace prefetch handler monitors compute.mem.miss topic and proactively reads from L2 ring buffer, populating L1 before Shim retry.

## Cache Miss Handling

On L1 cache miss (step 5 of bpf_wotan_read):

1. Shim calls bpf_wotan_read() → returns -ENOMEM
2. Helper emits wotan_cache_miss_event to compute.mem.miss ring buffer
3. Wotan userspace daemon reads compute.mem.miss events
4. For each miss:
   a. Fetch cache line from L2 ring buffer (Wotan per-flow storage)
   b. Write cache line to L1 BPF map
   c. Signal Shim for retry (via BPF_TAIL_CALL or message)
5. Shim retries bpf_wotan_read() → hit (returns len)

Cache miss event structure:

~~~~~
struct wotan_cache_miss_event {
    u32 flow_label;    // which flow
    u32 addr;          // miss address (cache-line-aligned by hardware)
    u32 len;           // requested length (1, 2, or 4)
    u8  access_type;   // 0=read, 1=write
    u8  pad[3];        // alignment
};
// Emitted to: compute.mem.miss ring buffer
~~~~~

## Write-Back Policy

Dirty cache lines are written back to L2 ring buffer via userspace handler:

1. Userspace monitors L1 cache map
2. On overflow or TTL timeout, marks dirty lines for write-back
3. Writes dirty line data + metadata to L2 ring buffer
4. Clears dirty bit in L1

Write-back frequency: RECOMMENDED flush every 100 ms or on L1 map occupancy >80%, whichever comes first. This balances latency (L1 hits) with memory efficiency (L2 persistence).

## Eviction (LRU)

When L1 map reaches capacity (configurable max_entries, default 1024 lines = 64 KB):

1. Identify least-recently-used cache line (lowest lru_counter)
2. If dirty: write back to L2 before eviction
3. Evict from L1 map
4. Allocate new line from freed space

LRU counter incremented on every cache hit.

### LRU Counter Overflow Handling

The lru_counter field is u16 (16 bits), range 0-65535. After 65536 hits,
counter overflows to 0. To prevent incorrect LRU ordering after overflow:

**Option A (Recommended): Periodic Counter Reset**

Userspace daemon periodically (every 1 second or 1000 hits, whichever first):
1. Scan L1 cache map
2. Divide all lru_counters by 2 (shift right):
   ```
   for each cache line: line.lru_counter >>= 1;
   ```
3. This prevents overflow while maintaining relative ordering

**Option B: Timestamp-Based LRU**

Replace lru_counter with timestamp-based LRU:
- Use bpf_ktime_get_ns() instead of counter
- Userspace still needs to divide by 2 periodically
- Provides better temporal accuracy

**Implementation Note**: Without overflow handling, oldest cache lines could
become newest (0 < 65535), causing incorrect eviction order. Operators MUST
implement one of these strategies.

# L2 Ring Buffer: Per-Flow RAM

## Ring Buffer Structure (PATCH W1)

Per-flow RAM is a BPF ring buffer (BPF_MAP_TYPE_RINGBUF). Each entry carries a
64-bit monotonically-increasing sequence number for integrity validation:

~~~~~
struct wotan_rb_entry {
    u64 seqno;              // PATCH W1: monotonically increasing sequence number
    u64 timestamp_ns;       // bpf_ktime_get_ns()
    u32 flow_label;
    u32 addr;               // address of write
    u8  data[64];           // cache line data
    u32 crc32;              // CRC-32 checksum over all fields
    u8  valid;              // 1 if entry is valid
    u8  _pad[3];            // alignment
};
// Total: 88 bytes per entry
~~~~~

**Seqno Semantics (PATCH W1)**:

The seqno field is initialized to 0 for the first entry and increments by 1
for each subsequent entry:

```
entry[N].seqno = entry[N-1].seqno + 1
entry[0].seqno = 0
```

**Monotonicity Validation (PATCH W1)**:

On reading ring buffer entries (recovery, prefetch, etc.), implementations MUST
validate monotonicity:

```
last_seqno = -1
for entry in ring_entries:
  if entry.seqno != last_seqno + 1:
    # Seqno gap detected
    log_alert("WAL seqno gap: expected", last_seqno + 1, "got", entry.seqno)
    skip_to_next_valid_seqno()
  last_seqno = entry.seqno
  validate_crc32(entry)
```

If seqno discontinuity is detected:
- Ring buffer or WAL is corrupted (compaction lost entries, or power failure during write)
- Replay to last known-good seqno
- Log security alert
- Emit ANOMALY event to Anamnesis
- Return error to application (data loss is visible, not silent)

**CRC-32 Computation**: CRC-32 is computed over all ring entry fields
(seqno through valid flag), ensuring integrity protection across all metadata.

Accessed by:
- Shim: via bpf_wotan_{read,write,cas} helpers (L2 is backing for L1 miss)
- Userspace: via ringbuf_consume() for write-back drain and recovery

### Ring Buffer Entry Size vs Cache Line Alignment

The 80-byte entry size does NOT align with the 64-byte cache line:
- Cache line (L1): 64 bytes
- Ring buffer entry: 80 bytes
- Per entry overhead: 16 bytes (2×64-bit fields + validity flag)

This asymmetry is intentional:
1. **L1 Cache Lines**: Data payloads are exactly 64 bytes (CPU cache size)
2. **L2 Ring Buffer**: Entries include metadata (timestamp, flow_label, addr, valid)
   Resulting in 80-byte entries that span 1.25 cache lines

**Memory Efficiency Trade-off**:
- Pro: 16-byte metadata enables recovery and ordering
- Con: entries don't pack perfectly into cache lines (12.5% overhead per entry)

For high-performance deployments, consider using exactly 64-byte entries:
```c
struct wotan_rb_entry_compact {
    u64 timestamp_ns;
    u32 flow_label;
    u32 addr;
    u8  data[60];        // Reduced to fit in 64 bytes total
    u8  valid;
    u8  _pad[3];
};
// Total: 64 bytes per entry (perfect cache alignment)
```

This requires padding data to 60 bytes, losing 4 bytes of capacity. Operator
configuration determines the trade-off.

## Allocation (per Flow Label)

On first access to a new flow_label:

~~~~~
Wotan allocates:
1. New BPF ring buffer (sized via --ring-size)
2. New L1 cache BPF hash map
3. New access control entry in Sophia auth map
4. Emit allocation event to Anamnesis
~~~~~

Allocation is lazy (on first Shim access) or eager (pre-allocated for known flows).

## Sizing (--ring-size)

Ring buffer size in bytes, configurable at Wotan startup. Default: 16 KB per flow.

Recommended sizing:

| Use Case | Size | Justification |
|----------|------|---------------|
| Tracing/metadata | 4-8 KB | Light state, mostly Monad replication |
| Service state | 16-32 KB | Stack + heap for small algorithms |
| Buffering | 64-256 KB | Input/output buffering |
| Large state | 512 KB-2 MB | Complex state machines (rare) |

Total Wotan memory per hop: (number of active flows) × (--ring-size + L1 cache overhead).

## Overflow Policy

Ring buffer overflow handling is critical for data durability. Implementations
MUST follow this policy:

### Proactive Drain (Primary)

When ring buffer occupancy exceeds 75%, userspace MUST initiate drain to L3
WAL. This is not a recommendation; it is a requirement to prevent packet loss.

Userspace monitor MUST:
1. Continuously poll Wotan L2 ring buffer occupancy
2. When occupancy > 75%: Initiate drain to L3
3. Drain speed: At least 10,000 events/sec (1 MB/sec)
4. Repeat drain until occupancy < 50%

### Overflow Response

If ring buffer becomes completely full before drain completes:

- Helper returns -ENOMEM (overflow)
- Shim receives error code
- Shim MUST implement stall/retry via BPF_TAIL_CALL
- Shim MUST NOT proceed without memory access

Event MUST NOT be dropped silently. Transparency is required.

### Capacity Planning

If userspace drain can't keep up with packet rate:
1. Admin must increase L2 ring buffer size, OR
2. Admin must reduce packet rate, OR
3. Admin must increase WAL storage speed

This guarantee ensures Wotan memory is never lost.

# L3 Write-Ahead Log: Persistent Storage

## WAL Format (PATCH W1, W4)

Per-flow persistent log on local storage (SQLite, RocksDB, or simple file):

~~~~~
struct wal_entry {
    u64 seqno;                    // PATCH W1: monotonically increasing
    u64 timestamp_ns;
    u32 flow_label;
    u32 addr;
    u8  data[64];                 // cache line data
    u32 crc32;                    // CRC-32 (accidental corruption detection)
    u8  hmac[32];                 // PATCH W4: HMAC-SHA256 (tampering detection)
    u8  valid;                    // 1 if entry is valid
    u8  _pad[3];
};
// Total: 136 bytes per entry
~~~~~

One WAL file per flow, or single WAL with flow_label demultiplexing (implementation choice).

**HMAC Authentication (PATCH W4)**:

In addition to CRC-32 (accidental corruption detection), WAL entries SHOULD be
authenticated with HMAC-SHA256 to detect intentional tampering:

```
hmac = HMAC_SHA256(
  key = session_key,
  message = seqno || timestamp_ns || flow_label || addr || data || crc32
)

session_key = PRK-Expand(session_secret, label="unheaded-wal", length=32)
```

Session key is derived during TLS handshake per [RFC 9001].

**Verification Procedure**:

On reading WAL entry:

```
1. Verify CRC-32 (corruption check)
2. If HMAC field present:
   - Recompute HMAC using session_key
   - Compare computed_hmac == stored_hmac
   - If mismatch: reject entry, log security alert, emit ANOMALY event
3. If HMAC field absent:
   - Entry is unauthenticated (from older implementation)
   - Accept entry (backward compatible)
```

HMAC validation is RECOMMENDED; implementations SHOULD implement for defense-in-depth
(CRC detects accidental bit flips; HMAC detects intentional tampering). Operators
MUST enable HMAC-SHA256 for production deployments handling sensitive per-flow state.

## Flush Policy (TTL-based)

Userspace daemon periodically:

1. Drain dirty L1 cache lines → L2 ring buffer (§6.4)
2. Drain full L2 ring buffer → L3 WAL (§7.4)
3. Flush L3 WAL to disk (fsync)

Flush frequency: RECOMMENDED every 100-1000 ms or on --wal-size-threshold bytes, whichever comes first. This trades write latency (disk fsync) against data durability (loss window on crash).

## Recovery on Restart

On Wotan startup or hop restart:

1. Scan L3 WAL directory for all flow_label files
2. For each flow:
   a. Rehydrate L2 ring buffer from WAL entries (in order)
   b. Verify CRC-16 on each entry (skip corrupted entries)
   c. Emit RECOVERY event to Anamnesis
3. L1 caches remain empty (lazy population on next access)

Recovery preserves per-flow state across restarts. Data durability: RPO (Recovery Point Objective) = flush interval (100-1000 ms).

## Compaction (PATCH W5)

Periodic WAL compaction (background task) is MANDATORY for operational
efficiency. Compaction MUST be protected by exclusive lock to prevent race
conditions:

1. Acquire exclusive compaction_lock (mutex, per-WAL instance)
2. Read entire WAL file
3. Group entries by address (collapse multiple writes to same address)
4. Keep only latest entry per address
5. Verify seqno monotonicity (PATCH W1) during merge
6. Write compacted WAL
7. Swap atomically (rename)
8. Release compaction_lock

Compaction reduces WAL file size and improves recovery speed.

**Compaction Lock Semantics (PATCH W5)**:

```
compaction_lock = Mutex()  // per-WAL instance

void compact_wal():
  if not compaction_lock.try_lock(timeout=10s):
    # Another compaction in progress; retry later
    log("Compaction already in progress, retrying in 5s")
    sleep(5)
    return

  try:
    validate_seqno_monotonicity()  // PATCH W1
    merge_segments_by_address()
    rewrite_index()
    fsync()  // Ensure durability
  finally:
    compaction_lock.release()
    wake_all_waiters()  // Broadcast new compacted state
```

While compaction holds the exclusive lock, concurrent writes from Shims:
- Acquire separate write_lock (different from compaction_lock)
- Append to current segment (not the segment being compacted)
- Release write_lock
- Proceed independently of compaction

This enables reads and writes to continue during compaction (different segments).

### Compaction Policy (MANDATORY)

Wotan daemon MUST compact WAL files when:
- File size exceeds 100 MB (--wal-max-size), OR
- File age exceeds 24 hours

Compaction is MANDATORY, not optional. Operator SHOULD monitor compaction frequency
via metrics:
- wotan_wal_compaction_events_total (counter)
- wotan_wal_file_size_bytes (gauge)
- wotan_wal_compaction_duration_seconds (histogram)
- wotan_wal_compaction_lock_contention_seconds (histogram)

Failing to compact WAL files can lead to:
- Unbounded disk growth (WAL becomes full)
- Slow recovery on restart (processing many duplicate entries)
- L3 flush performance degradation
- Increased risk of data loss on crash (larger RPO window)

# Wotan Protocol Control Frames

## SETTINGS Exchange (PATCH W7)

Before establishing a flow, sender and receiver exchange Wotan configuration parameters
via SETTINGS frame. This prevents cross-document divergence (X2 attacks) by ensuring
both sides agree on cache sizes, WAL segment sizes, and other critical parameters.

**SETTINGS Frame Format**:

```
struct WotanSettings {
  magic: u32 = 0xDEADBEEF,
  num_settings: u8,
  reserved: [u8; 3],
  settings: [Setting; num_settings],
}

struct Setting {
  id: u16,     // parameter ID (0x00-0x07)
  reserved: u16,
  value: u32,  // parameter value
}
```

**Defined Settings**:

| ID | Parameter | Default | Min | Max | Non-Negotiable |
|----|-----------|---------|-----|-----|-----------------|
| 0x00 | L1_CACHE_SIZE (bytes) | 16MB | 1MB | 256MB | No |
| 0x01 | L1_CACHE_LINE_SIZE (bytes) | 64 | 64 | 64 | YES |
| 0x02 | WAL_SEGMENT_SIZE (bytes) | 4MB | 1MB | 64MB | No |
| 0x03 | WAL_RETENTION_TIME (seconds) | 3600 | 60 | 86400 | No |
| 0x04 | MAX_FLOW_ENTRIES | 1M | 10K | 10M | No |
| 0x05 | CAS_ALIGNMENT (bytes) | 8 | 8 | 8 | YES |
| 0x06 | SEQNO_ENABLE (boolean) | 1 | 1 | 1 | YES |
| 0x07 | HMAC_ENABLE (boolean) | 1 | 0 | 1 | No |

**SETTINGS Exchange Procedure**:

On connection establishment:
```
1. Sender transmits SETTINGS frame (TLV type 0x40, critical) with local parameters
2. Receiver reads SETTINGS frame
3. Receiver extracts parameters: cache_size, wal_segment_size, etc.
4. Receiver validates each parameter against bounds
5. If any parameter out of range: send error SETTINGS frame (magic=0xDEADC0DE)
6. If all parameters valid: reply with ACK (empty SETTINGS frame, magic=0xDEADBEEF)
7. Both sides proceed with flow using negotiated parameters
```

**Parameter Validation**:

```
setting_bounds = {
  L1_CACHE_SIZE: (1_MB, 256_MB),
  WAL_SEGMENT_SIZE: (1_MB, 64_MB),
  WAL_RETENTION_TIME: (60, 86400),
  MAX_FLOW_ENTRIES: (10_000, 10_000_000),
}

for setting in received_settings:
  if setting.value < bounds[setting.id].min ||
     setting.value > bounds[setting.id].max:
    send_error_response()  // magic=0xDEADC0DE
    close_connection()
    log("SETTINGS negotiation failed: parameter", setting.id, "out of range")
```

Parameters marked "Non-Negotiable" (YES) MUST match exactly. If receiver detects
mismatch in non-negotiable parameters, it MUST reject the connection.

## GOAWAY Frame (PATCH W8)

GOAWAY frame signals graceful termination of flow. Receiver processes in-flight
operations before connection closes, ensuring data durability and consistency.

**GOAWAY Frame Format**:

```
struct GoAwayFrame {
  magic: u32 = 0xC0FFEE00,
  reason: u16,           // termination reason (0x00-0x07)
  reserved: u16,
  last_flow_id: u32,     // last flow ID processed by sender
  debug_data_len: u16,   // variable-length debug info (optional)
  debug_data: [u8; debug_data_len],
}
```

**Reason Codes**:

| Code | Meaning | Action |
|------|---------|--------|
| 0x00 | No error (normal termination) | Graceful close |
| 0x01 | Protocol error (invalid packet) | Log error, close |
| 0x02 | Flow ID exhaustion (too many flows) | Log error, close |
| 0x03 | WAL compaction in progress (pause new writes) | Wait 5s, retry |
| 0x04 | Server shutdown (graceful restart) | Finish operations, close |
| 0x05 | Resource limits (memory/CPU exhausted) | Log error, close |
| 0x06 | Timeout (no activity for 30s) | Log timeout, close |
| 0x07 | Explicit client request | Graceful close |

**Sending GOAWAY**:

Before closing connection:

```
1. Send GOAWAY frame (TLV type 0x41, critical) with reason code
2. Set last_flow_id to highest flow ID processed by sender
3. Wait up to 5 seconds for receiver to finish in-flight operations
4. Close TCP connection (send FIN on both sides)
5. Log event: "GOAWAY sent, reason=X, last_flow_id=Y, waited_N_seconds"
```

**Receiving GOAWAY**:

On receiving GOAWAY frame:

```
1. Extract reason_code and last_flow_id from frame
2. Log event: "GOAWAY received, reason=X, last_flow_id=Y"
3. Stop initiating new flows (reject new flow requests)
4. Allow in-flight operations (flows 0 to last_flow_id) to complete
5. Flush WAL to disk (fsync all pending writes) [PATCH W1]
6. Close TCP connection (send FIN)
7. Reconnect if needed (new connection, new flow IDs)
```

**Graceful Shutdown Guarantee**:

GOAWAY frame ensures:
- In-flight packets (flows 0 to last_flow_id) are processed before close
- WAL is flushed to disk (durable, survives crash)
- Flow state is consistent (no lost updates)
- Replay-safe (all seqnos are durable)

Example shutdown sequence:

```
Sender: Send GOAWAY(reason=0x00, last_flow_id=123)
Receiver: Process flows 0-123 to completion
Receiver: Flush WAL to disk
Receiver: Close TCP connection (send FIN)
Sender: Receive TCP FIN, confirm close
Both sides: In consistent state, no data loss
```

**Timeout Handling**:

If receiver doesn't close within 5 seconds:

```
Sender: Forcibly close TCP connection (timeout)
Sender: Log event: "GOAWAY timeout, forcing close after 5s"
Any in-flight writes: Rolled back (not durable until next WAL flush)
Recovery: Restart connection with new flow IDs
```

5-second timeout balances graceful shutdown vs. hung connection detection.

# Topic-Based I/O

## Topic Naming Convention

Wotan topics follow a hierarchical naming scheme:

~~~~~
Topic Format:    {service}.{component}.{flow_label}
Domain:          "compute" | "sophia" | "anamnesis"
Component:       "screen" | "input" | "mem" | "dictionary"
Flow Label:      {0-20-bit-label} or {event-type}

Examples:
  compute.screen.0x3f1a    → screen output for flow 0x3f1a
  compute.input.0x3f1a     → input events for flow 0x3f1a
  compute.mem.miss         → L1 cache miss events (all flows)
  compute.mem.write        → L1 dirty write-back events (all flows)
  sophia.dictionary.v47     → Sophia dictionary version 47
  anamnesis.birth          → Packet birth events (Shield ingress)
  anamnesis.death          → Packet death events (Shield egress)
  anamnesis.hop            → Per-hop computation events
  anamnesis.chaos          → Yaldabaoth chaos events
  anamnesis.anomaly        → Integrity/fingerprint failures
~~~~~

Topic implementation: BPF ring buffers (kernel) + pub/sub delivery (Wotan daemon).

## Memory-Mapped I/O Addresses

Writing to address ranges triggers topic publication:

~~~~~
Address      Topic Published              Semantics
------       ------------------          ----------
0x0000C000   compute.screen.{label}      Screen/framebuffer output
0x0000C001   (reserved)
...
0x0000FFFF   compute.input.{label}       Input event consumption (read-only)
~~~~~

## Screen Topic (compute.screen.{flow_label})

Writes to address 0x0000C000 publish to compute.screen topic:

~~~~~
struct screen_event {
    u32 flow_label;
    u32 len;               // payload length (1-4096 bytes)
    u8  data[4096];        // framebuffer data
};
~~~~~

Userspace subscribers (dashboard, user interface) consume and display data.

### MMIO Write-Fail Behavior

Writes to MMIO addresses (0x0000C000-0x0000DFFF) are non-blocking and
unconditionally successful from the Shim perspective:

1. Shim issues bpf_wotan_write() to MMIO address
2. Helper enqueues event to ring buffer immediately
3. Helper returns success (len) to Shim
4. If no subscriber is listening: Event is dropped silently with counter increment
5. If ring buffer is full: Event may be dropped; no error returned to Shim

**Semantics**: MMIO writes are "fire-and-forget" publish operations. The Shim
does not receive confirmation that the event was successfully consumed by a
subscriber.

## Input Topic (compute.input.{flow_label})

Reads from address 0x0000FFFF fetch from compute.input topic:

~~~~~
struct input_event {
    u32 event_type;        // keyboard, mouse, network, etc.
    u8  payload[60];       // input data (depends on type)
};
~~~~~

Userspace input providers (keyboard, mouse, network) publish to compute.input. Shim programs consume via bpf_wotan_read(flow_label, 0x0000FFFF, ...).

## Dictionary Topic (sophia.dictionary.v{N})

Sophia dictionary updates distributed via topic:

~~~~~
Topic: sophia.dictionary.v{version}

Payload: serialized Sophia dictionary entry
  {
    "dict_id": 5,
    "entry_key": 0x03,
    "entry_value": "realtime",
    "version": 47
  }
~~~~~

Wotan daemon subscribes, writes updates to BPF maps on all hops.

## Anamnesis Topics (anamnesis.{event_type})

Per [UNHEADED-FOUNDATION], Anamnesis ring buffers emit events:

~~~~~
Topic: anamnesis.birth
Topic: anamnesis.death
Topic: anamnesis.hop
Topic: anamnesis.chaos
Topic: anamnesis.anomaly
~~~~~

Wotan subscribes to all anamnesis.* topics and forwards to userspace observability systems (Prometheus, Grafana, ELK).

# Cache Miss Protocol

## Miss Event Structure

When L1 cache miss occurs:

~~~~~
struct wotan_cache_miss_event {
    u32 flow_label;    // which flow
    u32 addr;          // miss address (cache-line-aligned)
    u32 len;           // requested length
    u8  access_type;   // 0=read, 1=write
    u8  pad[3];
};
// Emitted to: compute.mem.miss ring buffer
~~~~~

## Userspace Handler

Userspace Wotan daemon:

1. Poll compute.mem.miss ring buffer (epoll)
2. On miss event:
   a. Extract flow_label, addr, access_type
   b. Look up L2 ring buffer for this flow
   c. Search for cache line at (addr & ~0x3F)
   d. If found: copy to L1 BPF map (hit promotion)
   e. If not found: return empty cache line (zero-fill for reads, write for writes)
   f. Return success signal to Shim

Handler MUST be non-blocking; misses are served in <10 µs on average (in-memory operations).

## Stall Mechanism

When Shim receives -ENOMEM (cache miss):

Option A (RECOMMENDED): Retry via BPF_TAIL_CALL
~~~~~
if (bpf_wotan_read(...) == -ENOMEM) {
    bpf_tail_call(ctx, &jmp_table, INDEX_RETRY_READ);
}
~~~~~

Option B: Software retry loop (max iterations to prevent livelock)
~~~~~
for (int i = 0; i < MAX_RETRIES; i++) {
    ret = bpf_wotan_read(...);
    if (ret > 0) break;
    if (ret != -ENOMEM) {
        // fatal error
        break;
    }
    // backoff or spin
}
~~~~~

Option C: Proceed without memory access (degrade functionality)
~~~~~
if (bpf_wotan_read(...) == -ENOMEM) {
    // skip complex logic, use default Monad values
    return XDP_PASS;
}
~~~~~

Choice of stall mechanism is operator policy, configured per Shim program.

## Prefetch Hinting

Shim program MAY prefetch likely-to-be-needed addresses:

~~~~~
// Prefetch stack frame
bpf_wotan_read(flow_label, sp - 64, &dummy, 1);
bpf_wotan_read(flow_label, sp - 128, &dummy, 1);
~~~~~

Userspace handler serves prefetch requests before Shim retry, reducing average miss latency from ~10 µs (on-demand) to ~5 µs (prefetch).

## Cache-Miss Rate Limiting (PATCH W6)

Per-program cache-miss rate limiting prevents denial-of-service attacks where a
malicious or buggy eBPF program causes excessive cache misses, exhausting bandwidth
to persistent storage and starving other programs.

**Rate Limit Thresholds**:

Each eBPF program (identified by program_id) has a per-second cache-miss budget:

```
Limit: 10,000 misses per second (10K cache misses / second)
Measurement window: 1 second (rolling window)
Action on excess: throttle reads, delay program execution by 100 ms
```

**Cache-Miss Counting**:

On each L1 cache read:

```
if key in L1_cache:
  hit_count[program_id]++
  cache_hit = true
else:
  miss_count[program_id]++
  cache_miss = true

  # Check rate limit
  if miss_count[program_id] > MISS_BUDGET_PER_SECOND:
    apply_throttle(program_id, delay=100ms)
    log("Program", program_id, "exceeded cache-miss budget, throttling")
    emit ANOMALY event to Anamnesis
```

**Throttle Mechanism**:

When miss rate exceeds budget:

```
1. Delay program execution: sleep(100 ms)
2. Recount misses in new window
3. If still over budget, increase delay to 200 ms
4. Exponential backoff: 100 ms → 200 ms → 400 ms → 800 ms → 1.6 s
5. Max delay: 5 seconds
6. After max delay exceeded for >10 seconds, forcibly terminate program
```

**Metrics and Monitoring**:

Wotan daemon SHOULD export metrics:
- wotan_cache_hits_per_sec[program_id]
- wotan_cache_misses_per_sec[program_id]
- wotan_cache_miss_rate[program_id] = misses / (hits + misses)
- wotan_cache_miss_throttle_events[program_id] (counter)
- wotan_cache_miss_max_delay_exceeded[program_id] (counter)

Alert on:
- Cache miss rate > 50% per program (poor locality, may indicate bug)
- Sustained throttling > 10 seconds (DoS attack likely)
- Cache miss budget exceeded for multiple programs simultaneously (resource exhaustion)

**Rationale**:

Cache-miss rate limiting prevents:
- Bandwidth exhaustion (repeated cache misses saturate I/O bus)
- Denial of service (one program starves others via cache traffic)
- CPU stalls (cache miss latency accumulates)

Limits are conservative:
- 10,000 misses/sec: typical well-written program has <100 misses/sec
- 100 ms throttle: penalizes pathological programs without affecting normal ones
- Programs with >10% cache-miss rate are automatically logged for investigation

# Computational Memory Model

## Program Memory (ROM via Sophia BPF Array)

Read-only program code and constant data are stored in Sophia dictionaries or in extended memory (0x00010000+). These are shared across flows and hops (L4 cache).

## Data Memory (RAM via Wotan Ring Buffer)

Per-flow writeable state (stack, heap, scratch) is in L2 ring buffer (0x00000000-0x0000BFFF).

## Stack (top-of-RAM, grows downward)

Shim programs allocate stack frames in per-flow RAM. Convention:

~~~~~
sp = ring_buffer_size - 1      // start at top
// Push frame
sp -= frame_size
bpf_wotan_write(flow_label, sp, &frame_data, frame_size)

// Pop frame
bpf_wotan_read(flow_label, sp, &frame_data, frame_size)
sp += frame_size
~~~~~

Stack overflow detection: if sp < heap_top, return error. Heap overflow detection: if heap_top > stack_base, return error.

## Heap (configurable region)

Configurable via Sophia: heap_base and heap_top pointers per flow. Shim programs use simple malloc-like allocation:

~~~~~
// Allocate N bytes
addr = heap_top
heap_top += N
if (heap_top > stack_sp) return -ENOMEM  // collision

// Free (optional, depends on allocator policy)
// Many Shims use linear allocation (no free) for simplicity
~~~~~

## Memory-Mapped I/O (designated address ranges)

Addresses 0x0000C000-0x0000FFFF trigger topic I/O:
- Writes publish data
- Reads consume events
- Non-blocking (returns immediately)

# Ring Buffer Formal Specification

This section provides precise formal specification of ring buffer behavior, required
for correct implementation and interoperability.

## Ring Buffer Capacity and Layout

**Default Capacity**: 10,000 entries per flow (configurable via --ring-buffer-capacity)

Capacity requirements:
```
min_capacity = 1,000 entries (minimum for correctness)
default_capacity = 10,000 entries (recommended baseline)
max_capacity = 1,000,000 entries (max before degradation)
```

**Fixed Entry Size**: 88 bytes per entry (including PATCH W1 seqno field):

```
Total per entry:
  seqno (u64):        8 bytes
  timestamp_ns (u64): 8 bytes
  flow_label (u32):   4 bytes
  addr (u32):         4 bytes
  data (u8[64]):      64 bytes
  crc32 (u32):        4 bytes
  valid (u8):         1 byte
  _pad (u8[3]):       3 bytes
  ──────────────────────────
  TOTAL:              88 bytes per entry
```

## Head/Tail Pointer Semantics

Ring buffer is implemented as circular FIFO with head and tail pointers:

```
struct RingBufferState {
  head: u32,           // Next write position (0 to capacity-1)
  tail: u32,           // Next read position (0 to capacity-1)
  entries[capacity]: wotan_rb_entry,
  seqno: u64,          // Global entry counter (monotonic, wraps at 2^64)
  overflow_count: u64, // Number of entries dropped on overflow
}

Invariants:
  - tail <= head (monotonic, mod capacity)
  - (head - tail) mod capacity = number of valid entries
  - seqno never decreases (monotonic)
  - seqno always equals entry[head-1].seqno (last written entry)
```

**Write Operation (by Shim or userspace)**:

```
Write(payload):
  entry = wotan_rb_entry{
    seqno: ++ring.seqno,
    timestamp_ns: bpf_ktime_get_ns(),
    data: payload,
    valid: 1,
  }

  if (head + 1) mod capacity == tail:
    # Ring buffer full (overflow)
    overflow_count++
    return -ENOMEM
  else:
    entries[head] = entry
    head = (head + 1) mod capacity
    return success
```

**Read Operation (by userspace handler)**:

```
Read():
  if head == tail:
    # Ring buffer empty
    return nil

  entry = entries[tail]
  if entry.valid == 0:
    # Stale entry (previous read)
    tail = (tail + 1) mod capacity
    return nil  (retry)

  # Mark as stale (for recovery detection)
  entry.valid = 0
  tail = (tail + 1) mod capacity
  return entry
```

## Oldest-First Eviction (Overflow Policy)

When ring buffer is full and new entry arrives:

```
if is_full():
  # Evict oldest entry (at tail)
  overflow_count++
  entries[tail].valid = 0  (mark stale)
  tail = (tail + 1) mod capacity
  # Now head == tail (one empty slot)
  Write(payload)  (insert new entry)
```

**Guarantee**: Oldest entries are lost first; newest entries are preserved.

This is a **lossy** policy (acceptable for observability; not for persistent storage).
For durable storage, use L3 WAL (Role 3 flushes to disk with exclusive lock).

## Lock-Free Read Path (BPF ringbuf Compatible)

L1 cache miss handler reads from L2 ring buffer without acquiring locks:

```go
func (h *MissHandler) ReadAsync(flow_label, addr) (*wotan_rb_entry, error) {
  // Lockless read from ring buffer
  for i := 0; i < 10; i++ {
    entry := ring.Read()  // May fail if full or empty
    if entry != nil && entry.addr == addr {
      return entry, nil
    }
  }
  return nil, ErrNotFound
}
```

**Lock-Free Property**: Reads do NOT acquire mutex or spinlock. BPF ringbuf
implementation (kernel) provides atomicity via memory barriers and CAS operations
on head/tail pointers.

**Correctness**: Lock-free reads are safe because:
1. Writes are atomic (CAS on head pointer)
2. Reads only inspect tail and entries (no modifications)
3. Head can only advance (never go backward)
4. Entry validity flag (valid=1/0) provides publish-subscribe semantics

## Memory Layout Alignment Requirements

Ring buffer must be allocated with proper alignment for optimal cache performance:

```
Ring buffer allocation:
  size_bytes = capacity * 88
  alignment = 4096 bytes (one page)

  // Linux kernel mmap alignment
  mmap(addr=0, length=size_bytes, prot=PROT_READ|PROT_WRITE,
       flags=MAP_PRIVATE|MAP_ANONYMOUS, ...)

  // Entry alignment within ring
  entry_address = ring_base + (index * 88)
  // Entry spans 1.375 cache lines (88 / 64 = 1.375)
  // Each entry touches 2 cache lines (64 + 24 bytes of next)
```

**Cache Alignment Strategy**:

For optimal cache utilization, align entries to 64-byte (CPU cache line) boundaries:

```
Aligned entry layout:
  offset 0-63:     seqno (8) + timestamp (8) + flow_label (4) + addr (4) + data[39]
  offset 64-87:    data[25] + crc32 (4) + valid (1) + pad (3)
  offset 88-127:   (next entry starts)
```

This results in entries spanning 1.375 cache lines, with some internal fragmentation.
For high-performance deployments, consider 64-byte aligned entries (reduce data to 60 bytes).

**Kernel Configuration**:

Ring buffer backed by BPF_MAP_TYPE_RINGBUF (kernel-level):

```
sysctl kernel.bpf.ringbuf_alignment = 64  (or 4096 for page alignment)
sysctl kernel.bpf.ringbuf_max_events = 1000000
```

Userspace ring buffer: allocate via mmap(MAP_ANONYMOUS) with page alignment.

# gRPC Streaming Contract

Wotan implements a topic subscription protocol via gRPC streaming for userspace
interaction. Clients subscribe to topics and receive events in real-time.

## Topic Subscription Protocol

Topic namespace:
```
system.*     - System-level events (start, stop, reconnect)
logs.*       - Per-program log output
anamnesis.*  - Observability events (birth, death, hop, chaos, anomaly)
user.*       - User-defined topic subscriptions
```

**Topic Format**: `{domain}.{component}.{flow_label}`

Example subscriptions:
```
system.start       - Server initialization
logs.program_0     - Output from BPF program 0
anamnesis.anomaly  - Integrity/fingerprint violations
user.custom_event  - Application-specific topics
```

## Stream Lifecycle

```
1. CONNECT
   Client initiates gRPC stream to Wotan.Subscribe()

2. SUBSCRIBE
   Client sends SubscribeRequest:
   {
     topic: "anamnesis.anomaly",
     filter: { severity: "ERROR" },
     start_at: "latest" | "oldest" | <timestamp>
   }

3. RECEIVE
   Server streams SubscriptionEvent:
   {
     topic: "anamnesis.anomaly",
     timestamp: <timestamp>,
     payload: <event_data>,
     seqno: <sequence_number>
   }

4. UNSUBSCRIBE
   Client sends UnsubscribeRequest (optional)

5. DISCONNECT
   Client closes stream, server cleans up subscription
```

## Backpressure Handling

Per-client buffer limits prevent memory exhaustion:

```
max_buffered_events_per_client = 10,000
max_buffered_bytes_per_client = 100 MB

If client receives < 1 event/100ms:
  Server pauses sending (backpressure)
  Log: "Client {id} backpressured, pausing events"

If client buffer exceeds limit:
  Server drops oldest events (FIFO)
  Emit counter: wotan_subscriber_buffer_overflow
  Client MUST re-subscribe or lose events
```

## Reconnection Policy

Client-side exponential backoff for reconnection:

```
Initial delay: 100 ms
Max delay: 30 seconds
Backoff multiplier: 1.5x per attempt

Reconnection sequence:
1. Connection lost
2. Wait 100 ms, retry
3. If fails: wait 150 ms, retry
4. If fails: wait 225 ms, retry
5. ... continue until max delay (30s)
6. Then retry every 30 seconds
```

Server-side TTL-based subscription reaping:

```
subscription_idle_timeout = 5 minutes

For each subscription:
  If no heartbeat received in 5 minutes:
    Server assumes client dead (network partition)
    Clean up subscription resources
    Log: "Subscription {id} reaped after 5m inactivity"
```

Heartbeat protocol: client sends empty SubscribeRequest every 1 minute to keep
subscription alive.

## Cross-Reference to Proto

Wotan streaming contracts are defined in:
- File: proto/unheaded/v1/protocol.proto
- Service: Wotan.Subscribe (gRPC streaming)
- Messages: SubscribeRequest, UnsubscribeRequest, SubscriptionEvent

Implementation MUST follow proto definitions exactly.

# Triple-Role Architecture

Wotan operates as three coordinated roles within the same process. Each role
runs in separate goroutines but shares the same per-flow BPF maps and ring buffers:

## Role 1: Ring Buffer (Per-Service Log Aggregation)

**Purpose**: Collect per-service state changes into ring buffers

**Responsibilities**:
- Allocate per-flow ring buffer (L2 memory)
- Append entries from BPF helpers (bpf_wotan_{read,write,cas})
- Implement overflow policy (proactive drain to L3)
- Export compute.mem.miss events to userspace miss handler
- Replicate state to Anamnesis for observability

**Data**: Per-flow ring buffers (struct wotan_rb_entry with seqno, timestamp, data)

**Exports**:
- compute.mem.miss (cache miss events to userspace)
- compute.mem.write (dirty cache line write-back events)
- wotan_ringbuf_occupancy_percent (metric)

## Role 2: Event Bus (Pub/Sub Message Routing)

**Purpose**: Route events between BPF programs and userspace subscribers

**Responsibilities**:
- Maintain topic subscriptions (per-subscriber)
- Route published events to matching subscriptions
- Implement backpressure (per-client buffer limits)
- Handle reconnection (exponential backoff)
- Reap idle subscriptions (5 minute TTL)

**Data**: Topic -> [Subscribers] map; per-subscriber event queue

**Implements**:
- gRPC streaming service (Wotan.Subscribe)
- Topic filtering and routing
- Backpressure and buffer management

**Exports**:
- wotan_subscribers_active (gauge)
- wotan_events_routed_total (counter)
- wotan_subscriber_buffer_overflow_total (counter)

## Role 3: Protocol RAM (Monad State Machine Backing Store)

**Purpose**: Persist per-flow Monad state to WAL for crash recovery

**Responsibilities**:
- Drain dirty L1 cache lines → L2 ring buffer → L3 WAL
- Implement WAL flush policy (TTL-based, 100-1000 ms)
- Compact WAL files (exclusive lock, PATCH W5)
- Recover state on restart (replay WAL entries)
- Validate seqno monotonicity (PATCH W1)
- Authenticate WAL entries (HMAC-SHA256, PATCH W4)

**Data**: WAL files on disk (struct wal_entry with seqno, crc32, hmac)

**Metrics**:
- wotan_wal_entries_written_total (counter)
- wotan_wal_bytes_written_total (counter)
- wotan_wal_compaction_events_total (counter)
- wotan_wal_flush_latency_seconds (histogram)

## Role Isolation Guarantees

All three roles operate in the same process but with strict separation:

```
Process Layout:
┌─────────────────────────────────┐
│ Wotan Daemon (one process)      │
├─────────────────────────────────┤
│ Role 1: Ring Buffer             │ (goroutine 1)
│ - Manage ring buffers           │
│ - Handle cache misses           │
├─────────────────────────────────┤
│ Role 2: Event Bus               │ (goroutine 2)
│ - Subscribe/publish             │
│ - gRPC streaming                │
├─────────────────────────────────┤
│ Role 3: Protocol RAM            │ (goroutine 3)
│ - WAL flush and compaction      │
│ - Recovery                      │
├─────────────────────────────────┤
│ Shared State:                   │
│ - Per-flow ring buffers (BPF)   │
│ - L1 cache maps (BPF)           │
│ - Flow metadata (Go map)        │
└─────────────────────────────────┘
```

Synchronization:
- Ring buffer writes: lock-free (BPF ringbuf is atomic)
- Event bus: per-topic RWMutex (readers = subscribers, writers = publishers)
- WAL access: compaction_lock (exclusive, PATCH W5)
- Flow metadata: per-flow RWMutex

## Failure Mode: Ring Buffer Overflow

**Scenario**: L2 ring buffer becomes full before drain to L3 WAL

**Behavior (MANDATORY)**:
- Ring buffer overflow returns -ENOMEM to Shim
- Ring buffer does NOT block event bus or WAL roles
- Role 1 (Ring Buffer) backpressures Shims via -ENOMEM
- Role 2 (Event Bus) continues routing events
- Role 3 (Protocol RAM) continues WAL flush and compaction

**Isolation Property**: Ring buffer overflow is isolated to one flow; other
flows continue operating normally.

**Recovery**:
1. Monitoring alerts on ring buffer occupancy > 75%
2. Role 3 (Protocol RAM) prioritizes drain to L3 (increase fsync frequency)
3. Role 1 (Ring Buffer) continues emitting -ENOMEM to Shims (stall packets)
4. Shim programs either retry via BPF_TAIL_CALL or degrade functionality
5. Once drained below 50%, resume normal operation

**Guarantee**: Ring buffer overflow does NOT cause data loss in WAL (crash-safe).
State is durable once flushed to L3.

# Wotan Reliability Guarantees

Wotan provides application-level reliability guarantees for event delivery and
state durability.

## PublishWithAck: At-Least-Once Delivery

Events published to topics are delivered at least once:

```go
func (s *WotanServer) PublishWithAck(ctx context.Context,
  req *PublishRequest) (*PublishAckResponse, error) {

  // 1. Write event to ring buffer (or topic queue)
  seqno := s.ringbuf.Append(req.Payload)

  // 2. Wait for WAL flush (durable)
  <-s.wal.FlushDone(seqno)

  // 3. Return ACK with seqno (idempotency key)
  return &PublishAckResponse{Seqno: seqno}, nil
}
```

Semantics:
- Application calls PublishWithAck()
- Event is buffered in L2 ring buffer
- Event is flushed to L3 WAL (disk)
- PublishWithAck returns, guaranteeing durability
- Subscribers receive event (may retry on network loss)

**Idempotency**: seqno enables deduplication. If client retransmits same event,
server recognizes seqno and returns cached response without duplicating.

## IdempotencyCache: 24-Hour TTL

Deduplication cache prevents duplicate event processing:

```
struct IdempotencyEntry {
  seqno: u64,
  timestamp: timestamp,
  response: bytes,
  ttl: 24h,
}

On PublishWithAck(event):
  1. Compute idempotency_key = BLAKE3(event.Payload)
  2. Check cache: if entry exists with same key:
     - Return cached response (same seqno)
  3. If not in cache: append to ring buffer (new seqno)
     - Insert cache entry (ttl=24h)
     - Return seqno

Every 1 hour: evict cache entries with age > 24h
```

**TTL Rationale**: 24 hours allows clients to retry during network outages.
After 24 hours, server assumes client has recovered or given up.

## OrderedPublisher: Per-Destination FIFO

Events to the same destination are delivered in order:

```
topic_to_queue = Map[topic_name] -> FIFO_Queue

PublishOrdered(topic, event):
  queue = topic_to_queue.get_or_create(topic)
  queue.enqueue(event)
  # Subscribers consume from queue in order (FIFO)
```

Each topic has its own FIFO queue. Multiple subscribers reading from same topic
see events in same order.

**Per-Destination**: Each (destination_ip, destination_port) pair has its own
queue to prevent head-of-line blocking.

## Dead Letter Queue (DLQ)

Events that fail delivery after max retries are queued for later processing:

```
MAX_DELIVERY_ATTEMPTS = 3

PublishEvent(event):
  for attempt in 1..MAX_DELIVERY_ATTEMPTS:
    try:
      deliver_to_subscribers(event)
      return success
    except DeliveryError as e:
      if attempt < MAX_DELIVERY_ATTEMPTS:
        wait(exponential_backoff(attempt))
        continue
      else:
        # Max retries exhausted
        dlq.enqueue(event)
        log("Event", event.seqno, "moved to DLQ after", attempt, "retries")
        emit ANOMALY event to Anamnesis
        return error
```

**DLQ Processing**:
1. Operator monitors wotan_dlq_size_bytes (gauge)
2. Operator inspects DLQ events via admin API
3. Fix underlying issue (subscriber reconnect, topic removal, etc.)
4. Manually replay DLQ entries or discard

**Information Leakage**: DLQ may contain sensitive state. Operators MUST:
- Restrict DLQ access (authentication + authorization)
- Encrypt DLQ storage (filesystem encryption)
- Monitor DLQ retention (auto-delete after 7 days)
- Audit DLQ access (log all queries)

# Performance Characteristics

## L1 Hit Latency

Approximately 100-200 ns per bpf_wotan_read/write on L1 cache hit (BPF hash map lookup + memcpy of 1-4 bytes).

Measured on:
- Modern x86-64 (Intel Skylake or newer, AMD EPYC)
- BPF JIT compilation enabled
- L1 cache warm (typical case)

Includes: BPF verifier overhead (~0 ns, amortized), hash table lookup (O(1), ~100 ns), memory access (~50 ns), return (~10 ns).

## L2 Access Latency

Approximately 1-10 µs for cache miss + userspace handler + L1 refill.

Measured on: Intel Skylake or newer / AMD EPYC processors, ring buffer in
userspace, handler latency measured end-to-end.

### Cache Miss Latency Breakdown

The 1-10 µs range assumes the following component latencies:

| Component | Latency | Conditions |
|-----------|---------|-----------|
| Emit miss event to ring buffer | ~100 ns | Ring buffer has space, CPU cache warm |
| Userspace poll wake-up | ~1-2 µs | High-priority thread, pinned to core |
| L2 ring buffer lookup | ~1-2 µs | Entry in memory, NUMA local |
| L1 BPF map update | ~1 µs | Hash bucket not contested |
| Shim retry BPF_TAIL_CALL | ~100 ns | JIT compiled, kernel cache warm |
| **Total (ideal case)** | **~4-5 µs** | **All conditions met** |

In high-contention scenarios (multiple cores missing same flow):
- Userspace poll latency may increase to 10-100 µs
- L2 ring buffer contention may cause serialization
- L1 cache line eviction may require prefetch retry
- Actual latency observed: 10-100 µs (100x slower)

### Deployment Guidance

Deployments SHOULD:
1. Pin Wotan userspace handler to dedicated cores
2. Configure RT scheduling priority for miss handler
3. Monitor miss rate via /sys/kernel/debug/bpf/
4. Plan capacity for <1% miss rate under expected workload

Total: ~4-5 µs average (ideal case, 9x slower than L1 hit).

## L3 Access Latency

Approximately 100 µs to 1 ms for WAL disk I/O.

Depends on:
- Storage backend (SSD: ~100-500 µs; HDD: ~5-20 ms)
- I/O scheduler (deadline, kyber)
- Concurrent I/O load
- Fsync policy (batched vs. immediate)

L3 is only accessed on Wotan restart (recovery) or explicit flushing; not on critical path for per-packet operations.

## Cache Hit Rate Targets

For well-tuned deployments: >90% L1 cache hit rate.

Assumptions:
- L1 cache size: 64 KB (1024 lines × 64 bytes)
- Working set: <10 KB per flow (typical for stateless Shims + 10-20 byte working set)
- Temporal locality: same addresses accessed repeatedly within 10 ms window

If hit rate <80%: increase L1 cache size (BPF map max_entries) or reduce L2 ring buffer size to promote prefetching.

# Security Considerations

## Topic Injection Attacks (Unauthorized Publish)

A malicious application could publish events to topics that are not authorized,
potentially injecting false observability data or triggering unintended behavior
in subscribers.

**Threat**: An unprivileged application publishes to "anamnesis.anomaly" topic
(reserved for system), causing false alarms or masking real anomalies.

**Mitigation (MANDATORY)**:
- Topic access control via Sophia (per-program topic whitelist)
- Verify publisher identity (program ID from BPF context)
- Enforce least-privilege (default: deny all topics except explicit whitelist)
- Log all unauthorized publish attempts
- Emit ANOMALY event to Anamnesis

```go
func (s *WotanServer) PublishWithAck(ctx context.Context,
  req *PublishRequest) (*PublishAckResponse, error) {

  program_id := ctx.BPFProgramID()
  allowed_topics := s.sophia.GetAllowedTopics(program_id)

  if topic not in allowed_topics:
    log.Error("Program", program_id, "attempted unauthorized publish to", topic)
    s.anamnesis.EmitAnomaly(...)
    return nil, status.Errorf(codes.PermissionDenied, "topic not authorized")
}
```

## Ring Buffer Memory Exhaustion

A malicious Shim could intentionally cause high cache-miss rates to exhaust
ring buffer memory, causing denial of service to other flows.

**Threat (PATCH W6)**: One Shim program repeatedly accesses non-resident memory,
triggering 100K+ cache misses/sec. Ring buffer fills, overflows, causes data loss
and stalls other flows.

**Mitigation (PATCH W6, MANDATORY)**:
- Per-program cache-miss rate limiting (10K misses/sec budget)
- Throttle programs exceeding budget (100 ms delay, exponential backoff)
- Export metrics (wotan_cache_miss_rate_exceeded_total)
- Force-terminate programs exceeding 5s max delay
- Monitor ring buffer occupancy (alert at 75%, proactive drain at L3)

See Section 8.2 (Cache-Miss Rate Limiting) for complete specification.

## Cross-Flow Memory Access via Composite Key Collision (LICH-009)

**Threat (PATCH W2)**: Using only 20-bit flow_label as L1 cache key enables
birthday attack collisions. Attacker crafts packets with same flow_label but
different src/dst IP, causing cache line to be shared between two flows.

```
Flow A: label=0x12345, src=10.0.0.1, dst=10.0.0.2
Flow B: label=0x12345, src=10.0.0.3, dst=10.0.0.4
  (same label, different src/dst)

With 20-bit key: both flows hash to same cache line
Result: Flow A can read/write Flow B's memory (data breach)
```

**Impact**: Confidentiality breach (read), integrity violation (write)

**Mitigation (PATCH W2, MANDATORY)**:
- Extend L1 cache key to 64-bit composite: (flow_label << 44) | (tuple_hash & 0xFFF...FFF)
- Tuple hash = SipHash-2-4(src_ip || dst_ip || src_port || dst_port)
- Composite entropy = 64 bits (negligible collision probability)
- Require composite keys for draft-02; reject 20-bit-only keys for new flows

See Section 4.1 (L1 Cache Composite Key) for complete specification.

## CAS Alignment Violations (LICH-008)

**Threat (PATCH W3)**: Unaligned CAS operations may not be atomic on ARM64 or
RISC-V, enabling race conditions and lost updates in concurrent flows.

```
ARM64: LDAXR instruction requires 8-byte alignment
RISC-V: AMO.SWAPD instruction requires 8-byte alignment
x86-64: LOCK CMPXCHG tolerates misalignment (slow, but atomic)

Unaligned CAS on ARM64: Illegal Instruction Exception (hardware fault, crash)
Unaligned CAS on RISC-V: Access Fault (hardware exception, crash)
```

**Impact**: Denial of service (program crash), potential privilege escalation

**Mitigation (PATCH W3, MANDATORY)**:
- BPF verifier MUST inject alignment checks before CAS operations
- Reject programs with unaligned CAS at load time (fail-fast)
- Enforce 8-byte alignment (not 4-byte)
- Runtime check in helper: `if ((addr & 0x7) != 0) return -EFAULT`

See Section 3.2.1 (CAS Alignment Enforcement) for complete specification.

## gRPC Stream Hijacking

A network attacker could intercept or modify gRPC streams between Wotan
subscribers and the Wotan server, causing data corruption or unintended
event delivery.

**Threat**: Attacker on local network intercepts gRPC stream, modifies
SubscriptionEvent payload, injects false events into subscriber application.

**Mitigation (MANDATORY)**:
- Require mTLS (mutual TLS) for all gRPC connections
- Verify server certificate (fingerprint pinning, TOFU)
- Verify client certificate (mutual authentication)
- Enforce encryption (TLS 1.3 minimum)
- Validate event signatures (HMAC-SHA256 over seqno + payload)

```go
// Server-side: require mTLS
tlsCredentials := credentials.NewTLS(&tls.Config{
  Certificates: []tls.Certificate{serverCert},
  ClientAuth: tls.RequireAndVerifyClientCert,
  ClientCAs: clientCertPool,
  MinVersion: tls.VersionTLS13,
})

listener, _ := net.Listen("tcp", ":9090")
grpcServer := grpc.NewServer(grpc.Creds(tlsCredentials))
```

## WAL Tampering Detection (PATCH W4)

Persistent storage (L3 WAL) may be accessed by unauthorized processes or
network attackers if storage is not encrypted.

**Threat**: Attacker with disk access modifies WAL entries, corrupting per-flow
state. CRC-32 alone cannot detect intentional tampering (checksum recomputable).

**Mitigation (PATCH W4, RECOMMENDED)**:
- Enable filesystem encryption (LUKS, BitLocker, ZFS encryption)
- Add HMAC-SHA256 authentication to WAL entries (PATCH W4)
- HMAC is computed over all fields with session key (derived from TLS handshake)
- Reject WAL entries with invalid HMAC (tampering detected)
- Emit ANOMALY event and skip corrupted entries during recovery
- Restrict WAL file permissions (0600 = read/write by Wotan only)

See Section 3.1 (WAL Format) for HMAC specification.

## WAL Compaction Race Conditions (PATCH W5)

Without proper locking, concurrent compaction threads could cause double-free,
use-after-free, or data corruption.

**Threat (LICH-010)**: Two compaction threads simultaneously rewrite WAL index.
First thread deletes old segments; second thread tries to access same segments
(use-after-free). Index becomes corrupted, recovery fails.

**Mitigation (PATCH W5, MANDATORY)**:
- Exclusive mutex during WAL compaction
- Only one compaction thread active at a time
- Concurrent writes use separate write_lock (different lock)
- Compaction is atomic: either succeeds completely or rolls back

See Section 3.3 (WAL Compaction Locking) for complete specification.

## Seqno Discontinuity Detection (PATCH W1)

Ring buffer overflow or WAL corruption can silently lose entries, violating
at-least-once delivery guarantee.

**Threat (LICH-010)**: Power failure during WAL write creates seqno gap. Recovery
replays entries 0-5, skips 6, resumes at 7. Entry 6 is lost (invisible data loss).

**Mitigation (PATCH W1, MANDATORY)**:
- Every ring buffer and WAL entry carries 64-bit monotonic seqno
- On recovery, validate seqno continuity (no gaps)
- If gap detected, skip to next valid seqno and log alert
- Emit ANOMALY event indicating data loss
- Data loss is visible to application (not silent corruption)

See Section 6.2.2 (Seqno Semantics) for complete specification.

## Dead Letter Queue Information Leakage

Dead letter queue may contain sensitive per-flow state that failed to deliver.
Unauthorized access to DLQ could leak confidential information.

**Threat**: Unprivileged operator queries DLQ API and reads sensitive state
(credentials, keys, private data) from failed events.

**Mitigation (MANDATORY)**:
- Restrict DLQ access (authentication + authorization)
- Require admin credentials to view DLQ entries
- Enforce Sophia-based RBAC (role=admin only)
- Encrypt DLQ storage (filesystem encryption)
- Audit all DLQ access (log query subject, timestamp, entries inspected)
- Auto-delete DLQ entries after 7 days (retention limit)
- Log DLQ monitoring frequency (alert on excessive queries)

```go
func (s *WotanServer) ViewDLQ(ctx context.Context,
  req *ViewDLQRequest) (*ViewDLQResponse, error) {

  caller := ExtractPrincipal(ctx)
  if !s.sophia.IsAdmin(caller):
    log.Error("Non-admin", caller, "attempted DLQ access")
    s.anamnesis.EmitAnomaly(caller, "unauthorized DLQ access")
    return nil, status.Errorf(codes.PermissionDenied, "admin only")

  s.auditLog.LogDLQQuery(caller, time.Now())
  return s.dlq.View(), nil
}
```

## GOAWAY Frame Denial of Service (PATCH W8)

An attacker could send malformed GOAWAY frames to cause server crashes or
connection termination.

**Threat**: Attacker sends GOAWAY with invalid reason code, causing server to
panic or incorrectly flush state.

**Mitigation (PATCH W8, MANDATORY)**:
- Validate GOAWAY frame format and reason codes
- Ignore unknown reason codes (forward compatibility)
- Verify magic field (0xC0FFEE00)
- Validate last_flow_id (must be <= highest processed)
- Implement timeout (close connection after 5 seconds if receiver doesn't close)
- Rate-limit GOAWAY frames (max 1 per second per client)

```go
func (s *WotanServer) ReceiveGOAWAY(frame *GoAwayFrame) error {
  if frame.Magic != 0xC0FFEE00:
    log.Error("Invalid GOAWAY magic", frame.Magic)
    return ErrInvalidFrame

  if frame.ReasonCode > 0x07:
    log.Warn("Unknown GOAWAY reason code", frame.ReasonCode)
    // Treat as code 0x00 (no error)

  if frame.LastFlowID > s.maxProcessedFlowID:
    log.Error("Invalid GOAWAY last_flow_id", frame.LastFlowID)
    return ErrInvalidFrame

  // Valid GOAWAY; proceed with graceful shutdown
  return nil
}

# IANA Considerations

This memo does not request IANA registration of option types or protocol numbers; those are handled by [UNHEADED-FOUNDATION].

Wotan topic naming uses informal convention (compute.*, sophia.*, anamnesis.*). If standardization is needed, IANA may create a registry:

~~~~~
Registry Name: Wotan Topic Namespace
Policy: Expert Review
Template: Topic Name, Component, Description, Reference
~~~~~

Example entries:
- compute.screen.*, Shim Output, Screen/framebuffer I/O
- compute.input.*, Shim Input, User input events
- sophia.dictionary.*, Sophia, Dictionary updates
- anamnesis.*, Anamnesis, Observability events

---


# Changes from draft-bellis-unheaded-wotan-memory-01

Draft-02 incorporates all 8 normative security patches (W1-W8) addressing S21 assessment
findings, plus four new architectural sections:

## Normative Security Patches (W1-W8)

1. **Patch W1: Seqno to Ring Buffer Entries + Monotonicity Validation**: Added 64-bit
   monotonically-increasing sequence numbers to ring buffer and WAL entries. Seqno
   gaps detected via validation during recovery. Corrupted entries (out-of-order writes,
   power failure) now cause visible data loss alerts instead of silent corruption.
   **Addresses**: LICH-010 (WAL Integrity Validation)

2. **Patch W2: Composite L1 Cache Key (Flow + Src/Dst Hash)**: Extended L1 cache key
   from 20-bit flow_label alone to 64-bit composite: (flow_label << 44) | (tuple_hash).
   Tuple hash is SipHash-2-4(src_ip, dst_ip, src_port, dst_port). Eliminates birthday
   attack collision vulnerability where two flows with same label but different src/dst
   could share cache lines.
   **Addresses**: LICH-009 (Cross-Flow Composite Key Collision Testing), privilege escalation

3. **Patch W3: CAS Alignment Enforcement (BPF Verifier)**: BPF verifier MUST inject
   alignment checks before CAS operations, enforcing 8-byte alignment (not 4-byte).
   Programs with misaligned CAS are rejected at load time. Prevents architecture-dependent
   crashes on ARM64/RISC-V (LDAXR, AMO.SWAPD require alignment).
   **Addresses**: D4 (Load-Store Unit Race Conditions), LICH-008 (L1 Cache Race Condition Fuzzing)

4. **Patch W4: HMAC-SHA256 for WAL Entries**: Added optional HMAC-SHA256 authentication
   to WAL entries (in addition to CRC-32). HMAC computed over all fields with session key
   derived from TLS handshake. Detects intentional tampering; CRC detects accidental bit flips.
   **Addresses**: LICH-010 (WAL Integrity), M1 (Monad CRC coverage)

5. **Patch W5: Exclusive Lock During WAL Compaction**: Added exclusive mutex protecting
   WAL compaction. Only one compaction thread executes at a time. Prevents double-free,
   use-after-free, and index corruption from concurrent compaction threads.
   **Addresses**: LICH-010 (Compaction Race Testing), D4 (Load-Store Unit Race Conditions)

6. **Patch W6: Per-Program Cache-Miss Rate Limiting**: Added per-program cache-miss budget
   (10,000 misses/sec). Programs exceeding budget incur throttling (100 ms delay, exponential
   backoff to 5 seconds). Prevents denial-of-service via intentional cache-miss exhaustion
   that could starve other flows.
   **Addresses**: Dark Grimoire Section 4 (Computational Completeness DoS)

7. **Patch W7: SETTINGS Exchange via Control Topic**: Added WOTAN SETTINGS frame negotiation
   (Section 10.1). Before flow establishment, sender and receiver exchange configuration:
   cache size, WAL segment size, retention time, CAS alignment (non-negotiable), seqno
   enable (mandatory). Prevents cross-document divergence (X2) where sender and receiver
   silently assume different parameters.
   **Addresses**: X2 (Cross-Document Consistency Attacks)

8. **Patch W8: GOAWAY Frame Specification**: Added GOAWAY frame (Section 10.2) for graceful
   flow termination. Sender signals last flow ID processed; receiver finishes in-flight
   operations and flushes WAL before closing. 5-second timeout prevents hung connections.
   Ensures crash-safe recovery (LICH-010).
   **Addresses**: LICH-010 (WAL Integrity / Crash-and-Recover), connection management

## New Architectural Sections (Draft-02)

9. **Ring Buffer Formal Specification (Section 12)**: Precise formal specification of ring
   buffer capacity (10,000 entries default, configurable), entry layout (88 bytes fixed),
   head/tail pointer semantics, FIFO ordering, oldest-first eviction policy. Covers lock-free
   read path (BPF ringbuf compatible), memory alignment (page alignment minimum, cache-line
   optimal), and overflow behavior.

10. **gRPC Streaming Contract (Section 13)**: Topic subscription protocol via gRPC streaming.
    Covers topic namespace (system.*, logs.*, anamnesis.*, user.*), stream lifecycle (connect,
    subscribe, receive, unsubscribe, disconnect), backpressure (per-client buffer limits),
    client-side reconnection (exponential backoff 100ms-30s), server-side subscription reaping
    (5-minute TTL). Cross-references proto/unheaded/v1/protocol.proto.

11. **Triple-Role Architecture (Section 14)**: Wotan operates as three coordinated roles in
    separate goroutines within single process:
    - Role 1 (Ring Buffer): Per-service log aggregation, cache miss handling
    - Role 2 (Event Bus): Pub/sub routing, gRPC streaming, backpressure
    - Role 3 (Protocol RAM): WAL flush, compaction (exclusive lock), recovery

    Roles share BPF maps and ring buffers but with strict synchronization. Failure mode:
    ring buffer overflow does NOT block event bus or WAL roles (isolated failure).

12. **Wotan Reliability Guarantees (Section 15)**: Application-level delivery semantics:
    - PublishWithAck: at-least-once delivery (event durability guaranteed before ACK)
    - IdempotencyCache: 24-hour TTL deduplication (prevents duplicate processing on retransmit)
    - OrderedPublisher: per-destination FIFO ordering
    - Dead Letter Queue: exhausted retry handling with information leakage mitigations
      (restrict access, encrypt storage, audit, auto-delete after 7 days)

---
# Acknowledgments

The Linux kernel BPF community (Alexei Starovoitov, Daniel Borkmann, Song Liu) for the infrastructure enabling per-packet computation in the kernel datapath.

The authors of RFC 9669 (BPF ISA), RFC 8799 (Limited Domains), and RFC 9673 (Hop-by-Hop Processing Rehabilitation) for the foundational protocols that make this design possible.

This document was co-authored with assistance from Claude (Anthropic).

---

# Author's Address

Stevie Bellis
Unheaded
Email: stevie@bellis.tech
