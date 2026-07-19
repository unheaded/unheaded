<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# EVOLUTION-1 Stage 1 — getpid proof-of-life (session log 2026-07-19)

Autonomous overnight churn (unattended, /loop + Marshal). Tree at start:
HEAD `5c8b5c8e` + 3 uncommitted doc edits (Stevie's). Base gate `== ALL GREEN ==`
(16 gates, ascend verifier 900031, Doom 737087). Goldens in `~/tmp/golden`
(12-boot, HEAD-derived) confirmed present.

Base paths (next.md's are relative to this): kernel/trap sources live under
`crates/xv6-mbc/adapters/` and `crates/xv6-mbc/upstream/kernel/`.

---

## S1.0 — sscratch entry-path fix (kernel-side, no eBPF) — ✅ COMPLETE + GREEN

**Goal:** make `uservec` save user regs to `p->trapframe` correctly when a real
trap is injected, replacing `li a0, TRAPFRAME` (VA `0x7FFFE000`, no UPC
translation → silent save-drop, the Phase 1.4–1.6 class of bug).

**Design deviation from next.md / EVOLUTION-1-TRAP-FLIP.md (justified):** both
docs prescribe the one-instruction swap `csrrw a0, sscratch, a0`. That idiom is
**mistranslated** by the rv32i→mbc translator: for CSRRW it emits "read old CSR
into rd; THEN store rs1 to CSR" (`crates/monad-mbc/src/translator.rs`, the
`0x73`/`funct3==1` arm, ~L1087–1095). With `rd==rs1` the read clobbers the
source before the store, so `sscratch` would end up holding the trapframe
instead of the user's a0 — a latent a0-corruption bug for every non-getpid
syscall. The prior authors couldn't have known without checking the translator.

**What shipped instead** (same goal, translator-safe, faithful to the
prepare_return half of the spec):
- `adapters/utrap.c` `prepare_return()`: `w_sscratch((uint64)p->trapframe)`
  each return (trapframe is a low PA from kalloc, inside RAM_MAP, so the
  memory-mapped CSR store at `0xF000 + 0x140*4 = 0xF500` translates). Exactly
  as next.md prescribes.
- `adapters/trampoline_mbc.S` `uservec`: split the swap into two well-translated
  CSR ops, using `mscratch` (0x340, otherwise unused on the UPC — grep-confirmed;
  its slot `0xF000+0x340*4 = 0xFD00`, word 0x3F40, is backed RAM adjacent to the
  live mepc slot, below the KBD device word 0x3FFF) to stash user a0:
  `csrw mscratch, a0` / `csrr a0, sscratch` ; later `csrr t0, mscratch`.
- `upstream/kernel/riscv.h`: added `w_sscratch` / `r_sscratch` helpers.

**Verification of the CSR memory model (why this is safe):**
- CSRs are memory-mapped generically at `0xF000 + csr*4` (ADR-067); the
  translator handles any csr index → LD/ST. sscratch (0x140) and mscratch
  (0x340) are ordinary backed RAM words (`Cpu::ram = vec![0; 0x300000]`).
- The BPF interpreter's own trap logic reads only mepc/mstatus/sepc/sstatus —
  never sscratch/mscratch — so writing them live during forkret is inert.

**Disasm delta** (RV, in `kernel/kernel.asm`):
- `0x23090` prepare_return: **+** `csrw sscratch,a5` (0x14079073); function
  body shifts +4 downstream (harmless).
- `0x27500` uservec: `csrw sscratch,a0` → `csrw mscratch,a0` (…14051073 → 34051073)
- `0x27504` uservec: **+** `csrr a0,sscratch` (0x14002573, replaces li a0,TRAPFRAME)
- `0x27548` uservec: `csrr t0,sscratch` → `csrr t0,mscratch` (…140022f3 → 340022f3)

**Translation count:** 7552 RV32I → **11769** MBC (was 11764; +5 from the CSRRS
expansion of `csrr a0,sscratch`). RV count unchanged (uservec −1 insn, the new
prepare_return w_sscratch +1). Doom untouched.

**Gates (all GREEN):**
- Kernel rebuild: clean (`Wrote … xv6-mbc.mbc`, 47076 bytes).
- 16-gate regression: **ALL GREEN**. ascend verifier 900031 (eBPF interpreter
  budget unchanged — kernel .mbc is data to it). Doom verifier **737087 exact**.
- 12-boot golden sweep: **ALL 12 BYTE-IDENTICAL** (TTY + stats). Counters match
  recorded values (multi forks=3 waitpid=2; echo_hello/uinit_echo tty_r=11;
  wc_README stats OK). Proves S1.0 is dormant — no observable boot-path change.

Files touched: `adapters/utrap.c`, `adapters/trampoline_mbc.S`,
`upstream/kernel/riscv.h` (all under `crates/xv6-mbc/`). Uncommitted (Stevie
commits).

---

## S1.1 — BPF getpid injection behind guard flag — ✅ SHIPPED (dormant, flag OFF)

**The gap the handoff glossed:** "INJECT the trap (SEPC=pc…)" needs the ecall's
*RV byte address* for SEPC, but the BPF model has **no MBC→RV reverse map** —
RV addresses only survive where an MBC operand already holds one (JMPR/CALLR).
The ecall's `op::SYSCALL` is emitted with empty operands.

**Fix (infrastructure):** the translator now carries the ecall's `rv_word`
((pc>>2)&0xFFFF) in the SYSCALL `imm16` (`translator.rs`, ECALL arm). The
&0xFFFF window matches the RV2MBC lookup, so 16 bits suffice for SEPC. This is
**inert** for every existing handler (none read a SYSCALL's imm16 — they
dispatch on a7) and for the kernel .mbc (the kernel issues no ecalls), so the
baseline is untouched.

**Injection (`main.rs`, getpid arm, behind `const GETPID_INJECT`):** the inverse
of the SRET handler — write SEPC=(imm<<2), SCAUSE=8, sstatus SPP←0/SPIE←SIE/
SIE←0, `priv_level=1`, then `pc = RV2MBC(STVEC)` with the kernel base (0), same
guarded lookup as MRET/SRET. `GETPID_INJECT=false` by default → LLVM
dead-eliminates it → verifier stays exactly 900031, 16 gates green, Doom 737087.

## S1.2 — getpid probe + injection experiment — ⚠️ ENTRY PROVEN, RETURN BLOCKED

Probe `user/getpidprobe.c` → `target/getpidprobe.mbc` (926 RV → 1584 MBC), built
with the new translator so its getpid ecall carries the rv_word. Prints
`GP: probe-start` / `GP: getpid-returned` / `GP: pid=N` (fixed strings, gate2
style). **Control** (flag OFF, getpid serviced in BPF): prints all three,
`GP: pid=0` (init). Correct.

**Injection experiment (flag ON):**
1. *First cut* (userret still `mv a0,a1`): injection vectors into the kernel
   cleanly — **no DEAD sentinel**, so the STVEC→RV2MBC→uservec path and the S1.0
   uservec entry (its `csrr a0, sscratch` reads the trapframe correctly at
   trap-entry) **work live**. But the CPU ends **stuck in supervisor mode**
   (priv=1): the trap never returns to user. Root cause = `userret`'s
   `mv a0, a1` (forkret's convention); on the injected fall-through a1 is a
   stale user value → registers restored from garbage.
2. *userret ← `csrr a0, sscratch`* (both return paths run prepare_return, which
   sets sscratch): the injected trap now reaches **priv=3** (returns to user) —
   progress — but this **breaks the live forkret boot** (all 12 goldens diverge,
   forks=0, corrupt sp). So `sscratch` does not read back as `p->trapframe` at
   *forkret's* userret, even though `uservec` reads it fine at trap-entry and
   nothing between prepare_return and userret writes CSR 0x140.

**Hypotheses falsified:** (a) satp-switch remaps the 0xF500 CSR read — no, the
trapframe restores right after also read at user-satp and work; (b) reading
sscratch *before* the `csrw satp` — also breaks forkret. Reverted userret to the
known-good `mv a0, a1`; **tree is GREEN**.

**ROOT CAUSE — CONFIRMED (this is the dual-books finding S1.2 was designed to
surface):** a temporary in-C bisection (32-bit compares, mmio_puts markers, all
removed) proved:
- `w_sscratch` stores correctly and the sscratch slot is **never clobbered** —
  it still equals what prepare_return stored, right up to forkret's userret
  (`SS-EQ-PRTF`). So the entry/return CSR plumbing is sound.
- BUT forkret's `myproc()->trapframe` **differs** from prepare_return's
  `myproc()->trapframe` (`TF-DIFF`). The two `myproc()` calls in the same launch
  return procs with **different trapframe pointers**.

So `prepare_return` configures the trap machinery (sscratch + kernel_satp/sp/
trap/hartid) against **one** proc's trapframe, while forkret hands `userret` a
**different** proc's trapframe via a1. `mv a0, a1` works because it uses the
proc actually being launched; sourcing the trapframe from sscratch fails because
it uses prepare_return's *stale* proc. The normal boot survives the mismatch
only because it never traps into uservec (BPF owns the syscall surface), so
prepare_return's stale setup is dead code there — the injected getpid is the
FIRST thing to actually consume it, which is why the bug hid until now.

This is precisely the `cpu->proc`-stale **dual-books** condition next.md's S1.2
warned about ("cpu->proc may hold stale init for a BPF-forked context"), and it
ties directly to the D1 note: `fork`/`exec`/`wait` dual-book `proc[]` →
"migrate LAST, behind a dedicated reconciliation sprint." Stage 1 has now
**falsified the entry-half footgun (S1.0 works live) AND positively identified
the dual-books as the concrete blocker** — both of Stage 1's stated goals.

**Fix path (for the reconciliation sprint, NOT a quick patch):** make the
kernel's `myproc()` return the same proc the BPF scheduler actually launched
(reconcile `cpu->proc` with `cpu.current_pid`), OR have `prepare_return` operate
on the caller's proc explicitly, OR have `userret` take the trapframe uservec
saved (via sscratch) AND fix myproc() so prepare_return and the launch agree.
The knot is that fixing userret alone is insufficient while myproc() is
two-faced — the trapframe identity itself is ambiguous.

**Latent hazard found:** the 64-bit CSR helpers (`r_sscratch`/`r_sepc`/… → `csrr
%0, csr` with a `uint64` dest) leave the **high 32 bits undefined** on rv32
ilp32e. Fine for 32-bit CSR use, but any full-`uint64` compare/use is unreliable
(it made an `r_sscratch()==(uint64)p->trapframe` diagnostic read `NE` on garbage
high bits alone). Worth auditing before trusting any 64-bit CSR value in C.

**Open blocker (the one Stage-1 unknown, now sharply characterized):** unify
trapframe provenance in `userret` across (i) forkret's explicit
`userret(satp, p->trapframe)` and (ii) an injected trap's fall-through (no
caller). sscratch is read correctly by uservec at trap-entry but NOT by userret
at forkret — the asymmetry is the crux. **[RESOLVED below — the root cause was
found: myproc()/trapframe dual-books, not a userret asm bug.]**

## MEASURE (S1.1/S1.2)
- **getpid verifier-delta = +5409** (flag OFF 900031 → flag ON 905440). The
  retired in-BPF handler is a single line, so the sign is **POSITIVE** — the
  injection costs far more BPF budget than servicing getpid in-BPF frees. Per
  next.md's rule ("net-zero/positive → keep (c) minimal"), this says the getpid
  migration is **not self-funding**; getpid is the cheapest, most stateless
  syscall (the scientist's biased-estimator caveat), so this lower-bounds the
  cost — it does NOT license migrating hot/stateful syscalls.
- **Cycle delta:** not cleanly measurable — the injected round-trip does not
  complete (return blocked), so no warm one-syscall round-trip figure yet.
- 16-gate harness GREEN, Doom 737087 exact, 12-boot goldens byte-identical
  throughout (flag OFF baseline). Translation 7552→11769 (S1.0); ascend verifier
  900031 unchanged with the flag off.

## STATE AT SESSION END (2026-07-19)
Tree GREEN. Uncommitted (Stevie reviews/commits — Claude does not commit):
- `crates/xv6-mbc/adapters/trampoline_mbc.S` — S1.0 uservec sscratch entry fix
  (+ userret S1.2 open-blocker note; userret itself is the known-good `mv a0,a1`).
- `crates/xv6-mbc/adapters/utrap.c` — prepare_return w_sscratch (S1.0).
- `crates/xv6-mbc/upstream/kernel/riscv.h` — w_sscratch/r_sscratch helpers.
- `crates/monad-mbc/src/translator.rs` — ECALL carries rv_word in imm16 (S1.1, inert).
- `ebpf/monad-cpu-ebpf/src/main.rs` — GETPID_INJECT flag (=false) + injection arm.
- `crates/xv6-mbc/upstream/user/getpidprobe.c` — S1.2 probe.
- Rebuilt artifact `crates/xv6-mbc/upstream/target/xv6-mbc.mbc` (kernel, 11769)
  and `target/getpidprobe.mbc` (probe). Plus the pre-existing 3 doc edits.
The injection is behind a default-OFF flag; flipping `GETPID_INJECT=true` +
rebuild + booting `getpidprobe.mbc` reproduces the experiment.

---

## CORRECTION / RE-DIAGNOSIS (2026-07-19, same-day follow-on) — DUAL-BOOKS FALSIFIED

The "dual-books / `TF-DIFF`" root cause recorded above is **WRONG**. It was
inferred from *comparison* results (`==`) that the session itself flagged as
corrupted by the 64-bit CSR-helper high-bits hazard. A clean re-probe that dumps
the actual 32-bit pointer/CSR **values** (temporary `mmio_puthex`, all reverted;
tree rebuilt byte-identical: 7552→11769 MBC, 47076 bytes) overturns both earlier
conclusions.

**Probe results (boot path, forkret → prepare_return → userret):**
- `Z:FRK id=0 p=001081dc tf=007be000` (forkret entry)
- `Z:PREK id=0 p=001081dc tf=007be000` (forkret, just before `prepare_return()`, re-reading `myproc()`)
- `Z:PRR id=0 p=001081dc tf=007be000` (inside `prepare_return`)
  → **`myproc()` is NOT two-faced.** `cpuid()`/`tp`, the proc pointer, and
  `p->trapframe` are IDENTICAL at all three sites. There is no dual-books
  divergence on the boot path. `TF-DIFF` is falsified.
- `Z:RT afterW=cafe1234 afterSEPC=cafe1234` → the `csrw`/`csrr sscratch` pair
  round-trips a sentinel perfectly, and `w_sepc` does NOT clobber sscratch.
- `Z:SEQ tf=007be000 v0=007be000 v1=007be000 v2=007be000` → replaying
  `prepare_return`'s **exact** CSR sequence (`w_sscratch(tf)` → r/mask/`w_sstatus`
  → `w_sepc`) inline at forkret entry keeps sscratch = tf through every step.
  `w_sstatus` is innocent too.
- **`Z:SS ss=000007ff a1=007be000`** → but the REAL `prepare_return`, called after
  `kexec`, stores `w_sscratch(0x7be000)` and sscratch reads back **`0x7ff`**.
  (Boot still reaches `$` because live `userret` uses `mv a0,a1`, ignoring sscratch.)

**RV disasm of `prepare_return` (`kernel/kernel.asm` 0x23208) is correct:**
`23278 lw a5,48(s0)` loads `p->trapframe` (0x7be000) into a5; a5 is the base for
the `kernel_satp/sp/trap/hartid` stores; `232a8 csrw sscratch,a5` writes it. RV
a5 = 0x7be000. So the C and RV are sound.

**CORRECTED ROOT CAUSE:** the return blocker is a **localized MBC
translation/execution defect on `prepare_return`'s `csrw sscratch,a5`**
(RV 0x232a8) — the stored value comes out `0x7ff` instead of `0x7be000`, even
though the identical `csrw sscratch` idiom round-trips everywhere else tested.
Same family as the S1.0 `csrrw rd==rs1` translator bug and the Phase-1.6 x17
spill-shadow (register-aliasing/codegen). **NOT** the `proc[]` reconciliation the
handoff assumed. `SS-EQ-PRTF` and `TF-DIFF` were both false-positives of the
64-bit CSR-helper hazard.

**Consequences for the decision record:**
- getpid's return was **never blocked on proc[] reconciliation.** The "migrate
  LAST / reconciliation sprint" framing in `next.md` for *getpid* is retracted.
  (fork/exec/wait may still be genuinely dual-booked — separate question, untouched.)
- The fix is a **localized codegen workaround** (mirror S1.0: avoid the fragile
  `csrw sscratch` in `prepare_return`, or fix the translator's handling of this
  case), not a multi-week architecture sprint. Cost estimate collapses accordingly.
- Strategic call is **unchanged**: verifier-delta = **+5409** (positive) still says
  getpid migration is not self-funding, so under D1 "keep hybrid (c) minimal" the
  getpid trap-flip stays **deferred**. This correction lowers the *cost* of ever
  finishing it; it does not change the *priority*.

**Next probe (for whoever finishes the flip):** dump the MBC bytecode for the
`csrw sscratch,a5` at RV 0x232a8 and compare against a round-tripping `csrw
sscratch` (e.g. the Z:SEQ sentinel) — the divergence is at the MBC level, below
the RV disasm. Then apply the S1.0-style idiom workaround and re-run the injection
experiment with `userret ← csrr a0,sscratch`.

**Tree state after this follow-on:** GREEN, all probes reverted, kernel .mbc
byte-identical to the S1.0/S1.1 build (11769 / 47076). No new uncommitted files
beyond the S1.0/S1.1 set already listed above.
