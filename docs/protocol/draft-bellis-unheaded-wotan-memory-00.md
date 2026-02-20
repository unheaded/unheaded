---
title: "Wotan Memory Protocol for the Unheaded Protocol"
abbrev: "Wotan Memory Protocol"
docname: draft-bellis-unheaded-wotan-memory-00
category: exp
ipr: trust200902
area: Internet
workgroup: Independent Submission
date: 2026-02-19
stand_alone: yes

author:
  - ins: S. Bellis
    name: Steven Bellis
    org: Unheaded
    email: stevenrbellis@gmail.com
    country: US

normative:
  RFC2119:
  RFC8174:
  RFC9669:
  UNHEADED-FOUNDATION:
    title: "The Unheaded Protocol Foundation"
    author:
      - ins: S. Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-protocol-foundation-03
  UNHEADED-SOPHIA:
    title: "Sophia Dictionary Format for the Unheaded Protocol"
    author:
      - ins: S. Bellis
    date: 2026-02
    seriesinfo:
      Internet-Draft: draft-bellis-unheaded-sophia-dictionary-00

informative:
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

This memo defines the memory model, helper functions, address space, cache miss protocol, and I/O topic naming conventions for systems implementing the Unheaded Protocol's computational layer.

---

# Introduction

The Unheaded Protocol Foundation [UNHEADED-FOUNDATION] specifies a 20-byte register file (the Monad) that travels with every packet through a Limited Domain. BPF programs at each hop read and write the Monad, performing stateless per-packet computation.

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

The address MUST be 4-byte aligned. Implementations MUST perform runtime
alignment verification:

```c
// In Wotan helper before CAS:
if (addr & 0x3) {  // Check if not 4-byte aligned
    return -EFAULT;  // Return alignment error
}
```

### CAS Atomicity Limitations

CAS operation atomicity is ONLY guaranteed for a single 32-bit word at the
specified address. Important limitations:

1. **Single Address Only**: CAS atomically compares and swaps one u32 at one
   address. It does NOT provide transactional semantics across multiple addresses.

2. **Per-Address Atomicity**: If Shim needs to update multiple related fields,
   CAS only guarantees atomicity for one of them. Concurrent accesses from other
   Shims may see intermediate states.

3. **Non-Blocking**: CAS is non-blocking—it returns immediately with success or
   -EAGAIN. It does NOT stall.

4. **Concurrent Flows**: CAS operations on different flows (different keys) do
   not interfere; each flow's CAS is independent.

5. **Alignment Requirement**: Address MUST be 4-byte aligned. The helper MUST
   check this at runtime and return -EFAULT if misaligned. The BPF verifier
   may not catch all alignment violations.

**Recommended Use Pattern**:
- Use CAS for per-flow state counters (e.g., packet count)
- Do NOT use CAS for multi-field updates expecting atomic visibility
- For complex transactions, use a per-flow spinlock implemented via CAS on a
  separate "lock" field
- Always ensure addresses are 4-byte aligned before calling bpf_wotan_cas()

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

## Cache Line Structure

L1 cache is a per-hop BPF hash map (BPF_MAP_TYPE_HASH):

~~~~~
struct l1_cache_key {
    u32 flow_label;    // 20-bit Flow Label (zero-extended)
    u32 addr;          // cache-line-aligned address (& ~0x3F)
};

struct l1_cache_line {
    u8  data[64];      // 64-byte cache line (8 x u64)
    u32 tag;           // address tag for mismatch detection
    u8  valid;         // 0=invalid, 1=valid
    u8  dirty;         // 0=clean, 1=dirty (needs write-back)
    u16 lru_counter;   // for LRU eviction ordering
};
~~~~~

Cache lines default to 64 bytes (one L3 cache line on x86-64), matching CPU
hardware cache line size for optimal performance on modern systems.

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

## Ring Buffer Structure

Per-flow RAM is a BPF ring buffer (BPF_MAP_TYPE_RINGBUF):

~~~~~
struct wotan_rb_entry {
    u64 timestamp_ns;       // bpf_ktime_get_ns()
    u32 flow_label;
    u32 addr;               // address of write
    u8  data[64];           // cache line data
    u8  valid;              // 1 if entry is valid
    u8  _pad[3];            // alignment
};
// Total: 80 bytes per entry
~~~~~

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

## WAL Format

Per-flow persistent log on local storage (SQLite, RocksDB, or simple file):

~~~~~
struct wal_entry {
    u64 timestamp_ns;
    u32 flow_label;
    u32 addr;
    u8  data[64];       // cache line data
    u16 checksum;       // CRC-16 over data (error detection)
    u8  _pad[2];
};
// Total: 80 bytes per entry (matching L2 entry)
~~~~~

One WAL file per flow, or single WAL with flow_label demultiplexing (implementation choice).

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

## Compaction

Periodic WAL compaction (background task) is MANDATORY for operational
efficiency:

1. Read entire WAL file
2. Group entries by address (collapse multiple writes to same address)
3. Keep only latest entry per address
4. Write compacted WAL
5. Swap atomically (rename)

Compaction reduces WAL file size and improves recovery speed.

### Compaction Policy (MANDATORY)

Wotan daemon MUST compact WAL files when:
- File size exceeds 100 MB (--wal-max-size), OR
- File age exceeds 24 hours

Operator SHOULD monitor compaction frequency via metrics:
- wotan_wal_compaction_events_total (counter)
- wotan_wal_file_size_bytes (gauge)
- wotan_wal_compaction_duration_seconds (histogram)

Failing to compact WAL files can lead to:
- Unbounded disk growth (WAL becomes full)
- Slow recovery on restart (processing many duplicate entries)
- L3 flush performance degradation

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

## Per-Program Access Control

Each Shim program (BPF program ID) is associated with a set of allowed flow_labels via Sophia. Wotan helper functions check authorization before allowing memory access.

Access control entry:

~~~~~
struct wotan_authz_entry {
    u32 program_id;
    u64 allowed_labels;     // bitmask (0=all, nonzero=selective)
    u64 allowed_addresses;  // address range whitelist (optional)
};
~~~~~

Enforcement: mandatory on every bpf_wotan_read/write/cas call. Violation → return -EACCES, log to Anamnesis.

## Flow Isolation

Per-flow ring buffers are isolated by BPF map (one map per flow_label). No Shim can read another flow's data via bpf_wotan_read unless explicitly authorized.

Isolation mechanism:
- Separate BPF ring buffer per flow_label
- Access control check in helper (§4.5)
- Sophia-configured whitelist per program

## Ring Buffer Overflow DoS

Ring buffer overflow returns -ENOMEM to Shim. A malicious or buggy Shim could trigger repeated overflows to cause performance degradation.

Mitigations:
- RECOMMENDED: per-program rate limiting on ring buffer writes
- RECOMMENDED: size L2 ring buffer appropriately for expected workload
- OPTIONAL: drop oldest entries on overflow (loose consistency)
- OPTIONAL: quotas per program (STAT_RB_QUOTA enforcement)

## WAL Encryption

Persistent storage (L3 WAL) may contain sensitive flow state. Recommendation:

- Enable filesystem encryption (LUKS, BitLocker, ZFS encryption)
- OPTIONAL: Application-level encryption of WAL entries via key stored in Sophia
- Restrict WAL file permissions (0600 = read/write by Wotan only)

No standard wire format for encrypted WAL defined in this memo; operators SHOULD implement per deployment requirements.

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

# Acknowledgments

The Linux kernel BPF community (Alexei Starovoitov, Daniel Borkmann, Song Liu) for the infrastructure enabling per-packet computation in the kernel datapath.

The authors of RFC 9669 (BPF ISA), RFC 8799 (Limited Domains), and RFC 9673 (Hop-by-Hop Processing Rehabilitation) for the foundational protocols that make this design possible.

This document was co-authored with assistance from Claude (Anthropic).

---

# Author's Address

Steven Bellis
Unheaded
Email: stevenrbellis@gmail.com
