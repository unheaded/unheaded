# Mímir's Law — Gleipnir Phase 0 PoC

UPC-controlled OS baseline delivery, drift detection, and (alerts-only v1)
self-healing — the dogfood spike for the Gleipnir convergence component
named in [[ADR-69420]]. Implements ADR-043's Two-Plane Architecture: Wotan
steady-state plane + Gjallarhorn discrete UPC trigger plane.

> **Status**: PoC COMPLETE 2026-04-11 — All 13 phases shipped. Merged to `main`. Bare-metal validated on WEST↔EAST WireGuard overlay.

## The Naming

| Component | Origin | Path | Role |
|---|---|---|---|
| **Mímir** | Norse — wise head, the rememberer | (concept) | Authoritative speaker of baseline truth |
| **Mjölnir** | Norse — Thor's hammer | `references/baseline/mjolnir.example.yaml` | Canonical baseline manifest |
| **Gungnir Seal** | Norse — Odin's spear | `pkg/gungnir/` | ML-DSA-65 signature wrapper |
| **Gjallarhorn** | Norse — Heimdall's horn | `pkg/gjallarhorn/`, `cmd/gjallarhorn-sender/` | UPC trigger packet (20-byte Monad register) |
| **Heimdall Daemon** | Norse — eternal watchman | `cmd/heimdall-daemon/`, `crates/heimdall-bpf/` | Drift watcher (BPF + userspace) |
| **Enkrateia** | Gnostic ἐγκράτεια ("self-control") | `pkg/enkrateia/` | Alerts-only drift aggregator (v1) |
| **Gleipnir** | Norse — already in [[ADR-69420]] | (Age 2b) | Parent vision; this PoC is its Phase 0 |

## Two-Plane Architecture

**Wotan plane (steady-state, existing)**
- gRPC pub/sub on `config.deltas.<node_id>`, `drift.detected.<node_id>`, `gjallarhorn.audit`
- ML-DSA-65 signed via [[Wotan Topic Signing|Wotan-Topic-Signing]]
- Continuous verification, drift events, audit trail

**Gjallarhorn plane (discrete UPC triggers, new)**
- Specially-formed Monad-wire packets (20-byte register) carrying one-shot signals
- **Bootstrap broadcast** (link-local IPv6 multicast `ff02::1:abba`): drifting motes accrete into cluster orbit
- **Reverify unicast** (over WireGuard overlay): re-check baseline against Mjölnir
- Zero new wire format changes — fits within frozen Monad v0x01

## Wire Layout (Gjallarhorn 20-byte Monad register)

| Bytes | Field | Type |
|---|---|---|
| 0:4 | magic `GJLR` | `[4]byte` |
| 4 | trigger_kind (0x01=BootstrapBroadcast, 0x02=ReverifyUnicast) | `uint8` |
| 5:9 | cluster_id | `uint32 BE` |
| 9:17 | Mjölnir manifest pointer | `uint64 BE` |
| 17:20 | reserved/padding | — |

## Eight Hard Conditions (ADR-043)

1. **Alerts-only v1** — NO auto-restore. `pkg/enkrateia` enforces zero filesystem mutations.
2. **Wotan signing prerequisite** — `services/wotan/internal/signing/` ML-DSA-65 enforcement on `config.*` topics.
3. **Baseline immutability** via dm-verity (Phase 8 deferred).
4. **HSM-grade key separation** — quarterly rotation ceremony documented.
5. **Semantic-aware drift detection** — v1 byte-level alerts; semantic diff is v2.
6. **Sacred Law clause** — no `pkg/discovery/`, `pkg/transport/`, `cmd/unheaded-daemon/` touches.
7. **No Monad wire format changes** — Gjallarhorn fits in frozen v0x01.
8. **LICH-012 campaign** — opened in parallel (`tomb/lich/LICH-012-config-convergence/`).

## Phase Status (13 Phases)

| # | Phase | Status |
|---|---|---|
| 0 | Preflight | ✓ |
| 1 | Spike branch + scaffold | ✓ |
| 2 | Schema definitions (Mjölnir YAML) | ✓ |
| 3 | `pkg/gungnir` ML-DSA-65 sign/verify | ✓ 4 tests |
| 4 | `pkg/gjallarhorn` UPC packets | ✓ 5 tests |
| 5 | `crates/heimdall-bpf` (vfs_write kprobe) | ✓ scaffold |
| 6 | `cmd/heimdall-daemon` userspace | ✓ smoke tested |
| 7 | `pkg/enkrateia` alerts-only | ✓ 3 tests |
| 8 | 2-node cluster prep (WEST + EAST) | ✓ deployed |
| 9 | Bootstrap flow benchmark | ✓ unicast WG; multicast L2-only N/A |
| 10 | Reminder flow + drift injection | ✓ injection caught on EAST |
| 11 | Stress test | ✓ 100/100 packets + forgery rejected |
| 12 | LICH-012 campaign | ✓ scaffold |
| 13 | Day-14 gate evaluation | ✓ PROMOTE verdict |

## Real-Metal Validation

Drift detection demonstrated on EAST (4-core, 8GB) on 2026-04-11:

| Test | Expected | Actual |
|---|---|---|
| Clean baseline scan | 0 alerts | 0 alerts ✓ |
| Drift injected (`echo >> /etc/ssh/sshd_config`) | 1 alert on sshd_config | 1 alert ✓ |
| Unchanged file (`/etc/os-release`) | 0 alerts | 0 alerts ✓ |
| Post-restore scan | 0 alerts | 0 alerts ✓ |
| Auto-restore attempted | NEVER | NEVER ✓ (hard condition #1) |

## Components Built

| Component | LOC | Tests |
|---|---|---|
| `pkg/gungnir/` | ~150 | 4 |
| `pkg/gjallarhorn/` | ~75 | 5 |
| `pkg/enkrateia/` | ~85 | 3 |
| `services/wotan/internal/signing/` | ~95 | 6 |
| `cmd/heimdall-daemon/` | ~190 | smoke |
| `cmd/gjallarhorn-sender/` | ~125 | smoke |
| `crates/heimdall-bpf/` | ~50 | (scaffold) |
| `references/baseline/mjolnir.example.yaml` | ~25 | — |
| `tomb/lich/LICH-012-config-convergence/README.md` | ~70 | — |
| **Total** | **~865** | **18+** |

---

> **Source:** [ADR-043](../docs/adr/ADR-043-mimirs-law-upc-baseline-gleipnir-phase-0.md) · [Battle Plan](../docs/battle-plans/BATTLE-PLAN-MIMIRS-LAW-GLEIPNIR-PHASE-0.md) · [Norse Mythology](../docs/lore/NORSE_MYTHOLOGY.md) · [[ADR-69420]]
