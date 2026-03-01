# Unheaded Kingdom -- eBPF Subsystem

**License:** GPL-2.0-only
**Updated:** 2026-03-01

---

## Overview

The eBPF subsystem is the data plane of the Unheaded Kingdom.  It provides
wire-speed packet processing, distributed tracing, and observability via
Linux eBPF programs written in Rust (Aya framework) with a pure-Rust
userspace engine.

All eBPF programs run at the XDP (eXpress Data Path) or TC (Traffic Control)
hook points, processing packets before they reach the kernel networking
stack.

---

## Crate Map

```
ebpf/
+-- af-xdp-common/       Shared AF_XDP types (no_std, kernel+userspace ABI)
+-- af-xdp/              Userspace AF_XDP engine (zero-copy packet I/O)
+-- xdp-redirect/         XDP program: AF_XDP packet steering via XSKMAP
+-- shield-ebpf/          XDP+TC program: Kingdom boundary (BIRTH/DEATH)
+-- packet-marker/        XDP program: Distributed trace ID propagation
+-- monad-common/         Monad wire format types (no_std, 20-byte register)
+-- common/               Shared eBPF helper types
+-- flow-tracker/         TC program: Connection tracking
+-- latency-probe/        Kprobe program: RTT measurement
+-- hop-ebpf/             Hop-by-Hop processing programs
+-- monad-cpu-ebpf/       Per-CPU Monad computation programs
+-- syscall-tracer/       Raw tracepoint: Syscall auditing
+-- yaldabaoth-ebpf/      Experimental: Demiurge boundary enforcement
+-- fuzz/                 Fuzzing harnesses
+-- trace-collector-go/   Go bridge for trace-collector integration
+-- Cargo.toml            Workspace root
+-- Cargo.lock            Locked dependencies
+-- rust-toolchain.toml   Rust nightly toolchain pinning
```

---

## AF_XDP: Zero-Copy Packet I/O

AF_XDP provides kernel-bypass packet delivery directly to userspace memory,
enabling line-rate packet processing without kernel stack overhead.

### Quick Start

```bash
# Build the userspace engine (produces lib + staticlib + cdylib)
cd ebpf/af-xdp
cargo build --release

# Run unit tests (no root required)
cargo test

# Run all tests including AF_XDP socket tests (requires root)
sudo cargo test -- --include-ignored
```

### Architecture

AF_XDP is integrated into two Kingdom eBPF programs:

- **shield-ebpf**: After BIRTH stamping, redirects Monad-carrying packets
  to `SHIELD_XSKS` for zero-copy userspace delivery
- **packet-marker**: After trace ID extraction, selectively redirects
  traced packets to `MARKER_XSKS`

Both support a dual-path architecture: AF_XDP when enabled and a socket
is bound, falling back to the kernel stack when not.

```
NIC -> XDP Hook -> shield-ebpf (BIRTH) -> AF_XDP redirect -> UMEM -> Userspace
                                       \-> XDP_PASS -> Kernel stack (fallback)
```

### Enabling AF_XDP

AF_XDP is disabled by default.  Enable via BPF map writes:

```bash
# For shield-ebpf: set SHIELD_CONFIG[0] bit 0 = 1
sudo bpftool map update pinned /sys/fs/bpf/unheaded/shield/SHIELD_CONFIG \
    key 0 0 0 0 value 1 0 0 0

# For packet-marker: set MARKER_CONFIG[0] bit 0 = 1
sudo bpftool map update pinned /sys/fs/bpf/unheaded/packet_marker/MARKER_CONFIG \
    key 0 0 0 0 value 1 0 0 0
```

### Documentation

- [AF_XDP Architecture](../docs/architecture/AF_XDP_ARCHITECTURE.md) --
  Data flow, UMEM layout, ring topology, XSKMAP pinning, kernel requirements,
  thread safety
- [Deployment Guide](../docs/DEPLOYMENT_GUIDE_AFXDP.md) -- Kernel config,
  hugepages, permissions, loading sequence, troubleshooting
- [Testing Guide](../docs/TESTING_AFXDP.md) -- Test organization, CI/CD
- [Migration Guide](../docs/MIGRATION_AFXDP.md) -- For existing shield/marker users
- [Changelog](../docs/CHANGELOG_AFXDP.md) -- Phase-by-phase changes

---

## Building

### Prerequisites

- Rust nightly toolchain (pinned in `rust-toolchain.toml`)
- `bpfel-unknown-none` target for eBPF programs
- `x86_64-unknown-linux-gnu` target for userspace code
- Linux headers (for BPF/XDP constants)

### Build Commands

```bash
# Build all crates (host + eBPF targets)
cd ebpf
cargo build

# Build userspace AF_XDP engine only
cd ebpf/af-xdp
cargo build --release

# Build eBPF programs (requires bpfel target)
cd ebpf/shield-ebpf
cargo build --target bpfel-unknown-none --release

cd ebpf/xdp-redirect
cargo build --target bpfel-unknown-none --release

cd ebpf/packet-marker
cargo build --target bpfel-unknown-none --release
```

### Running Tests

```bash
# All unit tests (no root)
cd ebpf/af-xdp && cargo test

# Common types tests
cd ebpf/af-xdp-common && cargo test

# Monad protocol types tests
cd ebpf/monad-common && cargo test

# Integration tests (requires root)
cd ebpf/af-xdp && sudo cargo test -- --include-ignored
```

---

## Design Principles

1. **Sacred Law:** No external dependencies in the userspace AF_XDP path.
   Rust std only.  All kernel interaction via raw syscalls through
   `std::arch::asm!`.

2. **Zero-Copy:** Packet data is never copied between kernel and userspace.
   UMEM is mmap'd shared memory.

3. **Lock-Free:** All ring buffers are SPSC (single-producer single-consumer)
   with atomic index updates.  No mutexes in the hot path.

4. **Dual-Path:** Every AF_XDP integration supports graceful fallback to
   the kernel stack.  No packet loss when AF_XDP is disabled or no socket
   is bound.

5. **Runtime Toggle:** AF_XDP can be enabled/disabled via BPF map writes
   without reloading eBPF programs.
