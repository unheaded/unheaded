# Build — Anamnesis Lite

## Standalone (Go components — work today)

```bash
# from repo root
go build -o bin/trace-collector-go ./cmd/trace-collector-go/
go build -o bin/dashboard-backend  ./cmd/dashboard-backend/
go build -o bin/ebpf-exporter      ./cmd/ebpf-exporter/

./bin/trace-collector-go -help   # verify
./bin/dashboard-backend -help
./bin/ebpf-exporter -help
```

The Go side is the consumer of the eBPF ring buffers. It runs without the
BPF programs loaded — it just won't see any traces until they are.

## eBPF programs (gated on aya upgrade — kanban: ebpf-aya-upgrade-mn05)

Currently the BPF crates use aya-ebpf 0.1.1 which emits the legacy ELF
`maps` section that libbpf 1.0+ rejects (the `bpftool` on a 6.x kernel
won't load them). The diagnosis + fix path is documented at the kanban
entry. Until that lands, the eBPF programs build but won't load:

```bash
# Build (works):
cd ebpf
cargo +nightly build --target bpfel-unknown-none -Z build-std=core --release \
    -p packet-marker -p flow-tracker -p latency-probe

# Load (currently fails — aya 0.1.1 maps section incompatible):
sudo UNHEADED_INTERFACE=lo ../scripts/load-ebpf.sh
# → libbpf: legacy map definitions in 'maps' section not supported by libbpf v1.0+
```

When the aya upgrade ships, the load path produces:
```bash
sudo UNHEADED_INTERFACE=lo ./scripts/load-ebpf.sh
# → 4/4 BPF programs loaded + pinned at /sys/fs/bpf/unheaded/
sudo bpftool prog show
# → packet_marker, flow_tracker, latency_probe, syscall_tracer
```

## Sealed Cask (signed deterministic artifact)

```bash
./scripts/build-sealed-cask.sh \
    --name anamnesis-lite \
    --version "$(git rev-parse --short HEAD)" \
    --include "bin/trace-collector-go" \
    --include "bin/dashboard-backend" \
    --include "bin/ebpf-exporter" \
    --include "ebpf/target/bpfel-unknown-none/release/packet-marker" \
    --include "ebpf/target/bpfel-unknown-none/release/flow-tracker" \
    --include "ebpf/target/bpfel-unknown-none/release/latency-probe" \
    --include "scripts/load-ebpf.sh" \
    --include "scripts/unload-ebpf.sh" \
    --include "configs/anamnesis.yaml"

./scripts/verify-binding-rune.sh dist/anamnesis-lite-*.cask
```

## Smoke after build

```bash
# 1. Start the trace collector (it opens ring-buffer listeners; will sit idle
#    until something publishes events)
./bin/trace-collector-go -ring-buffer-pin /sys/fs/bpf/unheaded/trace_events &

# 2. Generate some loopback traffic to populate the ring buffer (only after
#    BPF programs are loaded — gated on aya upgrade above)
ping -c 5 ::1

# 3. Watch the trace-collector log for `flow_event` entries
tail -f /var/log/anamnesis/trace-collector.log
```

## Verification this BUILD.md is current

```bash
for tgt in trace-collector-go dashboard-backend ebpf-exporter; do
    go build -o /tmp/al-build-test ./cmd/$tgt/ && rm /tmp/al-build-test
done
# Result: zero output, exit 0. If any fail, BUILD.md is stale.
```
