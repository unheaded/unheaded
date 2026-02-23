# eBPF Programs

The Whispering Void — eBPF-based observability from L2 to L7.

## Toolchain

| Layer | Tool | Role |
|-------|------|------|
| Kernel programs | **Aya** (Rust) | XDP/TC/kprobe/tracepoint eBPF programs |
| Userspace loader | **Aya** (Rust) | Program loading, map pinning, ring buffer reading |
| Go userspace | **cilium/ebpf** (Go) | BPF map operations, program attachment from Go services |
| Host library | **libbpf** | System-level BPF support (bpftool, BTF) |

## Programs

| Program | Layer | Purpose |
|---------|-------|---------|
| packet_marker | XDP | Trace ID injection at ingress |
| flow_tracker | TC | Connection tracking |
| latency_probe | kprobe | RTT measurement |
| syscall_tracer | tracepoint | Syscall auditing |
| shield-ebpf | XDP/TC | Protocol Shield — Monad validation |
| yaldabaoth-ebpf | XDP | Monad CPU — computational execution engine |
| monad-cpu-ebpf | XDP | Doom-over-IPv6 substrate (Section 12 proof) |

23,991 LOC Rust across 8 eBPF programs.

## Licensing

- **Aya/aya-ebpf**: MIT / Apache-2.0
- **cilium/ebpf**: Apache-2.0
- **doomgeneric**: GPL-2.0 (isolated in `doom/doomgeneric/`, does not link Unheaded code)

See [[Third Party Licenses|Third-Party-Licenses]] for full attribution.

---

> **Source:** [ebpf/](../ebpf/) · [docs/RUST_COMPONENTS.md](../docs/RUST_COMPONENTS.md) · [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md)
