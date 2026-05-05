# Anamnesis Lite — Packet-Zero APM

**License:** GPL-3.0 (component-uniform)
**Status:** packet-flow pipeline operational on WEST + EAST; cross-host BPF flow graph live

## What it is

Anamnesis Lite is a drop-in **APM appliance** that traces every packet from
XDP (kernel ingress) through the application layer and back, using eBPF
markers and the Monad wire format. Stack adopters get distributed-trace
visibility comparable to Datadog APM / Honeycomb / Lightstep, without the
SaaS dependency, vendor lock-in, or subscription.

In Norse mythology / Gnostic cosmology, Anamnesis is *recollection of what
came before* — the substrate that lets a soul remember. The APM tool is
the substrate that lets your services remember what packet caused what
behaviour.

## Who it's for

- Anyone running infrastructure who wants distributed tracing without
  shipping data to a vendor.
- Compliance shops that need SOC2 CC7.2 (system-monitoring) or PCI 10.1
  (audit logging) evidence and don't want to pay for a SaaS APM.
- Teams who care about packet-zero visibility (the trace begins at XDP, not
  at the first instrumented HTTP middleware).

## What's in the box

| Binary / Service | What it does |
|---|---|
| `trace-collector-go` | unified BPF-trace collector (Go); reads ring-buffer events, decodes Monad markers, publishes to Wotan |
| `ebpf-collector` | Rust collector variant (uses aya for richer kernel-side semantics) |
| `ebpf-exporter` | Prometheus exporter for BPF map metrics |
| `services/anamnesis/` | event-history service — replayable timeline of what happened |
| `pkg/anamnesis/` | Suricata IDS integration (optional — for adopters who want IDS events in the same trace timeline) |
| `dashboard-backend` | aggregated metrics + WebSocket streaming (optional UI; the trace data is usable headless) |

Plus three eBPF programs (Rust, via Aya):

- `ebpf/packet-marker/` — XDP layer trace-ID injection
- `ebpf/flow-tracker/` — TC layer connection tracking
- `ebpf/latency-probe/` — kprobe RTT measurement

Note (2026-05-04): `scripts/load-ebpf.sh` works against modern aya targets;
the existing 0.1.1 BPF crates need an upgrade pass to load against
`bpftool 7.7+ / libbpf 1.7+` (tracked as kanban `ebpf-aya-upgrade-mn05`).
The Go-side `trace-collector-go` works as-is against the ring buffers when
they're populated.

## Differentiator vs commercial APM

| | Anamnesis Lite | Datadog APM / New Relic / Honeycomb |
|---|---|---|
| License | GPL-3.0, free, redistributable | proprietary SaaS, per-host pricing |
| Trace start point | XDP (packet-zero, kernel ingress) | first instrumented HTTP middleware (lossy at L2-L4) |
| Wire format | Monad / RFC-aligned IPv6 HbH (RFC 8200) | proprietary or OpenTelemetry-over-HTTPS |
| Storage | local Wotan + optional PostgreSQL — your data stays on your hosts | vendor cloud — your data leaves your network |
| Sidecar overhead | none (eBPF, ring buffer) | Envoy / Datadog agent / OTel collector |
| Sampling | configurable per-trace; default 100% sampled | typically 1-10% sampled at scale |

## Build + adopter quickstart

See `BUILD.md`. Short version:

```bash
# Go components
go build -o bin/trace-collector-go ./cmd/trace-collector-go/
go build -o bin/dashboard-backend  ./cmd/dashboard-backend/
go build -o bin/ebpf-exporter      ./cmd/ebpf-exporter/

# Rust components (when aya upgrade lands)
cd cmd/ebpf-collector && cargo build --release

# eBPF programs (after aya upgrade per kanban ebpf-aya-upgrade-mn05)
cd ebpf && cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release \
    -p packet-marker -p flow-tracker -p latency-probe
```

## See also

- `cmd/tools/README.md` — curation pattern these tools share
- `docs/adr/ADR-005-wotan-message-backbone.md` — the message bus this tool
  publishes traces onto
- `services/anamnesis/anamnesis.go` — the event-history substrate
- `wiki/Wave-10C-Backprop.md` and adjacent — the development history of the
  trace pipeline
