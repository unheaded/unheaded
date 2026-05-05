# Anamnesis Lite Components — in-tree source inventory

## Userspace binaries

| Path | Purpose |
|---|---|
| `cmd/trace-collector-go/main.go` | Go BPF-trace collector; reads ring-buffer events, decodes Monad markers, publishes to Wotan or stdout |
| `cmd/ebpf-collector/` | Rust trace-collector variant (aya-based, richer kernel-side semantics) |
| `cmd/ebpf-exporter/main.go` | Prometheus exporter for BPF map metrics |
| `cmd/dashboard-backend/` | aggregated metrics + WebSocket streaming UI backend |

## Library packages

| Path | Purpose |
|---|---|
| `services/anamnesis/anamnesis.go` | event-history service — replayable timeline of what happened |
| `pkg/anamnesis/suricata.go` | optional Suricata IDS integration (IDS events in the same trace timeline) |
| `pkg/ebpf/maploader/` | BPF map management library |
| `pkg/ebpf/loader.go` | BPF program loader (Go) |

## eBPF programs (Rust, via Aya)

| Path | Attachment | Purpose |
|---|---|---|
| `ebpf/packet-marker/` | XDP | inject Monad trace IDs at L2 |
| `ebpf/flow-tracker/` | TC | connection state tracking at L3-L4 |
| `ebpf/latency-probe/` | kprobe | RTT measurement on `tcp_v4_connect` |
| `ebpf/syscall-tracer/` | raw_tracepoint | syscall timeline |

## Configuration + ops

| Path | Purpose |
|---|---|
| `configs/anamnesis.yaml` | trace collector config |
| `configs/wotan.yaml` | topic-allowlist (anamnesis publishes here) |
| `scripts/load-ebpf.sh` | adopter-side BPF program loader |
| `scripts/unload-ebpf.sh` | clean teardown |

## Test surface

```bash
go test ./services/anamnesis/... ./pkg/anamnesis/... ./pkg/ebpf/...
go test ./cmd/trace-collector-go/...
```

## Adversarial validation

eBPF surface is covered by `tomb/lich/LICH-001` (Monad parser), `LICH-002`
(Sophia BPF maps), and the broader Lich framework defined in ADR-062.

## Provenance trail

The trace pipeline is the original Whispering Void / Tier 4 component;
backreferences include WAVE10C-Backprop, ADR-005 (Wotan as the substrate),
and the cross-host BPF flow graph milestone (S76 EAST online).
