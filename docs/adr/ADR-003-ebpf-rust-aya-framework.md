# ADR-003: eBPF in Rust with Aya Framework (Over C)

## Status: Accepted

## Date: 2026-01-26

## Context

Unheaded's core value proposition is packet-level observability from L2 through L7. This requires eBPF programs running in kernel space at the XDP, TC, and kprobe attach points, plus a userspace collector that reads events from BPF ring buffers and forwards them to the Wotan message bus. The data plane must process every packet traversing the host network interface with less than 5% CPU overhead and sub-microsecond per-packet latency.

The eBPF programs and the trace-collector must be written in a systems language. The candidates were:

### Option A: C (traditional eBPF)

- The original and most widely documented language for eBPF.
- libbpf and BCC provide mature tooling.
- Used by Cilium, Falco, and most production eBPF deployments.
- However: manual memory management, no type safety, buffer overflows are the developer's problem, and the verifier error messages for C programs are notoriously cryptic.

### Option B: Rust with Aya Framework

- Aya is a pure-Rust eBPF library that does not depend on libbpf or BCC.
- Provides compile-time memory safety guarantees that complement the kernel verifier's runtime checks.
- Aya handles ELF loading, map creation, program attachment, and ring buffer reading.
- The same language (Rust) can be used for both eBPF programs (compiled to BPF bytecode via `rustc` + `bpf-linker`) and the userspace trace-collector.
- However: smaller ecosystem, fewer examples, and Aya is younger than libbpf.

### Option C: Go with cilium/ebpf

- Go is the primary language for the rest of the Unheaded control plane.
- cilium/ebpf is a mature Go library for loading and interacting with eBPF programs.
- However: eBPF programs themselves still need to be written in C; this only covers the userspace loader. Additionally, Go's garbage collector introduces latency spikes incompatible with the sub-microsecond requirements of the ring buffer reader.

## Decision

We write all eBPF programs and the trace-collector in **Rust using the Aya framework**. Specifically:

- **eBPF programs** (`ebpf/packet-marker/`, `ebpf/flow-tracker/`, `ebpf/latency-probe/`, `ebpf/syscall-tracer/`) are written in Rust, compiled to BPF bytecode via `aya-bpf` macros.
- **trace-collector** (`cmd/trace-collector/`) is a Rust binary that uses Aya to load eBPF programs, read ring buffers with zero-copy, and publish events to Wotan via gRPC.
- **Shared types** (`ebpf/common/src/lib.rs`) define structures like `TraceId` and `FlowKey` used by both eBPF programs and the userspace collector, ensuring type-safe communication across the kernel/userspace boundary.

The Go services in the control plane (Layer 2-5) remain in Go. Rust is used exclusively for the data plane (Layer 1) and its bridge to Layer 3.

## Consequences

### Positive

- **Memory safety in kernel space**: Rust's ownership model catches buffer overflows, use-after-free, and data races at compile time. Combined with the kernel verifier, this provides defense in depth -- the compiler catches what the verifier misses, and vice versa.
- **Single language for the data plane**: Both eBPF programs and the userspace collector are Rust, eliminating the C/Go impedance mismatch. Shared types in `ebpf/common/` prevent serialization bugs between kernel and userspace.
- **No libbpf dependency**: Aya is pure Rust with no C dependencies, simplifying the build toolchain and eliminating a class of linking issues. The eBPF programs compile to BPF bytecode directly via `bpf-linker`.
- **Performance**: Rust's zero-cost abstractions and lack of garbage collector make it ideal for the ring buffer reader in trace-collector, which must sustain sub-microsecond latency under high packet rates. Lock-free data structures and memory-mapped I/O are idiomatic Rust.
- **Future extensibility**: The `docs/RUST_COMPONENTS.md` roadmap identifies Wotan hot-path, custom metrics system, and network policy enforcement as future Rust rewrites. Establishing Rust expertise now prepares the team.

### Negative

- **Smaller ecosystem**: Aya has fewer examples and Stack Overflow answers than libbpf/BCC. Debugging novel issues requires reading Aya source code rather than finding existing solutions.
- **Two-language codebase**: The Go/Rust split means the team needs expertise in both languages. Build tooling must handle `cargo` alongside `go build`. The `Makefile` manages this with separate `make ebpf` and `make build` targets.
- **eBPF on Rust is newer**: Production deployments of Rust eBPF are less common than C eBPF. While Aya is actively maintained and used by projects like Bpfilter, it has not reached the battle-tested status of libbpf.
- **Linux-only**: eBPF programs require a Linux kernel 5.8+ environment. Development on macOS requires cross-compilation or a Linux VM. This constraint is now resolved with the WEST bare metal Linux environment, where eBPF programs are operational.
- **Verifier compatibility**: The BPF verifier was designed for C-generated bytecode. Rust-generated bytecode occasionally produces patterns that confuse older verifiers, though kernel 5.8+ handles this well.

## References

- `docs/RUST_COMPONENTS.md` -- Full Rust components roadmap
- `ebpf/` -- All eBPF program source (Rust + Aya)
- `cmd/trace-collector/` -- Rust trace-collector (Cargo project)
- `docs/ARCHITECTURE.md` -- Layer 1: Data Plane specification
- Aya framework: https://aya-rs.dev/
