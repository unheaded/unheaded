# Mímir Components — in-tree source inventory

The Mímir tool is a curation of these in-tree paths. Adopters who want to
modify Mímir's behaviour should edit the source listed here; nothing in
`cmd/tools/mimir/` is a separate copy.

## Binaries

| Path | Purpose |
|---|---|
| `cmd/heimdall-daemon/main.go` | watching daemon: Mjölnir scan + drift detection + Wotan publish |
| `cmd/gjallarhorn-sender/main.go` | UPC trigger CLI — fires a 20-byte Monad packet to request immediate scan |
| `cmd/gjallarhorn-listener/main.go` | UDP-trigger fallback for non-UPC environments |

## Library packages

| Path | Purpose |
|---|---|
| `pkg/enkrateia/` | alerts-only drift aggregator (FS-write enforcement: zero mutations on the watched host) |
| `pkg/gjallarhorn/` | 20-byte Monad trigger packet types |
| `pkg/gungnir/` | ML-DSA-65 sealed-payload signing (used for trigger-packet authentication) |
| `crates/heimdall-bpf/` | Aya kprobe scaffold (Rust eBPF; for kernel-side drift detection on Linux 5.15+) |

## Configuration + policy

| Path | Purpose |
|---|---|
| `configs/heimdall.yaml` | scan policy + baseline path config |
| `tomb/ansible/roles/heimdall/` | adopter deployment role: install + configure + start the daemon |

## Adversarial validation

| Path | Purpose |
|---|---|
| `tomb/lich/LICH-012-config-convergence/` | the Lich campaign (per ADR-062) that probes Mímir's contract — split-brain, partial-write, drift-vs-noise discrimination |

## Test surface

```bash
go test ./pkg/enkrateia/... ./pkg/gjallarhorn/... ./pkg/gungnir/...
go test ./cmd/heimdall-daemon/...
```

## Provenance trail

The components above came from the Mímir's Law / Gleipnir Phase 0 PoC spike
(branch `spike/mimirs-law`, ADR-043). The 11 of 13 phases that completed
during that spike are the foundation; the 2 incomplete phases (Phase 9 full
bootstrap, Phase 11 stress, Phase 13 gate eval) are tracked under the
ADR-043 timeline and are NOT prerequisites for adopting Mímir as a drift
sentry today.
