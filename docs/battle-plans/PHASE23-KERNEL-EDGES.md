# PHASE 2.3 — KERNEL EDGES BATTLE PLAN — 7 Phases, 96 Steps

**Date**: 2026-07-05
**Sprint**: Track 2 Phase 2.3 — own the kernel edges (ADR-081 "evolve from xv6,
never break the boot"). Replace the four remaining MIT kernel files in the
xv6-mbc link with Unheaded-authored equivalents.
**Prerequisite**: Phase 2.2 COMPLETE (`56d82cb4`); tree clean on `main`; goldens
from the phase22 session on disk at `/tmp/phase22-golden` (12 captures, TTY+stats).
**Target**: `entry.S`, `syscall.c`, `printf.c`, `console.c` out of the OBJS list,
replaced by `adapters/u{entry.S,syscall.c,printf.c,console.c}` — byte-identical
TTY+stats on the golden corpus, 16-gate harness green per commit, zero eBPF
change (kernel is MBC bytecode: the verifier numbers 900,031 / 737,087 cannot
move by construction — verify anyway).
**Estimated Duration**: 3.5-4.5 hours, single session
**Agent Strategy**: Coordinator solo (sudo boots; the boot is the shared
resource; edges are strictly sequential — each commit is the next one's base)
**Commit Cadence**: 5 atomic GREEN commits (one per edge + docs). Overrides the
generic 3-5-step formula per Track 2 doctrine "staying green every commit" —
a mid-edge commit (Makefile without the u-file, or vice versa) breaks the boot.
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts;
restore stock wiring for THAT edge (Makefile line is the only wiring), commit
nothing red, STUCK marker + report. Later edges do NOT depend on earlier ones
functionally — an edge can be skipped and the next attempted.

---

## VARIABLES

```bash
ROOT="$(git rev-parse --show-toplevel)"        # the unheaded checkout
EBPF="$ROOT/ebpf"                              # bootctl MUST run from here
BC="$ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
UP="$ROOT/crates/xv6-mbc/upstream"             # vendored xv6
AD="$ROOT/crates/xv6-mbc/adapters"             # adapters (OURS) + Makefile.mbc
K="$UP/target"                                 # kernel/userland artifacts
GOLD="${GOLD:-/tmp/phase22-golden}"            # goldens (from phase22 session)
boot() { (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk "$K/fs.img" \
  --userland "$K/init.mbc" --triggers 4000000 --instance 111 --input "$1" 2>&1); }
tty_of() { grep -a 'ascii:' <<<"$1"; }
stats_of() { grep -aoE 'forks=[0-9]+|waitpid=[0-9]+|tty_r=[0-9]+' <<<"$1" | tr '\n' ' '; }
kbuild() { (cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel); }
kfile() { riscv64-unknown-elf-objdump -t "$UP/kernel/kernel.elf" | grep 'df \*ABS\*'; }
smoke() { out="$(boot $'echo hello\n')"; \
  diff <(tty_of "$out") "$GOLD/echo_hello.tty" && diff <(stats_of "$out") "$GOLD/echo_hello.stats"; }
```

## LEGEND

`[B]` bash · `[V]` verify (must pass) · `[D]` debug (only on fail) · `[W]` write
file · `[R]` read · `[S]` sudo · `[CODE]` implementation · `[TEST]` test run ·
`[C]` commit checkpoint · `[DECIDE]` pre-seeded decision (proceed autonomously) ·
`[SEC]` security review · `[GATE]` phase exit gate · `[DOC-UPDATE]` doc ripple

---

## SITUATION

**Inherited**: Phase 2.2 COMPLETE 2026-07-05 (`56d82cb4`): the exec set
(init/sh/echo/cat/wc/ls) is 100% Unheaded-authored. 12 goldens on disk from this
morning, captured against the CURRENT kernel + CURRENT userland — kernel swaps
are exactly what they gate. Harness proven 16 PASS ALL GREEN post-commit.

**Architecture (verified against code, not assumed)**:
- `Makefile.mbc` OBJS already replaces `start.o`→`start_mbc.o`,
  `uart.o`→`console-mmio.o`, drops `plic.o`, `virtio_disk.o`→`blk-ramdisk.o`.
  The **remaining MIT objects at the edges**: `$K/entry.o` (21 lines),
  `$K/console.o` (198), `$K/printf.o` (151), `$K/syscall.o` (147). Everything
  else MIT is Phase 2.4 kernel-core territory (proc/vm/fs/...).
- **User syscalls NEVER reach kernel `syscall.c`** — the ascend ecall dispatch
  in BPF (`ebpf/monad-cpu-ebpf/src/main.rs:2151+`) handles every syscall
  in-BPF. `syscall.c` (and console.c's read/write/intr halves) are LINK-TIME
  requirements only: trap.c references `syscall()`, sysproc/sysfile use
  `arg*()`, devsw[CONSOLE] wants read/write pointers. Runtime-dead code still
  must link and must not change layout-visible behavior.
- **Live at runtime**: `entry.S` `_entry` (first instructions), `printf.c`
  (printfinit + every kernel boot print + panic), `console.c` consoleinit +
  consputc (the kernel print path down to our console-mmio uartputc). The
  golden TTY line CONTAINS the kernel boot prints — byte-identical TTY through
  our printf/console is a strong live gate.
- **Zero eBPF change by construction**: the kernel is MBC bytecode loaded as
  data by bootctl; no eBPF source is touched. Budgets must read EXACTLY
  ascend 900,031 (90.0%) / Doom 737,087 at every harness run.
- Replacement mechanism = the OBJS list in `$AD/Makefile.mbc` (one line per
  edge), mirroring the Phase 2.2 explicit-rule mechanism. Our sources live in
  `$AD/` (the established home of ours kernel-side: start_mbc.c etc.), named
  `uentry.S` / `usyscall.c` / `uprintf.c` / `uconsole.c` per the Track 2
  u-prefix (uinit/uecho/ush precedent).

**Edge order (risk-graded, smallest blast radius first)**:
1. `entry.S` — 21 lines of asm; instant boot signal; near-merger-doctrine.
2. `syscall.c` — runtime-dead; TTY cannot change; proves the mechanism on C.
3. `printf.c` — live on every print; goldens are the proof.
4. `console.c` — live consoleinit/consputc + dead read/write/intr halves;
   biggest file, most interfaces (devsw, cons lock, uart callbacks).

**Four-lens panel**:
- **Architect**: blast radius per edge = one .c/.S + one OBJS line; bootctl,
  BPF, userland, fs.img ALL untouched — goldens stay valid all sprint.
  Sequencing smallest-first; each edge is a separate green commit. Approved.
- **BlackMage**: kernel edges are not user-input-facing on the UPC (BPF owns
  the syscall surface; console read path is dead). Demands: u-files preserve
  EVERY bounds check verbatim-in-behavior (argstr MAXPATH, consoleread copyout
  contract), no relaxation of dead-path checks (they may come alive in Phase
  2.4), SPDX GPL-3.0 on every new file. Approved with §PHASE 5 checklist.
- **Developer**: gates exist already (goldens + 16-gate harness) and were
  proven against the current kernel THIS session — behavior locked before any
  change, same inverted-TDD shape as Phase 2.2. Per-edge loop: read vendored
  file (note UPC patches!) → write u-file → wire → build → FILE-symbol proof →
  smoke diff → full harness → commit. Approved.
- **Scientist**: H1-H4 below falsify the stale-state risks before edge 1;
  the determinism probe (H2) is the load-bearing one — goldens gate kernel
  bytes that must be reproducible from HEAD sources. Approved.

## PREFLIGHT HYPOTHESES

| # | Hypothesis | Verify via | If false |
|---|-----------|-----------|----------|
| H1 | Tree clean on main at `79ef1173`+; goldens (36 files) readable | Step 1-2 | STOP — wrong base |
| H2 | Rebuilding the kernel from HEAD sources reproduces current boot behavior (smoke vs golden identical) | Steps 3-5 | Goldens gate a stale kernel → STOP, re-baseline goldens against fresh build first |
| H3 | The four vendored files carry no UPC-specific patches that must survive into u-files (or: patches enumerated) | Step 6 (read + grep UPC_/ifdef) | Enumerate patches; u-files must preserve them; note in plan |
| H4 | `stack0` is declared in `start_mbc.c` (ours) so uentry.S links against OUR symbol | Step 7 | Find the real home; adjust uentry.S |

## KNOWN FAILURES BASELINE

None. Harness 16 PASS ALL GREEN post-`56d82cb4` (phase22 session, same tree
modulo docs-only `79ef1173`). gate_nway NWAY-FAIL is pre-existing and OUTSIDE
the harness.

---

## PHASE 0: PREFLIGHT — BASELINE + DETERMINISM PROBE (Steps 1-10)

**Goal**: Prove the base is green, the goldens gate THIS kernel, and the four
vendored files hold no surprises.
**Time**: ~15m (one boot; no full harness — post-commit run from phase22 is the
baseline, delta since is docs-only)
**Agent**: Coordinator

- [ ] **Step 1** [B] ~1m: Tree clean, on main, goldens present
  ```bash
  cd "$(git rev-parse --show-toplevel)" && git status --short && git log --oneline -1 && ls /tmp/phase22-golden | wc -l
  ```
- [ ] **Step 2** [V]: Clean tree; HEAD is `79ef1173` (or later docs-only); 36 golden files
- [ ] **Step 3** [DECIDE]: Baseline harness = the post-`56d82cb4` run (16 PASS,
  ALL GREEN, budgets exact) from this session. Delta since is docs-only →
  proceed WITHOUT a fresh 10-minute baseline run. Override ONLY if Step 5 fails.
- [ ] **Step 4** [B][BUILD] ~2m: Determinism probe — rebuild kernel from HEAD sources
  ```bash
  cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel 2>&1 | tail -3
  ```
  (NEVER `make -f Makefile.mbc clean` this sprint — its `rm -rf target` nukes
  fs.img + every userland artifact and invalidates the goldens.)
- [ ] **Step 5** [B][S][V] ~3m: **H2 GATE** — smoke boot vs golden after rebuild
  ```bash
  smoke && echo H2-HOLDS
  ```
  - If diff → the on-disk kernel was stale vs sources → STOP; re-capture all 12
    goldens against the fresh build BEFORE any edge (they'd gate dead bytes).
- [ ] **Step 6** [R] ~5m: H3 — read all four vendored files end to end; grep patches
  ```bash
  grep -n "UPC_\|ifdef\|ifndef" "$UP"/kernel/entry.S "$UP"/kernel/syscall.c "$UP"/kernel/printf.c "$UP"/kernel/console.c
  ```
- [ ] **Step 7** [B] ~1m: H4 — stack0 home
  ```bash
  grep -n "stack0" "$AD/start_mbc.c" "$UP/kernel/start.c" | head -5
  ```
- [ ] **Step 8** [V]: H3/H4 resolved — patches (if any) enumerated for carry-over;
  stack0 confirmed in start_mbc.c
- [ ] **Step 9** [B] ~30s: Snapshot the current kernel translation counts (for
  commit messages: expect the make output line `Translation successful: N RV32I → M MBC`)
  ```bash
  cd "$UP" && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel 2>&1 | grep Translation
  ```
- [ ] **Step 10** [V][GATE]: **PHASE 0 EXIT GATE** — H1-H4 green/resolved, smoke
  identical after a from-source rebuild, zero repo changes yet. **DoD**: goldens
  proven to gate HEAD sources; footgun list confirmed (no `clean`).

## PHASE 1: EDGE 1 — entry.S → uentry.S (Steps 11-24)

**Goal**: The first kernel instructions are ours.
**Prerequisite**: Phase 0 gate
**Time**: ~30m
**Agent**: Coordinator

- [ ] **Step 11** [W][CODE] ~5m: Write `$AD/uentry.S` — SPDX GPL-3.0-or-later;
  header: UNHEADED entry, Track 2 Phase 2.3 (ADR-081), replaces MIT entry.S in
  the OBJS list; MIT file stays vendored unwired. Behavior identical: sp =
  stack0 + (mhartid+1)*4096, call start, spin. Near-merger-doctrine (a dozen
  instructions with one correct shape) — document that in the header. Keep the
  `csrr mhartid` read (the translator maps CSRs to the 0x000_F000 region;
  hartid is 0 → same sp) so the sp computation is bit-equal to stock.
- [ ] **Step 12** [W] ~2m: Wire it — `$AD/Makefile.mbc` OBJS: `$K/entry.o` →
  `$A/uentry.o`; update the DROPPED comment block (`$K/entry.o → $A/uentry.o`).
- [ ] **Step 13** [B] ~30s: Diff check — exactly 2 hunks (OBJS line + comment)
  ```bash
  git diff --stat && git diff "$AD/Makefile.mbc" | head -30
  ```
- [ ] **Step 14** [B][BUILD] ~2m: Rebuild kernel
  ```bash
  kbuild 2>&1 | grep -E "Translation|error" 
  ```
- [ ] **Step 15** [V]: Build green; Translation line present (count may shift a
  few insns if our asm differs — record it)
- [ ] **Step 16** [B] ~30s: **REPLACEMENT PROOF**
  ```bash
  kfile | grep -E "entry"
  ```
- [ ] **Step 17** [V]: Shows `uentry.o` (or uentry.S FILE symbol) and NOT `entry.o`
- [ ] **Step 18** [B][S][TEST] ~3m: Smoke
  ```bash
  smoke && echo EDGE1-SMOKE-IDENTICAL
  ```
- [ ] **Step 19** [D] ~5m: No TTY at all → sp wrong → compare `objdump -d` of
  `_entry` in kernel.elf vs stock build (git stash the Makefile, rebuild,
  objdump, unstash). Max 2 attempts then Skip Protocol (restore `$K/entry.o`).
- [ ] **Step 20** [B][S][TEST] ~10m: Full harness
  ```bash
  bash scripts/upc-regression.sh 2>&1 | tail -6
  ```
- [ ] **Step 21** [V]: `== ALL GREEN ==`, ascend 900,031 (90.0%), Doom 737,087
- [ ] **Step 22** [W] ~2m: Commit-1 message file (use -F): subject
  `feat(upc): Track 2 Phase 2.3 — kernel edge replaced: entry (uentry.S)`,
  body: mechanism (OBJS line), merger-doctrine note, smoke + harness evidence,
  zero eBPF line, trailer `Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`
- [ ] **Step 23** [C][B] ~30s: **COMMIT — EDGE 1**
  ```bash
  git add "$AD/uentry.S" "$AD/Makefile.mbc" && git commit --no-gpg-sign -F /tmp/p23-c1.txt && git log --oneline -1
  ```
- [ ] **Step 24** [V][GATE]: **PHASE 1 EXIT GATE** — first instructions ours,
  ALL GREEN, tree clean. **DoD**: `_entry` from uentry.S, budgets exact.

## PHASE 2: EDGE 2 — syscall.c → usyscall.c (Steps 25-40)

**Goal**: The kernel syscall dispatch (runtime-dead on UPC, link-time required)
is ours; proves the C-file mechanism with zero TTY risk.
**Prerequisite**: Phase 1 gate
**Time**: ~45m
**Agent**: Coordinator

- [ ] **Step 25** [R] ~5m: Re-read vendored `syscall.c` — inventory exports:
  `syscall()`, `argint/argaddr/argstr`, `fetchaddr/fetchstr`, the `syscalls[]`
  table + `extern sys_*` decls; note which SYS_ numbers the table must cover
  (all of syscall.h — the table shape is the interface even if dead).
- [ ] **Step 26** [W][CODE] ~15m: Write `$AD/usyscall.c` — SPDX GPL; header:
  runtime-dead on UPC (BPF ecall dispatch owns the syscall surface,
  main.rs:2151+), linked because trap.c/sysproc/sysfile reference it, and it
  comes alive in Phase 2.4 when the kernel core is ours. Own structure where
  reasonable (e.g. bounds-checked dispatch helper), behavior identical:
  arg-fetch semantics, `-1` on unknown syscall num + the same `%d %s: unknown
  sys call %d` print shape, fetchstr MAXPATH-style bounds via copyinstr.
  PRESERVE every bounds check (BlackMage: dead paths come alive later).
- [ ] **Step 27** [W] ~1m: Wire OBJS: `$K/syscall.o` → `$A/usyscall.o` + comment
- [ ] **Step 28** [B][BUILD] ~2m: `kbuild 2>&1 | grep -E "Translation|error|undefined"`
- [ ] **Step 29** [V]: Links clean (no undefined sys_* — the extern set must
  match sysproc/sysfile exactly)
- [ ] **Step 30** [B] ~30s: Replacement proof: `kfile | grep -E "syscall"` →
  `usyscall.c`, no `syscall.c`
- [ ] **Step 31** [V]: Confirmed
- [ ] **Step 32** [B][S][TEST] ~3m: Smoke — `smoke && echo EDGE2-SMOKE-IDENTICAL`
  (dead code: ANY TTY change means we broke layout/link, not logic)
- [ ] **Step 33** [D] ~5m: TTY changed → symbol/layout issue: compare `nm` of
  both kernels; check `syscalls[]` table size vs syscall.h max. 2 attempts max.
- [ ] **Step 34** [B][S][TEST] ~10m: Full harness
- [ ] **Step 35** [V]: ALL GREEN + budgets exact
- [ ] **Step 36** [SEC][R] ~3m: BlackMage spot-check — every arg*/fetch* bounds
  check present; no relaxation vs stock
- [ ] **Step 37** [V]: Checklist clean
- [ ] **Step 38** [W] ~2m: Commit-2 message file (subject: `… kernel edge
  replaced: syscall dispatch (usyscall.c)`; body notes runtime-dead + why linked)
- [ ] **Step 39** [C][B] ~30s: **COMMIT — EDGE 2**
- [ ] **Step 40** [V][GATE]: **PHASE 2 EXIT GATE** — dispatch ours, ALL GREEN,
  tree clean.

## PHASE 3: EDGE 3 — printf.c → uprintf.c (Steps 41-56)

**Goal**: Every kernel print goes through our code; goldens prove it byte-exact.
**Prerequisite**: Phase 2 gate
**Time**: ~45m
**Agent**: Coordinator

- [ ] **Step 41** [R] ~5m: Re-read vendored `printf.c` — exports `printf`,
  `panic`, `printfinit`, global `volatile int panicked` (consputc spins on it);
  static printint/printptr/digits; `pr` lock struct + `locking` flag; %d %x %p
  %s format set. Note any UPC patch from H3.
- [ ] **Step 42** [W][CODE] ~15m: Write `$AD/uprintf.c` — SPDX GPL; own
  structure (e.g. emit() helper + table-driven format switch) but
  OUTPUT-IDENTICAL: same format-specifier set and semantics (including the
  "unknown %" fallback printing `%` then the char), same panicked semantics
  (set panicked=1, print, freeze via infinite loop), same locking shape
  (acquire/release pr.lock when locking).
- [ ] **Step 43** [W] ~1m: Wire OBJS: `$K/printf.o` → `$A/uprintf.o` + comment
- [ ] **Step 44** [B][BUILD] ~2m: `kbuild 2>&1 | grep -E "Translation|error|undefined"`
- [ ] **Step 45** [V]: Build + link green
- [ ] **Step 46** [B] ~30s: Replacement proof: `kfile | grep printf` → `uprintf.c` only
- [ ] **Step 47** [V]: Confirmed
- [ ] **Step 48** [B][S][TEST] ~3m: Smoke — the golden TTY line embeds every
  boot print; identical = our printf renders every kernel message byte-exact
  ```bash
  smoke && echo EDGE3-SMOKE-IDENTICAL
  ```
- [ ] **Step 49** [D] ~5m: Divergence → diff the two TTY strings char-by-char;
  classify: missing print (init order), wrong digits (printint base/sign),
  extra bytes (locking double-print). Fix, rebuild, retry. 2 attempts max.
- [ ] **Step 50** [B][S][TEST] ~3m: Panic-path probe (dead in green boots —
  exercise once out-of-band): boot with a bad ramdisk path and confirm the
  failure shape unchanged vs stock behavior noted in Phase 0
  ```bash
  (cd "$EBPF" && sudo "$BC" boot --kernel "$K/xv6-mbc.mbc" --ramdisk /dev/null --userland "$K/init.mbc" --triggers 100000 --instance 111 --input $'\n' 2>&1) | tail -3
  ```
- [ ] **Step 51** [DECIDE]: If the /dev/null probe errors out in bootctl before
  the kernel panic path, accept — panic() is exercised by usertests in Phase
  2.4, not reachable from a green boot; note it. Override only if a cheap
  in-kernel panic trigger exists.
- [ ] **Step 52** [B][S][TEST] ~10m: Full harness
- [ ] **Step 53** [V]: ALL GREEN + budgets exact
- [ ] **Step 54** [W] ~2m: Commit-3 message file
- [ ] **Step 55** [C][B] ~30s: **COMMIT — EDGE 3**
- [ ] **Step 56** [V][GATE]: **PHASE 3 EXIT GATE** — kernel prints 100% ours,
  goldens byte-identical, ALL GREEN.

## PHASE 4: EDGE 4 — console.c → uconsole.c (Steps 57-72)

**Goal**: The console driver top half is ours — the last MIT edge file.
**Prerequisite**: Phase 3 gate
**Time**: ~60m
**Agent**: Coordinator

- [ ] **Step 57** [R] ~5m: Re-read vendored `console.c` — live: `consoleinit`
  (lock init, uartinit call, devsw[CONSOLE] wiring), `consputc` (uartputc_sync
  + panicked spin + BACKSPACE triple-write). Dead-at-runtime but linked:
  `consoleread`/`consolewrite` (devsw targets; BPF owns fd I/O),
  `consoleintr` (no UART IRQ; console-mmio owns the ring). Inventory the cons
  struct (lock + buf[INPUT_BUF] + r/w/e indices) and every constant (BACKSPACE,
  C(x)). Note UPC patches from H3.
- [ ] **Step 58** [W][CODE] ~20m: Write `$AD/uconsole.c` — SPDX GPL; own
  structure; behavior identical for the live half (consputc byte-for-byte
  semantics incl. BACKSPACE `\b \b`, panicked freeze; consoleinit same init
  order: lock, uartinit(), devsw hookup). Dead half: same signatures + same
  bounds/copy semantics as stock (either_copyin/copyout contracts, C('D') EOF,
  echo behavior in consoleintr) — they come alive in Phase 2.4.
- [ ] **Step 59** [W] ~1m: Wire OBJS: `$K/console.o` → `$A/uconsole.o` + comment
- [ ] **Step 60** [B][BUILD] ~2m: `kbuild 2>&1 | grep -E "Translation|error|undefined"`
- [ ] **Step 61** [V]: Build + link green
- [ ] **Step 62** [B] ~30s: Replacement proof: `kfile | grep console` →
  `uconsole.c` + `console-mmio.c` only (both ours), no `console.c`
- [ ] **Step 63** [V]: Confirmed
- [ ] **Step 64** [B][S][TEST] ~3m: Smoke — `smoke && echo EDGE4-SMOKE-IDENTICAL`
- [ ] **Step 65** [D] ~5m: Divergence → consputc is the only live byte path:
  diff TTY; check init-order (consoleinit before first printf? — main.c calls
  consoleinit first) and devsw idx. 2 attempts max.
- [ ] **Step 66** [B][S][TEST] ~10m: Full harness
- [ ] **Step 67** [V]: ALL GREEN + budgets exact
- [ ] **Step 68** [SEC][R] ~5m: **BLACKMAGE CHECKLIST (all four u-files)**:
  - [ ] No bounds check weaker than stock (arg*, fetchstr, consoleread n-loop,
        INPUT_BUF ring indices)
  - [ ] No new globals without stock equivalent; panicked/cons/pr layouts sane
  - [ ] Dead paths preserved verbatim-in-behavior (Phase 2.4 will wake them)
  - [ ] SPDX GPL-3.0-or-later on all four files
- [ ] **Step 69** [V]: Checklist clean
- [ ] **Step 70** [W] ~2m: Commit-4 message file
- [ ] **Step 71** [C][B] ~30s: **COMMIT — EDGE 4**
- [ ] **Step 72** [V][GATE]: **PHASE 4 EXIT GATE** — all four MIT edge files
  out of the link; OBJS MIT set = kernel-core only (Phase 2.4 scope).

## PHASE 5: FULL GOLDEN SWEEP + COMPLIANCE (Steps 73-82)

**Goal**: The complete 12-golden corpus byte-identical through the all-ours-edges
kernel; license boundary clean.
**Prerequisite**: Phase 4 gate
**Time**: ~30m
**Agent**: Coordinator

- [ ] **Step 73** [B][S][TEST] ~20m: Sweep all 12 goldens (8 corpus + uinit +
  nosuch + multi + blank), TTY + stats, script per phase22
  `scratchpad/sweep-ush.sh` pattern (reuse boot/tty_of/stats_of from VARIABLES)
- [ ] **Step 74** [V]: 12/12 IDENTICAL (the phase22 `ls` sh-size note does not
  recur — fs.img untouched this sprint)
- [ ] **Step 75** [D] ~5m/line: Any divergence → the failing edge is identifiable
  by content (prints=printf/console; stats=impossible unless proc touched —
  STOP if stats move). 2 attempts then Skip Protocol on the offending edge
  (revert THAT OBJS line, keep the rest).
- [ ] **Step 76** [B][COMPLIANCE] ~1m: SPDX check
  ```bash
  head -1 "$AD"/uentry.S "$AD"/usyscall.c "$AD"/uprintf.c "$AD"/uconsole.c | grep -c GPL-3.0-or-later
  ```
- [ ] **Step 77** [V]: 4/4
- [ ] **Step 78** [B][COMPLIANCE] ~1m: MIT files untouched, still vendored
  ```bash
  git diff --stat HEAD -- "$UP/kernel/entry.S" "$UP/kernel/syscall.c" "$UP/kernel/printf.c" "$UP/kernel/console.c" | wc -l
  ```
- [ ] **Step 79** [V]: 0 lines — vendored, unwired
- [ ] **Step 80** [B] ~1m: OBJS census for the docs: remaining `$K/` objects =
  kernel-core list (kalloc spinlock string main vm proc trap sysproc bio fs
  log sleeplock file pipe exec sysfile)
  ```bash
  grep -E '^\s+\$K/' "$AD/Makefile.mbc"
  ```
- [ ] **Step 81** [V]: Census matches — the edge set is empty of MIT
- [ ] **Step 82** [V][GATE]: **PHASE 5 EXIT GATE** — corpus green, boundary
  clean, census recorded.

## PHASE 6: SHIP + DOC RIPPLES (Steps 83-96)

**Goal**: Commit 5 (docs), handoff refreshed.
**Prerequisite**: Phase 5 gate
**Time**: ~30m
**Agent**: Coordinator

- [ ] **Step 83** [DOC-UPDATE][W] ~3m: ADR-081 Progress bullet — Phase 2.3
  COMPLETE: the four edges, the runtime-dead insight (BPF owns the syscall
  surface), goldens byte-identical, zero eBPF
- [ ] **Step 84** [DOC-UPDATE][W] ~3m: Master plan — Phase 2.3 heading ✓
  COMPLETE + per-edge checklist with dates + translation counts
- [ ] **Step 85** [W] ~10m: Session log `references/phase23-kernel-edges-2026-07-05.md`
  (commits, per-edge counts, the dead-vs-live map, method note: goldens reused
  from phase22 — validity argument = fs.img/userland untouched)
- [ ] **Step 86** [W] ~2m: SHIPPED banner atop THIS plan
- [ ] **Step 87** [W] ~5m: Refresh `~/tmp/next.md` — Phase 2.3 COMPLETE; next
  candidates: Phase 2.4 kernel core (big — needs its own Warmonger plan),
  Epic 1.3.1 device inodes, verifier-headroom spike; carry footguns + add any
  new ones learned
- [ ] **Step 88** [B] ~30s: Doc diff sanity — docs + plan only
- [ ] **Step 89** [W] ~2m: Commit-5 message file
- [ ] **Step 90** [C][B] ~30s: **COMMIT 5 — DOCS**
- [ ] **Step 91** [B][S][TEST] ~10m: Post-commit harness on the committed tree
- [ ] **Step 92** [V]: ALL GREEN
- [ ] **Step 93** [B] ~30s: Ascend object left on disk (harness default) — verify
- [ ] **Step 94** [V]: Confirmed
- [ ] **Step 95** [W] ~2m: Memory update (project memory: phase23 shipped + any
  new footgun)
- [ ] **Step 96** [V][GATE]: **SPRINT COMPLETE** — 5 green commits, docs rippled,
  handoff fresh. **DoD**: a fresh session can pick up Phase 2.4 from next.md.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1 — No TTY at all after an edge swap**: entry/console/printf broke the
print path. Revert THAT edge's OBJS line (`git checkout -- $AD/Makefile.mbc`
then re-apply the still-good edges, or `git stash`), `kbuild`, re-smoke. The
u-file is inert unwired.

**A2 — `undefined reference` at link**: the u-file's export set ≠ stock.
`riscv64-unknown-elf-nm -u` the failing .o; add the missing symbol with stock
signature.

**A3 — Boot diverges only in stats**: impossible for these four files (none
touch fork/wait/tty_r accounting) → the diff is upstream state (stale fs.img?
`ls -la $K/fs.img` — must be untouched all sprint) or a raced build. Re-run
once from a quiet tree.

**A4 — `invalid value size 144/136` / `BOOT FAIL: MRET fall-through`**: wrong
eBPF variant on disk: `cd "$EBPF" && cargo build --release -p monad-cpu-ebpf
--features ascend-linux`.

**A5 — make clean disaster**: if `Makefile.mbc clean` ran, `target/` is gone:
rebuild userland (`make -f ../adapters/Makefile.mbc-userland ramdisk` builds
all programs + fs.img), rebuild kernel, THEN re-verify smoke vs goldens — if
smoke diverges, re-capture all goldens before proceeding.

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallel? | Depends on | Est |
|-------|-------|-----------|------------|-----|
| 0 Preflight | Coordinator | no | — | 15m |
| 1 uentry.S | Coordinator | no | 0 | 30m |
| 2 usyscall.c | Coordinator | no | 1 | 45m |
| 3 uprintf.c | Coordinator | no | 2 | 45m |
| 4 uconsole.c | Coordinator | no | 3 | 60m |
| 5 Sweep+compliance | Coordinator | no | 4 | 30m |
| 6 Ship+docs | Coordinator | no | 5 | 30m |

Critical path = all of it. Total ≈ 4h15m nominal, ~5h30m with debug branches.

## APPENDIX C: QUICK REFERENCE

**Kernel rebuild**: `cd $UP && rm -f kernel/kernel.elf && make -f ../adapters/Makefile.mbc kernel`
**NEVER**: `make -f Makefile.mbc clean` (nukes target/ = fs.img + userland)
**Replacement proof**: `objdump -t kernel/kernel.elf | grep 'df \*ABS\*'`
**Smoke**: boot `echo hello`, diff TTY+stats vs `$GOLD/echo_hello.{tty,stats}`
**Live kernel surface**: entry.S → start_mbc(ours) → main.c inits →
consoleinit/printfinit → prints via printf→consputc→uartputc_sync(console-mmio,
ours) → scheduler → forkret → kexec/userret. User syscalls: BPF only.
**Budgets (frozen)**: ascend 900,031 (90.0%) · Doom 737,087
**Commit style**: `-F <file>`, `--no-gpg-sign`, trailer
`Co-Authored-By: Claude Fable 5 <noreply@anthropic.com>`

---

*Phase 2.3 Battle Plan — Forged 2026-07-05*
*7 Phases. 96 Steps. The kernel's edges come home.*
*Entry, dispatch, print, console — ours.*
