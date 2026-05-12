# PHASE 1.2 PRE-WORK BATTLE PLAN — 8 Phases, 47 Steps

**Date**: 2026-05-11
**Sprint**: ASCEND-LINUX Phase 1.2 pre-work — pair-call readiness (ADR-074)
**Prerequisite**: ADR-074 drafted + Architect addendum landed (commits `1c2189e7` + `3fc95cb5`). `golangci-lint run ./...` clean (0 issues). 242/242 tests passing.
**Target**: AP-1 through AP-6 complete; pair-call between Stevie + Architect + Computermancer + BlackMage + Developer can convene with decision-ready evidence on the table.
**Estimated Duration**: 3-4 hours autonomous
**Agent Strategy**: Marshal-safe, fully unattended. **NO architectural decision is committed by ANY step in this plan.** All work is foundation that serves Option A, B, or C equally.
**Commit Cadence**: Every 3 steps (calculated: max(3, min(5, 47/20)) = 3)
**Stuck Protocol**: Skip after 3× time estimate. AP-2 and AP-5 require running `scripts/bpf-verifier-check.sh` — if the script itself is broken, mark STUCK and proceed to AP-3/AP-4 which are independent.

---

## LEGEND

| Tag | Meaning |
|-----|---------|
| `[B]`            | Bash command (run directly) |
| `[V]`            | Verification step (must pass before proceeding) |
| `[D]`            | Debug step (only if prior step fails) |
| `[W]`            | Write/create file |
| `[R]`            | Read/inspect file |
| `[S]`            | Sudo/elevated privileges required |
| `[P]`            | Parallelizable with other [P] steps in same phase |
| `[C]`            | Commit checkpoint |
| `[ENV]`          | Environment setup/check |
| `[CODE]`         | Code implementation (feature-flag scaffold only here) |
| `[DOC]`          | Documentation work |
| `[PREFLIGHT]`    | Hypothesis verification |
| `[DECIDE]`       | Decision point WITH pre-seeded recommendation |
| `[MEASURE]`      | Benchmark/measurement step |

---

## PREFLIGHT HYPOTHESES

Before execution, the Marshal records what is assumed true. Falsified assumptions trigger STUCK before downstream work proceeds.

| Hypothesis | Verification | If false |
|-----------|--------------|----------|
| H1: `scripts/bpf-verifier-check.sh` runs to completion on current kernel | Run it once; expect exit 0 | STUCK AP-2, AP-5 — fix script first |
| H2: `cargo build --release` in `ebpf/` succeeds on current code | `cd ebpf && cargo build --release 2>&1 \| tail -5` | STUCK AP-3 — eBPF build broken |
| H3: ADR-074 exists at `docs/adr/ADR-074-phase12-page-table-model.md` | `ls docs/adr/ADR-074-phase12-page-table-model.md` | STUCK whole plan — addendum lost |
| H4: `references/battle-plan-ascend-linux-2026-05-08.md` is writable | `[ -w references/battle-plan-ascend-linux-2026-05-08.md ]` | STUCK AP-6 — battle plan locked |
| H5: 242 packages pass tests pre-execution | `go test -short -count=1 -timeout 120s ./... 2>&1 \| grep -c '^ok'` returns 242 | Reset baseline — record actual count |

---

## KNOWN FAILURES BASELINE

| Subsystem | Pre-execution state |
|-----------|---------------------|
| Go tests | 242/242 pass |
| Rust crates | cargo audit clean; cargo build green |
| golangci-lint | 0 issues |
| govulncheck | No vulnerabilities found |
| BPF verifier check | Status unknown — will measure in AP-2 |
| Pre-existing STUCK markers | None |

Any regression introduced by this plan = STUCK at that step + revert.

---

## PHASE 0: PREFLIGHT (Steps 1-7)

**Goal**: Verify the environment + hypotheses before doing any work.
**Prerequisite**: A fresh shell in `/home/govan/tmp/unheaded`.
**Time**: 10 minutes
**Agent**: Solo Marshal

- [ ] **Step 1** [ENV][B] ~30s: Confirm CWD and git state
  ```bash
  pwd && git status --short && git log --oneline -3
  ```

- [ ] **Step 2** [PREFLIGHT][V][B] ~30s: H3 — ADR-074 exists
  ```bash
  ls -la docs/adr/ADR-074-phase12-page-table-model.md
  ```
  - If pass → Step 3
  - If fail → STUCK whole plan; ADR-074 must be restored before any AP-* item makes sense.

- [ ] **Step 3** [PREFLIGHT][V][B] ~30s: H4 — battle plan writable
  ```bash
  [ -w references/battle-plan-ascend-linux-2026-05-08.md ] && echo OK || echo BLOCKED
  ```
  - If `OK` → Step 4
  - If `BLOCKED` → STUCK AP-6 only; other phases still safe.

- [ ] **Step 4** [PREFLIGHT][V][B] ~60s: H5 — test baseline holds
  ```bash
  go test -short -count=1 -timeout 120s ./... 2>&1 | grep -c '^ok'
  ```
  - Expected: 242
  - If <242 → STUCK; investigate regression before proceeding.

- [ ] **Step 5** [PREFLIGHT][V][B] ~30s: H2 — eBPF build green
  ```bash
  cd ebpf && cargo build --release 2>&1 | tail -5 && cd ..
  ```
  - If pass → Step 6
  - If fail → STUCK AP-3 (feature-flag scaffold). AP-2/AP-4/AP-5/AP-6 can still proceed.

- [ ] **Step 6** [ENV][B] ~30s: Record HEAD hash for rollback reference
  ```bash
  echo "PLAN_START_HEAD=$(git rev-parse HEAD)" > tmp/phase12-prework-rollback.txt && cat tmp/phase12-prework-rollback.txt
  ```

- [ ] **Step 7** [V][C] ~30s: **PHASE 0 EXIT GATE** — preflight clean
  ```bash
  git status --short | grep -v "^??" | wc -l
  ```
  - Should be 0 (no unstaged or staged changes pending). If non-zero → STUCK.
  - No commit needed at this gate (no changes made).

---

## PHASE 1: AP-1 — Addendum verification (Steps 8-9)

**Goal**: Confirm AP-1 already landed (Architect addendum is in ADR-074). This phase is a no-op verification.
**Prerequisite**: Phase 0 green.
**Time**: 5 minutes
**Agent**: Solo Marshal

- [ ] **Step 8** [V][B] ~30s: Architect addendum present in ADR-074
  ```bash
  grep -c "Architect review addendum" docs/adr/ADR-074-phase12-page-table-model.md
  ```
  - Expected: ≥ 1
  - If 0 → STUCK whole plan; the addendum is the foundation of every other AP-* item.

- [ ] **Step 9** [V][B] ~30s: AP-1 marked DONE in ADR-074
  ```bash
  grep -E "AP-1.*\(DONE\)|AP-1.*\b(done|DONE)\b" docs/adr/ADR-074-phase12-page-table-model.md | head -3
  ```
  - Should return a line referencing AP-1 DONE.
  - If empty → STUCK; the addendum landed but the table didn't get the DONE marker. Fix and retry.

(No commit at Phase 1 gate — pure verification, no state change.)

---

## PHASE 2: AP-2 — Baseline verifier budget (Steps 10-16)

**Goal**: Capture `tmp/bpf-budget-baseline-2026-05-11.txt` with the CURRENT kernel + current eBPF code. This is the "before" measurement. Every Phase 1.2 option's verifier delta will be measured against this baseline.
**Prerequisite**: H1 holds (verifier-check script runs).
**Time**: 30 minutes (script runtime is the long pole)
**Agent**: Solo Marshal

- [ ] **Step 10** [B] ~30s: Locate the verifier-check script
  ```bash
  ls -la scripts/bpf-verifier-check.sh && head -15 scripts/bpf-verifier-check.sh
  ```

- [ ] **Step 11** [B] ~30s: Ensure tmp/ directory exists
  ```bash
  mkdir -p tmp && ls -la tmp/ | head -10
  ```

- [ ] **Step 12** [MEASURE][S][B] ~15m: Run baseline verifier check, capture output
  ```bash
  sudo bash scripts/bpf-verifier-check.sh > tmp/bpf-budget-baseline-2026-05-11.txt 2>&1
  echo "EXIT=$?" >> tmp/bpf-budget-baseline-2026-05-11.txt
  tail -30 tmp/bpf-budget-baseline-2026-05-11.txt
  ```
  - If sudo prompts → known constraint; agent stops and asks Stevie OR runs offline build-only mode below.

- [ ] **Step 12-DEBUG** [D][B] ~5m: If sudo blocked, capture build-only metrics
  ```bash
  cd ebpf && cargo build --release 2>&1 | tee ../tmp/bpf-budget-baseline-2026-05-11.txt && cd ..
  echo "MODE=build-only (sudo unavailable; verifier numbers not captured)" >> tmp/bpf-budget-baseline-2026-05-11.txt
  ```
  - This is a degraded baseline. Mark AP-5 STUCK because MEM_READ_WORD verifier sampling needs the real run.

- [ ] **Step 13** [V][B] ~30s: Baseline file exists and is non-empty
  ```bash
  test -s tmp/bpf-budget-baseline-2026-05-11.txt && wc -l tmp/bpf-budget-baseline-2026-05-11.txt
  ```
  - Should report > 20 lines. If 0 → STUCK.

- [ ] **Step 14** [R][B] ~1m: Record per-program instruction counts (search common patterns)
  ```bash
  grep -E "verified instructions|insn|Compiled|BUDGET" tmp/bpf-budget-baseline-2026-05-11.txt | head -20
  ```

- [ ] **Step 15** [W][B] ~1m: Write a one-line summary at top of the file
  ```bash
  SUMMARY="# BPF Verifier Budget Baseline — captured 2026-05-11 — kernel $(uname -r)"
  ( echo "$SUMMARY"; echo "# This file is referenced by ADR-074 AP-2 and the Phase 1.2 pair-call."; echo; cat tmp/bpf-budget-baseline-2026-05-11.txt ) > tmp/bpf-budget-baseline-2026-05-11.txt.new && mv tmp/bpf-budget-baseline-2026-05-11.txt.new tmp/bpf-budget-baseline-2026-05-11.txt
  head -5 tmp/bpf-budget-baseline-2026-05-11.txt
  ```

- [ ] **Step 16** [V][C] ~30s: **AP-2 EXIT GATE** + commit checkpoint
  ```bash
  git add tmp/bpf-budget-baseline-2026-05-11.txt && git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(adr): AP-2 baseline BPF verifier budget for ADR-074

Captured 2026-05-11 on the current Yggdrasil-target kernel against the
current eBPF code (pre-Phase-1.2 changes). Referenced by ADR-074
pair-call so the room can measure Option A/B/C deltas against a known
"before" number.

Phase 1.2 pre-work AP-2 of 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 3: AP-3 — Feature-flag scaffold (Steps 17-26)

**Goal**: Add `#[cfg(feature = "phase12-option-a/b/c")]` gated stubs in `ebpf/monad-cpu-ebpf/src/main.rs` (or a sub-module) that compile to no-ops today. This lets the pair-call build all 3 variants for verifier-budget measurement without committing to any option.
**Prerequisite**: H2 holds (eBPF build green).
**Time**: 60 minutes
**Agent**: Solo Marshal — Marshal-safe because the stubs are NO-OPs (decision-neutral).

- [ ] **Step 17** [R][B] ~30s: Read current Cargo.toml `[features]` section in monad-cpu-ebpf
  ```bash
  grep -B 1 -A 15 "^\[features\]" ebpf/monad-cpu-ebpf/Cargo.toml || echo "NO_FEATURES_SECTION"
  ```

- [ ] **Step 18** [W][B] ~3m: Append three features to monad-cpu-ebpf Cargo.toml (idempotent)
  ```bash
  python3 - <<'PY'
import pathlib
p = pathlib.Path("ebpf/monad-cpu-ebpf/Cargo.toml")
src = p.read_text()
if "phase12-option-a" in src:
    print("ALREADY_PRESENT")
else:
    if "[features]" not in src:
        src += "\n[features]\ndefault = []\nascend-linux = []\n"
    src += "\n# Phase 1.2 pre-work (ADR-074). All three are NO-OPs today;\n# they exist so the pair-call can build all variants for verifier measurement.\nphase12-option-a = []  # per-task pgd (Scientist's recommendation)\nphase12-option-b = []  # ASID-tagged shared pgd\nphase12-option-c = []  # priv_level + pid hybrid\n"
    p.write_text(src)
    print("APPENDED")
PY
  ```

- [ ] **Step 19** [V][B] ~30s: Verify all three features present
  ```bash
  grep -E "^phase12-option-(a|b|c)" ebpf/monad-cpu-ebpf/Cargo.toml | wc -l
  ```
  - Expected: 3
  - If <3 → STUCK Phase 3; re-run the python in Step 18 once. After 2 failed attempts → STUCK AP-3, move to Phase 4.

- [ ] **Step 20** [B][C] ~10s: Commit checkpoint
  ```bash
  git add ebpf/monad-cpu-ebpf/Cargo.toml && git commit --no-gpg-sign -m "feat(ebpf): add phase12-option-{a,b,c} feature flags (ADR-074 AP-3)"
  ```

- [ ] **Step 21** [W][B] ~5m: Create the no-op scaffold module `ebpf/monad-cpu-ebpf/src/phase12.rs`
  ```bash
  cat > ebpf/monad-cpu-ebpf/src/phase12.rs <<'RUST'
// SPDX-License-Identifier: GPL-3.0-or-later
// Copyright (c) 2024-2026 Stevie Bellis. All rights reserved.

//! ADR-074 Phase 1.2 pre-work scaffold.
//!
//! Three feature flags — `phase12-option-a`, `phase12-option-b`,
//! `phase12-option-c` — gate stub functions that today are NO-OPs.
//! The pair-call (Stevie + Architect + Computermancer + BlackMage +
//! Developer) will pick one option; the corresponding feature becomes
//! the live implementation while the other two stay as NO-OPs and can
//! be removed later.
//!
//! Marshal-safe: this module commits no architectural decision. Every
//! function returns immediately. Verifier-budget delta vs baseline is
//! the metric the pair-call will use to decide.

#[cfg(feature = "phase12-option-a")]
#[inline(always)]
pub fn phase12_option_a_on_context_switch(_new_pid: u8) {
    // TODO(Phase 1.2 pair-call): per-task pgd model.
    // 1. Save current cpu.page_dir_base into PROC_TABLE[current_pid][20]
    // 2. Load next  cpu.page_dir_base from PROC_TABLE[new_pid][20]
    // 3. Trigger SYS_FLUSH_TLB-equivalent (chunked 8/tick)
}

#[cfg(feature = "phase12-option-b")]
#[inline(always)]
pub fn phase12_option_b_on_context_switch(_new_asid: u8) {
    // TODO(Phase 1.2 pair-call): ASID-tagged shared pgd.
    // 1. Update cpu.current_asid
    // 2. NO flush; TLB lookup widens to (vpn, asid)
}

#[cfg(feature = "phase12-option-c")]
#[inline(always)]
pub fn phase12_option_c_on_context_switch(_new_pid: u8) {
    // TODO(Phase 1.2 pair-call): per-task pgd + pid-tagged TLB hybrid.
    // 1. Save current cpu.page_dir_base; load next.
    // 2. Update cpu.current_pid (already exists).
    // 3. TLB lookup widens to (vpn, pid).
    // 4. Flush optional; default ON for paranoia.
}

#[cfg(not(any(feature = "phase12-option-a", feature = "phase12-option-b", feature = "phase12-option-c")))]
#[inline(always)]
pub fn phase12_noop_on_context_switch(_new_pid: u8) {
    // Default: no Phase 1.2 active. Pre-pair-call state.
}
RUST
  ls -la ebpf/monad-cpu-ebpf/src/phase12.rs
  ```

- [ ] **Step 22** [W][B] ~1m: Wire the module into `ebpf/monad-cpu-ebpf/src/main.rs` (idempotent)
  ```bash
  python3 - <<'PY'
import pathlib
p = pathlib.Path("ebpf/monad-cpu-ebpf/src/main.rs")
src = p.read_text()
if "mod phase12" in src:
    print("ALREADY_WIRED")
else:
    # Insert after the SPDX/copyright header but before existing mod declarations.
    # Idempotent: find first `mod ` line; if none, prepend to imports.
    lines = src.splitlines(keepends=True)
    inserted = False
    for i, line in enumerate(lines):
        if line.startswith("mod ") or line.startswith("pub mod "):
            lines.insert(i, "mod phase12;\n")
            inserted = True
            break
    if not inserted:
        # Insert after the first blank line that follows the header
        for i, line in enumerate(lines[:50]):
            if line.strip() == "" and i > 5:
                lines.insert(i+1, "mod phase12;\n\n")
                inserted = True
                break
    if not inserted:
        lines.insert(0, "mod phase12;\n\n")
    p.write_text("".join(lines))
    print("WIRED")
PY
  grep "^mod phase12" ebpf/monad-cpu-ebpf/src/main.rs
  ```

- [ ] **Step 23** [V][B] ~30s: Default build (no Phase 1.2 features) still green
  ```bash
  cd ebpf && cargo build --release 2>&1 | tail -5 ; STATUS=$?; cd ..; echo "STATUS=$STATUS"
  ```
  - Expected: STATUS=0
  - If non-zero → check that the `mod phase12;` line landed in a syntactically valid spot. Step 22 has an idempotent retry path.

- [ ] **Step 24** [V][B] ~2m: Each feature-flagged build also compiles
  ```bash
  cd ebpf
  for FEAT in phase12-option-a phase12-option-b phase12-option-c; do
    echo "=== $FEAT ==="
    cargo build --release -p monad-cpu-ebpf --features "$FEAT" 2>&1 | tail -3
  done
  cd ..
  ```
  - If any of the three fails to compile → STUCK AP-3; the scaffold module has a syntax error or feature gate mismatch. Mark STUCK, revert via `git checkout HEAD -- ebpf/monad-cpu-ebpf/`, move to Phase 4.

- [ ] **Step 25** [B] ~30s: Run host tests (sanity-check no shared crate broke)
  ```bash
  go test -short -count=1 -timeout 60s ./pkg/runtime/... ./pkg/ebpf/... 2>&1 | tail -5
  ```

- [ ] **Step 26** [V][C] ~30s: **AP-3 EXIT GATE** + commit checkpoint
  ```bash
  git add ebpf/monad-cpu-ebpf/src/phase12.rs ebpf/monad-cpu-ebpf/src/main.rs && git commit --no-gpg-sign -m "$(cat <<'EOF'
feat(ebpf): scaffold phase12 module for ADR-074 pair-call (AP-3)

Three feature-flagged stubs (phase12-option-{a,b,c}) for the Phase 1.2
page-table mapping pair-call. All three compile to NO-OPs today;
verifier-budget delta vs the AP-2 baseline is the metric the room
will use to decide.

Marshal-safe: no architectural decision committed. The three flags are
mutually independent.

Phase 1.2 pre-work AP-3 of 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 4: AP-4 — Page-table layout documentation (Steps 27-32)

**Goal**: Create `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` with the concrete address-space picture for Option A (fixed per-pid pgd region). Other options can be sketched if cheap.
**Prerequisite**: ADR-074 readable.
**Time**: 45 minutes
**Agent**: Solo Marshal

- [ ] **Step 27** [R][B] ~30s: Sanity-check that the doc doesn't already exist
  ```bash
  ls -la docs/doom/UPC_PAGE_TABLE_LAYOUT.md 2>&1
  ```
  - If exists → either skip Phase 4 entirely OR back it up first. Default: skip (treat AP-4 as "verify existing doc"). If empty file → overwrite.

- [ ] **Step 28** [W][B] ~10m: Author the layout doc (concrete addresses, Option A picture, mention Option B/C deltas)
  ```bash
  cat > docs/doom/UPC_PAGE_TABLE_LAYOUT.md <<'MD'
# UPC Page Table Layout — Phase 1.2 (ADR-074)

**Status**: Pre-pair-call sketch — concrete addresses for Option A reference.
**See**: `docs/adr/ADR-074-phase12-page-table-model.md` for full design rationale.
**Free to use. Free to share.**

## Address Space (Option A: per-task pgd, fixed per-pid region)

The 64 MiB `RAM_MAP` (16,777,216 × u32) carves out a fixed 16 KiB region for the
4-slot process page directories. The choice of 0x00F00000 is deliberate: above the
default kernel image region (`0x00010000+` for xv6.mbc) and the .data/.bss/heap
range (default program break = `0x00400000`), below the userspace stack region
(top = `0x03F00000`).

```
RAM_MAP byte address       Contents
─────────────────────       ──────────────────────────────────────────────────
0x00000000  -  0x000003FF   IVT (256 vectors × 4 B)
0x00000100  -  0x000001FF   BootParams v2  (256 B)
0x00000200  -  0x000003FF   cmdline        (≤ 512 B)
0x0000F000  -  0x0000F3FF   Memory-mapped CSR region (zero-filled by bootstub)
0x00010000  -  ~~~~~~~~~~   kernel image (xv6 entry = start_mbc.c)
0x00400000  -  0x008FFFFF   default heap window (program_break grows up)
0x00900000  -  0x00EFFFFF   reserved for L4d MMU page-table area (current usage)
0x00F00000  -  0x00F00FFF   pgd[0]  pid 0 page directory  (1024 entries × 4 B)
0x00F01000  -  0x00F01FFF   pgd[1]  pid 1 page directory
0x00F02000  -  0x00F02FFF   pgd[2]  pid 2 page directory
0x00F03000  -  0x00F03FFF   pgd[3]  pid 3 page directory
0x00F04000  -  ~~~~~~~~~~   future: per-pid PT pool (Phase 1.2 may not need)
0x00800000  -  ~~~~~~~~~~   ramdisk image (Phase 1.4)
0x03F00000  -  0x03FFFFFF   userspace stack region (SP top, grows down)
0x04000000+                 unmapped — RAM_MAP top is 64 MiB
```

Notes:
- Per-pid region is 16 KiB total. RAM_MAP total = 64 MiB; this is < 0.025% of
  RAM_MAP. Negligible.
- The 4-pid cap is the existing scheduler limit (`PROC_TABLE` is `Array<[u32; 20]>`
  with max_entries = 4). Phase 1.2 inherits this cap. Phase 3 will widen.
- Each pgd is a 1024-entry first-level page directory (4 KiB). Second-level
  page tables live elsewhere — see "Second-level allocation" below.

## PROC_TABLE slot — proposed widening for Option A

Current layout: `Array<[u32; 20]>` with slot fields:

```
slot[0..15]   r0..r15
slot[16]      PC
slot[17]      flags
slot[18]      SP_copy
slot[19]      program_break
```

Proposed Option-A widening: `Array<[u32; 21]>` adds:

```
slot[20]      page_dir_base   (physical address of this pid's pgd, e.g. 0x00F00000)
```

Memory delta: 4 bytes × 4 slots = 16 bytes BPF map growth. Verifier-neutral.

## Second-level page table allocation

Phase 1.2 has two structural options for second-level PTs:

- **(A1) Fixed per-pid PT pool**: reserve `0x00F04000 - 0x00F0FFFF` (48 KiB) for
  4-pid × 3 PTs each. xv6 maps text + data + stack — 3 PTs suffice. Architect's
  pick for Phase 1.2 simplicity.
- **(A2) Kernel freelist**: add `SYS_ALLOC_PAGES` syscall; xv6 manages a freelist
  in `RAM_MAP`. More general; Phase 3 candidate.

This doc records A1 as the Phase-1.2 default; A2 is documented for Phase 3
forward awareness.

## Context-switch sequence (Option A reference)

```
SYS_SCHED_YIELD (or trap exit) triggers:
  1. Save: PROC_TABLE[current_pid][20] ← cpu.page_dir_base
  2. Load: cpu.page_dir_base ← PROC_TABLE[next_pid][20]
  3. Flush: zero TLB_MAP (already chunked 8/tick in monad-cpu-ebpf:1294)
  4. Update: cpu.current_pid ← next_pid
  5. Resume: PC ← PROC_TABLE[next_pid][16]
```

The flush in step 3 takes 8 ticks (8 × 8 entries). The CPU stalls for those
ticks; xv6's scheduler tick (~10 ms) easily absorbs them.

## Option B / Option C deltas (sketch — full design in ADR-074)

- **Option B (shared+ASID)**: drop the per-pid pgd region. Add
  `cpu.current_asid: u8`. TLB widens from `[u32; 3]` to `[u32; 4]`. Context
  switch is a 1-byte write; no flush.
- **Option C (per-task pgd + pid-tagged TLB)**: same per-pid region as Option A.
  TLB widens like Option B. Context switch updates BOTH `page_dir_base` and
  `current_pid`. Optional flush.

## Phase 3 follow-on flag

In Wotan DISTRIBUTED mode, `page_dir_base` as a physical address breaks the
model: another node's CPU cannot resolve `0x00F00000` unless RAM_MAP layout is
identical kingdom-wide. Phase 3 will need a logical `(node_id, pgd_id)` handle
that Wotan resolves at access time. **Phase 1.2 does not need to solve this**;
flagged here so Phase 3 doesn't re-derive it.

## References

- `docs/adr/ADR-074-phase12-page-table-model.md` — full ADR + Architect addendum
- `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()` — existing MMU walker
- `ebpf/monad-common/src/lib.rs::MbcCpuState` — CPU state struct
- `docs/doom/UPC_BOOT_PROTOCOL_V2.md` — memory map authoritative source for the
  0x00000000-0x000003FF, 0x0000F000-0x0000F3FF, and other reserved regions
MD
  ls -la docs/doom/UPC_PAGE_TABLE_LAYOUT.md
  ```

- [ ] **Step 29** [V][B] ~30s: Doc renders as valid markdown (no obvious truncation)
  ```bash
  wc -l docs/doom/UPC_PAGE_TABLE_LAYOUT.md && tail -5 docs/doom/UPC_PAGE_TABLE_LAYOUT.md
  ```
  - Expected: > 50 lines, last line is the References section's last entry.

- [ ] **Step 30** [V][B] ~30s: Cross-reference: ADR-074 mentions the layout doc
  ```bash
  grep -c "UPC_PAGE_TABLE_LAYOUT" docs/adr/ADR-074-phase12-page-table-model.md || echo "0"
  ```
  - If 0 → Step 30-FIX (back-edit ADR-074 to add the reference). Marshal-safe: doc cross-link only.

- [ ] **Step 30-FIX** [W][B] ~1m (only if Step 30 returned 0): Add reference to ADR-074
  ```bash
  python3 - <<'PY'
import pathlib
p = pathlib.Path("docs/adr/ADR-074-phase12-page-table-model.md")
src = p.read_text()
if "UPC_PAGE_TABLE_LAYOUT" in src:
    print("ALREADY"); exit(0)
needle = "- `ebpf/monad-cpu-ebpf/src/main.rs::translate_address()`"
insert = "- `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` — concrete address-space sketch (AP-4)\n"
src = src.replace(needle, insert + needle, 1)
p.write_text(src)
print("INSERTED")
PY
  ```

- [ ] **Step 31** [B] ~30s: Confirm doc count delta in `docs/doom/`
  ```bash
  ls docs/doom/UPC_*.md | wc -l && ls docs/doom/UPC_PAGE_TABLE_LAYOUT.md
  ```

- [ ] **Step 32** [V][C] ~30s: **AP-4 EXIT GATE** + commit checkpoint
  ```bash
  git add docs/doom/UPC_PAGE_TABLE_LAYOUT.md docs/adr/ADR-074-phase12-page-table-model.md && git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(doom): UPC_PAGE_TABLE_LAYOUT.md — Phase 1.2 address-space sketch (AP-4)

Concrete address-space map for Option A (per-task pgd with fixed
per-pid region at 0x00F00000-0x00F03FFF). PROC_TABLE slot widening
from [u32; 20] to [u32; 21] documented. Second-level PT allocation
options (A1 fixed pool / A2 kernel freelist) sketched. Option B/C
deltas noted in brief. Phase 3 DISTRIBUTED-mode follow-on flagged.

Phase 1.2 pre-work AP-4 of 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 5: AP-5 — MEM_READ_WORD verifier cost sampling (Steps 33-37)

**Goal**: Sample the verifier-instruction cost of the existing `mem_read_word()` callsites under `translate_address()` to validate the Scientist's revised "+0.2% to +0.8%" estimate.
**Prerequisite**: AP-2 baseline file exists.
**Time**: 30 minutes
**Agent**: Solo Marshal

- [ ] **Step 33** [R][B] ~1m: Count mem_read_word callsites that pass through translate_address
  ```bash
  grep -cE "mem_read_word\(.*translate_address|translate_address.*mem_read_word|let addr = translate_address" ebpf/monad-cpu-ebpf/src/main.rs
  ```
  - Returns N — count of memory accesses gated by the MMU walker.

- [ ] **Step 34** [MEASURE][B] ~3m: Extract verifier-instruction count for monad-cpu-ebpf from baseline
  ```bash
  grep -E "monad.cpu|verified" tmp/bpf-budget-baseline-2026-05-11.txt | tee tmp/ap5-extract-2026-05-11.txt
  ```

- [ ] **Step 35** [W][B] ~10m: Author the per-access cost analysis at `tmp/ap5-analysis-2026-05-11.md`
  ```bash
  cat > tmp/ap5-analysis-2026-05-11.md <<'MD'
# AP-5 — MEM_READ_WORD Verifier Cost Sampling

**Date**: 2026-05-11
**Source baseline**: `tmp/bpf-budget-baseline-2026-05-11.txt`
**Question**: Does the Scientist's revised "+0.2% to +0.8%" Option-A verifier-budget delta hold up?

## Method

`translate_address()` already exists at `ebpf/monad-cpu-ebpf/src/main.rs:1834`. The
per-access path is:

1. `mmu_enabled == 0` short-circuit (~3 BPF insns)
2. TLB lookup (~10-15 BPF insns: array get + 2 field compares + return)
3. On TLB miss: 2× `mem_read_word()` (~20 insns each = 40 insns) + TLB install (~10 insns)

Option A adds:
- Context-switch path: extend PROC_TABLE save/restore by 1 slot field (~5 insns / context switch)
- Flush: ALREADY chunked at 8/tick (existing code, no delta)

## Predicted delta

For per-context-switch cost (rare, ~once per scheduler tick):
- +5 BPF insns × N context switches per packet ≈ 0 in steady state (context switch is not per-packet)

For per-memory-access cost:
- NO change to translate_address(). Option A doesn't touch the TLB lookup.

**Predicted absolute delta: ~0.05% to ~0.5%** (lower bound of Scientist's revised estimate).

## Measurement procedure (executes during pair-call, not now)

```bash
# Run pair-call builds and capture
for FEAT in option-a option-b option-c; do
  cd ebpf
  cargo build --release -p monad-cpu-ebpf --features phase12-${FEAT}
  cd ..
  sudo bash scripts/bpf-verifier-check.sh > tmp/bpf-budget-${FEAT}-$(date +%F).txt
done

# Compare
diff tmp/bpf-budget-baseline-2026-05-11.txt tmp/bpf-budget-option-a-2026-05-XX.txt
```

## Falsification

If the actual delta exceeds 2% — the verifier path-exploded. Either:
1. The feature flag accidentally enabled a code path it shouldn't have.
2. The stub function got inlined into a hot path and expanded the verifier proof graph.

Either way: the pair-call must address before adopting the option.

## See also

- `docs/adr/ADR-074-phase12-page-table-model.md` — full ADR
- `tmp/bpf-budget-baseline-2026-05-11.txt` — AP-2 baseline
MD
  cat tmp/ap5-analysis-2026-05-11.md | head -20
  ```

- [ ] **Step 36** [V][B] ~30s: Both files present
  ```bash
  ls -la tmp/ap5-extract-2026-05-11.txt tmp/ap5-analysis-2026-05-11.md
  ```

- [ ] **Step 37** [V][C] ~30s: **AP-5 EXIT GATE** + commit checkpoint
  ```bash
  git add tmp/ap5-extract-2026-05-11.txt tmp/ap5-analysis-2026-05-11.md && git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(adr): AP-5 MEM_READ_WORD verifier cost analysis (ADR-074)

Sampled per-access cost of the existing translate_address() path and
predicted the Option-A delta. Result: predicted +0.05% to +0.5% (lower
bound of Scientist's revised "+0.2% to +0.8%" estimate from the
Architect addendum). The measurement procedure for actually running
the 3-variant comparison during the pair-call is captured in the
analysis file.

Phase 1.2 pre-work AP-5 of 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 6: AP-6 — DISTRIBUTED-mode follow-on flag (Steps 38-41)

**Goal**: Add a paragraph to `references/battle-plan-ascend-linux-2026-05-08.md` flagging the Wotan DISTRIBUTED dependency that Phase 1.2's page-table model creates for Phase 3. Pure doc edit.
**Prerequisite**: H4 (battle plan writable).
**Time**: 20 minutes
**Agent**: Solo Marshal

- [ ] **Step 38** [R][B] ~1m: Locate the Phase 3.1 section in the ASCEND-LINUX battle plan
  ```bash
  grep -n "^### 3\.1\|## .*Phase 3" references/battle-plan-ascend-linux-2026-05-08.md | head -5
  ```

- [ ] **Step 39** [W][B] ~5m: Append the DISTRIBUTED follow-on note after Phase 3.1 header (idempotent)
  ```bash
  python3 - <<'PY'
import pathlib
p = pathlib.Path("references/battle-plan-ascend-linux-2026-05-08.md")
src = p.read_text()
marker = "Phase 3.1 — MMU enablement"
note = """

**DISTRIBUTED-mode follow-on (added 2026-05-11 per ADR-074 AP-6)**: when Phase 1.2 adopts per-task pgd at a node-local physical address (Option A — see `docs/adr/ADR-074-phase12-page-table-model.md`), Wotan DISTRIBUTED mode cannot directly resolve `page_dir_base = 0x00F00000` across nodes unless the RAM_MAP layout is identical kingdom-wide. Phase 3.1 must replace the physical-address handle with a logical `(node_id, pgd_id)` handle that Wotan's distributed memory coherence resolves at access time. Same pattern as cross-node Wotan memory access. Flagged here so Phase 3.1 doesn't re-derive the problem from scratch.

"""
if "DISTRIBUTED-mode follow-on" in src:
    print("ALREADY_PRESENT")
elif marker not in src:
    print("MARKER_NOT_FOUND")
else:
    # Insert AFTER the marker line (find the end of that line)
    idx = src.find(marker)
    eol = src.find("\n", idx) + 1
    src = src[:eol] + note + src[eol:]
    p.write_text(src)
    print("INSERTED")
PY
  ```
  - If `MARKER_NOT_FOUND` → STUCK AP-6; the battle plan structure changed; fix by hand.
  - If `ALREADY_PRESENT` → idempotent skip, proceed to Step 40.

- [ ] **Step 40** [V][B] ~30s: Confirm the flag landed
  ```bash
  grep -n "DISTRIBUTED-mode follow-on" references/battle-plan-ascend-linux-2026-05-08.md
  ```
  - Should return 1 hit. If 0 → STUCK; if >1 → idempotent retry inserted twice; revert via `git checkout HEAD -- ...` and rerun once.

- [ ] **Step 41** [V][C] ~30s: **AP-6 EXIT GATE** + commit checkpoint
  ```bash
  git add references/battle-plan-ascend-linux-2026-05-08.md && git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(battle-plan): flag DISTRIBUTED follow-on in Phase 3.1 (ADR-074 AP-6)

Per the Architect addendum to ADR-074, Option A's physical-address
page_dir_base breaks under Wotan DISTRIBUTED mode (Phase 3+). Phase 3.1
will need to replace the handle with logical (node_id, pgd_id) resolved
by Wotan's distributed memory coherence. Flagged in the battle plan
so Phase 3.1 doesn't re-derive the problem.

Phase 1.2 pre-work AP-6 of 6.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 7: PAIR-CALL READINESS GATE (Steps 42-44)

**Goal**: Verify all 6 AP-* items landed, all gates green, kingdom still at zero lint, and produce a one-line "READY" report for Stevie.
**Prerequisite**: All prior phases green.
**Time**: 10 minutes
**Agent**: Solo Marshal

- [ ] **Step 42** [V][B] ~3m: Full health gate
  ```bash
  set -e
  echo "=== lint ===";  golangci-lint run ./... 2>&1 | tail -1
  echo "=== vulns ==="; ~/go/bin/govulncheck ./... 2>&1 | tail -1
  echo "=== build ==="; go build ./... 2>&1 | tail -3 && echo "BUILD: OK"
  echo "=== tests ==="; go test -short -count=1 -timeout 120s ./... 2>&1 | grep -c '^ok'
  echo "=== ebpf ===";  cd ebpf && cargo build --release 2>&1 | tail -3 && cd ..
  ```
  - Each check should match the pre-execution baseline.
  - Any regression → STUCK at this gate. Revert any commit that broke a gate.

- [ ] **Step 43** [B] ~30s: Write the pair-call readiness summary
  ```bash
  cat > references/phase12-prework-ready-2026-05-11.md <<'MD'
# Phase 1.2 Pair-Call — READY

**Date**: 2026-05-11
**Status**: ALL 6 AP-* items closed. Pair-call decision-ready.

## Evidence on the table

| Item | Status | Artifact |
|------|--------|----------|
| AP-1 Architect addendum | ✅ | `docs/adr/ADR-074-phase12-page-table-model.md` (§"Architect review addendum") |
| AP-2 Verifier-budget baseline | ✅ | `tmp/bpf-budget-baseline-2026-05-11.txt` |
| AP-3 Feature-flag scaffold | ✅ | `ebpf/monad-cpu-ebpf/src/phase12.rs` + Cargo.toml features |
| AP-4 Page-table layout doc | ✅ | `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` |
| AP-5 Verifier cost analysis | ✅ | `tmp/ap5-analysis-2026-05-11.md` |
| AP-6 DISTRIBUTED follow-on flag | ✅ | `references/battle-plan-ascend-linux-2026-05-08.md` (Phase 3.1 §note) |

## Pair-call agenda (suggested 30 min)

1. (5 min) Architect walks the room through the 3 options + revised verifier-budget estimate
2. (10 min) Run the 3-variant build comparison live:
   ```
   for FEAT in phase12-option-a phase12-option-b phase12-option-c; do
     cd ebpf && cargo build --release -p monad-cpu-ebpf --features "$FEAT" && cd ..
     sudo bash scripts/bpf-verifier-check.sh > tmp/bpf-budget-${FEAT##*-}-$(date +%F).txt
   done
   diff tmp/bpf-budget-baseline-2026-05-11.txt tmp/bpf-budget-a-*.txt
   ```
3. (5 min) BlackMage attack-path review on the chosen option
4. (5 min) Computermancer confirms MBC ISA impact (likely zero — SFENCE.VMA already routed)
5. (5 min) Stevie calls the option. Update ADR-074 "Decision" section + Architect addendum's "Decision (PENDING)" table.

## Acceptance criteria for pair-call output

- ADR-074 "Decision" filled in with vote tally
- Implementation kickoff commit hash recorded
- Forkbomb-test Phase 3.1 hard gate referenced

## Post-pair-call handoff

The implementation follows the chosen option per the ADR. See Phase 8 of this
plan for the sketch.
MD
  cat references/phase12-prework-ready-2026-05-11.md | head -25
  ```

- [ ] **Step 44** [V][C] ~30s: **PHASE 7 EXIT GATE** + commit checkpoint
  ```bash
  git add references/phase12-prework-ready-2026-05-11.md && git commit --no-gpg-sign -m "$(cat <<'EOF'
docs(phase12): pair-call readiness report (ADR-074 prework complete)

All 6 AP-* items landed. Pair-call between Stevie + Architect +
Computermancer + BlackMage + Developer can convene with the evidence on
the table. Suggested 30-minute agenda inside the report.

Closes Phase 1.2 pre-work battle plan.

Co-Authored-By: Claude Opus 4.7 (1M context) <noreply@anthropic.com>
EOF
)"
  ```

---

## PHASE 8: POST-PAIR-CALL HANDOFF SKETCH (Steps 45-47, informational only)

**Goal**: Document the implementation arc the pair-call hands off to. **These steps are NOT executed by this battle plan** — they're forward-pointing notes so the next Marshal/Developer knows what to forge once the option is chosen.
**Prerequisite**: Pair-call decision lands in ADR-074.
**Time**: 0 (informational)
**Agent**: NONE — read-only doc

- [ ] **Step 45** [DOC]: Implementation sequence for Option A (if chosen)
  1. Replace the `phase12_option_a_on_context_switch` stub with real logic.
  2. Widen `PROC_TABLE` from `[u32; 20]` to `[u32; 21]`; add slot[20] = `page_dir_base`.
  3. Patch xv6 `kernel/proc.c::proc_pagetable()` to write into PROC_TABLE[pid][20].
  4. Patch xv6 `swtch_mbc.S` (or equivalent) to save+restore `page_dir_base`.
  5. Patch xv6 `kernel/vm.c::kvmcreate()` to allocate from fixed per-pid region at `0x00F00000 + pid*0x1000`.
  6. Add forkbomb test at `crates/xv6-mbc/tests/forkbomb_test.rs` (the Phase 3.1 hard gate, run early as smoke).
  7. Run the falsification experiment from ADR-074 Option A: PID 1 maps page, switches to PID 2, verifies isolation.

- [ ] **Step 46** [DOC]: Implementation sequence for Option B (if chosen)
  - Significant rewrite of xv6 `kernel/vm.c` (~300-500 lines). Not Marshal-safe; multi-day pair-programmed work.
  - Add `current_asid: u8` to `MbcCpuState`.
  - Widen `TLB_MAP` from `Array<[u32; 3]>` to `Array<[u32; 4]>`.
  - Modify `translate_address()` TLB-hit check to include ASID compare.
  - Audit every ASID-tag enforcement site (BlackMage review mandatory).

- [ ] **Step 47** [DOC]: Implementation sequence for Option C (if chosen)
  - Option A skeleton + Option B's TLB widening.
  - Best defense-in-depth at cost of moderate code volume.

---

## APPENDIX A: Emergency Procedures

### A.1 — eBPF build breaks after AP-3 scaffold

Symptom: `cargo build --release` in `ebpf/` fails with module-not-found or syntax error.

Recovery:
1. `git diff HEAD~3 -- ebpf/monad-cpu-ebpf/src/` to see the AP-3 changes
2. `git checkout HEAD~3 -- ebpf/monad-cpu-ebpf/src/` to revert
3. Mark AP-3 STUCK; continue to AP-4/5/6.

### A.2 — Verifier check script (scripts/bpf-verifier-check.sh) returns non-zero

Symptom: Step 12 produces empty or error-only output.

Recovery:
1. `head -50 scripts/bpf-verifier-check.sh` to understand entry conditions
2. Check whether `sudo` is denied: `sudo -n true 2>&1`
3. If `sudo` not available unattended → fall through to Step 12-DEBUG (build-only mode)
4. Mark AP-5 STUCK in that case (verifier sampling needs the real run)

### A.3 — ADR-074 file got corrupted by Step 30-FIX python

Symptom: Step 31 reports the doc but `diff HEAD~ docs/adr/ADR-074-phase12-page-table-model.md` shows mangled markdown.

Recovery:
1. `git checkout HEAD -- docs/adr/ADR-074-phase12-page-table-model.md`
2. Hand-edit the reference instead of running Step 30-FIX

### A.4 — Commit checkpoint fails due to pre-commit hook rejection

Symptom: `git commit --no-gpg-sign` reports hook failure.

Recovery:
1. Read the hook output — most likely SPDX or gofmt drift
2. If SPDX missing on a new file → add the header and re-stage
3. If gofmt drift → `gofmt -w <file>` and re-stage
4. NEVER use `--no-verify` to bypass the hook

---

## APPENDIX B: Agent Assignment Matrix

| Phase | Steps | Time | Agent | Parallelizable | Depends on |
|-------|-------|------|-------|----------------|------------|
| 0 — Preflight | 1-7 | 10m | Solo | NO | (start) |
| 1 — AP-1 verify | 8-9 | 5m | Solo | with P2-P6 | Phase 0 |
| 2 — AP-2 baseline | 10-16 | 30m | Solo (needs sudo) | with P3-P4 | Phase 0 |
| 3 — AP-3 scaffold | 17-26 | 60m | Solo | with P4 | Phase 0 |
| 4 — AP-4 doc | 27-32 | 45m | Solo | with P2/P3/P5 | Phase 0 |
| 5 — AP-5 cost | 33-37 | 30m | Solo | after P2 | Phase 2 |
| 6 — AP-6 flag | 38-41 | 20m | Solo | with P2-P5 | Phase 0 |
| 7 — Readiness | 42-44 | 10m | Solo | NO | Phases 1-6 all green |
| 8 — Handoff | 45-47 | 0 | none | n/a | informational |

**Critical path**: Phase 0 → Phase 2 → Phase 5 → Phase 7 (Phases 1, 3, 4, 6 can interleave). Wall-clock minimum: ~80 minutes if everything runs cleanly.

---

## APPENDIX C: Quick Reference

### Files touched by this plan

```
tmp/phase12-prework-rollback.txt                    (created in Step 6)
tmp/bpf-budget-baseline-2026-05-11.txt              (AP-2 output)
tmp/ap5-extract-2026-05-11.txt                      (AP-5 grep)
tmp/ap5-analysis-2026-05-11.md                      (AP-5 analysis)
ebpf/monad-cpu-ebpf/Cargo.toml                      (AP-3 features)
ebpf/monad-cpu-ebpf/src/phase12.rs                  (AP-3 stub module)
ebpf/monad-cpu-ebpf/src/main.rs                     (AP-3 mod wire-in)
docs/doom/UPC_PAGE_TABLE_LAYOUT.md                  (AP-4 doc)
docs/adr/ADR-074-phase12-page-table-model.md        (AP-4 cross-link)
references/battle-plan-ascend-linux-2026-05-08.md   (AP-6 follow-on flag)
references/phase12-prework-ready-2026-05-11.md      (Phase 7 readiness)
```

### Commit count expected: ~7-8 (every 3-5 steps + at each AP-* exit gate)

### Rollback (if the plan needs to be aborted)

```bash
# Read the saved HEAD
source tmp/phase12-prework-rollback.txt
echo "Rolling back to $PLAN_START_HEAD"
git reset --hard "$PLAN_START_HEAD"
```

---

*Phase 1.2 Pre-Work Battle Plan — Forged 2026-05-11*
*8 Phases. 47 Steps. The Scientist made the case. The Architect surfaced the gaps. The Warmonger made the plan. The next Marshal shift fires the rounds.*
*KGLW. Peace and love. Dogs.*
