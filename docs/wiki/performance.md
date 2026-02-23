# Performance Analysis

## Overview

This page summarizes the performance characteristics of the Doom-over-IPv6 execution engine and the optimization roadmap for achieving playable frame rates. Detailed methodology, raw data, and profiling scripts are documented in `docs/doom/PERFORMANCE.md`.

---

## Baseline Performance (Pre-Optimization)

| Metric | Value |
|--------|-------|
| Injection rate | ~333 pps (3000 us inter-packet delay) |
| Internal FPS | ~6 fps |
| Instructions per packet | ~4,080 (128 insn/tick x 255 bounces / 8 hops, approx) |
| Instructions per frame | ~1,470,000 |
| Instructions per second | ~1,350,000 |
| WebSocket FPS | ~30 fps (frame repeats when no new data) |
| XDP hardware capacity (est.) | ~10,000,000 pps |
| Current utilization | 0.003% of XDP capacity |

The critical insight: at 0.003% of estimated XDP capacity, the bottleneck is always userspace packet injection, never the kernel data plane. XDP can process millions of packets per second; we inject hundreds.

---

## Execution Model

```
Python/Go Injector (userspace)
     |
     v  AF_PACKET raw socket
 [veth01]  (monad0 namespace)
     |
     v  XDP_TX forwarding chain
 [monad0] -> [monad1] -> [monad2] -> [monad3] -> [monad4] -> [monad5]
     ^                                                           |
     |___________________________________________________________|
                        Packet circulation ring

Each hop: monad_cpu XDP program executes 128 MBC instructions
Full ring (6 hops): 768 instructions
Full packet (255 bounces): 128 x 255 = 32,640 instructions
```

The turbo configuration (128 instructions per tick, 255 bounces per packet) delivers 32,640 instructions per injected packet. At the baseline rate of 333 packets per second, this yields approximately 10.9 million instructions per second -- enough for roughly 7 frames per second.

---

## Optimization Hypotheses

Three hypotheses guide the WS3 scaling profiler work:

### H1: Timing Bounds

**Hypothesis:** The 3 ms inter-packet delay compensates for Python socket overhead, not XDP hard limits. The actual XDP processing time per packet is far below 3 ms.

**Validation approach:** Instrument the STATS BPF map with per-bounce nanosecond timestamps. Measure actual XDP_TX cycle time. Sweep delays from 3000 us to 500 us and measure corruption rate at each level.

**Expected results:**

| Delay (us) | PPS | Est. FPS | Status |
|-----------|-----|----------|--------|
| 3000 | ~333 | ~6.7 | Baseline |
| 2000 | ~500 | ~10 | Expected safe |
| 1500 | ~667 | ~13 | Expected safe |
| 1000 | ~1000 | ~20 | Target |
| 750 | ~1333 | ~27 | Aggressive |
| 500 | ~2000 | ~40 | Limit test |

### H2: Burst Injection (Netflix Model)

**Hypothesis:** Burst injection (fire N packets, drain ring, repeat) yields 2-3x throughput versus steady-state injection. This mirrors how Netflix CDN nodes deliver video: burst a chunk of packets, pause briefly, repeat.

**Validation approach:** Test batch sizes of 10, 50, 100, and 200 packets. Measure throughput and corruption rate for each batch size. Compare to steady-state injection at equivalent average PPS.

### H3: Native Injector

**Hypothesis:** A Go (or Rust) injector eliminates Python's socket.send() overhead, achieving 3-5x speedup over Python's bulk_inject.py.

**Validation approach:** Build Go injector using AF_PACKET raw sockets (`golang.org/x/sys/unix`). A/B benchmark: same packet count, same configuration, measure wall-clock time and achieved PPS.

---

## Go Injector

Location: `cmd/doom-go-injector/main.go`

The Go injector replaces Python for high-throughput packet injection. It uses AF_PACKET raw sockets for zero-copy sending.

**Modes:**

| Mode | Description | Use Case |
|------|-------------|----------|
| `steady` | Fixed inter-packet delay | Baseline comparison with Python |
| `burst` | Fire batch, brief pause, repeat | Production target (Netflix model) |
| `fast` | No delay, maximum throughput | Stress testing only |

**Usage:**
```bash
go build -o /tmp/doom-go-injector ./cmd/doom-go-injector/
sudo ip netns exec monad0 /tmp/doom-go-injector \
    --count 10000 --mode burst --batch 100 --iface veth01
```

---

## Netflix PPS Comparison

To contextualize our packet rates against real-world network traffic:

| Scenario | PPS | Notes |
|----------|-----|-------|
| Home router max | 50,000 - 100,000 | Multiple 4K streams |
| Netflix 4K steady | 1,500 | One viewer, constant bitrate |
| Netflix 4K + ACKs | 2,250 | Bidirectional total |
| Doom baseline (3 ms) | ~333 | Current single-stream |
| Doom target (burst) | 1,000+ | Phase 4 result |
| Doom aggressive (500 us) | 2,000+ | Phase 3 limit test |
| XDP hardware max (est.) | 10,000,000 | Never tested |

At the target rate of 1,000 pps, Doom consumes traffic equivalent to approximately two-thirds of a single Netflix 4K stream. The XDP data plane has capacity for approximately 6,600 simultaneous Doom instances before reaching hardware limits.

---

## Future Optimization Paths

1. **sendmmsg():** Batch multiple packets per syscall (available in Go and C). Reduces syscall overhead by amortizing it across N packets.

2. **TPACKET_V3:** mmap-based zero-copy ring buffer injection. Eliminates all copy overhead between userspace and kernel.

3. **Multi-hop parallelism:** Inject packets at multiple ring entry points simultaneously. Each entry point feeds into the same circulation ring but starts execution at different namespaces.

4. **Adaptive injection:** Dynamically adjust inter-packet delay based on STATS map feedback. If the ring is processing packets faster than injection, reduce delay. If corruption is detected, increase delay.

5. **XDP_REDIRECT:** Use `bpf_redirect()` to forward packets directly between interfaces, bypassing the TCP/IP stack entirely for inter-namespace forwarding.

---

## Key Files

| File | Purpose |
|------|---------|
| `docs/doom/PERFORMANCE.md` | Full methodology, raw data, and profiling scripts |
| `cmd/doom-go-injector/main.go` | Go packet injector (AF_PACKET) |
| `scripts/ws3/baseline-measure.sh` | Baseline measurement script |
| `scripts/ws3/delay-profile.sh` | Delay sweep profiler |
| `scripts/ws3/burst-profile.sh` | Burst injection profiler |
| `scripts/ws3/read-stats.py` | BPF STATS map reader |
| `scripts/ws3/analyze-delays.py` | Delay analysis tool |

---

*See also: [Doom over IPv6](doom-over-ipv6.md) | [Architecture](architecture.md) | [Bug Kill Chain](bug-kill-chain.md)*
