# Phase 10 Next-Gen Sender — Breaking the 920K pps Ceiling

## Context

Phase 10 stress testing established that **Shield eBPF is NOT the bottleneck**.
The ceiling is the Python/kernel UDP sendto() syscall at ~920K pps. BPF processes
100% of packets at all tested rates with only 11.5% overhead vs baseline.

To find the actual BPF breaking point, we need a sender that bypasses the kernel
networking stack entirely.

## Current Benchmarks (Python sender)

| Rate | Throughput | Ring Drops | Consumer |
|------|-----------|------------|----------|
| 920K pps | 1.0 Gbps | 82M (no consumer) | N/A |
| 832K pps | 905 Mbps | 2,932 | zero-copy drain |
| 839K pps | 7.35 Gbps (1KB) | — | zero-copy drain |

## Three Approaches (Pick One or Combine)

### Option A: XDP_TX Packet Bouncer (Recommended First)

**Concept**: Attach an XDP program to `br-tomb` that generates Monad packets and
TX-redirects them to `tap-tomb`. Zero syscall overhead — runs entirely in kernel.

**Pros**:
- Fastest possible: millions of pps, limited only by CPU cycles
- No userspace involvement during the flood
- Tests the exact same path (XDP ingress → TC egress)
- Already have the Aya infrastructure

**Cons**:
- Packets are identical unless we add a BPF map for sequence rotation
- Harder to control rate from userspace (need a map-based throttle)
- Requires a second XDP program (can't attach two to same iface)

**Implementation** (`ebpf/stress-xdp/src/main.rs`):
```
1. New eBPF crate: stress-xdp
2. XDP program attached to br-tomb (or a new veth pair)
3. On each incoming packet (e.g., ICMP ping), generate N Monad packets
   using bpf_xdp_adjust_head + manual header construction
4. Return XDP_TX to send back out the same interface (which bridges to tap-tomb)
5. Rate control via a BPF array map: userspace writes target_pps,
   BPF uses bpf_ktime_get_ns() to pace
6. Stats in a shared STRESS_STATS map for userspace monitoring
```

**Expected ceiling**: 5-14 Mpps (single core XDP on modern hardware)

### Option B: Rust AF_XDP (io_uring) Sender

**Concept**: Userspace program using AF_XDP sockets for zero-copy TX.
The kernel's XDP layer handles the packet directly from userspace UMEM
without copying through the socket buffer.

**Pros**:
- Full control over packet contents (unique seq per packet)
- Can saturate 10G+ links
- Production-grade pattern (used by DPDK alternatives)

**Cons**:
- More complex setup (UMEM, fill/completion rings)
- Requires `CAP_NET_ADMIN` + `CAP_SYS_ADMIN` (or root)
- AF_XDP needs XDP program on the sending interface

**Implementation** (`cmd/stress-sender/`):
```
1. New Rust binary using `xsk-rs` or raw `libbpf` AF_XDP bindings
2. Allocate UMEM (2048 frames × 4096 bytes)
3. Pre-build Monad packet templates in UMEM frames
4. Bind AF_XDP socket to br-tomb or a veth
5. Tight loop: fill TX ring → kick → complete → refill
6. Update Monad seq/trace_id per frame for uniqueness
7. Metrics thread reading from completion ring
```

**Expected ceiling**: 10-14 Mpps (AF_XDP zero-copy mode)

### Option C: C raw socket with sendmmsg()

**Concept**: Simple C program using `sendmmsg()` to batch multiple UDP6+HBH
sends in a single syscall. Reduces syscall overhead by 256-1024x.

**Pros**:
- Simplest to implement (~200 lines of C)
- Uses existing kernel path (same as Python but batched)
- No special kernel features needed
- Easy to compile and run

**Cons**:
- Still limited by kernel stack (~2-4 Mpps vs 920K)
- Not as fast as XDP_TX or AF_XDP
- Diminishing returns past ~3 Mpps

**Implementation** (`scripts/stress-cannon.c`):
```
1. Pre-build IPV6_HOPOPTS for Monad injection
2. Create N UDP6 sockets (one per thread)
3. Build iovec + mmsghdr arrays for sendmmsg (batch=256)
4. Tight loop: sendmmsg() with vlen=256
5. Each msg has unique Monad seq via incrementing template
6. Report rate via shared atomic counters
```

**Expected ceiling**: 2-4 Mpps (sendmmsg batching)

## Recommended Execution Order

1. **Option C first** (30 min): Quick win, 2-4x improvement, validates if BPF
   has headroom. Simple C, no special deps.

2. **Option A second** (1-2 hrs): If C sender still doesn't break BPF, go
   straight to XDP_TX for the kernel-bypass path. This is the real stress test.

3. **Option B if needed** (2-4 hrs): Only if we need sustained high-rate with
   unique per-packet Monads and full control. Production-grade but complex.

## Success Criteria

- Find the actual BPF processing ceiling (pps where DEATHS < TC_HBH)
- Identify which BPF operation is the bottleneck (map lookup? ring buffer? checksum?)
- Optimize Shield eBPF based on findings
- Document the ceiling in terms of:
  - Mpps at 64B payload
  - Gbps at MTU payload
  - Events/sec to ANAMNESIS
  - Latency percentiles (p50/p99) from packet arrival to DEATH emit

## Dependencies

- Aya eBPF build system (already working)
- ebpf-loader (already handles XDP + TC attachment)
- bpftool for map inspection
- For Option B: `xsk-rs` crate or `libbpf-sys`
- For Option C: gcc/clang (already on WEST)
