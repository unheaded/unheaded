# eBPF Programs

Rust/Aya packet tracing programs running at XDP and TC layers.

| Program | Purpose |
|---------|---------|
| packet_marker | Trace ID injection at XDP |
| flow_tracker | Connection tracking |
| latency_probe | RTT measurement |

23,991 LOC Rust across 8 eBPF programs.

---

> **Source:** [ebpf/](../ebpf/) · [docs/RUST_COMPONENTS.md](../docs/RUST_COMPONENTS.md)
