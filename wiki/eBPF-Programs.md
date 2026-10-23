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

Workspace members of `ebpf/Cargo.toml` (GPL-2.0-only):

| Program | Hook | Purpose |
|---------|------|---------|
| `packet-marker` | XDP | Trace ID injection at ingress |
| `flow-tracker` | TC | Connection tracking |
| `latency-probe` | kprobe / kretprobe | TCP latency (RTT) |
| `syscall-tracer` | raw tracepoint | Syscall auditing |
| `shield-ebpf` | XDP + TC | Protocol Shield: Monad stamp/validate at the boundary |
| `hop-ebpf` | XDP | Per-hop Monad processing |
| `yaldabaoth-ebpf` | TC egress | Chaos: corrupt, delay, duplicate or truncate registered flows |
| `monad-cpu-ebpf` | XDP | The UPC: MBC interpreter (Doom, xv6) |
| `firewall-ebpf`, `maglev-ebpf`, `canary-ebpf`, `qos-ebpf`, `failover-ebpf`, `compliance-ebpf`, `nfv-ebpf`, `version-ebpf`, `anomaly-ebpf` | XDP | Monad CPU applications (apps 03-12) |
| `xdp-redirect` | XDP | AF_XDP redirect (with userspace `af-xdp`) |

Shared crates: `common`, `monad-common`, `pqc-common`, `af-xdp-common`. `trace-collector-go` loads packet-marker, flow-tracker and latency-probe on west and east ([[Systemd Units|Systemd-Units]]).

## Not tracing ourselves

Two loops of the tracer watching its own output, both cut:

- **Dashboard viewers** (2026-10-18): each WebSocket frame to a browser is a packet the tracer reports, sent on as another frame. dashboard-backend drops packets and flows of its own live viewer connections, in batches too (`traceRecords`, 2026-10-23).
- **The collector's own publishes** (2026-10-23): every batch `trace-collector-go` sends to Wotan is packets it reports in the next batch. It now reads its own TCP sockets from `/proc/self` (fd inodes, `net/tcp`, `net/tcp6`) every 5 s and leaves out packets and flows with one of them as an endpoint; `trace_collector_self_dropped_total` counts them. On east that day the whole packet stream was loopback traffic to Wotan's 18000/18001.

Other services' traffic to Wotan is still traced: it is real Kingdom traffic.

## Trace topics: shapes, clocks, units

`trace-collector-go --unified` publishes every topic as a JSON array (a batch). Audited end to end 2026-10-23 against east's live payloads.

| Topic | Record | `timestamp_ns` clock |
|-------|--------|----------------------|
| `traces.packet` | `trace_id` (32 hex), `src_ip`/`dst_ip`, `src_port`/`dst_port` (host order), `protocol` name, `packet_len` | ktime |
| `ebpf.packet.events` | `trace_id {high,low}`, `flow_key {src_addr,dst_addr,src_port,dst_port,protocol number}`, `packet_len`, `action`, `direction` | ktime |
| `traces.flow`, `ebpf.flow.events` | `flow_key`, `flow_state {trace_id {high,low}, start_ns, last_seen_ns, packets_*, bytes_*, state}`, `event_type` (new/state_change/expired) | ktime (all three) |
| `traces.latency`, `ebpf.latency.events` | `latency_ns`, `min_rtt_ns`, `max_rtt_ns`, `samples`, `operation` | wall clock |
| `traces.correlated` | `forward`/`reverse {five_tuple {SrcIP,DstIP: [16]byte}, first_seen_ns, ...}`, `rtt_ns` | ktime |

- **ktime** is `bpf_ktime_get_ns()`, CLOCK_MONOTONIC: ns since boot, not a date. Ages in the collector use `ktimeNow()` on the same clock; the dashboard's `observedAt` uses a timestamp only when it reads as recent wall time, else the arrival time.
- **trace_id is all zeros** on every packet and flow: packet_marker's IP/TCP option extraction is disabled (verifier, `TODO(E1)`) and nothing writes `TRACE_INJECT`. The dashboard treats all zeros as no trace ID.
- **flow_key from flow_tracker is normalized** (both directions share one key, ordered by address then port), not client to server; `packets_out` counts src to dst. The dashboard's flow graph keys packets by their own direction, so one connection can appear under both orders.
- **Correlated RTT** is first response minus first request of a 5-tuple pair, emitted once per pair. packet_marker sees ingress only, so a pair matches only where both directions arrive on the traced interface (loopback on east).
- **Latency** is the time spent inside `tcp_sendmsg`/`tcp_recvmsg`/`tcp_connect` (kprobe to kretprobe), not a network RTT. latency-probe's `LATENCY_MAP` is keyed by (trace ID, pid & 0xffff, tid & 0xffff), with send, recv and connect merged under one key, so the collector cannot tell which operation a sample came from (it labels all `tcp_send`) and publishes no `flow_key` for it.

## Licensing

- **Aya/aya-ebpf**: MIT / Apache-2.0
- **cilium/ebpf**: Apache-2.0
- **Doom engine** (id Software linuxdoom-1.10): GPL-2.0, compiled from a checkout outside the repo by `demos/doom/Makefile` into `doom.elf` for the UPC ([[SBOM]])

See [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md) for full attribution.

---

> **Source:** [ebpf/](../ebpf/) · [docs/RUST_COMPONENTS.md](../docs/architecture/RUST_COMPONENTS.md) · [LICENSES/THIRD_PARTY.md](../LICENSES/THIRD_PARTY.md)
