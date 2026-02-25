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

*The Whispering Void speaks in Rust. The control plane orchestrates in Go.*
*The Kingdom rises with both.*

⚔️🦀🛡️
