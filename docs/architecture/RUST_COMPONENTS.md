# Unheaded Kingdom - Rust Components Roadmap

## The Whispering Void - eBPF Layer (MUST BE RUST)

**Rationale:** Go is too slow for packet-path processing. eBPF programs run in kernel space and the userspace collector must be ultra-lean to not lag data flows.

### Components to Build in Rust

#### 1. eBPF Programs (Rust + Aya framework or raw)
- `packet_marker.bpf` - XDP layer trace ID injection
- `flow_tracker.bpf` - Connection tracking via TC
- `latency_probe.bpf` - RTT measurement via kprobes
- `syscall_tracer.bpf` - Security audit via raw tracepoints
- `container_events.bpf` - Container lifecycle via tracepoints

#### 2. trace-collector (Rust binary)
**Location:** `cmd/trace-collector/` (already exists as Cargo project)
**Purpose:** Bridge from kernel eBPF to Fae Chamber (Wotan)

Features:
- Ring buffer reading with zero-copy
- Perf event array processing
- Direct publishing to Wotan via gRPC
- Sub-microsecond latency
- Memory-mapped I/O
- Lock-free data structures

#### 3. packet-processor (Rust library)
**Purpose:** Parse packet headers at wire speed

Features:
- Zero-copy packet parsing
- SIMD-accelerated where possible
- Protocol dissection (Ethernet, IP, TCP, UDP, QUIC, HTTP/3)
- Flow state machine

---

## Future Rust Rewrites (Post-Alpha)

### Observability Stack (Replace Prometheus)
**Current:** Using prometheus/client_golang for alpha PoC
**Future:** Kingdom's own metrics system in Rust

- `unheaded-metrics` - Time-series database
- `unheaded-scraper` - Metrics collection
- `unheaded-query` - PromQL-compatible query engine

### High-Performance Data Path
- Message bus hot path (Wotan core) - consider Rust rewrite
- Network policy enforcement (XDP/TC programs)
- TLS termination (if we do our own)

---

## Integration Points

### Go Services (Control Plane)
The Go services remain for:
- REST/gRPC APIs
- Business logic
- Orchestration
- State management

### Rust Components (Data Plane)
Rust handles:
- Kernel interaction (eBPF)
- Packet processing
- High-frequency metrics
- Performance-critical paths

### Communication
- Go ↔ Rust via gRPC (protobuf)
- Rust ↔ Kernel via BPF maps and ring buffers
- Shared memory for ultra-low-latency (future)

---

## Optional Integrations (User Choice)

These are **optional** - users can enable if they want:
- Cilium CNI integration
- Prometheus remote write
- OpenTelemetry export
- Jaeger tracing export

The Kingdom works standalone without any of these.

---

## Build Strategy

### Phase 1: Alpha (Current)
- Go services with Prometheus metrics (acceptable for PoC)
- Rust trace-collector skeleton
- Mock eBPF integration

### Phase 2: Beta
- Full Rust eBPF programs
- Real trace-collector
- Begin metrics system design

### Phase 3: MVP
- Kingdom's own observability
- Full Rust data plane
- Production-ready eBPF

---

## AF_XDP Crates (Zero-Copy Packet I/O)

The AF_XDP subsystem provides kernel-bypass packet delivery via Linux
AF_XDP sockets.  All crates live under `ebpf/` and follow the Sacred Law:
**no external dependencies** in the userspace path (Rust std + raw syscalls only).

### `af-xdp-common` -- Shared Types

**Location:** `ebpf/af-xdp-common/`
**Target:** `no_std` compatible (compiles for both `bpfel-unknown-none` and host)
**License:** GPL-2.0-only

Shared type definitions that cross the kernel/userspace ABI boundary:

| Type               | Size   | Purpose                                         |
|--------------------|--------|-------------------------------------------------|
| `XskDesc`          | 16 B   | RX/TX ring descriptor (addr + len + options)     |
| `XskConfig`        | 16 B   | User-facing UMEM configuration                  |
| `XskUmemReg`       | 32 B   | Kernel UMEM registration (setsockopt)            |
| `XskRingOffsets`   | 32 B   | Per-ring mmap layout offsets                     |
| `XskMmapOffsets`   | 128 B  | All four ring offsets from getsockopt            |
| `XskStatistics`    | 48 B   | Socket-level statistics                          |
| `Sockaddr_xdp`     | 16 B   | AF_XDP bind address                              |
| `FillDesc`         | 8 B    | Fill ring entry (frame address)                  |
| `CompletionDesc`   | 8 B    | Completion ring entry (frame address)            |
| `XdpRedirectConfig`| 4 B    | Per-queue redirect config (enable + filter)      |
| `XdpRedirectStats` | 24 B   | Per-queue redirect counters                      |

Also exports all AF_XDP constants: `AF_XDP` (44), `SOL_XDP` (283),
socket option codes, bind flags, XDP actions, mmap page offsets,
and default ring/frame sizes.

All struct sizes are verified by compile-time `const_assert_size!` macros.

### `af-xdp` -- Userspace Engine

**Location:** `ebpf/af-xdp/`
**Target:** `x86_64-unknown-linux-gnu` (host only, std required)
**License:** GPL-2.0-only
**Crate types:** lib, cdylib, staticlib

The userspace AF_XDP engine with zero external dependencies:

| Module     | Purpose                                                    |
|------------|------------------------------------------------------------|
| `syscall`  | Raw Linux syscalls via inline asm (mmap, socket, bind, setsockopt, getsockopt, sendto, poll, close, ioctl, epoll) |
| `umem`     | UMEM allocator (mmap + free-list), FillRing, CompletionRing |
| `xsk`      | XskSocket (RX/TX rings + fill/completion + bind lifecycle)  |
| `ring`     | Generic SPSC lock-free ring buffer (Ring<T>)               |
| `engine`   | XdpEngine (burst RX/TX), EventLoop (epoll), SignalHandler  |
| `ffi`      | C-compatible FFI: `afxdp_create`, `afxdp_recv`, `afxdp_send`, `afxdp_stats`, `afxdp_poll`, `afxdp_destroy` |

The FFI layer produces `libaf_xdp.a` (static) and `libaf_xdp.so` (dynamic)
for consumption by the Go bridge via CGo.  The C header is `af_xdp.h`.

### `xdp-redirect` -- XDP Redirect Program

**Location:** `ebpf/xdp-redirect/`
**Target:** `bpfel-unknown-none` (kernel eBPF)
**License:** GPL-2.0-only

Standalone XDP program that steers packets to AF_XDP sockets:

- **Maps:** `XSKS` (XSKMAP), `CONFIG` (per-queue enable/filter), `STATS` (counters)
- **Logic:** Parse ETH+IPv4, check per-queue CONFIG, redirect via XSKMAP
- **Fallback:** XDP_PASS if redirect disabled or no socket bound

### `shield-ebpf` -- Kingdom Boundary with AF_XDP Dual-Path

**Location:** `ebpf/shield-ebpf/`
**Target:** `bpfel-unknown-none` (kernel eBPF)
**License:** GPL-2.0-only

Two programs in one binary:

- **shield_xdp** (XDP ingress): BIRTH stamping + optional AF_XDP redirect
- **shield_tc** (TC egress): DEATH capture + HBH header stripping

AF_XDP integration maps:
- `SHIELD_XSKS` -- XSKMAP for zero-copy redirect after BIRTH
- `SHIELD_CONFIG` -- Key 0 bit 0 toggles AF_XDP redirect

### `packet-marker` -- Trace ID with Selective AF_XDP

**Location:** `ebpf/packet-marker/`
**Target:** `bpfel-unknown-none` (kernel eBPF)
**License:** GPL-2.0-only

XDP program for distributed trace propagation with AF_XDP integration:

- **Maps:** `MARKER_XSKS` (XSKMAP), `MARKER_CONFIG` (enable toggle),
  `FLOW_STATE`, `TRACE_INJECT`, `PACKET_EVENTS`, `STATS`
- **Logic:** Trace-marked packets redirect to AF_XDP; unmarked packets
  pass to kernel stack
- **Selective:** Only packets with non-zero trace_id are redirected

### `monad-common` -- Protocol Types with AF_XDP Constants

**Location:** `ebpf/monad-common/`
**Target:** `no_std` compatible (bpfel + host)
**License:** GPL-2.0-only

Monad wire format types (20-byte register file, frozen at v0x01).
AF_XDP-relevant exports:

- `redirect_action::NO_REDIRECT` (0) -- standard kernel path
- `redirect_action::AF_XDP` (1) -- redirected to AF_XDP socket
- `redirect_action::KERNEL_STACK` (2) -- AF_XDP enabled but no socket bound
- `flow_flags::TRACED_AF_XDP` (0x10) -- flow eligible for AF_XDP redirect

---

*The Whispering Void speaks in Rust. The control plane orchestrates in Go.*
*The Kingdom rises with both.*
