# eBPF Programs

eBPF-based packet-level observability from L2 to L7. XDP ingress, TC egress, kprobe/tracepoint instrumentation.

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

## Not tracing ourselves

Two loops of the tracer watching its own output, both cut:

- **Dashboard viewers** (2026-10-18): each WebSocket frame to a browser is a packet the tracer reports, sent on as another frame. dashboard-backend drops packets and flows of its own live viewer connections, in batches too (`traceRecords`, 2026-10-23).
- **The collector's own publishes** (2026-10-23): every batch `trace-collector-go` sends to Wotan is packets it reports in the next batch. It now reads its own TCP sockets from `/proc/self` (fd inodes, `net/tcp`, `net/tcp6`) every 5 s and leaves out packets and flows with one of them as an endpoint; `trace_collector_self_dropped_total` counts them. On east that day the whole packet stream was loopback traffic to Wotan's 18000/18001.

Other services' traffic to Wotan is still traced: it is real Kingdom traffic.

## Licensing

- **Aya/aya-ebpf**: MIT / Apache-2.0
- **cilium/ebpf**: Apache-2.0
- **doomgeneric**: GPL-2.0 (isolated in `doom/doomgeneric/`, does not link Unheaded code)

See [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md) for full attribution.

---

> **Source:** [ebpf/](../ebpf/) · [docs/RUST_COMPONENTS.md](../docs/architecture/RUST_COMPONENTS.md) · [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md)
