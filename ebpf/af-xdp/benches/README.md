# AF_XDP Benchmark Suite

Userspace-only benchmark suite for the AF_XDP zero-copy packet I/O library.
Measures UMEM allocator performance, lock-free ring buffer throughput, and
simulated packet processing latency without requiring kernel AF_XDP support.

**Sacred Law: NO external dependencies. Rust std only. No criterion, no test crate.**

## Building

```bash
cd ebpf/af-xdp/
cargo build --bin af-xdp-bench
cargo build --bin af-xdp-bench --release   # for production numbers
```

## Running

```bash
# Full output: JSON to stdout, human-readable to stderr
./target/debug/af-xdp-bench

# JSON only (pipe stderr to /dev/null)
./target/debug/af-xdp-bench 2>/dev/null

# Human-readable only (pipe stdout to /dev/null)
./target/debug/af-xdp-bench 1>/dev/null

# Save JSON results to file
./target/release/af-xdp-bench > results.json 2>results.md
```

## Benchmarks

### 1. UMEM Frame Alloc/Free (`umem_alloc_free_1000`)

Allocates 1,000 frames from a simulated UMEM pool, then frees them all.
Repeated 5,000 times. Measures the throughput of the LIFO frame allocator
in frames/sec.

**What it tests:** Vec::pop/push performance as a frame pool allocator.
This is the same algorithm used by the real `Umem::alloc_frame()` and
`Umem::free_frame()`.

**Expected results:** >100M frames/sec (allocation is a Vec::pop).

### 2. Ring Buffer Produce/Consume (`ring_produce_consume_batch_*`)

Pushes 10,000 events through a lock-free SPSC ring buffer and consumes
them. Tests batch sizes of 1, 4, 16, 64, and 256 entries.

**What it tests:** Atomic load/store overhead, cache line effects at
different batch sizes, and the cost of the `Acquire`/`Release` memory
ordering used by the ring.

**Expected results:** Larger batches amortize atomic overhead. Expect
batch_256 to be 5-20x faster per-element than batch_1.

### 3. End-to-End Packet RX Latency (`e2e_packet_rx_latency`)

Simulates the full RX path: UMEM alloc, packet write (64B Ethernet frame),
ring push, ring pop, packet read, UMEM free. 10,000 iterations.

**What it tests:** The complete userspace hot path that a real AF_XDP
packet would traverse. Does not include kernel-side latency (XDP program
execution, ring doorbell, etc).

**Expected results:** <500ns per packet on modern hardware.

### 4. Batch Size Sweep (`batch_sweep_*`)

Varies batch size from 1 to 256 packets. Each iteration allocates a batch
of frames, writes simulated packet data, pushes through the ring, pops,
reads, and frees. 2,000 iterations per batch size.

**What it tests:** Optimal batch size for throughput vs latency tradeoff.
Larger batches amortize per-packet overhead but increase tail latency.

**Expected results:** Throughput should scale roughly linearly with batch
size until hitting cache/memory bandwidth limits (typically around 64-128).

### 5. Memory Bandwidth (`memory_bandwidth_1500B_batch64`)

Writes 1,500-byte (MTU-sized) packets to UMEM frames in batches of 64,
pushes through the ring, and reads them back. 5,000 iterations.

**What it tests:** Achievable memory bandwidth through the UMEM + ring
path. The bottleneck is memcpy (write) and cache-line reads.

**Expected results:** >1 GB/sec on modern hardware (limited by memory
bandwidth, not software overhead).

### 6. BPF Map Lookup Simulation (`bpf_map_lookup_sim`)

Looks up keys in a `std::collections::HashMap` simulating a BPF hash map.
4,096 entries, 100,000 lookups.

**What it tests:** Hash table lookup cost as a proxy for BPF map operations.
Real BPF map lookups have additional kernel overhead, so this is a lower
bound.

**Expected results:** <100ns per lookup (HashMap is well-optimized).

### 7. Comparison Matrix (`ring_only_push_pop`, `umem_only_alloc_free`, `umem_ring_combined`)

Isolates ring buffer operations from UMEM operations for direct comparison:

- **ring_only**: Pure push/pop with no UMEM involvement
- **umem_only**: Pure alloc/free with no ring involvement
- **combined**: Full cycle (alloc + write + push + pop + read + free)

**What it tests:** Where time is spent in the packet processing path.
The combined result minus (ring + umem) gives the integration overhead
(memcpy, coordination, etc).

**Expected results:** Combined should be close to ring + umem + memcpy cost.

## CPU Time Measurement

Each benchmark reports user and system CPU time via `getrusage(RUSAGE_SELF)`,
called through a raw `syscall` instruction (no libc dependency). This measures
actual CPU consumption rather than wall-clock time, making results more
reproducible under load.

## JSON Output Schema

```json
{
  "name": "string",
  "timestamp": 1234567890,
  "iterations": 10000,
  "avg_latency_ns": 450,
  "p50_ns": 420,
  "p99_ns": 1200,
  "throughput": 2222222.22,
  "unit": "ops/sec"
}
```

| Field | Type | Description |
|-------|------|-------------|
| `name` | string | Benchmark identifier |
| `timestamp` | u64 | Unix epoch seconds when benchmark completed |
| `iterations` | u32 | Number of times the benchmark was executed |
| `avg_latency_ns` | u64 | Mean latency per iteration in nanoseconds |
| `p50_ns` | u64 | 50th percentile (median) latency in nanoseconds |
| `p99_ns` | u64 | 99th percentile latency in nanoseconds |
| `throughput` | f64 | Throughput in the units specified by `unit` |
| `unit` | string | Unit for throughput (e.g., "ops/sec", "frames/sec", "bytes/sec") |

## Interpreting Results

### Latency

- **avg_latency_ns**: Mean per-iteration time. Affected by outliers.
- **p50_ns**: Median. Represents the "typical" iteration. Best for steady-state.
- **p99_ns**: Tail latency. High p99 relative to p50 indicates jitter (GC pressure, page faults, CPU migration). For AF_XDP, you want p99/p50 ratio < 3x.

### Throughput

- Calculated as `(items_per_iteration * iterations) / total_wall_time`.
- Compare across batch sizes to find the optimal operating point.
- For real AF_XDP, multiply by ~0.7 to account for kernel ring doorbell and XDP program overhead.

### CPU Time

- **user time**: Time spent in userspace code (allocator, ring, memcpy).
- **system time**: Time spent in kernel (mmap, page faults). Should be near zero after warmup.
- High system time suggests excessive page faults or syscall overhead.

### Comparison Matrix

The comparison matrix answers: "Where is my time going?"

- If `ring_only >> umem_only`: Ring atomics dominate. Consider larger batches.
- If `umem_only >> ring_only`: Allocator is the bottleneck. Consider pool pre-warming.
- If `combined >> ring + umem`: Integration overhead (memcpy, cache misses) dominates.

## Architecture Notes

The benchmarks mirror the real AF_XDP data path but run entirely in userspace:

```
Real AF_XDP path:
  NIC -> kernel XDP -> AF_XDP socket -> UMEM (shared) -> Ring -> Userspace

Benchmark path:
  BenchUmem (mmap'd) -> BenchRing (heap-backed) -> measurement
```

The `BenchUmem` uses the same `mmap(MAP_ANONYMOUS)` allocation strategy as the
real `Umem`. The `BenchRing` uses the same `AtomicU32` producer/consumer indices
with `Acquire`/`Release` ordering as the real `Ring<T>`.

What the benchmarks do NOT measure:
- Kernel XDP program execution time
- AF_XDP socket ring doorbell (sendto/poll) overhead
- NIC DMA and interrupt coalescing latency
- Kernel-to-userspace ring synchronization cost

These require a real AF_XDP-capable NIC and root privileges. The benchmarks
provide a lower bound on achievable latency and an upper bound on throughput
for the userspace components.
