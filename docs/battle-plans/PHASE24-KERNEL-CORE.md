# PHASE 2.4 — KERNEL CORE BATTLE PLAN — Tranche Structure, T1 Detailed (68 Steps)

> **✅ TRANCHE 1 SHIPPED 2026-07-07** — commits `ab9ccc25` (ustring.c + this
> plan) → `6cfb000c` (uspinlock.c) → `285816e5` (usleeplock.c) → `75b0cc61`
> (ukalloc.c) + docs. The four boot-hot leaves out of the link; MIT remaining =
> 12 core files (T2 main/bio/log/file → T3 sysproc/sysfile/pipe/fs → T4 summit
> vm/proc/trap/exec). Gates: smoke + 16-gate harness ALL GREEN per file; final
> sweep 12/12 byte-identical (TTY+stats). Translation count invariant every
> build: 7,552 RV32I → 11,764 MBC. Verifier 900,031 (90.0%) EXACT, Doom
> 737,087. H1 was FALSE at preflight — a reboot had killed the /tmp goldens
> (third strike); recaptured all 12 from HEAD `c10f5fb5` instead of the A7
> stock-revert dance (2.2/2.3 equivalence already committed → HEAD is the
> valid baseline; retires the `ls` sh-size special case). ukalloc's probe
> ladder observed live in stock order: freerange enter · kfree-A · kfree-B ·
> kfree-C · freerange exit. Session log:
> `references/phase24-t1-2026-07-07.md`.

> **✅ TRANCHE 2 SHIPPED 2026-07-07 (same day)** — commits `e7495a95`
> (umain.c + the T2 detailed steps below) → `61073f2e` (ubio.c) → `03c9e4e0`
> (ulog.c) → `3ea6ecf2` (ufile.c) + docs. Init spine + FS support layer ours;
> MIT remaining = 8 (T3 sysproc/sysfile/pipe/fs → T4 summit vm/proc/trap/
> exec). Same gates, same invariants (count 7,552 → 11,764 every build;
> verifier 900,031 / 737,087 exact; 12/12 sweep byte-identical). Session log:
> `references/phase24-t2-2026-07-07.md`.

> **✅ TRANCHE 3 SHIPPED 2026-07-07 (same day, third tranche)** — commits
> `3d9534ba` (usysproc.c + the T3 detailed steps below) → `c766fcc7`
> (usysfile.c) → `1324fbfe` (upipe.c) → `8ef4e89c` (ufs.c) + docs. The last
> pristine-MIT files; u-files generated header + VERBATIM body via cat
> (zero transcription risk on ufs.c's 720 lines). **MIT remaining = 4: the
> T4 summit (vm/proc/trap/exec)** — heavily patched, scheduler live, needs
> NEW runtime gates (usertests subset) — own session recommended. Same
> invariants held (count 7,552 → 11,764; verifier exact; 12/12 sweep).
> Session log: `references/phase24-t3-2026-07-07.md`.

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

## TRANCHE 2 DETAILED STEPS (forged 2026-07-07 at tranche start, per the
## tranche-map contract) — main.c, bio.c, log.c, file.c (Steps T2-1..T2-40)

**Goal**: the init sequence and the FS support layer (buffer cache, redo log,
fd table) are ours. **Patch audit (from source read)**: main.c PATCHED —
14 mmio_puts markers (each IS golden TTY content), plicinit/plicinithart/
virtio_disk_init commented out with their exact comments, `volatile static
int started`; bio.c / log.c / file.c pristine MIT. **Live map**: main()
entirely boot-hot; binit live; initlog live via forkret→fsinit
(recover_from_log READS the log header from fs.img through bread → the
blk-ramdisk adapter); fileinit + devsw[] live (uconsole.c wires
devsw[CONSOLE] — the array LIVES in file.c); bodies of fileread/filewrite/
filealloc + log commit paths dormant (BPF owns fd I/O) — keep stock bounds
verbatim.

Same loop as T1 per file (write u-file → OBJS swap → kbuild → count
7,552→11,764 → FILE symbol → smoke vs golden → 16-gate harness → commit
`--no-gpg-sign`). Same stuck protocol (2 attempts → unwire, note, next).

- [ ] **T2-1..4** PREFLIGHT: tree clean at T1-docs HEAD; goldens 36 files
  (reboot check!); post-docs harness ALL GREEN = baseline.
- [ ] **T2-5..13** FILE 1 — `$AD/umain.c`: markers VERBATIM (after
  consoleinit/printfinit/kinit/kvminit/kvminithart/procinit/trapinit/
  trapinithart/binit/iinit/fileinit/userinit/started=1 + the three printf
  boot lines); commented-out plic/virtio calls kept as comments; `volatile
  static int started` + both __sync_synchronize(); scheduler() tail-call.
  Commit subject `… kernel core: main (umain.c)`.
- [ ] **T2-14..22** FILE 2 — `$AD/ubio.c`: binit's insert-at-head LRU
  construction; bget forward-cached / backward-LRU two-pass + "bget: no
  buffers" panic; holdingsleep panics in bwrite/brelse; brelse MRU re-link;
  bpin/bunpin. virtio_disk_rw extern resolves to blk-ramdisk.c (Phase 1.1
  adapter) — unchanged. Commit `… kernel core: bio (ubio.c)`.
- [ ] **T2-23..31** FILE 3 — `$AD/ulog.c`: "initlog: too big logheader"
  sizeof guard; recover_from_log boot path (read_head → install_trans(1) →
  clear); begin_op/end_op sleep-wakeup protocol + "log.committing" panic;
  log absorption loop in log_write + its two panics; install_trans's
  `recovering` printf kept verbatim. Commit `… kernel core: log (ulog.c)`.
- [ ] **T2-32..40** FILE 4 — `$AD/ufile.c`: devsw[NDEV] + ftable globals
  (devsw is referenced by uconsole.c consoleinit — link must keep it here);
  fileclose's struct-copy-then-release pattern; filestat copyout; fileread/
  filewrite dispatch incl. filewrite's MAXOPBLOCKS batching formula; panics
  "filedup"/"fileclose"/"fileread"/"filewrite". Commit
  `… kernel core: file (ufile.c). Tranche 2 COMPLETE`.
- [ ] **T2-SWEEP+SHIP**: 12-golden sweep 12/12; compliance (SPDX 8/8 total,
  MIT untouched, `$K/` census = 8: vm proc trap sysproc fs pipe exec
  sysfile); docs (ADR-081 bullet, master plan, session log
  `references/phase24-t2-<date>.md`, T2 banner here, next.md, memory);
  docs commit; post-commit harness ALL GREEN.

---

## TRANCHE 3 DETAILED STEPS (forged 2026-07-07 at tranche start) —
## sysproc.c, sysfile.c, pipe.c, fs.c (Steps T3-1..T3-40)

**Goal**: the syscall wrapper layer and the on-disk FS are ours — the last
pristine-MIT files. **Patch audit (grep + read)**: all four pristine (no
UPC_/mmio markers). **Live map**: fs.c iinit at main() + fsinit via forkret
(readsb FSMAGIC check + initlog) boot-live; EVERYTHING else dormant — the
BPF dispatch owns the user syscall surface, in-BPF fs_walk (ADR-078) owns FS
reads, pipe(4) is unimplemented. BlackMage rule: dormant bodies keep stock
bounds verbatim-in-behavior (they are T4's / the trap-flip's attack surface).

u-files generated header + VERBATIM body (mechanical cat — zero hand-copy
risk on fs.c's 720 lines). Same loop, same gates, same stuck protocol as
T1/T2.

- [ ] **T3-1..4** PREFLIGHT: tree clean at T2-docs HEAD; goldens 36; T2
  post-docs harness ALL GREEN = baseline.
- [ ] **T3-5..13** FILE 1 — `$AD/usysproc.c`: sys_sbrk lazy-path overflow
  guards (addr+n<addr, >TRAPFRAME) + SBRK_EAGER split; sys_pause killed()
  check. Commit `… kernel core: sysproc (usysproc.c)`.
- [ ] **T3-14..22** FILE 2 — `$AD/usysfile.c`: argfd bounds ladder; sys_exec
  MAXARG cap + uarg==0 terminator; link/unlink lock order + nlink
  accounting; create() reuse semantics. Commit `… kernel core: sysfile
  (usysfile.c)`.
- [ ] **T3-23..31** FILE 3 — `$AD/upipe.c`: fullness test nwrite==nread+
  PIPESIZE; sleep/wakeup pairing; last-end kfree; piperead's i=-1
  first-byte convention. Commit `… kernel core: pipe (upipe.c)`.
- [ ] **T3-32..40** FILE 4 — `$AD/ufs.c` (the big one, 720 lines): iinit +
  fsinit boot-live (readsb FSMAGIC + initlog); bmap bounds + panics;
  readi/writei overflow guards; namex hand-over-hand locking; dirlink
  DIRSIZ discipline. Commit `… kernel core: fs (ufs.c). Tranche 3 COMPLETE`.
- [ ] **T3-SWEEP+SHIP**: 12-golden sweep 12/12; compliance (SPDX 12/12
  total, MIT untouched, `$K/` census = 4: vm proc trap exec); docs (ADR-081
  bullet, master plan, session log `references/phase24-t3-<date>.md`, T3
  banner here, next.md, memory); docs commit; post-commit harness.

---

## TRANCHE 4 DETAILED STEPS (forged 2026-07-07 at tranche start) — THE
## SUMMIT: vm.c, proc.c, trap.c, exec.c (Steps T4-1..T4-44)

**Goal**: the last four MIT files leave the link. When T4 ships, the kernel
is 100% Unheaded-authored — it isn't xv6 anymore (naming ceremony queued for
Stevie/Lore, ADR-081 Q5 — Claude does not self-bless).

**Patch audit (read)**: all four PATCHED, Phase 1.2-1.7 provenance, carried
VERBATIM via the T3 cat method: vm.c `UPC_SKIP_KVMINIT` (page table
decorative — BPF translate_address authoritative; kvmmap calls skipped);
trap.c `UPC_FLAT_TRAMPOLINE` STVEC at uservec's link address; proc.c kstack
from kalloc (KSTACK VA past RAM_MAP window), `uvmcreate(p->pid)` per-pid pgd
(ADR-074), sched/forkret mmio markers + F1/F2 bisect chars (golden TTY
content), flat-trampoline userret(satp, p->trapframe) call (Phase 1.6);
exec.c kexec `$`/`%` sentinels + `KX:enter` + the MBC-userland non-ELF path
(epc=0, sp=0x500000-16, a0=1, a1=0 — Gate B/C provenance).

**The T4 gate upgrade — .mbc byte-identity**: because bodies are verbatim,
the correct summit gate is stronger than usertests: the translated
`target/xv6-mbc.mbc` artifact must be BYTE-IDENTICAL before and after each
swap (sha256). If it is, the swap is a pure provenance change — no runtime
gate can distinguish the kernels, by construction. (usertests-subset gates
remain queued for the day the code EVOLVES — the trap→syscall() flip etc. —
per the original tranche map; they gate change, not ownership.) Fallback if
the hash unexpectedly differs (e.g. path metadata leaks into the artifact):
objdump -d both kernel.elf builds and diff the disassembly; STOP if real
code drift appears. Smoke + 16-gate harness + 12-sweep still run per file.

- [ ] **T4-1..4** PREFLIGHT: tree clean at T3-docs HEAD; goldens 36; T3
  post-docs harness ALL GREEN; record baseline `sha256sum target/xv6-mbc.mbc`
  from a fresh kbuild at HEAD.
- [ ] **T4-5..14** FILE 1 — `$AD/uvm.c`: UPC_SKIP_KVMINIT block verbatim.
  Gate: .mbc sha unchanged + smoke + harness. Commit `… kernel core: vm
  (uvm.c)`.
- [ ] **T4-15..24** FILE 2 — `$AD/utrap.c`: UPC_FLAT_TRAMPOLINE stvec.
  Commit `… kernel core: trap (utrap.c)`.
- [ ] **T4-25..34** FILE 3 — `$AD/uexec.c`: kexec sentinels + MBC-userland
  path. Commit `… kernel core: exec (uexec.c)`.
- [ ] **T4-35..44** FILE 4 — `$AD/uproc.c` (742 lines, the last MIT file):
  kstack patch, uvmcreate(pid), sched/forkret markers, flat userret.
  Commit `… kernel core: proc (uproc.c). Tranche 4 COMPLETE — the kernel
  is 100% Unheaded-authored`.
- [ ] **T4-SWEEP+SHIP**: 12-golden sweep 12/12; compliance (SPDX 16/16, MIT
  sixteen untouched, `$K/` census = 0 C objects); docs (ADR-081 bullet +
  the "not xv6 anymore" milestone, master plan Phase 2.4 ✓ COMPLETE,
  session log `references/phase24-t4-<date>.md`, T4 banner, next.md,
  memory); docs commit; post-commit harness; **naming ceremony handoff to
  Stevie/Lore** (ADR-081 Q5 stays open until blessed).

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
