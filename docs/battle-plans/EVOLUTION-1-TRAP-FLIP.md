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

### Stage 2 — MIGRATION LADDER (big; needs D1=(a)/(c), own battle plan)
Retire BPF handlers one at a time, cheapest-state-first (getpid → uptime/
pause → write → read → fstat/dup/close → open (fs_walk hand-off) → fork/
exec/wait (PROC_TABLE hand-off) → NEW syscalls sbrk/pipe/kill (kernel paths
exist since T3/T4, were never BPF-implemented — these ARRIVE rather than
migrate)). Each retirement: corpus green + freed-verifier-insns recorded.
usertests subset gates enter here, growing with each arrival. Full Warmonger
plan + four-lens panel at kickoff; do NOT forge steps now.

## Non-goals of this brief
Renaming (Q5 ceremony is Stevie's, pre-requisite to nothing here); Phase 2.5
reconciliation (ADR-081 Q4, separate brief if wanted); performance work on
the translated path.
