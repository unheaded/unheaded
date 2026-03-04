# Doom in the Data Plane: Observing Every Packet with eBPF

## Abstract

Traditional observability samples traffic. We observe EVERY packet. Unheaded is a 49-service infrastructure platform where eBPF XDP/TC programs stamp packet metadata, track flows, and measure latency — all with zero context switches in the hot path.

Our wild claim: we've proven eBPF is computationally complete by running Doom inside the kernel. If our kernel can render a game, it can observe your infrastructure.

## What You'll Learn

- **Monad wire format**: 20-byte packet metadata with CRC-16 validation (version 0x01, FROZEN)
- **XDP pipeline**: IPv6 flow label stamping for per-packet trace_id injection
- **TC programs**: Stateful connection tracking with BPF hash maps
- **kprobe instrumentation**: Kernel-level RTT measurement without userspace overhead
- **Go-Rust FFI**: AF_XDP zero-copy packet processing bridging Aya (Rust) and cilium/ebpf (Go)
- **Production deployment**: Bare metal cross-host services with systemd, mTLS, and pure-stdlib JWT auth

## Live Demo

We'll deploy our full stack across two bare metal servers and demonstrate:
- Real-time packet flow visualization on a custom dashboard
- trace-collector bridging eBPF maps to Wotan message bus
- 100+ concurrent request tracing with zero packet drops
- Cross-host service communication (WEST → EAST over point-to-point link)

## The Numbers

| Metric | Value |
|--------|-------|
| Total LOC | 719,632 |
| Go services | 26 binaries + 23 service integrations |
| eBPF crates | 15 (Rust/Aya) |
| Auth coverage | 90.8% (pure stdlib JWT) |
| Build health | ZERO errors (Go + Rust) |
| Test suite | ZERO failures (race detector enabled) |
| Bare metal hosts | 2 (WEST dev + EAST staging) |

## Speaker Bio

Steven Bellis — Building infrastructure automation that traces every packet from kernel to dashboard. Creator of the Monad wire protocol and the Wotan message bus. Believer that if you can't observe it, you can't trust it.

## Technical Requirements

- Projector with HDMI
- Two terminal windows visible
- Network access between demo machines (or pre-recorded fallback)
- 20 minutes + 5 min Q&A
