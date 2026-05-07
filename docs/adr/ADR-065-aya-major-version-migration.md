# ADR-065 — aya 0.1.x → 0.13.x Major-Version Migration Plan

**Status:** Proposed (migration plan only; implementation gated on Captain approval + Linux verification host)
**Date:** 2026-05-07
**Deciders:** unheaded-architect (BPF target / verifier impact) + unheaded-developer (impl + smoke recipe) + unheaded-marshal (gate enforcement)
**Aligns with:** ADR-003 (eBPF Rust + Aya framework choice), ADR-012 (BPF verifier risk mitigation), ADR-052 (timeline + battle-plan source-of-truth policy)

**Triggered by:** Marshal-shift-2026-05-07 Phase 3 + parking lot. The patch-level upgrade path within `^0.1` semver is exhausted (`cargo update -p aya-ebpf -p aya-log-ebpf` on 2026-05-07 returned "Locking 0 packages"). Upstream current is **aya 0.13.x**. The kanban-tracked goal `ebpf-aya-upgrade-mn05` cannot be closed by a patch update — it requires a major-version bump, which requires a Cargo.toml edit, which requires this ADR per ADR-052.

---

## Context

### What we have today

- **`ebpf/Cargo.toml`** — `aya-ebpf = "0.1"` (resolves to 0.1.1), `aya-log-ebpf = "0.1"` (resolves to 0.1.0), plus `aya-ebpf-bindings 0.1.2`, `aya-ebpf-macros 0.1.2`, `aya-ebpf-cty 0.2.3`.
- **`cmd/ebpf-loader/Cargo.toml`** — userspace `aya = "0.13"` (already on the current major release on the userspace side).
- **`scripts/bpf-verifier-check.sh`** — gate at 7 % budget today (~69,793 / 900,000 instructions per program).
- **23 eBPF programs** in the `ebpf/` workspace covering the protocol foundation (monad-cpu, packet-marker, flow-tracker, latency-probe), the application tier (firewall, maglev, canary, qos), and Doom-on-Monad (monad-cpu-ebpf, hop-ebpf, etc.).

### The asymmetry that triggered this

The userspace half of the toolchain (`aya = "0.13"`) is on the current major. The kernel half (`aya-ebpf = "0.1"`) is two majors behind. Every BPF map shape, every helper signature, every program-type macro that crosses the userspace ↔ kernel boundary depends on the two halves agreeing. Today they nominally do (the verifier check passes at 7 % budget), but the further the kernel half drifts behind, the more likely a future userspace `aya` update introduces an incompatibility that we cannot unwind without first doing this migration.

### What "0.1 → 0.13" actually changes

Material changes upstream between 0.1.1 and 0.13.x (drawn from the public aya CHANGELOG; verify before code lands):

1. **Map types as proper generics** — `BPF_MAP_TYPE_HASH` etc. are now `HashMap<K, V>` with associated-type-bound constructors instead of the older `Array`/`HashMap` raw helpers. Affects every `.rs` file that declares a map.
2. **Program-type attribute macros refactored** — `#[xdp]`, `#[tracepoint]`, `#[kprobe]` had their internals reworked; signature changes possible for some attach types.
3. **Helper function module reshuffled** — many `aya_bpf::*` paths moved under `aya_ebpf::helpers::*`. Mass `use` rewrites.
4. **`aya-log-ebpf` string formatting** — output format compatible with the new userspace `aya-log` crate; older format may need adaptation.
5. **MSRV** — 0.13 likely raises minimum supported Rust to whatever the toolchain matrix needs. Our nightly cargo (1.95-nightly per Phase 3 verification) is well past any plausible MSRV.

This is a non-trivial migration. The kanban issue's "ELF-loader compatibility" framing is one symptom; the real shape is "all 23 BPF programs need their map declarations + helper imports rewritten."

---

## Decision

**Adopt a phased migration plan, executed in a feature branch, gated on a Linux verification host with `bpftool ≥ 7.7` and `libbpf ≥ 1.7`. Land the migration as one merge to `main` only after the full BPF verifier gate is green at the new major.**

The plan is exhaustively pre-flighted on this branch; main stays on 0.1.x until the merge.

---

## Migration phases

### Phase A — Pre-flight (no code changes)
- A1. Inventory all `aya_bpf::*` and `aya_ebpf::*` imports across `ebpf/` (grep + count).
- A2. Inventory all `BPF_MAP_TYPE_*` declarations and their `<K, V>` shapes.
- A3. Inventory all program-type attribute macros (`#[xdp]`, `#[tracepoint]`, `#[kprobe]`, `#[uprobe]`, `#[fexit]`, `#[fentry]`, `#[lsm]`, etc.).
- A4. Snapshot current `scripts/bpf-verifier-check.sh` output as the regression baseline (instruction count per program). Save to `tmp/bpf-baseline-pre-aya13.txt`.
- A5. Confirm Linux verification host has `bpftool ≥ 7.7`, `libbpf ≥ 1.7`, `bpf-linker` (Rust nightly + matching LLVM), and a kernel ≥ 6.0.

### Phase B — Feature branch
- B1. Branch `feat/aya-0.13-migration` off current `main`.
- B2. Bump `ebpf/Cargo.toml`: `aya-ebpf = "0.13"`, `aya-log-ebpf = "0.13"`.
- B3. `cargo update --workspace`. Capture lockfile diff.
- B4. `cargo build --release`. Catch the first wave of errors. Expect: `unresolved import` from path moves, `incompatible` for old map declarations, `attribute macro` errors for changed program types.

### Phase C — Mechanical fixes
- C1. Apply path rewrites: `aya_bpf::*` → `aya_ebpf::*`, helpers from `aya_bpf::helpers` → `aya_ebpf::helpers` (or wherever they actually moved — check upstream module layout).
- C2. Rewrite map declarations to the new generic shape. One PR-stack commit per category (HashMap, Array, RingBuf, etc.) for reviewability.
- C3. Fix program-type macros where signatures changed.
- C4. After each commit: `cargo build --release` must be green.

### Phase D — Verifier gate
- D1. `bash scripts/bpf-verifier-check.sh`. Compare instruction count per program against Phase A4 baseline.
- D2. Any program that crosses the budget ceiling (currently 900K) gets a focused fix; do NOT raise the budget unless every other option is exhausted.
- D3. `bpftool prog load` smoke against a running kernel for at least: monad-cpu-ebpf, packet-marker, flow-tracker, latency-probe, plus one application-tier program.

### Phase E — Integration smoke
- E1. `make test` from repo root (catches userspace consumers of the BPF maps).
- E2. Run the doom-runner end-to-end to confirm monad-cpu still executes (per `docs/doom/RUNBOOK.md`).
- E3. If WEST + EAST cross-host BPF flow graph is in a known-good state, smoke that too.

### Phase F — Merge
- F1. Round Table sign-off (Architect + Developer + BlackMage minimum).
- F2. Single merge commit to `main`. No squashing — preserve the per-category C2 commits for future archaeology.
- F3. Update `CLAUDE.md` Tech Stack table: aya version note.
- F4. Close kanban `ebpf-aya-upgrade-mn05`.
- F5. Park the next-recurring concern: `aya 0.14`, `0.15`, etc. drift policy.

---

## Smoke-test recipe (for daytime resumption)

```bash
# Phase A pre-flight
cd ~/tmp/unheaded
grep -r 'aya_bpf::' ebpf/ | wc -l                                    # path-rewrite count
grep -r 'BPF_MAP_TYPE_' ebpf/ | wc -l                                # map-decl count
bash scripts/bpf-verifier-check.sh > tmp/bpf-baseline-pre-aya13.txt  # instruction baseline

# Phase B branch
git switch -c feat/aya-0.13-migration
sed -i 's/aya-ebpf = "0.1"/aya-ebpf = "0.13"/' ebpf/Cargo.toml
sed -i 's/aya-log-ebpf = "0.1"/aya-log-ebpf = "0.13"/' ebpf/Cargo.toml
cd ebpf && cargo update --workspace && cd ..
cd ebpf && cargo build --release 2>&1 | tee /tmp/aya-build-1.log

# Phases C / D / E follow from the build errors per the spec above.
```

---

## Consequences

### Positive
- Userspace ↔ kernel `aya` versions in lockstep at 0.13.
- ELF map format aligned with `bpftool 7.7+` / `libbpf 1.7+`, closing `ebpf-aya-upgrade-mn05`.
- Removes a dependency-drift carry-cost that compounds with every userspace `aya` update.
- Establishes a BPF verifier-budget regression baseline for future major bumps.

### Negative
- Non-trivial migration — estimate **1-2 days of focused Developer time on a Linux host**, not unattended-safe.
- 23 BPF programs to touch; some may exceed verifier budget at the new major (Phase D2 risk).
- Until merged, the feature branch and `main` diverge — keep the branch short-lived.

### Open questions (resolve during migration)
- Does `aya-log-ebpf` 0.13 require userspace `aya-log` updates we haven't tracked?
- Does any of our 23 programs use a helper that was deprecated/removed between 0.1 and 0.13?
- Does the new map-type generic shape compose correctly with our existing `monad-common` / `pqc-common` shared types?

---

## Compliance

This ADR satisfies ADR-052 Rule 3: *no Cargo.toml edit without an in-tree ADR or battle-plan of record.* The migration itself, when it lands, will be cross-referenced from this ADR and from `references/timeline.md` per Rule 5.
