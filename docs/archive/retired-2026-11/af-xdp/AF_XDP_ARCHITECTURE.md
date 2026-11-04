# AF_XDP Architecture — Unheaded Kingdom

**Status:** Phase 11 (Documentation and Final Integration)
**License:** GPL-2.0-only
**Updated:** 2026-03-01

---

## Overview

AF_XDP (Address Family XDP) provides zero-copy packet delivery from the Linux
kernel directly to userspace memory.  In the Unheaded Kingdom, AF_XDP is the
high-performance data path that bypasses the entire kernel networking stack,
delivering Monad-stamped packets to the trace-collector and other userspace
consumers at line rate.

AF_XDP is integrated into two Kingdom eBPF programs:

- **shield-ebpf** (XDP ingress): After BIRTH stamping, redirects packets to
  `SHIELD_XSKS` for zero-copy delivery.
- **packet-marker** (XDP): After trace-ID extraction/injection, redirects
  marked packets to `MARKER_XSKS` for zero-copy delivery.

Both programs support a dual-path architecture: AF_XDP redirect when enabled
and a socket is bound, falling back to the standard kernel stack (`XDP_PASS`)
when no socket is present.

---

## Data Flow Diagram

```
                          INGRESS PACKET
                               |
                               v
                     +-------------------+
                     |    NIC Hardware    |
                     |  (RX queue N)     |
                     +--------+----------+
                              |
                              v
                     +-------------------+
                     |    XDP Hook       |
                     |  (driver mode)    |
                     +--------+----------+
                              |
               +--------------+--------------+
               |                             |
               v                             v
     +-------------------+         +-------------------+
     |  shield-ebpf XDP  |         | packet-marker XDP |
     |                   |         |                   |
     | 1. Parse ETH/IPv6 |         | 1. Parse ETH/IPv4 |
     | 2. Blocklist check|         | 2. Extract flow   |
     | 3. Strip ext hdrs |         | 3. Trace ID check |
     | 4. Insert HBH+    |         | 4. Update flow    |
     |    Monad (BIRTH)  |         |    state           |
     | 5. Set Flow Label |         |                   |
     +--------+----------+         +--------+----------+
              |                             |
              v                             v
     +-------------------+         +-------------------+
     | AF_XDP Enabled?   |         | AF_XDP Enabled?   |
     | SHIELD_CONFIG[0]  |         | MARKER_CONFIG[0]  |
     | bit 0 == 1?       |         | bit 0 == 1?       |
     +---+----------+----+         +---+----------+----+
         |          |                  |          |
        YES         NO               YES         NO
         |          |                  |          |
         v          v                  v          v
  +-----------+ +--------+     +-----------+ +--------+
  | XSKMAP    | | XDP_   |     | XSKMAP    | | XDP_   |
  | redirect  | | PASS   |     | redirect  | | PASS   |
  | SHIELD_   | | (kernel|     | MARKER_   | | (kernel|
  | XSKS[q]   | | stack) |     | XSKS[q]   | | stack) |
  +-----+-----+ +--------+     +-----+-----+ +--------+
        |                             |
        +----------+------------------+
                   |
                   v
          +-------------------+
          |   AF_XDP Socket   |
          |   (userspace)     |
          +--------+----------+
                   |
                   v
          +-------------------+
          |    UMEM Region    |
          |  (shared memory)  |
          |                   |
          | Frame 0 [4096 B]  |
          | Frame 1 [4096 B]  |
          | Frame 2 [4096 B]  |
          | ...               |
          | Frame N [4096 B]  |
          +--------+----------+
                   |
                   v
          +-------------------+
          |   RX Ring         |
          | (kernel -> user)  |
          |                   |
          | desc[0]: addr,len |
          | desc[1]: addr,len |
          | ...               |
          +--------+----------+
                   |
                   v
          +-------------------+
          |  XdpEngine        |
          |  rx_burst()       |
          |                   |
          | -> PacketBuf[]    |
          | -> process packet |
          | -> free_frame()   |
          +-------------------+
                   |
                   v
          +-------------------+
          |  Application      |
          |  (trace-collector,|
          |   Go bridge, etc) |
          +-------------------+
```

### Complete Packet Journey (Shield Path)

```
1. NIC receives Ethernet frame on RX queue N
2. XDP hook fires before sk_buff allocation (zero-copy from DMA)
3. shield_xdp() processes the packet:
   a. Parse Ethernet header, check for IPv6 (ETH_P_IPV6 = 0x86DD)
   b. Parse IPv6 fixed header (40 bytes)
   c. Check source address against BLOCKLIST map -> XDP_DROP if blocked
   d. Check for existing HBH header with Monad -> skip if already stamped
   e. Strip any IPv6 extension headers from Shadow traffic
   f. Generate Flow Label from bpf_get_prandom_u32()
   g. Build Monad register file (20 bytes) with BIRTH state
   h. bpf_xdp_adjust_head(-24) to make room for HBH header
   i. Shuffle ETH+IPv6 headers forward, write HBH+Monad at offset 54
   j. Update IPv6 Next Header, Payload Length, Flow Label
4. AF_XDP decision point:
   a. Read SHIELD_CONFIG[0] -> check bit 0
   b. If enabled: SHIELD_XSKS.redirect(queue_id, 0)
      - Success: packet goes to AF_XDP socket (zero-copy) -> return action
      - Failure (no socket): fall through to kernel stack
   c. If disabled: XDP_PASS -> kernel stack
5. Emit BIRTH event to ANAMNESIS ring buffer
6. Packet arrives in userspace UMEM via RX ring descriptor
7. XdpEngine::rx_burst() reads descriptors, returns PacketBuf references
8. Application processes packet data directly from UMEM (zero-copy)
9. Application calls free_frame() to return frame to Fill ring
10. Kernel reuses frame for next incoming packet
```

---

## UMEM Layout

UMEM (User Memory) is a contiguous shared memory region mapped into both
kernel and userspace address spaces.  It serves as the packet buffer pool
for all AF_XDP operations.

### Memory Layout

```
UMEM Base Address (mmap'd via MAP_SHARED | MAP_ANONYMOUS | MAP_POPULATE)
|
+--- Frame 0 ----+--- Frame 1 ----+--- Frame 2 ----+--- ... ---+--- Frame N-1 -+
|   [headroom]   |   [headroom]   |   [headroom]   |           |   [headroom]  |
|   [pkt data ]  |   [pkt data ]  |   [pkt data ]  |           |   [pkt data ] |
|   [padding  ]  |   [padding  ]  |   [padding  ]  |           |   [padding  ] |
+-- 4096 bytes --+-- 4096 bytes --+-- 4096 bytes --+--- ... ---+- 4096 bytes --+
|<-------------- total_size = frame_size * frame_count ----------------------->|
```

### Configuration Parameters

| Parameter      | Default | Constraints                                    |
|----------------|---------|------------------------------------------------|
| `frame_size`   | 4096    | Power of 2, minimum 2048                       |
| `frame_count`  | 4096    | > 0, maximum 1,000,000                         |
| `headroom`     | 0       | Must be < frame_size                           |
| `flags`        | 0       | XDP_UMEM_UNALIGNED_CHUNK_FLAG for unaligned     |

**Default UMEM size:** 4096 frames * 4096 bytes = 16 MiB

### Frame Structure

```
+--------------------------------------------------+
| Frame at offset (frame_index * frame_size)        |
|                                                  |
| Byte 0                                           |
| +-- headroom bytes (configurable, default 0) --+ |
| |   (reserved for XDP metadata / prepend)      | |
| +----------------------------------------------+ |
| |   Packet data                                | |
| |   (up to frame_size - headroom bytes)        | |
| +----------------------------------------------+ |
| |   Unused padding (if packet < frame_size)    | |
| +----------------------------------------------+ |
| Byte (frame_size - 1)                            |
+--------------------------------------------------+
```

### Allocation Strategy: Free List (LIFO Stack)

The UMEM allocator uses a LIFO stack (Vec<u64>) for O(1) alloc/free:

```
Initial state (4 frames):
  free_frames: [0, 4096, 8192, 12288]   (all frames free)

After alloc_frame():
  free_frames: [0, 4096, 8192]          -> returns 12288

After alloc_frame():
  free_frames: [0, 4096]                -> returns 8192

After free_frame(12288):
  free_frames: [0, 4096, 12288]         -> frame recycled (LIFO)

After alloc_frame():
  free_frames: [0, 4096]                -> returns 12288 (LIFO reuse)
```

**Properties:**
- O(1) allocation (Vec::pop)
- O(1) deallocation (Vec::push)
- No fragmentation (all frames same size)
- LIFO ordering provides temporal cache locality
- Out-of-range addresses silently rejected in free_frame()

### Registration Sequence

```rust
// 1. mmap anonymous shared memory
let ptr = mmap(NULL, total_size, PROT_READ|PROT_WRITE, MAP_SHARED|MAP_ANONYMOUS|MAP_POPULATE, -1, 0);

// 2. Register with AF_XDP socket
let reg = XskUmemReg { addr: ptr, len: total_size, chunk_size: 4096, headroom: 0, flags: 0 };
setsockopt(sock_fd, SOL_XDP, XDP_UMEM_REG, &reg, sizeof(reg));

// 3. Set fill and completion ring sizes
setsockopt(sock_fd, SOL_XDP, XDP_UMEM_FILL_RING, &ring_size, sizeof(u32));
setsockopt(sock_fd, SOL_XDP, XDP_UMEM_COMPLETION_RING, &ring_size, sizeof(u32));
```

---

## Ring Topology

AF_XDP uses four ring buffers, each a shared-memory single-producer
single-consumer (SPSC) queue between kernel and userspace.

### Ring Overview

```
             USERSPACE                              KERNEL
           (producer)                            (consumer)
    +-------------------+                  +-------------------+
    |   Fill Ring       |  ---frames--->   |   Fill Ring       |
    |   (empty frames   |                  |   (takes frames,  |
    |    for kernel RX)  |                  |    fills with     |
    +-------------------+                  |    RX packets)    |
                                           +-------------------+

             KERNEL                              USERSPACE
           (producer)                            (consumer)
    +-------------------+                  +-------------------+
    |   RX Ring         |  ---packets-->   |   RX Ring         |
    |   (received pkt   |                  |   (reads packet   |
    |    descriptors)    |                  |    addr + len)    |
    +-------------------+                  +-------------------+

             USERSPACE                              KERNEL
           (producer)                            (consumer)
    +-------------------+                  +-------------------+
    |   TX Ring         |  ---packets-->   |   TX Ring         |
    |   (packets to     |                  |   (transmits      |
    |    transmit)       |                  |    packets)       |
    +-------------------+                  +-------------------+

             KERNEL                              USERSPACE
           (producer)                            (consumer)
    +-------------------+                  +-------------------+
    |   Completion Ring |  ---frames--->   |   Completion Ring |
    |   (frames done    |                  |   (recycles frames|
    |    transmitting)   |                  |    to free pool)  |
    +-------------------+                  +-------------------+
```

### Ring Data Structures

Each ring is mmap'd from the kernel and contains:

```
+----------------------------------------------------------+
| mmap'd Ring Region                                        |
|                                                          |
| +-- producer (u32 atomic) --+  (offset from mmap base)  |
| +-- consumer (u32 atomic) --+                            |
| +-- flags    (u32)         --+  (NEED_WAKEUP bit)        |
| +-- desc[0]                --+                            |
| +-- desc[1]                --+  (power-of-2 entries)     |
| +-- desc[2]                --+                            |
| +-- ...                    --+                            |
| +-- desc[size-1]           --+                            |
+----------------------------------------------------------+
```

### Ring Types and Descriptors

| Ring       | Producer  | Consumer  | Descriptor Type           | Size |
|------------|-----------|-----------|---------------------------|------|
| Fill       | Userspace | Kernel    | FillDesc { addr: u64 }    | 8 B  |
| Completion | Kernel    | Userspace | CompletionDesc { addr: u64 } | 8 B |
| RX         | Kernel    | Userspace | XskDesc { addr, len, opts } | 16 B |
| TX         | Userspace | Kernel    | XskDesc { addr, len, opts } | 16 B |

### Ring Operation Protocol

```
Fill Ring (userspace -> kernel):
  1. Read consumer index (Acquire barrier)
  2. Calculate free_slots = size - (producer - consumer)
  3. Write frame addresses at desc[producer & mask .. (producer+n) & mask]
  4. Store-release barrier
  5. Update producer index
  6. Kernel takes addresses, uses them for incoming packet DMA

RX Ring (kernel -> userspace):
  1. Load-acquire barrier, read producer index
  2. Calculate available = producer - consumer
  3. Read descriptors: addr (UMEM offset) + len (packet size)
  4. Store-release barrier
  5. Update consumer index
  6. Process packet data at UMEM[addr..addr+len]

TX Ring (userspace -> kernel):
  1. Read consumer index (Acquire barrier)
  2. Calculate free_slots
  3. Write packet descriptors (addr + len) into ring
  4. Store-release barrier
  5. Update producer index
  6. Call sendto(fd, NULL, 0, MSG_DONTWAIT, NULL, 0) to kick kernel

Completion Ring (kernel -> userspace):
  1. Load-acquire barrier, read producer index
  2. Read completed frame addresses
  3. Update consumer index
  4. Return addresses to UMEM free pool
```

### Memory Ordering

On x86_64 (the only supported architecture), the hardware provides Total
Store Order (TSO).  The ring buffers use:

- **Acquire** fence when reading the remote index (smp_rmb)
- **Release** fence when writing the local index (smp_wmb)
- **Relaxed** loads for reading the local index (only one writer)

This maps to `std::sync::atomic::fence()` with appropriate `Ordering`.

---

## XSKMAP Pinning

eBPF maps of type `BPF_MAP_TYPE_XSKMAP` store AF_XDP socket file descriptors
indexed by NIC queue ID.  The kernel uses these to decide where to redirect
packets.

### Map Names and Paths

| Map Name         | Program        | Max Entries | Purpose                              |
|------------------|----------------|-------------|--------------------------------------|
| `XSKS`           | xdp-redirect   | 64          | Generic AF_XDP redirect              |
| `SHIELD_XSKS`    | shield-ebpf    | 64          | Shield BIRTH path redirect           |
| `MARKER_XSKS`    | packet-marker  | 64          | Trace-marked packet redirect         |

### Pinning Paths (BPF filesystem)

```
/sys/fs/bpf/unheaded/
  +-- xdp_redirect/
  |     +-- XSKS              (BPF_MAP_TYPE_XSKMAP, key=u32 queue_id)
  |     +-- CONFIG             (BPF_MAP_TYPE_HASH, key=u32, val=XdpRedirectConfig)
  |     +-- STATS              (BPF_MAP_TYPE_HASH, key=u32, val=XdpRedirectStats)
  +-- shield/
  |     +-- SHIELD_XSKS       (BPF_MAP_TYPE_XSKMAP, key=u32 queue_id)
  |     +-- SHIELD_CONFIG      (BPF_MAP_TYPE_HASH, key=u32, val=u32)
  |     +-- ANAMNESIS          (BPF_MAP_TYPE_RINGBUF, 8 MiB)
  |     +-- BLOCKLIST          (BPF_MAP_TYPE_HASH)
  |     +-- STATS              (BPF_MAP_TYPE_HASH)
  +-- packet_marker/
        +-- MARKER_XSKS       (BPF_MAP_TYPE_XSKMAP, key=u32 queue_id)
        +-- MARKER_CONFIG      (BPF_MAP_TYPE_HASH, key=u32, val=u32)
        +-- FLOW_STATE         (BPF_MAP_TYPE_HASH)
        +-- PACKET_EVENTS      (BPF_MAP_TYPE_RINGBUF)
        +-- STATS              (BPF_MAP_TYPE_HASH)
```

### XSKMAP Population

Userspace writes socket file descriptors into the XSKMAP after binding:

```
// Pseudocode
let xsk_fd = socket(AF_XDP, SOCK_RAW, 0);
// ... register UMEM, set ring sizes, bind to queue ...
bpf_map_update_elem(xskmap_fd, &queue_id, &xsk_fd, BPF_ANY);
```

The XDP program then uses `bpf_redirect_map(&XSKS, queue_id, flags)` to
redirect matching packets to the socket.

### Configuration Maps

**SHIELD_CONFIG** (shield-ebpf):
- Key 0, bit 0: AF_XDP redirect enable (1=on, 0=off)
- Default: disabled (0)

**MARKER_CONFIG** (packet-marker):
- Key 0, bit 0: AF_XDP redirect enable for trace-marked packets
- Default: disabled (0)

**CONFIG** (xdp-redirect):
- Key: queue_id (u32)
- Value: XdpRedirectConfig { enabled: u8, protocol_filter: u8 }
- protocol_filter: 0=all, 4=IPv4 only, 6=IPv6 only

---

## Kernel Version Requirements

| Kernel Version | Feature                                         | Status      |
|----------------|--------------------------------------------------|-------------|
| 4.18+          | AF_XDP core support                              | Minimum     |
| 5.4+           | XDP_USE_NEED_WAKEUP flag                         | Recommended |
| 5.8+           | BPF ring buffer (BPF_MAP_TYPE_RINGBUF)           | Minimum     |
| 5.10+          | XskMap improvements, BTF support                 | Recommended |
| 5.15+          | bpf_redirect_map with XSKMAP, multi-buffer XDP   | **Required** |
| 6.0+           | AF_XDP multi-buffer support                      | Optional    |

**Unheaded requirement:** Linux 5.15+ (documented in af-xdp-common header).
The `af-xdp-common` crate comment states "Kernel >= 5.15 required."

**Recommended:** Linux 5.15 LTS or later.  The WEST bare metal cluster
runs kernel 6.17.

---

## Thread Safety Guarantees

### Ring Buffers: SPSC (Single-Producer / Single-Consumer)

Each ring buffer has exactly one producer and one consumer.  No locks
are needed because the producer only writes the producer index and the
consumer only writes the consumer index.

```
Fill Ring:       Userspace (producer) <-> Kernel (consumer)
Completion Ring: Kernel (producer)    <-> Userspace (consumer)
RX Ring:         Kernel (producer)    <-> Userspace (consumer)
TX Ring:         Userspace (producer) <-> Kernel (consumer)
```

**Guarantee:** No lock contention in the hot path.  All synchronization
is via atomic index updates with acquire/release ordering.

### Statistics Counters

Engine statistics (`rx_packets`, `tx_packets`, `rx_bytes`, etc.) use
plain u64 counters.  The `XdpEngine` is designed for single-threaded
access -- no atomic operations needed for stats.

For cross-thread stats sharing, the `EngineStats` snapshot is returned
as an owned struct from `engine.stats()`.

### eBPF Map Counters

In-kernel statistics (STATS maps in shield-ebpf and packet-marker) use
`saturating_add(1)` on per-CPU map values.  BPF map operations are
atomic at the per-CPU level.

### Signal Handling

The `SignalHandler` struct uses `Arc<AtomicBool>` with:
- `Ordering::Relaxed` for reads (polling in event loop)
- `Ordering::Release` for writes (signal handler context)

This is safe because the signal handler only sets the flag, and the
event loop only reads it -- no data depends on the ordering.

### Summary of Safety Properties

| Component          | Mechanism            | Lock-Free | Contention |
|--------------------|----------------------|-----------|------------|
| Fill Ring          | Atomic SPSC          | Yes       | Zero       |
| Completion Ring    | Atomic SPSC          | Yes       | Zero       |
| RX Ring            | Atomic SPSC          | Yes       | Zero       |
| TX Ring            | Atomic SPSC          | Yes       | Zero       |
| UMEM Free List     | Vec<u64> (single-threaded) | N/A  | N/A        |
| Engine Stats       | Plain u64 (single-threaded) | N/A  | N/A        |
| Signal Handler     | AtomicBool           | Yes       | Zero       |
| Lock-free Ring<T>  | Atomic SPSC          | Yes       | Zero       |

---

## Related Documents

- [DEPLOYMENT_GUIDE_AFXDP.md](../DEPLOYMENT_GUIDE_AFXDP.md) -- Deployment and troubleshooting
- [TESTING_AFXDP.md](../TESTING_AFXDP.md) -- Test organization and CI/CD
- [MIGRATION_AFXDP.md](../MIGRATION_AFXDP.md) -- Migration guide for existing users
- [CHANGELOG_AFXDP.md](../CHANGELOG_AFXDP.md) -- Phase-by-phase changelog
- [RUST_COMPONENTS.md](RUST_COMPONENTS.md) -- Rust crate inventory
