# Doom-over-IPv6 Performance Tuning (WS3)

## Executive Summary

WS3 targets 15+ fps sustained Doom gameplay by optimizing the packet injection
pipeline. Three hypotheses guide the optimization:

| Hypothesis | Description | Validation |
|-----------|-------------|------------|
| **H1 (Timing)** | The 3ms inter-packet delay compensates for Python overhead, not XDP hard limits | Phase 3: Delay profiling |
| **H2 (Burst)** | Netflix-model burst injection yields 2-3x throughput vs steady-state | Phase 4: Burst testing |
| **H3 (Injector)** | A Go injector eliminates Python socket.send() bottleneck (10x+ improvement) | Phase 5: Go vs Python |

## Architecture

```
  Python/Go Injector
       |
       v  AF_PACKET raw socket
   [veth01]  (monad0 namespace)
       |
       v  XDP_TX forwarding
   [monad0] -> [monad1] -> [monad2] -> [monad3] -> [monad4] -> [monad5]
       ^                                                           |
       |___________________________________________________________|
                          Packet circulation ring

   Each hop: monad_cpu XDP program executes 128 MBC instructions
   Full ring: 128 * 255 bounces / 8 = 4080 instructions per packet
```

## Baseline (Pre-WS3)

| Metric | Value |
|--------|-------|
| Injection rate | ~333 pps (3000us delay) |
| Internal FPS | ~6 fps |
| Insns/packet | ~4,080 |
| Insns/frame | ~1,470,000 |
| Insns/sec | ~1.35M |
| WebSocket FPS | ~30 fps (frame repeats) |
| XDP capacity (est.) | ~10,000,000 pps |
| Our utilization | 0.003% of XDP capacity |

**Key insight:** We use 0.003% of estimated XDP capacity. The bottleneck is
always userspace injection, never the kernel.

## Tooling

### Scripts

| Script | Phase | Purpose |
|--------|-------|---------|
| `scripts/ws3/baseline-measure.sh` | 1 | Baseline measurement and documentation |
| `scripts/ws3/read-stats.py` | 1,2 | Read STATS map counters (standard + bounce timestamps) |
| `scripts/ws3/delay-profile.sh` | 3 | Sweep delays from 3000us to 500us |
| `scripts/ws3/analyze-delays.py` | 3 | Parse delay test results, identify threshold |
| `scripts/ws3/burst-profile.sh` | 4 | Test burst injection at various batch sizes |
| `scripts/ws3/compare-injectors.sh` | 5 | Python vs Go injector throughput comparison |
| `scripts/ws3/sustained-test.sh` | 6 | 60-second sustained playability test |

### Go Injector

Location: `cmd/doom-go-injector/main.go`

The Go injector replaces Python's inject.py for high-throughput packet injection.
It uses AF_PACKET raw sockets via `golang.org/x/sys/unix` for zero-copy sending.

**Build:**
```bash
go build -o /tmp/doom-go-injector ./cmd/doom-go-injector/
```

**Usage:**
```bash
# Burst mode (recommended for production)
sudo ip netns exec monad0 /tmp/doom-go-injector \
    --count 10000 --mode burst --batch 100 --iface veth01

# Steady mode (for comparison with Python baseline)
sudo ip netns exec monad0 /tmp/doom-go-injector \
    --count 1000 --mode steady --delay 1000 --iface veth01

# Maximum throughput (testing only)
sudo ip netns exec monad0 /tmp/doom-go-injector \
    --count 50000 --mode fast --iface veth01
```

**CLI flags:**

| Flag | Default | Description |
|------|---------|-------------|
| `--count` | 1000 | Number of packets to inject |
| `--mode` | burst | Injection mode: `steady`, `burst`, `fast` |
| `--batch` | 100 | Packets per burst (burst mode only) |
| `--delay` | 3000 | Inter-packet delay in microseconds (steady mode) |
| `--iface` | veth01 | Network interface |
| `--flow-label` | 0xDE | IPv6 flow label (identifies Doom instance) |
| `--src-mac` | 02:42:ac:11:00:02 | Source MAC address |
| `--dst-mac` | 02:42:ac:11:00:03 | Destination MAC address |

## Phase Results

### Phase 1: Baseline Measurement

Run `scripts/ws3/baseline-measure.sh` to establish current-state metrics.

### Phase 2: BPF Timestamp Instrumentation

Design documented in `scripts/ws3/bounce-timestamps-design.md`. Adds STATS map
keys 11-15 for nanosecond-precision bounce cycle measurement.

### Phase 3: Delay Profiling

Run `scripts/ws3/delay-profile.sh` to sweep delays from 3000us to 500us.
Analyze with `scripts/ws3/analyze-delays.py`.

**Expected results table:**

| Delay (us) | PPS | Est. FPS | Status |
|-----------|-----|----------|--------|
| 3000 | ~333 | ~6.7 | Baseline |
| 2000 | ~500 | ~10 | TBD |
| 1500 | ~667 | ~13 | TBD |
| 1000 | ~1000 | ~20 | TBD |
| 750 | ~1333 | ~27 | TBD |
| 500 | ~2000 | ~40 | TBD |

### Phase 4: Burst Injection

Run `scripts/ws3/burst-profile.sh` to test Netflix-model burst injection.

### Phase 5: Go vs Python Injector

Run `scripts/ws3/compare-injectors.sh` after building the Go injector.

### Phase 6: Sustained Playability

Run `scripts/ws3/sustained-test.sh` for the final 60-second validation.

## Netflix PPS Comparison

| Scenario | PPS | Notes |
|----------|-----|-------|
| Home router max | 50,000-100,000 | Multiple 4K streams |
| Netflix 4K steady | 1,500 | One viewer, constant bitrate |
| Netflix 4K + ACKs | 2,250 | Bidirectional total |
| Doom baseline (3ms) | ~333 | Current single-stream |
| Doom target (burst) | 1,000+ | Phase 4 result |
| Doom aggressive (500us) | 2,000+ | Phase 3 limit test |
| XDP hardware max (est.) | 10,000,000 | Never tested |

## Future Optimization Paths

1. **sendmmsg():** Batch multiple packets per syscall (Go or C injector)
2. **TPACKET_V3:** mmap-based zero-copy ring buffer injection
3. **Multi-hop parallelism:** Inject on multiple ring entry points
4. **Adaptive injection:** Dynamically adjust delay based on STATS feedback
5. **XDP_REDIRECT:** Bypass TCP/IP stack entirely for inter-namespace forwarding
