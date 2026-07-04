# UPC Linux — Master Battle Plan

**Forged:** 2026-07-03 (Fable 5 session, Warmonger-style, forged in-line to conserve budget)
**Owner:** Stevie + kernel team (Claude Code)
**Supersedes/absorbs:** `references/battle-plan-ascend-linux-2026-05-08.md` §5 "Phase 2 vendor
uClinux + busybox" (killed by ADR-081)
**Grounding:** ADR-067 (ISA v2), ADR-074/075/077/078 (xv6 substrate), ADR-079 (RET tagging),
ADR-080 (UPC programmatic API), ADR-081 (Unheaded Linux from scratch), the Phase 2.0 capacity
audit (`references/phase2-preflight-capacity-audit-2026-07-02.md`).

---

## 0. The two horizons (read this first)

This effort runs on **two tracks with different time/budget profiles.** Do not conflate them.

| | **Track 1 — UPC Substrate (NOW)** | **Track 2 — Unheaded Linux (LATER)** |
|---|---|---|
| **What** | Keep hardening/extending the UPC + xv6 as we've been going | Build our own OS from scratch (ADR-081) |
| **When** | Active — affordable, incremental, no-regression | Gated on budget (the $200 Max plan) + Track 1 readiness |
| **Cost** | Light: bounded handlers, one gate at a time | Heavy: sustained kernel authorship |
| **Risk rule** | Doom stays playable, xv6 stays green, verifier under 1M — every commit | Same, plus "the boot never breaks" as we swap MIT code for ours |

**Standing constraints (both tracks, non-negotiable):**
1. **1M verifier budget.** ascend build is at ~858K (85.8%). Every addition competes for the ~14% left. Measure with `UPC_VERIFIER_STATS=1`.
2. **Load-test each build variant** (ADR-080) — Doom (non-ascend) AND xv6 (ascend). Compile-checking is not enough; the Doom outage of 2026-07-03 proved it.
3. **No regression to shipped progress.** Doom playable, xv6 corpus green (ls/cat/echo/wc), gate2 ISOLATION-PASS.

---

## TRACK 1 — UPC Substrate (near-term, active)

*"Keep on track as we've been going." Each epic is bounded, affordable, and directly reusable by
Track 2 later. Do them roughly in order; each has a green gate.*

### Epic 1.1 — Regression harness + budget guard (foundation) ✅ DONE 2026-07-03
- [x] **1.1.1** `scripts/upc-regression.sh` — one command: builds + LOADS both variants through
      their real loaders. xv6 (ascend): `ls`/`cat`/`echo`/`wc` + gate2 ISOLATION-PASS + verifier
      budget via `UPC_VERIFIER_STATS`. Doom (non-ascend): doom-runner load probe ("pipeline
      complete" == under 1M). Restores the ascend object on exit. Exit 0 iff all green.
- [x] **1.1.2** `scripts/bpf-verifier-check.sh` now points CI at the harness as the real
      load-test-each-variant gate (the verifier-check script only compile/static-checks — it
      cannot catch an over-budget object; the harness loads them).
- [x] **1.1.3 GATE:** `bash scripts/upc-regression.sh` → `== ALL GREEN ==` in ~1m47s
      (xv6 857,998 = 85.8%; Doom loads). The safety net every later change runs against.
      (Follow-up: mirror `UPC_VERIFIER_STATS` into doom-runner for the exact Doom insn count —
      today the harness reports Doom as a binary load PASS/FAIL, which is the actual regression.)

### Epic 1.2 — Dogfood the UPC programmatic API (ADR-080)
*This is the prerequisite that makes "add a new guest" (eventually Unheaded Linux) a declarative
act instead of a third bespoke loader. Panel-reviewed 2026-07-03 (Architect/BlackMage/Developer/
Scientist).*
- [x] **1.2.1** `crates/upc-api` bones COMPLETE (agent scaffolded image/memory/lib, spend-limit
      cut it off; the 4 missing modules — surface/boot/workload/registry — were finished as bones,
      crate builds). NOTE: the agent used object-safe **traits** (`&dyn`); the panel preferred
      **descriptor structs + free functions** — settle that in 1.2.3 adoption. Standalone crate.
- [~] **1.2.2** Characterization tests — upc-bootctl's three pure transforms golden-tested
      (`relocate_call_word`, `relocate_rv2mbc_entry`, `fnv1a` vs canonical FNV-1a; 11 tests);
      doom-runner already has 24 tests over its load logic. REMAINS: full map-population byte
      capture (exact ROM/RV2MBC slots + entry PC + BootParams) as the ultimate no-regression oracle.
- [ ] **1.2.3** Introduce `Workload` as a descriptor both loaders construct internally; route the
      existing load code through one shared `load(workload, &mut runner)`; re-assert byte-identical
      maps at each step.
- [ ] **1.2.4** Add BlackMage's validation hooks to the shared loader: image fits ROM_MAP (guard
      exists), **rv2mbc SHA integrity** (ADR-075 gate), **surface allowlist** (refuse a guest whose
      declared syscalls exceed the loaded object's feature set), per-pid **slice non-overlap**.
- [ ] **1.2.5 GATE:** both loaders build a `Workload` and call one validated loader; golden tests
      byte-identical; Doom + xv6 green. "Add a guest" is now declarative.

### Epic 1.3 — Extend xv6 OS-primitive coverage (FS stretch + syscalls)
*Grows the substrate's primitives — every one is reusable by Unheaded Linux. Each is one bounded,
budget-aware handler with a green gate. Pick off as budget/interest allows.*
- [~] **1.3.1** FS reader stretch. **`>12 KiB` files (single-indirect blocks) DONE** — read(5)
      resolves `fbn ≥ NDIRECT` via the indirect block; `cat BIGFILE` (14,570 B) reads its tail
      past offset 12,288 (marker `INDIRECT-READ-OK`), verifier +656 insns (858,654), harness
      guards it permanently. **Subdirectory path walk DONE 2026-07-04** — open(15) resolves
      multi-component paths (`cat sub/NOTE`, `ls sub`, `sub//NOTE`, `./sub/NOTE`; `README/x`
      fails clean); walk state in a3 (OPEN_TAG|dir inum) + a2 cursor, a0 advanced past each
      consumed component on descend. Verifier 900,031 (90.0%, +41K for the feature). BUDGET
      LESSON: a component *offset* carried in state and added to `base` cost +119K (blew 1M) —
      a second variable offset into RAM_MAP inside the name-compare loop defeats verifier
      state merging; advancing a0 keeps one scalar. mkfs grew one-level `dir/name` support;
      harness gates `cat sub/NOTE` + `ls sub`. REMAINS: device inodes.
- [ ] **1.3.2** File **writes** (`write` to FD_INODE) + `>` redirection in sh — the first mutating
      FS path (needs a block-alloc story; scope carefully vs verifier budget).
- [ ] **1.3.3** Pipes (`|`) + a couple more shell builtins.
- [ ] **1.3.4 GATE (per item):** new capability works; regression sweep (1.1.3) green.

### Epic 1.4 — Code-store readiness (only when an image needs it)
- [x] **1.4.1** Option A `large-image` feature (ROM_MAP 4×, RV2MBC 8×) — DONE + validated
      (zero verifier cost, xv6 boots at Linux-scale maps).
- [x] **1.4.1a** Map-size **single source of truth** (`monad_common::mbc_maps`) — consolidation
      done after `large-image` exposed a drift bug (upc-bootctl's stale 262K guard would have
      wrongly rejected large images). mbc_maps defined + tested; all 5 eBPF map declarations
      (ROM/RV2MBC/RAM/SCREEN/CPU), upc-bootctl's guard migrated; doom-runner cross-referenced
      (keeps its own copy by design). Object byte-identical (verifier 857,998 unchanged).
- [ ] **1.4.2** Keep options B (demand-translate) + C (RAM-resident code) documented in the audit;
      **measure a spike before committing** to either. Do NOT build speculatively — a small
      own-kernel (Track 2) may fit today's maps and moot this.

---

## TRACK 2 — Unheaded Linux (long-term, budget-gated)

*ADR-081. Kicks off when budget allows sustained heavy work ($200 Max plan) AND Track 1 has made
"add a guest" declarative (1.2) + has the primitives the kernel needs (1.3). Method: **evolve from
xv6, never break the boot** — replace MIT code with ours piece by piece, staying green every
commit. C toolchain (proven RV32→MBC pipeline); minimal own-ABI first.*

### Phase 2.1 — Own PID 1 ✓ SHIPPED 2026-07-04
Replace xv6 `user/init.c` with our own `init`. Smallest self-contained win: "our code runs as PID 1
on our kernel." **Gate:** boots, our init forks+execs sh, prompt returns, Doom still plays.
- [x] `user/uinit.c` (UNHEADED-INIT) boots as PID 1 via `--userland target/uinit.mbc` — same
      kernel + fs.img, only the resident PID 1 program swapped. Banner token `0xP1D1-0UR5` →
      `uinit: starting sh` → `$` → `echo hello` → `hello` → fresh `$` (forks=2, waitpid=1).
      Zero eBPF change: ascend verifier **900,031 unchanged**, Doom green by construction.
      Two permanent harness gates added to `scripts/upc-regression.sh`.

### Phase 2.2 — Own the shell + coreutils
Our own minimal `sh`, then our own `ls`/`cat`/`echo`/`wc`, replacing the MIT userland one at a time.
**Gate (per program):** identical observable behavior, regression sweep green.

### Phase 2.3 — Own the kernel edges
Progressively replace xv6 kernel pieces we already understand: entry/`start`, console driver,
syscall dispatch table. **Gate:** boot green after each swap; the replaced file has no MIT code.

### Phase 2.4 — Own the kernel core
Our implementations of the MMU, scheduler, and FS. At the end of this phase it is **Unheaded Linux**,
not xv6 — 100% ours. **Gate:** full corpus green on an all-ours kernel; Lore has blessed the name
(it's from-scratch, disambiguate from the GPL Linux mark — ADR-081 Q5).

### Phase 2.5 — Golden-image scaling (Yggdrasil reconciliation)
Take Unheaded Linux from UPC-only toward the production golden image (ADR-69420): bare-metal target,
soft-fork discipline (`OS-FORK-DISCIPLINE.md`) on an OWNED base instead of the Debian anchor.
Resolve ADR-081 Q4 (replace the anchor / UPC-only / parallel track) here. **This is the north star**
— UPC PoC → Unheaded Linux → the OS that Unheaded actually ships.

---

## Open decisions (ADR-081 §Open questions — resolve as we reach them, none block Track 1)
1. Kernel language — **default C** (reuse the toolchain); Rust for safety-critical pieces later.
2. Evolve-from-xv6 vs greenfield — **default evolve, never break the boot.**
3. Linux-ABI depth — **minimal own-ABI first**, Linux-ABI subset later where it earns its keep.
4. Yggdrasil reconciliation — **defer to Phase 2.5.**
5. Naming — **Lore blesses "Unheaded Linux."**

## Immediate next step
Track 1, Epic 1.1 (regression harness) — cheapest, highest-leverage, and it's the safety net for
everything after. Then 1.2 (dogfood the API) as budget/appetite allows.

*Peace and love. The machine that plays Doom will run our own kernel. 🌀🐕*
