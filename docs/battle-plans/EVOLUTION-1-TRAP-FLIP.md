# EVOLUTION-1 — Waking the Owned Kernel's Syscall Path (Decision Brief + Gate Ladder)

**Date**: 2026-07-07 (forged same day Phase 2.4 completed; execution GATED on
Stevie — every stage past Stage 0 is eBPF spend under the freeze)
**Status**: BRIEF — no code in this doc's scope. Detailed steps get forged
per-stage after the [DECIDE]s below.
**Context**: Phase 2.4 made the kernel link 100% Unheaded-authored. This is
the first *evolution* question: the dormant kernel syscall path
(uservec → usertrap → syscall() → SRET) is entirely ours (trampoline_mbc.S,
utrap.c, usyscall.c, uproc.c) but has NEVER executed — the BPF ecall
dispatch has owned the user syscall surface since Phase 1.6. The Phase 2.4
tranche map deferred usertests-style runtime gates to exactly this moment:
they gate change, not ownership.

## Ground truth (verified by read, 2026-07-07)

- `ebpf/monad-cpu-ebpf/src/main.rs` `op::SYSCALL` handler services user
  ecalls IN-BPF: a7(r1) = nr, xv6 family = 14 handlers (fork 1, exit 2,
  wait 3, read 5, exec 7, dup 10, getpid 11, mknod 17, close 21, open,
  write, fstat, pause, yield). **Not implemented**: sbrk(12), pipe(4),
  chdir(9), kill, link, unlink, mkdir, sleep — exactly ush's dead-syscall
  list.
- The CPU never vectors to STVEC for user ecalls. Kernel trapframe
  save/restore has only been exercised on the RETURN half (forkret →
  userret under UPC_FLAT_TRAMPOLINE, Phase 1.6); the ENTRY half (uservec
  saving user regs to p->trapframe) is unproven on UPC memory reality.
- State ownership is split: BPF maps (FD_INODE_MAP, PROC_TABLE, KBD ring,
  fs_walk) are authoritative for fd/proc/FS state; the kernel's ftable /
  ofile / inode cache initialize at boot but hold no live user state.
- Verifier budget: ascend sits at **900,031 / 1,000,000 (90.0%)** with the
  INT-0x80 dead-elim trick already spent. The xv6 SYS_* dispatch block
  (~main.rs:2151-3000+) is a large fraction of that; retiring handlers
  frees budget, adding anything costs it.
- `upstream/user/usertests.c` = 3,248 lines; it leans hard on sbrk, pipe,
  kill, fork-bombs — full usertests is unreachable under the current BPF
  dispatch REGARDLESS of the flip (missing handlers), and unreachable
  after a flip until the kernel is authoritative over memory (sbrk →
  growproc → kalloc + vmfault against the decorative page table).

## The trade, stated honestly

**For the flip**: authenticity (syscalls run through our kernel, the code
Phase 2.4 took ownership of stops being dormant); verifier budget RELIEF
(every retired in-BPF handler frees insns — the headroom Epic 1.3.2 needs);
usertests becomes meaningful; the kernel stops being a boot-time-only
artifact.

**Against**: cycle cost per syscall explodes (one BPF handler → hundreds/
thousands of translated MBC instructions through uservec/usertrap/syscall/
SRET) — trigger budgets and interactivity change materially; the entry-half
trapframe save is unproven (same silent-store-drop class of bug that
Phase 1.4-1.6 fought); fd/proc state migration (BPF maps → kernel structs)
is a real architectural migration with a long tail; and the BPF-owns-
syscalls design is arguably the UPC's IDENTITY (the plumbing doctrine —
in-BPF services are the product, the kernel is the guest). The flip may be
a falsification gate, not a destination.

## [DECIDE] — Stevie's calls, in order

1. **D1**: Is the steady-state goal (a) kernel-authoritative syscalls
   (authenticity), (b) BPF-authoritative forever (the flip is only ever a
   proof-of-life gate), or (c) hybrid — hot syscalls in BPF, cold/complex
   ones in-kernel? *No default — this is architecture.*
2. **D2**: Unfreeze eBPF spend for Stage 1 (small, measurable, reversible)?
3. **D3**: If D1=(a) or (c): sequence Stage 2 against Phase 2.5 / Track 1.

### [DECIDED 2026-07-17] — Stevie's calls (architect + scientist consulted)

Both `unheaded-architect` and `unheaded-scientist` were consulted on D1 and
converged independently on **(c)**.

- **D1 = (c) HYBRID.** Hot + BPF-state-coherent syscalls STAY in BPF; cold /
  complex / memory-bound syscalls MIGRATE to the owned kernel. The split, by
  frequency × state-coherence:
  - **STAY IN BPF** (identity + hot path): `write`/`read` console I/O (per-byte,
    the live interactive faucet; devsw already treats console as a device);
    `fork`/`exec`/`wait` (hottest process ops AND the dual-books
    PROC_TABLE-vs-`proc[]` split-brain — migrate LAST, possibly never, and only
    behind a dedicated `proc[]`-reconciliation sprint).
  - **MIGRATE TO KERNEL** (authenticity + verifier relief): `getpid`/`pause`/
    `yield` (Stage 1); the **FS-metadata bundle** `open`/`close`/`dup`/`fstat`
    + file read/write path (`fs_walk` → `readi`/`namex`) migrated as ONE unit
    (splitting it = FD-table split-brain); `sbrk` (ARRIVES kernel-only — memory
    authority, usertests' hardest dep); the never-implemented `pipe`/`kill`/
    `mkdir`/`unlink`/`link`/`chdir`/`sleep` (ARRIVE in kernel, cheap authenticity).
  - **Architect's dealbreaker (would flip to (b))**: only if the owned kernel
    NEVER needs memory authority — i.e. if real `sbrk`/usertests is not a genuine
    destination. ADR-081 funds Unheaded Linux as that destination, so (c) holds.
  - **How far (c) reaches is NOT decided in advance.** Per the Scientist: Stage 1
    getpid is a falsification experiment whose result is independent of the
    steady-state declaration. The **sign of the getpid verifier-delta** tunes the
    reach: net-negative → the migration ladder is self-funding (push toward (a));
    net-zero/positive → keep (c) minimal, migrate only where authenticity demands.
  - **Scientist's caveat (do not over-read Stage 1)**: getpid is a *biased
    estimator* — the cheapest, most stateless syscall by design. A green /
    correct-pid result says nothing about the hot, *state-mutating* syscalls
    (fork/exec) that actually decide migration cost. Stage 1 falsifies the
    entry-half footgun and observes the dual-books split; it does NOT license
    generalizing to the write side.

- **D2 = YES — unfreeze for Stage 1.** Small, guard-flagged, reversible; net-zero
  verifier expected (one injection block vs one retired handler). Stage 1 is both
  the first rung of (c) and the experiment that tunes it.

- **D3 = Stage 1 FIRST, then reassess.** Stage 2 (the migration ladder) is a
  separate Warmonger battle plan, gated on the Stage 1 verifier-delta measurement
  + a `proc[]`-reconciliation prerequisite before any fork/exec/wait migration.
  It is NOT sequenced ahead of the parallel Debian/Yggdrasil track (ADR-081 Q4),
  which stays Age-2-deferred.

### D1 reach re-read (2026-10-28) — DECIDED (a), E2 2026-10-28

**Status: DECIDED 2026-10-28 (Stevie, E2): (a), push toward kernel-authoritative syscalls, rung by rung, stopping at the first rung whose freed instructions are below zero.** (S-ASCEND-2 Step 56; M-221, decision 45.) Written because the rule
above names the sign of the getpid delta as the input, and the sign has changed. The
recommendation below is not a decision.

**The rule as written (2026-07-17).** "The sign of the getpid verifier-delta tunes the
reach: net-negative → the migration ladder is self-funding (push toward (a));
net-zero/positive → keep (c) minimal, migrate only where authenticity demands."

**The two measurements.** Ascend object, `UPC_VERIFIER_STATS=1`, verified insns.

| Measured | Build | flag OFF | flag ON | ON − OFF |
|---|---|---|---|---|
| 2026-07-19, Stage 1 S1.1 (`references/evolution1-stage1-2026-07-19.md`) | July nightly + bpf-linker, July host kernel | 900,031 | 905,440 | **+5,409** |
| 2026-10-23 S5 Step 97, re-measured 2026-10-28 S-ASCEND-2 Step 37 | October nightly + bpf-linker, host kernel 6.17.0-41 | 926,542 | 911,319 | **−15,223** |

- **Attribution: toolchain and kernel verifier, not source.** `monad-cpu-ebpf` logic is
  unchanged since `8de08368` (the Stage 1 commit) apart from `233b6451` (clippy
  annotation, no rewrite); see `references/s5-upc-wire-log.md` §Phase 0. The OFF build
  moved +26,511 and the delta moved 20,632 with no logic change.
- **The delta is not the handler's size.** The retired in-BPF getpid handler is one
  line. A −15,223 swing is how the verifier's path exploration changed with the
  injection block present, not insns the handler held. Verifier counts are not
  additive per handler, and the sign of one pair does not predict the next pair.
- **Headroom today.** OFF: 73,458 below 1,000,000, 23,458 below the 950,000 working
  bar. ON: 88,681 and 38,681.
- **Cycle cost** (not budget): one getpid round trip through the kernel costs +1,348
  MBC instructions (S5 Step 101). The verifier delta says nothing about this.

**What "self-funding" means concretely.** A rung retires one BPF handler behind a
guard const (the `GETPID_INJECT` shape) and routes its syscall number into the
injection block that Stage 1 already pays for. The rung is self-funding when
OFF − ON ≥ 0 for that rung's guard, measured as a pair on one toolchain and one host
kernel in one session. Freed insns are measured per rung, never estimated from
handler size. An arrival (a syscall BPF never implemented) has no handler to retire;
its cost is the added dispatch, recorded the same way.

**Scientist caveat, restated.** getpid is the cheapest, stateless syscall. A negative
getpid delta proves that the trap path's fixed cost fits the budget today. It proves
nothing about the cost of fork/exec/wait, the FS-metadata bundle, or migrating state
out of BPF maps. The rule is therefore applied per rung, not once from getpid.

**New evidence (S-ASCEND-2 Phase 1, 2026-10-28).** The named next probe ran
(`user/getpidfork.c`, log `references/s-ascend-2-log.md` §Phase 1). With the flag ON,
init's getpid returns 1 and a BPF-forked child's getpid returns **−1**; `fork()`
returns 1 in the parent under both states. The owned kernel's `proc[]` holds only
init: user `fork()` is serviced in BPF, and only `userinit` calls `allocproc`. The
dual-books row of Appendix B is not only a numbering mismatch: the kernel has no row
at all for any process but init. Where the −1 originates was not isolated.

Consequence for reach: sh is a BPF-forked child of init, and every program it runs is
a child of sh. Any syscall routed into the kernel from those processes runs against a
process the kernel does not have. A kernel row per BPF fork is therefore a
prerequisite of every stateful rung (write onward) and of the arrivals, not only of
the fork/exec/wait hand-off, and it is needed under either E2 answer. Two levels:

- **Shadow row** (Stage 2 rung 0): on each BPF `SYS_FORK` the kernel gets a
  `struct proc` for the child (pid per E1, trapframe and pagetable matching the
  PROC_TABLE slot), the injected trap resolves `myproc()` to the trapping slot's row,
  and exit/wait free it. BPF stays authoritative over scheduling and lifetime. This
  also closes Stage 1: the flag cannot default ON while a child's getpid is −1.
- **Hand-off** (the fork/exec/wait rung): `proc[]` becomes the authority; PROC_TABLE
  derives from it or retires.

**Recommendation (Computermancer + Scientist, pending E2).** Push toward (a) in
Stage 2 ordering, cheapest-state-first per the ladder below, with three conditions:
1. Shadow rows per BPF fork land first (rung 0), before any stateful rung.
2. Each rung is gated on corpus 12/12 SAME, harness ALL GREEN, freed insns ≥ 0
   (ON/OFF pair, same toolchain), and its cycle cost recorded.
3. A rung with freed insns < 0 stops the push there: that rung and the later
   migration rungs stay in BPF unless authenticity requires them, which is (c) for
   the remainder. Arrivals stay kernel-only under both answers.

The skeleton is `EVOLUTION-1-STAGE-2-skeleton.md`. Note for E2: D1 (c) as decided on
2026-07-17 already places getpid/pause/yield and the FS-metadata bundle in the kernel.
The plan's "(c) minimal" (arrivals only) is narrower than that decision.

## Gate ladder (each stage independently shippable + reversible)

### Stage 0 — no eBPF, no unfreeze needed (~1 session)
State-ownership matrix: per syscall, which BPF map vs kernel struct holds
truth today and what migrating it means. Static audit that the kernel-side
chain links complete (it does — all ours since T4) + a written trace of the
UNPROVEN entry half: what uservec writes, to which VAs, and whether the
UPC_FLAT_TRAMPOLINE posture covers the save path the way it covers userret.
Deliverable: appendix to this doc. Falsifies "the entry half is a footgun"
on paper before any spend.

### Stage 1 — PROOF-OF-LIFE (small eBPF spend; needs D2) (~1 session)
Route exactly ONE stateless syscall — getpid(11) — through the kernel: the
BPF SYSCALL handler, for nr==11 under ascend, injects the trap (SEPC=pc,
scause=8, priv 3→1, pc=STVEC) instead of servicing. Kernel runs
uservec → usertrap → syscall() → sys_getpid → usertrap_ret → SRET.
Gates: TTY byte-identical on the full 12-boot corpus (getpid isn't in the
corpus path — add a tiny getpid probe program modeled on gate2.c); cycle
count delta measured; verifier delta measured (expect ~net-zero: one
injection block vs one retired handler); 16-gate harness ALL GREEN; Doom
untouched (737,087 exact). Rollback = one guard flag. This falsifies the
entry-half question for real, at minimum blast radius.

**Stage 1 status (S5, 2026-10-23 — `docs/battle-plans/S-UPC-WIRE-COMPUTE-battle-plan.md`
Phase 3): trap path DONE; flag stays OFF.**
- **S1.2 root cause** was the translator, not the kernel: its CSR arm used r13 (= a5) as an
  RMW temp, so `prepare_return`'s `csrr a2,satp` overwrote a5 (p->trapframe) with satp
  (0x7ff) right before `csrw sscratch,a5`. Fixed in `crates/monad-mbc` (ADR-067 Decision 2
  implementation note). `userret` now restores from `csrr a0, sscratch` (not `mv a0, a1`);
  the 12-boot corpus is byte-identical with it.
- **Round trip proven** with `GETPID_INJECT = true`: getpidprobe prints
  `GP: getpid-returned`, ends at priv=3 at the same exit PC as the flag-OFF control,
  rom_fault=0, no DEAD sentinel. Cost: +1,348 MBC instructions for the round trip
  (insn_count 5,085,866 vs 5,084,518).
- **Verifier**: flag ON 911,319 vs flag OFF 926,542 (−15,223 on this toolchain; July
  measured +5,409). Harness ALL GREEN and 8/8 corpus SAME with the flag ON; Doom 761,580
  unchanged either way.
- **Observed pid differs**: flag ON prints `GP: pid=1`, flag OFF `GP: pid=0`. The kernel
  answers from its own proc[] (xv6 `allocpid` gives init pid 1); BPF answers with its slot
  index (0 = init), which gate2 relies on. That is the dual-books row above showing up on
  pid identity, not on the trap path. By read (not yet run): a BPF-forked child would also
  enter the kernel with `cpu->proc` = init and get pid 1.
- **Decision (plan Step 102)**: the recommendation was "ship `true` iff the ON/OFF probe
  lines match"; they do not, so `GETPID_INJECT` stays `false`. Rollback/forward is still
  one const. Turning it on needs a pid-identity decision first (kernel numbering vs BPF
  slots, i.e. proc[] reconciliation — out of S5 scope). Next probe: a child process that
  calls getpid with the flag ON.
- **Child probe run (S-ASCEND-2, 2026-10-28)**: with the flag ON the child's getpid
  returns −1, not 1: the kernel has no process for a BPF-forked child. See §D1 reach
  re-read.

### Stage 2 — MIGRATION LADDER (big; needs D1=(a)/(c), own battle plan)
Retire BPF handlers one at a time, cheapest-state-first (getpid → uptime/
pause → write → read → fstat/dup/close → open (fs_walk hand-off) → fork/
exec/wait (PROC_TABLE hand-off) → NEW syscalls sbrk/pipe/kill (kernel paths
exist since T3/T4, were never BPF-implemented — these ARRIVE rather than
migrate)). Each retirement: corpus green + freed-verifier-insns recorded.
usertests subset gates enter here, growing with each arrival. Full Warmonger
plan + four-lens panel at kickoff; do NOT forge steps now.
Skeleton (phases, gates, prerequisites; no steps): [EVOLUTION-1-STAGE-2-skeleton.md](EVOLUTION-1-STAGE-2-skeleton.md)
(2026-10-28, pending E2).

## Non-goals of this brief
Renaming (Q5 ceremony is Stevie's, pre-requisite to nothing here); Phase 2.5
reconciliation (ADR-081 Q4, separate brief if wanted); performance work on
the translated path.

---

## APPENDIX — STAGE 0 DELIVERABLE (completed 2026-07-07, read-verified)

### A. The entry half IS the predicted footgun (confirmed by read)

`adapters/trampoline_mbc.S` uservec still executes `li a0, TRAPFRAME` and
saves all user registers to the high VA 0x7FFFE000 (lines 37-63), then
loads the kernel sp from `4(a0)`. Under UPC, translate_address has no entry
for that VA: **every register save silently drops, and `lw sp, 4(a0)`
returns 0** — instant kernel-stack corruption on the first injected trap.
Only the userret half carries the UPC_FLAT_TRAMPOLINE patch (a1 =
p->trapframe low PA, Phase 1.6). Stage 1 therefore has a hard kernel-side
precondition:

**Pre-step S1.0 (kernel-side only, no eBPF)**: adopt upstream xv6's modern
sscratch pattern — usertrapret stores p->trapframe (low PA from kalloc,
inside RAM_MAP, so stores translate fine) into sscratch; uservec becomes
`csrrw a0, sscratch, a0` (swap user-a0 ↔ trapframe pointer), saves at
20(a0)…, then stores the swapped-out user a0 via a second csrr. CSRs are
memory-mapped at 0x000F000+ so csrrw already translates. This can land
green under the existing gates (dormant code, .mbc byte-identity WON'T hold
— it's a real change — so it ships under smoke + harness + sweep instead,
with the disasm diff recorded). It stays unexercised until Stage 1's
injection, but it converts Stage 1 from "two unknowns" to "one".

usertrapret (utrap.c) must also populate kernel_sp/kernel_trap/
kernel_satp/kernel_hartid in the trapframe each return — stock does; verify
the dormant body still does after the sscratch change.

### B. State-ownership matrix (who holds truth today)

| Domain | BPF-side truth | Kernel-side today | Migration meaning |
|--------|----------------|-------------------|-------------------|
| fd table | FD_INODE_MAP {inum, offset} per fd | ftable/ofile init empty, never populated post-boot | kernel open()/dup() populate ofile; fs_walk retires per-syscall |
| processes | PROC_TABLE (8 slots, pgd, saved state); BPF SYS_fork clones child state directly | proc[] knows ONLY init (userinit); BPF-forked children have NO kernel proc entry — **dual books** | kfork becomes authoritative; PROC_TABLE derives from proc[] (or retires); the ADR-074 pgd hook re-anchors |
| console in | BPF KBD ring feeds read(5) directly | cons.buf dormant (consoleintr never fires) | consoleintr wakes; KBD ring becomes the interrupt source |
| console out | SYS_write emits to TTY MMIO per byte | consolewrite dormant | write(fd=1) routes devsw → uconsole → console-mmio |
| FS reads | in-BPF fs_walk over fs.img in RAM_MAP (ADR-078) | bcache/inodes live at boot only (fsinit/iinit) | readi/namex become the read path; fs_walk retires |
| memory/sz | none (sbrk unimplemented) | growproc/vmfault exist, never called | sbrk ARRIVES (new capability, no migration) — usertests' hardest dependency |

The **dual-books processes row is the sleeper risk**: any Stage 2 sequencing
must reconcile proc[] before fork/exec/wait migrate — until then the kernel
scheduler only ever schedules init's descendants it knows about (today:
none). getpid (Stage 1) dodges this: myproc() inside the injected trap
context depends on cpu->proc, which is only valid for kernel-known procs —
**so even Stage 1-getpid must verify what cpu->proc holds when the trapped
process is BPF-forked sh, not init**. If it holds stale init, sys_getpid
returns the WRONG pid — which is itself a perfect falsification gate:
expected-wrong-answer proves the dual-books diagnosis; expected-right
proves more state syncs than believed. Either result is informative; the
gate should assert the OBSERVED value and document it.

### C. Corrected Stage 1 scope

Stage 1 = S1.0 (uservec sscratch fix, kernel-side, green under standard
gates) + S1.1 (BPF injection for nr==11 behind a feature guard) + S1.2
(getpid probe program + dual-books observation gate). Verifier delta
expectation unchanged (~net-zero). Rollback: S1.1 guard flag; S1.0 can stay
(dormant-but-correct either way).
