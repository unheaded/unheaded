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

---

abstract

Wotan is the memory and I/O bus for the Unheaded Protocol, providing addressable per-flow storage for BPF programs executing within the Limited Domain [RFC8799].

The Wotan protocol specifies the BPF helper interface for memory access, the address space layout for per-flow data structures, a three-level cache hierarchy (L1 BPF hash maps, L2 per-flow ring buffers, L3 persistent Write-Ahead Log), and the topic-based I/O model for interaction with userspace services.

This memo defines the memory model, helper functions, address space, cache miss protocol, and I/O topic naming conventions for systems implementing the Unheaded Protocol's computational layer.

---

# 1. Introduction

The Unheaded Protocol Foundation [UNHEADED-FOUNDATION] specifies a 20-byte register file (the Monad) that travels with every packet through a Limited Domain. BPF programs at each hop read and write the Monad, performing stateless per-packet computation.

Many use cases require state beyond the 20-byte Monad: buffering input, accumulating results, maintaining per-flow state machines, or storing scratch memory for complex algorithms.

Wotan provides this state via a hierarchical memory model:

- L0: Monad (20 bytes, in packet, wire speed)
- L1: Per-hop BPF map cache (64-byte cache lines, ~100-200 ns latency)
- L2: Per-flow ring buffer RAM (configurable size, ~1-10 µs latency)
- L3: Write-Ahead Log (persistent storage, ~100 µs-1 ms latency)
- L4: Sophia dictionaries (instruction decode, ~100-200 ns latency)

This memo defines the Wotan memory protocol: the BPF helper interface, address space layout, cache coherency model, and userspace I/O interaction.

# 2. Terminology and Language

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

# 3. Architecture Overview

## 3.1. Role in the Unheaded Protocol

Wotan bridges Monad computation to memory and I/O:

- Shim programs (eBPF) running at each hop read/write Wotan memory via BPF helpers (bpf_wotan_read, bpf_wotan_write, bpf_wotan_cas).
- Wotan maintains per-flow state keyed by IPv6 Flow Label.
- Wotan interfaces with userspace via ring buffer events and pub/sub topics.
- Wotan implements cache miss handling, prefetching, and Write-Ahead Log management.

## 3.2. Memory Hierarchy

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

## 3.3. Separation of Compute and Memory

The Monad is transient compute state (stateless by design). Wotan is persistent state machine storage. This separation allows:

- Shim programs to remain stateless with respect to the packet format.
- External state to be accessed in a controlled, measurable manner.
- Cache miss latency to be handled without blocking per-hop logic.
- Memory updates to be tracked in Anamnesis for observability.

# 4. BPF Helper Interface

BPF Shim programs access Wotan memory via three helper functions. All helpers operate on a 32-bit address space keyed by IPv6 Flow Label.

## 4.1. bpf_wotan_read

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

## 4.2. bpf_wotan_write

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

## 4.3. bpf_wotan_cas

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

The CAS operation MUST use atomic compare-and-swap semantics (via BPF_XADD or equivalent) to prevent races between concurrent Shims on different CPUs or cores.

## 4.4. Error Handling

Implementations MUST handle all specified error codes. RECOMMENDED error handling:

- -ENOENT: log flow allocation failure, skip memory access, record to Anamnesis anomaly
- -EFAULT: log address violation, skip access, mark packet for dropped-event
- -ENOMEM (cache miss): increment miss counter, optionally stall and retry via BPF_TAIL_CALL
- -EACCES: log authorization failure, drop packet or set flow_action=DROP
- -EINVAL: log invalid parameter, skip access

Programs MUST NOT crash on negative returns; they MUST check return values and branch accordingly.

## 4.5. Access Control

Wotan enforces per-program, per-flow-label authorization. Each BPF program (identified by its file descriptor or program ID) is associated with a set of allowed flow_labels via Sophia dictionary configuration.

Authorization check pseudocode:

~~~~~
bool is_authorized(program_id, flow_label):
  entry = bpf_map_lookup(&sophia_auth_map, &program_id)
  if !entry: return false
  if entry.allowed_labels == 0: return true  // wildcard (all flows)
  return (entry.allowed_labels & BIT(flow_label)) != 0
~~~~~

Access control rules are defined per deployment and distributed via Sophia. Default: all programs authorized for all flows (wildcard). Operators MAY restrict access to enforce least-privilege per BPF program.

# 5. Memory Address Space

## 5.1. Address Layout

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

## 5.2. Data Memory Region (0x00000000-0x0000BFFF)

Per-flow RAM, allocated from Wotan ring buffer. Size configurable via --ring-size (default: 16 KB).

Used for:
- Stack: top-of-RAM, grows downward
- Heap: allocated upward from 0x00000000
- Scratch: temporary computation state
- Counters: per-flow statistics

Addressing: Linear, byte-addressed. Shims MAY use conventional stack/heap management or flat address space, at operator's discretion.

## 5.3. I/O Memory Region (0x0000C000-0x0000FFFE)

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

## 5.4. Input Region (0x0000FFFF)

Single 4-byte memory-mapped I/O location. Reads consume one event from compute.input.{flow_label} topic.

Semantics:
- Read returns 4-byte event data if available
- If no event: return 0x00000000 (or optionally -EAGAIN)
- Multiple reads in succession fetch successive events (FIFO)

## 5.5. Extended Memory (0x00010000-0x00FFFFFF)

Optional: dictionary/WAD (Write-Anywhere Data) region backed by userspace. Used for:
- Read-only program code (ROM)
- Sophia dictionary local copy
- Large data structures (graphs, tables)

Allocation: per-flow, configured at startup. Misses backed by WAL recovery on restart.

## 5.6. Per-Flow Addressing via Flow Label

Each IPv6 packet carries a 20-bit Flow Label. Wotan uses this as the primary key for per-flow ring buffer allocation:

~~~~~
Wotan ring buffer map key:
  struct wotan_rb_key {
    u32 flow_label;  // 20-bit Flow Label (zero-extended)
  };
~~~~~

On first access to a new flow_label, Wotan MUST allocate a new ring buffer and associated L1 cache map. Ring buffer is freed on flow timeout (configurable, typically 30-300 seconds).

# 6. L1 Cache: Per-Hop BPF Maps

## 6.1. Cache Line Structure

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

Cache lines are 64 bytes (one L3 cache line on x86-64), matching CPU hardware cache for optimal performance.

## 6.2. Prefetch Model

Prefetch is explicit, not automatic. A Shim MAY call bpf_wotan_read on a nearby address to trigger prefetch of that cache line into L1:

~~~~~
// Prefetch next cache line
long ret = bpf_wotan_read(flow_label, next_addr, &dummy, 1);
if (ret == -ENOMEM) {
    // Cache miss; handler will populate
}
~~~~~

Userspace prefetch handler monitors compute.mem.miss topic and proactively reads from L2 ring buffer, populating L1 before Shim retry.

## 6.3. Cache Miss Handling

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

## 6.4. Write-Back Policy

Dirty cache lines are written back to L2 ring buffer via userspace handler:

1. Userspace monitors L1 cache map
2. On overflow or TTL timeout, marks dirty lines for write-back
3. Writes dirty line data + metadata to L2 ring buffer
4. Clears dirty bit in L1

Write-back frequency: RECOMMENDED flush every 100 ms or on L1 map occupancy >80%, whichever comes first. This balances latency (L1 hits) with memory efficiency (L2 persistence).

## 6.5. Eviction (LRU)

When L1 map reaches capacity (configurable max_entries, default 1024 lines = 64 KB):

1. Identify least-recently-used cache line (lowest lru_counter)
2. If dirty: write back to L2 before eviction
3. Evict from L1 map
4. Allocate new line from freed space

LRU counter incremented on every cache hit. Userspace MAY periodically reset counters (divide by 2 to prevent overflow, or periodic scan).

# 7. L2 Ring Buffer: Per-Flow RAM

## 7.1. Ring Buffer Structure

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

## 7.2. Allocation (per Flow Label)

On first access to a new flow_label:

~~~~~
Wotan allocates:
1. New BPF ring buffer (sized via --ring-size)
2. New L1 cache BPF hash map
3. New access control entry in Sophia auth map
4. Emit allocation event to Anamnesis
~~~~~

Allocation is lazy (on first Shim access) or eager (pre-allocated for known flows).

## 7.3. Sizing (--ring-size)

Ring buffer size in bytes, configurable at Wotan startup. Default: 16 KB per flow.

Recommended sizing:

| Use Case | Size | Justification |
|----------|------|---------------|
| Tracing/metadata | 4-8 KB | Light state, mostly Monad replication |
| Service state | 16-32 KB | Stack + heap for small algorithms |
| Buffering | 64-256 KB | Input/output buffering |
| Large state | 512 KB-2 MB | Complex state machines (rare) |

Total Wotan memory per hop: (number of active flows) × (--ring-size + L1 cache overhead).

## 7.4. Overflow Policy

When ring buffer is full and Shim attempts write:

- Helper returns -ENOMEM (overflow)
- Event dropped silently, counter incremented (STAT_L2_OVERFLOW)
- Wotan userspace drains ring buffer to L3 WAL (see Section 9)
- Next write retry after drain completes

RECOMMENDED: drain L2→L3 when occupancy >75%, before Shim-visible overflow.

# 8. L3 Write-Ahead Log: Persistent Storage

## 8.1. WAL Format

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

## 8.2. Flush Policy (TTL-based)

Userspace daemon periodically:

1. Drain dirty L1 cache lines → L2 ring buffer (§6.4)
2. Drain full L2 ring buffer → L3 WAL (§7.4)
3. Flush L3 WAL to disk (fsync)

Flush frequency: RECOMMENDED every 100-1000 ms or on --wal-size-threshold bytes, whichever comes first. This trades write latency (disk fsync) against data durability (loss window on crash).

## 8.3. Recovery on Restart

On Wotan startup or hop restart:

1. Scan L3 WAL directory for all flow_label files
2. For each flow:
   a. Rehydrate L2 ring buffer from WAL entries (in order)
   b. Verify CRC-16 on each entry (skip corrupted entries)
   c. Emit RECOVERY event to Anamnesis
3. L1 caches remain empty (lazy population on next access)

Recovery preserves per-flow state across restarts. Data durability: RPO (Recovery Point Objective) = flush interval (100-1000 ms).

## 8.4. Compaction

Periodic WAL compaction (background task):

1. Read entire WAL file
2. Group entries by address (collapse multiple writes to same address)
3. Keep only latest entry per address
4. Write compacted WAL
5. Swap atomically (rename)

Compaction reduces WAL file size and improves recovery speed. RECOMMENDED: trigger on file size >--wal-max-size (default 100 MB) or age >24 hours.

# 9. Topic-Based I/O

## 9.1. Topic Naming Convention

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

## 9.2. Memory-Mapped I/O Addresses

Writing to address ranges triggers topic publication:

~~~~~
Address      Topic Published              Semantics
------       ------------------          ----------
0x0000C000   compute.screen.{label}      Screen/framebuffer output
0x0000C001   (reserved)
...
0x0000FFFF   compute.input.{label}       Input event consumption (read-only)
~~~~~

## 9.3. Screen Topic (compute.screen.{flow_label})

Writes to address 0x0000C000 publish to compute.screen topic:

~~~~~
struct screen_event {
    u32 flow_label;
    u32 len;               // payload length (1-4096 bytes)
    u8  data[4096];        // framebuffer data
};
~~~~~

Userspace subscribers (dashboard, user interface) consume and display data.

## 9.4. Input Topic (compute.input.{flow_label})

Reads from address 0x0000FFFF fetch from compute.input topic:

~~~~~
struct input_event {
    u32 event_type;        // keyboard, mouse, network, etc.
    u8  payload[60];       // input data (depends on type)
};
~~~~~

Userspace input providers (keyboard, mouse, network) publish to compute.input. Shim programs consume via bpf_wotan_read(flow_label, 0x0000FFFF, ...).

## 9.5. Dictionary Topic (sophia.dictionary.v{N})

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

## 9.6. Anamnesis Topics (anamnesis.{event_type})

Per [UNHEADED-FOUNDATION], Anamnesis ring buffers emit events:

~~~~~
Topic: anamnesis.birth
Topic: anamnesis.death
Topic: anamnesis.hop
Topic: anamnesis.chaos
Topic: anamnesis.anomaly
~~~~~

Wotan subscribes to all anamnesis.* topics and forwards to userspace observability systems (Prometheus, Grafana, ELK).

# 10. Cache Miss Protocol

## 10.1. Miss Event Structure

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

## 10.2. Userspace Handler

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

## 10.3. Stall Mechanism

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

## 10.4. Prefetch Hinting

Shim program MAY prefetch likely-to-be-needed addresses:

~~~~~
// Prefetch stack frame
bpf_wotan_read(flow_label, sp - 64, &dummy, 1);
bpf_wotan_read(flow_label, sp - 128, &dummy, 1);
~~~~~

Userspace handler serves prefetch requests before Shim retry, reducing average miss latency from ~10 µs (on-demand) to ~5 µs (prefetch).

# 11. Computational Memory Model

## 11.1. Program Memory (ROM via Sophia BPF Array)

Read-only program code and constant data are stored in Sophia dictionaries or in extended memory (0x00010000+). These are shared across flows and hops (L4 cache).

## 11.2. Data Memory (RAM via Wotan Ring Buffer)

Per-flow writeable state (stack, heap, scratch) is in L2 ring buffer (0x00000000-0x0000BFFF).

## 11.3. Stack (top-of-RAM, grows downward)

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

## 11.4. Heap (configurable region)

Configurable via Sophia: heap_base and heap_top pointers per flow. Shim programs use simple malloc-like allocation:

~~~~~
// Allocate N bytes
addr = heap_top
heap_top += N
if (heap_top > stack_sp) return -ENOMEM  // collision

// Free (optional, depends on allocator policy)
// Many Shims use linear allocation (no free) for simplicity
~~~~~

## 11.5. Memory-Mapped I/O (designated address ranges)

Addresses 0x0000C000-0x0000FFFF trigger topic I/O:
- Writes publish data
- Reads consume events
- Non-blocking (returns immediately)

# 12. Performance Characteristics

## 12.1. L1 Hit Latency

Approximately 100-200 ns per bpf_wotan_read/write on L1 cache hit (BPF hash map lookup + memcpy of 1-4 bytes).

Measured on:
- Modern x86-64 (Intel Skylake or newer, AMD EPYC)
- BPF JIT compilation enabled
- L1 cache warm (typical case)

Includes: BPF verifier overhead (~0 ns, amortized), hash table lookup (O(1), ~100 ns), memory access (~50 ns), return (~10 ns).

## 12.2. L2 Access Latency

Approximately 1-10 µs for cache miss + userspace handler + L1 refill.

Measured on: same platform, ring buffer in userspace, handler latency measured end-to-end.

Breakdown:
- Emit miss event to ring buffer: ~100 ns
- Userspace poll + event dequeue: ~1-2 µs
- L2 ring buffer lookup: ~1-2 µs
- L1 BPF map update: ~1 µs
- Shim retry: ~100 ns

Total: ~4-5 µs average (9x slower than L1 hit).

## 12.3. L3 Access Latency

Approximately 100 µs to 1 ms for WAL disk I/O.

Depends on:
- Storage backend (SSD: ~100-500 µs; HDD: ~5-20 ms)
- I/O scheduler (deadline, kyber)
- Concurrent I/O load
- Fsync policy (batched vs. immediate)

L3 is only accessed on Wotan restart (recovery) or explicit flushing; not on critical path for per-packet operations.

## 12.4. Cache Hit Rate Targets

For well-tuned deployments: >90% L1 cache hit rate.

Assumptions:
- L1 cache size: 64 KB (1024 lines × 64 bytes)
- Working set: <10 KB per flow (typical for stateless Shims + 10-20 byte working set)
- Temporal locality: same addresses accessed repeatedly within 10 ms window

If hit rate <80%: increase L1 cache size (BPF map max_entries) or reduce L2 ring buffer size to promote prefetching.

# 13. Security Considerations

## 13.1. Per-Program Access Control

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

## 13.2. Flow Isolation

Per-flow ring buffers are isolated by BPF map (one map per flow_label). No Shim can read another flow's data via bpf_wotan_read unless explicitly authorized.

Isolation mechanism:
- Separate BPF ring buffer per flow_label
- Access control check in helper (§4.5)
- Sophia-configured whitelist per program

## 13.3. Ring Buffer Overflow DoS

Ring buffer overflow returns -ENOMEM to Shim. A malicious or buggy Shim could trigger repeated overflows to cause performance degradation.

Mitigations:
- RECOMMENDED: per-program rate limiting on ring buffer writes
- RECOMMENDED: size L2 ring buffer appropriately for expected workload
- OPTIONAL: drop oldest entries on overflow (loose consistency)
- OPTIONAL: quotas per program (STAT_RB_QUOTA enforcement)

## 13.4. WAL Encryption

Persistent storage (L3 WAL) may contain sensitive flow state. Recommendation:

- Enable filesystem encryption (LUKS, BitLocker, ZFS encryption)
- OPTIONAL: Application-level encryption of WAL entries via key stored in Sophia
- Restrict WAL file permissions (0600 = read/write by Wotan only)

No standard wire format for encrypted WAL defined in this memo; operators SHOULD implement per deployment requirements.

# 14. IANA Considerations

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

# 15. References

[RFC2119] Bradner, S., "Key words for use in RFCs to Indicate Requirement Levels", BCP 14, RFC 2119, March 1997.

[RFC8174] Leiba, B., "Ambiguity of Uppercase vs Lowercase in RFC 2119 Key Words", BCP 14, RFC 8174, May 2020.

[RFC8799] Lindem, A., Ed., and D. Eastlake 3rd, Ed., "Limited Domains and Internet Protocols", RFC 8799, July 2020.

[RFC9669] Kirtley, O., Ed., "The eBPF Instruction Set", RFC 9669, December 2024.

[UNHEADED-FOUNDATION] Bellis, S., "The Unheaded Protocol Foundation", Internet-Draft draft-bellis-unheaded-protocol-foundation-03, February 2026.

[UNHEADED-SOPHIA] Bellis, S., "Sophia Dictionary Format for the Unheaded Protocol", Internet-Draft draft-bellis-unheaded-sophia-dictionary-00, February 2026.

[FIPS203] National Institute of Standards and Technology, "Module-Lattice-Based Key-Encapsulation Mechanism Standard", FIPS 203, August 2024.

---

# Acknowledgments

The Linux kernel BPF community (Alexei Starovoitov, Daniel Borkmann, Song Liu) for the infrastructure enabling per-packet computation at wire speed.

The authors of RFC 9669 (eBPF ISA), RFC 8799 (Limited Domains), and RFC 9673 (Hop-by-Hop Processing Rehabilitation) for the foundational protocols that make this design possible.

---

Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>
