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

# Build eBPF programs (requires bpfel target)
cd ebpf/shield-ebpf
cargo build --target bpfel-unknown-none --release

cd ebpf/packet-marker
cargo build --target bpfel-unknown-none --release
```

### Running Tests

```bash
# Monad protocol types tests
cd ebpf/monad-common && cargo test
```

---
