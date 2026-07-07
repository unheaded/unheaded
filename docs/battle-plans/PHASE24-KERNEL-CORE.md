# PHASE 2.4 — KERNEL CORE BATTLE PLAN — Tranche Structure, T1 Detailed (68 Steps)

**Date**: 2026-07-05
**Sprint**: Track 2 Phase 2.4 — own the kernel core (ADR-081). Replace the 16
remaining MIT kernel files. At the end it is **Unheaded Linux, not xv6** (Lore
blesses the name at the summit — ADR-081 Q5).
**Prerequisite**: Phase 2.3 COMPLETE (`c10f5fb5`); tree clean; goldens valid
(`/tmp/phase22-golden`, machine not rebooted since capture; fs.img untouched).
**Target (full phase)**: zero MIT objects in the kernel link, full corpus green.
**Target (THIS session = Tranche 1)**: string.c, spinlock.c, sleeplock.c,
kalloc.c replaced — the boot-hot leaf files, one green commit each.
**Estimated Duration**: T1 ~3h this session; T2-T4 are follow-on sessions, each
forging its detailed steps at tranche start (this doc holds their scope + gates).
**Agent Strategy**: Coordinator solo (the boot is the shared resource).
**Commit Cadence**: one green commit per file + docs commit per tranche
(Track 2 "staying green every commit" doctrine — overrides the generic formula).
**Stuck Protocol**: 2 failed debug attempts → restore that file's OBJS line
(u-file inert unwired), commit nothing red, STUCK marker, next file.

---

## VARIABLES (same as PHASE23; boot/tty_of/stats_of/kbuild/kfile/smoke)

```bash
ROOT="$(git rev-parse --show-toplevel)"; EBPF="$ROOT/ebpf"
BC="$ROOT/cmd/upc-bootctl/target/release/upc-bootctl"
UP="$ROOT/crates/xv6-mbc/upstream"; AD="$ROOT/crates/xv6-mbc/adapters"
K="$UP/target"; GOLD="${GOLD:-/tmp/phase22-golden}"
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

Same as PHASE23-KERNEL-EDGES.md (`[B] [V] [D] [W] [R] [S] [CODE] [TEST] [C]
[DECIDE] [SEC] [GATE] [DOC-UPDATE]`).

---

## SITUATION

**Inherited**: Phase 2.3 removed the four edge files. MIT remaining = 16 core
files. Patch map (verified): **patched** = kalloc.c, main.c, vm.c, trap.c,
exec.c, proc.c (UPC_* ifdefs / mmio_puts probes — the probe markers appear IN
the golden TTY and must carry over byte-for-byte); **pristine MIT** = string.c,
spinlock.c, sleeplock.c, sysproc.c, pipe.c, bio.c, file.c, log.c, sysfile.c, fs.c.

**Live-vs-dormant map (what the boot corpus actually exercises)**:
- **Boot-hot**: string (everywhere), spinlock (every print + kmem + cons),
  sleeplock (bio/log init + iinit paths), kalloc (freerange ~1900-page walk +
  probe markers), main (init sequence + instrumentation prints), bio/log/fs
  init functions, proc (procinit/userinit/scheduler/forkret), vm (kvminit
  skipped via UPC_SKIP_KVMINIT but uvm paths live in fork/exec plumbing),
  trap (trapinit/trapinithart).
- **Dormant on UPC** (BPF owns the syscall surface): sysproc/sysfile bodies,
  pipe, fs read/write walks, exec.c's ELF loader (BPF exec(7) stages from
  PROGRAM_TABLE), trap's usertrap/kerneltrap. They wake in later tranches /
  when the trap path flips — NEW gates needed then (usertests subset), not now.

**Tranche map** (each later tranche forges its detailed steps at start):
- **T1 (this session)**: string, spinlock, sleeplock, kalloc — leaf files,
  boot-hot, zero/known patches. Gates: smoke + 16-gate harness per file;
  goldens already exercise them intensively (every TTY byte transits string/
  spinlock; kalloc's markers are IN the goldens).
- **T2**: main.c (instrumented init sequence — the golden prints ARE its
  gate), bio.c, log.c, file.c (devsw/ftable init live; bodies dormant).
- **T3**: sysproc.c, sysfile.c, pipe.c, fs.c (iinit live; walks dormant).
  BlackMage: dormant bodies keep stock bounds verbatim-in-behavior.
- **T4 (the summit)**: vm.c, proc.c, trap.c, exec.c — heavily patched,
  scheduler live, needs NEW runtime gates (wake usertests subset; consider
  flipping trap→syscall() live as its own gate). Lore naming ceremony after
  the last MIT file leaves (ADR-081 Q5).

**Four-lens panel (T1)**:
- **Architect**: same proven mechanism as 2.3 (one OBJS line per file);
  sequencing string → spinlock → sleeplock → kalloc (dependency-light first;
  kalloc last because its probes touch the goldens most directly). Approved.
- **BlackMage**: string/memmove overlap semantics and strncpy padding are
  memory-safety load-bearing; kfree's bounds panic (`pa % PGSIZE`, `< end`,
  `>= PHYSTOP`) must not weaken; spinlock holding()/push_off discipline
  unchanged (deadlock detection). Approved.
- **Developer**: goldens + harness were re-proven green on this exact tree
  ~30 min ago (post-c10f5fb5 run) — behavior locked; per-file loop identical
  to 2.3. Approved.
- **Scientist**: translation-count tripwire generalizes: count MAY move if
  GCC allocates differently, but any drift demands a disassembly diff of the
  changed function before booting. H-checks below. Approved.

## PREFLIGHT HYPOTHESES (T1)

| # | Hypothesis | Verify via | If false |
|---|-----------|-----------|----------|
| H1 | Tree clean at `c10f5fb5`+, goldens 36 files, fs.img mtime unchanged since goldens | Step 1 | STOP |
| H2 | Baseline = post-c10f5fb5 harness (16 PASS) — no re-run needed | [DECIDE] | re-run harness |
| H3 | T1 patch set = kalloc.c only (mmio_puts markers + UPC_SKIP ifdefs); string/spinlock/sleeplock pristine | done (recon) | carry patches |
| H4 | sleeplock initlock name is the literal "sleep lock" (quirk) | done (read) | — |

## KNOWN FAILURES BASELINE

None. 16 PASS ALL GREEN post-`c10f5fb5`. gate_nway NWAY-FAIL pre-existing,
outside the harness.

---

## PHASE 0: PREFLIGHT (Steps 1-4)

**Goal**: base green, goldens valid. **Time**: ~5m.

- [ ] **Step 1** [B] ~1m: `git status --short && git log --oneline -1 && ls $GOLD | wc -l && ls -la $K/fs.img`
- [ ] **Step 2** [V]: clean; HEAD c10f5fb5+; 36 goldens; fs.img mtime = 18:22 (pre-goldens... fs.img was rebuilt 18:22, goldens captured after — mtime must simply be UNCHANGED since capture)
- [ ] **Step 3** [DECIDE]: baseline harness = the post-c10f5fb5 ALL GREEN run
  (same tree). Proceed without re-run. Override only if any T1 smoke diverges
  in a way that implicates the base.
- [ ] **Step 4** [V][GATE]: **PHASE 0 EXIT** — proceed.

## PHASE 1: T1-FILE 1 — string.c → ustring.c (Steps 5-18)

**Goal**: the kernel's memory/string primitives are ours. **Time**: ~35m.

- [ ] **Step 5** [W][CODE] ~10m: `$AD/ustring.c` — SPDX GPL; header notes the
  three subtle contracts: memmove's overlap test (`s < d && s + n > d`, copy
  backward), strncpy's pad-to-n via double-decrement loops, memcmp/strncmp
  compare as uchar. memcpy stays an alias of memmove ("placate GCC"). Types
  exact: `uint n` for mem*, `int n` for strncpy/safestrcpy/strlen.
- [ ] **Step 6** [W] ~1m: OBJS: `$K/string.o` → `$A/ustring.o` + comment line
- [ ] **Step 7** [B][BUILD] ~2m: `kbuild | grep -E "Translation|error"`
- [ ] **Step 8** [V]: builds; note count (drift → [D] Step 9 disasm diff before boot)
- [ ] **Step 9** [D]: `objdump -d` diff of changed functions vs stock build; classify before booting
- [ ] **Step 10** [B] ~30s: `kfile | grep string` → ustring.c only
- [ ] **Step 11** [V]: confirmed
- [ ] **Step 12** [B][S][TEST] ~3m: `smoke && echo F1-IDENTICAL`
- [ ] **Step 13** [D] ~5m: divergence → string funcs are everywhere; diff TTY
  char-by-char; prime suspects memmove backward path, strncpy pad. 2 attempts.
- [ ] **Step 14** [B][S][TEST] ~10m: full harness
- [ ] **Step 15** [V]: ALL GREEN + budgets exact (900,031 / 737,087)
- [ ] **Step 16** [W] ~2m: commit msg file (subject: `… kernel core: string (ustring.c)`)
- [ ] **Step 17** [C][B]: **COMMIT F1**
- [ ] **Step 18** [V][GATE]: tree clean, ALL GREEN.

## PHASE 2: T1-FILE 2 — spinlock.c → uspinlock.c (Steps 19-32)

**Goal**: mutual exclusion is ours. **Time**: ~35m.

- [ ] **Step 19** [W][CODE] ~10m: `$AD/uspinlock.c` — SPDX GPL; identical
  semantics: initlock, acquire (push_off → holding panic → __sync_lock_test_
  and_set spin → __sync_synchronize → lk->cpu), release (holding panic →
  cpu=0 → __sync_synchronize → __sync_lock_release → pop_off), holding,
  push_off/pop_off (noff/intena discipline, both panic strings). Header
  notes: the __sync builtins compile to libcalls under rv32i (no A extension)
  and land in OUR adapters/libgcc_stubs.c — the atomicity story is already
  Unheaded's; panic strings "acquire"/"release"/"pop_off"/"pop_off -
  interruptible" are diagnostics contracts.
- [ ] **Steps 20-31**: same loop as F1 (wire → build → count → kfile →
  smoke → harness), commit msg subject `… kernel core: spinlock (uspinlock.c)`
- [ ] **Step 32** [V][GATE]: **COMMIT F2**, tree clean, ALL GREEN.

## PHASE 3: T1-FILE 3 — sleeplock.c → usleeplock.c (Steps 33-46)

**Goal**: sleeping locks are ours. **Time**: ~25m.

- [ ] **Step 33** [W][CODE] ~5m: `$AD/usleeplock.c` — SPDX GPL; QUIRK KEPT:
  initsleeplock initializes the inner spinlock with the literal name
  "sleep lock" (not the caller's name — lk->name carries that); acquiresleep
  sleep-loop shape, releasesleep wakeup, holdingsleep pid check.
- [ ] **Steps 34-45**: same loop; commit subject `… kernel core: sleeplock (usleeplock.c)`
- [ ] **Step 46** [V][GATE]: **COMMIT F3**, tree clean, ALL GREEN.

## PHASE 4: T1-FILE 4 — kalloc.c → ukalloc.c (Steps 47-60)

**Goal**: the physical page allocator is ours — with its UPC instrumentation
carried over byte-for-byte (the markers are IN the goldens). **Time**: ~35m.

- [ ] **Step 47** [W][CODE] ~10m: `$AD/ukalloc.c` — SPDX GPL; carries VERBATIM-
  IN-BEHAVIOR: `extern void mmio_puts(...)` + "freerange enter\n"/"freerange
  exit\n" + the kfree-A/B/C one-shot probe ladder (static probed 0→1→2→3,
  same acquire/release interleaving), `#ifndef UPC_SKIP_KFREE_MEMSET` /
  `UPC_SKIP_KALLOC_MEMSET` blocks, kfree's triple bounds panic
  (pa%PGSIZE, <end, >=PHYSTOP), PGROUNDUP freerange walk, freelist push/pop.
- [ ] **Steps 48-59**: same loop; PLUS one extra gate: the golden TTY contains
  `freerange enter·kfree-A·kfree-B·kfree-C·freerange exit` — smoke identical
  proves the probe ladder byte-exact. Commit subject
  `… kernel core: kalloc (ukalloc.c). Tranche 1 COMPLETE`
- [ ] **Step 60** [V][GATE]: **COMMIT F4**, tree clean, ALL GREEN.

## PHASE 5: T1 SWEEP + SHIP (Steps 61-68)

**Goal**: full golden sweep, docs, tranche commit. **Time**: ~45m.

- [ ] **Step 61** [B][S][TEST] ~20m: 12-golden sweep (ls vs the phase22
  ush-era capture, rest vs goldens — the PHASE23 sweep script pattern)
- [ ] **Step 62** [V]: 12/12 IDENTICAL
- [ ] **Step 63** [B][COMPLIANCE] ~1m: SPDX 4/4; MIT four untouched (`git
  diff --stat HEAD -- string.c spinlock.c sleeplock.c kalloc.c` = 0); OBJS
  census: `$K/` = 12 files (main vm proc trap sysproc bio fs log file pipe
  exec sysfile)
- [ ] **Step 64** [SEC][R] ~5m: BlackMage T1 checklist — memmove overlap,
  strncpy pad, kfree triple bounds, holding/push_off discipline, no relaxed
  checks anywhere
- [ ] **Step 65** [DOC-UPDATE][W] ~10m: ADR-081 Progress bullet; master plan
  Phase 2.4 heading (T1 4/16 done, per-file checkboxes); session log
  `references/phase24-t1-2026-07-05.md`; T1-COMPLETE banner atop THIS plan;
  refresh `~/tmp/next.md` (T2 next: main/bio/log/file)
- [ ] **Step 66** [C][B]: **DOCS COMMIT**
- [ ] **Step 67** [B][S][TEST] ~10m: post-commit harness → ALL GREEN
- [ ] **Step 68** [V][GATE]: **TRANCHE 1 COMPLETE** — 12 MIT core files
  remain; memory updated; next session forges T2 steps from this doc.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1 — smoke diverges after ustring**: string funcs are ubiquitous — do NOT
debug in-boot first; disassembly-diff the four changed functions vs a stock
build (`git stash` → kbuild → objdump → unstash).
**A2 — freerange markers wrong after ukalloc**: probe ladder order/one-shot
state diverged — compare the static `probed` transitions to stock exactly.
**A3 — harness red, smoke green**: raced build; re-run once from quiet tree.
**A4 — anything else**: PHASE23 Appendix A applies verbatim (build variant,
`make clean` disaster, stats-move = STOP).

## APPENDIX B: T1 MATRIX

| Phase | File | Patched? | Est |
|-------|------|----------|-----|
| 1 | string.c | no | 35m |
| 2 | spinlock.c | no | 35m |
| 3 | sleeplock.c | no | 25m |
| 4 | kalloc.c | YES (markers+ifdefs) | 35m |
| 5 | sweep+ship | — | 45m |

Critical path = all of it, ~3h nominal.

## APPENDIX C: QUICK REFERENCE

Everything in PHASE23-KERNEL-EDGES.md Appendix C applies. T1 additions:
**strncpy quirk**: first loop's `n--` still decrements on the NUL iteration —
pad loop runs with the already-decremented n (pads to exactly n bytes total).
**sleeplock quirk**: inner spinlock named literal "sleep lock".
**kalloc markers**: `freerange enter` → kfree-A (first kfree, pre-acquire) →
kfree-B (post-acquire) → kfree-C (post-release) → … → `freerange exit`;
one-shot via static probed.

---

*Phase 2.4 Battle Plan — Forged 2026-07-05*
*4 Tranches. T1: 68 steps, 4 leaf files. The core starts coming home.*
*When the last MIT file leaves, it isn't xv6 anymore.*
