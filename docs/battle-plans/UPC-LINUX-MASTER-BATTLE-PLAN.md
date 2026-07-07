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

### Phase 2.2 — Own the shell + coreutils ✓ COMPLETE 2026-07-05
Our own minimal `sh`, then our own `ls`/`cat`/`echo`/`wc`, replacing the MIT userland one at a time.
**Gate (per program):** identical observable behavior, regression sweep green.
- [x] **echo** (2026-07-04) — `user/uecho.c` builds AS `target/echo.mbc` (the PROGRAM_TABLE
      basename sh execs); MIT `echo.c` unwired. Explicit Makefile rule overriding the pattern
      rule = the per-program replacement mechanism for the rest of the set. GCC -O2 emits
      bit-identical code for the replacement (objdump-diff verified) — identical behavior by
      construction. (The boot-log `hash` field is FNV-1a of the basename — exec's
      PROGRAM_TABLE key — constant across replacements by design; replacement proof is the
      ELF FILE symbol.) New multi-arg harness gate (`echo peace and love`). Verifier 900,031
      unchanged.
- [x] **cat** (2026-07-05) — `user/ucat.c` (own structure: `stream()` helper returns errors
      to main instead of exiting mid-helper). Corpus locks all three read paths through it:
      direct blocks (README), single-indirect (BIGFILE >12 KiB), multi-component path walk
      (sub/NOTE). Verifier 900,031 unchanged.
- [x] **wc** (2026-07-05) — `user/uwc.c`. Preserves two subtle contracts explicitly: the
      separator set { space \t \r \n \v } spelled out (xv6's strchr never matches '\0', so
      NUL counts as a word char — is_sep() encodes that instead of inheriting the quirk),
      and the stdin form's empty-name trailing space. Gate: `wc README` → `5 49 283 README`
      identical. Verifier 900,031 unchanged.
- [x] **ls** (2026-07-05) — `user/uls.c` (own structure: forward-scan basename_padded +
      print_entry helper split out of the original's fmtname/switch). Quirks preserved:
      ≥DIRSIZ names print unpadded; unknown inode types print nothing. Gates: root listing +
      `ls sub` identical. Verifier 900,031 unchanged. **Coreutils complete — exec set is now
      100% ours except sh.**
- [x] **sh** (2026-07-05) — `user/ush.c`, EXEC-only + malloc-free by design: pipe(4)/chdir(9)/
      sbrk(12) are dead syscalls on the UPC, so stock's PIPE/REDIR/LIST/BACK clauses and `cd`
      cannot function, and stock's malloc-per-cmd-node parser only worked by accident (sbrk
      no-op garbage taken as a heap pointer). Battle plan `PHASE22-SH-REPLACE.md`: 3 behavior-
      lock gates landed green against STOCK sh first (`6e9b5850`), then the swap — corpus +
      uinit byte-identical TTY+stats, 3 hostile probes survived (metachar → `syntax`, 9 args →
      `too many args`, 63-byte ring-cap line), documented divergence `cd` → `exec cd failed`
      (stock: silent no-op "success"). 1,095 RV32I → 1,874 MBC. Verifier 900,031 unchanged.
      **Phase 2.2 COMPLETE — the exec set is 100% Unheaded-authored.**

### Phase 2.3 — Own the kernel edges ✓ COMPLETE 2026-07-05
Progressively replace xv6 kernel pieces we already understand: entry/`start`, console driver,
syscall dispatch table. **Gate:** boot green after each swap; the replaced file has no MIT code.
Battle plan `PHASE23-KERNEL-EDGES.md`. `start.c`/`uart.c`/`plic.c`/`virtio_disk.c` were already
ours (Phase 1.1 adapters); this sprint removed the four remaining MIT edge files from the link,
one green commit each, kernel translation count unchanged at every step (7,552 RV32I → 11,764
MBC — GCC -O2 folds our restructures flat):
- [x] **entry** (2026-07-05) — `adapters/uentry.S`. Near-merger-doctrine (one correct shape);
      `_entry` disassembly instruction-identical; sp = stack0+(mhartid+1)*4096 against OUR
      stack0 (start_mbc.c).
- [x] **syscall dispatch** (2026-07-05) — `adapters/usyscall.c`. Runtime-dead on the UPC (the
      BPF ecall dispatch owns the syscall surface; linked for trap.c/sysproc/sysfile — alive
      again in Phase 2.4). All bounds preserved at stock strength (fetchaddr dual overflow
      guard, copyinstr-bounded fetchstr); dispatch via bounds-checked handler_for().
- [x] **printf** (2026-07-05) — `adapters/uprintf.c`. LIVE edge: every kernel boot print in
      the golden TTY line renders through it. Per-specifier va_arg decode types preserved
      (ilp32e: uint64 = 4 bytes — decode widths are ABI); panicking/panicked pair kept.
- [x] **console** (2026-07-05) — `adapters/uconsole.c`. Top half ours + bottom half
      console-mmio.o (Phase 1.1) = console stack 100% Unheaded. Live: consoleinit/consputc.
      Dead-but-linked line discipline (^H/^U/^D/^P, ring indices) preserved for Phase 2.4.
**Remaining MIT in the kernel link: core only** (kalloc spinlock string main vm proc trap
sysproc bio fs log sleeplock file pipe exec sysfile) — exactly the Phase 2.4 scope.

### Phase 2.4 — Own the kernel core ✓ COMPLETE 2026-07-07 (16/16 — the kernel link is 100% ours)
Our implementations of the MMU, scheduler, and FS. At the end of this phase it is **Unheaded Linux**,
not xv6 — 100% ours. **Gate:** full corpus green on an all-ours kernel; Lore has blessed the name
(it's from-scratch, disambiguate from the GPL Linux mark — ADR-081 Q5).
- **T1 SHIPPED 2026-07-07** (`ab9ccc25`→`75b0cc61`): string, spinlock, sleeplock, kalloc — the
  boot-hot leaves — replaced via `adapters/u*.c`, one green commit each; 12/12 golden sweep
  byte-identical; translation count 7,552 → 11,764 invariant; verifier budgets exact. Battle plan
  `docs/battle-plans/PHASE24-KERNEL-CORE.md`; log `references/phase24-t1-2026-07-07.md`.
- **T2 SHIPPED 2026-07-07 (same day)** (`e7495a95`→`3ea6ecf2`): main, bio, log, file — the init
  spine + FS support layer. umain's 14 init-bisection markers are golden content (strongest
  per-file gate); ulog's recover_from_log is boot-live through blk-ramdisk; ufile anchors
  devsw[] for the console stack. 12/12 sweep byte-identical again. Log
  `references/phase24-t2-2026-07-07.md`.
- **T3 SHIPPED 2026-07-07 (same day)** (`3d9534ba`→`8ef4e89c`): sysproc, sysfile, pipe, fs —
  the last pristine-MIT files. ufs.c is the largest single MIT file (720 lines; iinit/fsinit
  boot-live, walks dormant behind in-BPF fs_walk). u-files generated header + verbatim body
  via cat. 12/12 sweep byte-identical. Log `references/phase24-t3-2026-07-07.md`.
- **T4 SHIPPED 2026-07-07 (same day)** (`4be13c1b`→`26f29327`): vm, trap, exec, proc — the
  summit. Gate upgrade: `target/xv6-mbc.mbc` byte-identical (sha256) across all four swaps —
  pure provenance change. All Phase 1.2-1.7 patches verbatim. OBJS census: ZERO $K/ objects.
  **Phase 2.4 gate MET: full corpus green on an all-ours kernel.** usertests gates queued for
  the first evolution (trap→syscall() flip); **Q5 naming ceremony queued for Stevie/Lore** —
  the "Lore has blessed the name" clause stays open until performed. Log
  `references/phase24-t4-2026-07-07.md`.

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
