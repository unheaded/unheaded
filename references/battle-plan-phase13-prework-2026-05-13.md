# Battle Plan — Phase 1.3 PRE-WORK (xv6 process model bring-up kickoff)

**Codename:** ASCEND-LINUX Phase 1.3 PRE-WORK
**Authored:** 2026-05-13
**Predecessor:** Phase 1.2 IMPL CLOSED at `b9572c26` (uvmcreate per-pid pgd allocator) + `references/phase12-impl-complete-2026-05-13.md`
**Parent plan:** `references/battle-plan-ascend-linux-2026-05-08.md` §4.3 (items 33-36)
**Status:** READY TO EXECUTE — kickoff phase, AP-1..AP-6 unlock ADR-075 pair-call then IMPL plan authoring

---

## 1. Why this PRE-WORK exists

Phase 1.3 advances xv6 past the banner into a real process model: SYS_FORK, SYS_EXECVE, SYS_WAITPID, SYS_EXIT, SYS_SCHED_YIELD. Multiple non-trivial decisions need to land in an ADR before IMPL can execute confidently. This kickoff mirrors the Phase 1.2 pattern that worked: PRE-WORK enumerates the open questions as AP-N artifacts, ADR-075 captures the decisions via pair-call, then a separate IMPL plan executes against the decided substrate.

### Predecessor substrate already in place

- **PROC_TABLE [u32; 21]** in BPF (`ebpf/monad-cpu-ebpf/src/main.rs`) + host emulator. Slot[20] = `page_dir_base`.
- **`SYS_FORK` + `SYS_VFORK`** assign per-pid pgd via `pgd_base_for_pid(child_pid)`.
- **`scheduler_context_switch`** saves/loads slot[20]; feature-gated hook `phase12_option_a_on_context_switch` wired.
- **`uvmcreate(int pid)`** in xv6 returns pgd from `0x00F00000+pid*0x1000` for pid 0..3, kalloc fallback otherwise (commit `84fc38d1`).
- **2 falsification tests pass** on host emulator: `phase12_fork_assigns_distinct_pgd_per_child`, `phase12_context_switch_save_restore_page_dir_base`.
- **BPF verifier:** 8.46% of 900K budget (no recent delta).

### What blocks Phase 1.3 IMPL right now

xv6 boots, emits `xv6 booting...\n`, halts at MRET fall-through in 383 insns. The kernel has not yet reached `main()` → `userinit()` → `scheduler()`. Until xv6 advances into the scheduler, none of the page-table or process-model work can be exercised live. Phase 1.3 must FIRST advance the boot path past the MRET trap, THEN wire the process model.

---

## 2. Open decisions (each AP-N produces a piece of the ADR-075 pair-call dossier)

### D-1 — Process slot count: 4 vs 16

Parent plan item 34 explicitly flags this. PROC_TABLE is currently `[u32; 21]` per slot, with `MAX_PROCESSES = 4`. xv6's shell + `init` needs 2 slots minimum. Adding `ls`/`cat`/`echo`/`uname`/`ps` pushes us toward 7-8. Decision: grow to 8 (next power of 2 above current need), 16, or stay at 4 with explicit gate.

**Cost:** BPF map size + verifier insn budget delta. Each additional slot is 21*4 = 84 bytes + 1 iteration in any loop over PROC_TABLE.

**AP-1 deliverable:** verifier-budget delta projection for 4 → 8 → 16 slots, signed off by Architect.

### D-2 — MRET fall-through fix

Phase 1.1 SHIP halts at MRET fall-through 383 insns in. Phase 1.3 needs xv6 to advance to `scheduler()` (proc.c:521) — that requires the `mret` after the M-mode setup in `start_mbc.c` to actually transfer control to `main()` in S-mode.

**Hypothesis:** the host translator emits MRET as opcode `0x47` (per ADR-067) but the eBPF interpreter's MRET handler may not be wiring MEPC → PC + MSTATUS.MPP → priv_level correctly.

**AP-2 deliverable:** reproduction case (single-instruction MBC program: `MRET only`, observe PC + priv transition). If MRET is broken in the interpreter, fix BEFORE Phase 1.3 IMPL; otherwise the bug is in `start_mbc.c`'s MEPC setup.

### D-3 — Trapframe collapse strategy

Parent item 35: xv6's `kernel/trampoline.S` does M→S→U trap via SATP+TRAPFRAME swap. Our UPC ABI v1 says "save MbcCpuState, jump to handler, IRET restores." Decision: keep xv6's trampoline + emulate SATP/TRAPFRAME in the eBPF interpreter, OR replace the trampoline with a thin MBC-native syscall stub.

**AP-3 deliverable:** prototype the MbcCpuState save/restore around a single syscall (e.g., SYS_EXIT from a userland stub) and compare against the xv6 trampoline path. Pick whichever has lower verifier cost AND fewer LoC in xv6 adapters.

### D-4 — `init` program build pipeline

Parent item 36: a static-linked C program that opens `/dev/console`, prints `init starting`, forks `sh`, waits. We need a userspace build pipeline parallel to the kernel pipeline (rv32i-to-mbc translator pointed at userland binaries).

**Open sub-questions:**
- Where in the address space does `init` land? (Phase 1.2 reserved 0x00F00000-0x00F03FFF for per-pid pgd; userland code starts where?)
- xv6 userland tree (`user/`) builds statically with `libc-mbc` — do we vendor xv6's own minimalist libc, or stub it out per-syscall?
- Does the translator handle userland RV32I differently from kernel-mode (e.g., S-mode vs U-mode CSR access)?

**AP-4 deliverable:** address-space sketch update to `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` covering userland regions; decide on libc strategy.

### D-5 — SYS_FORK / SYS_EXECVE wiring

Parent item 33: bridge xv6's hand-rolled scheduler to our existing L4c scheduler via SYS_FORK (2), SYS_EXECVE (11), SYS_WAITPID (7), SYS_EXIT (1), SYS_SCHED_YIELD (158).

**Open:** xv6's `proc.c::fork()` does its own slot allocation + page-table copy. Our L4c SYS_FORK in eBPF does the same. Which one is authoritative? Bridge layer or replace xv6's path?

**AP-5 deliverable:** call-graph diff between xv6's fork() and our SYS_FORK BPF handler; choose which to keep, document the bridge in `syscall_shims.S`.

### D-6 — Verifier budget pre-IMPL baseline

Phase 1.2 took us from 8.43% → 8.46%. Phase 1.3 will add process-table operations, fork-handler code, and scheduler-iteration loops. Each is potentially expensive in BPF insn budget.

**AP-6 deliverable:** instrument every NEW BPF helper Phase 1.3 will introduce (sketch level), estimate per-helper insn cost, project final budget. Hard gate: stay under 12% of 900K.

---

## 3. AP-N execution order

| Step | Owner | Duration | Output | Blocks |
|------|-------|----------|--------|--------|
| **AP-1** Slot-count budget projection | Architect | ~30 min | Table 4/8/16 vs verifier insns | ADR-075 §D-1 |
| **AP-2** MRET fall-through repro | Developer + Computermancer | ~45 min | Repro test + root-cause memo | Phase 1.3 IMPL Step 0 |
| **AP-3** Trapframe collapse prototype | Computermancer | ~60 min | Working syscall round-trip, two variants compared | ADR-075 §D-3 |
| **AP-4** Userland address-space sketch | Architect | ~30 min | UPC_PAGE_TABLE_LAYOUT.md amendment | ADR-075 §D-4 |
| **AP-5** Fork call-graph diff | Developer | ~45 min | xv6 fork() vs SYS_FORK comparison memo | ADR-075 §D-5 |
| **AP-6** Verifier budget projection | Architect + Developer | ~30 min | Per-helper insn cost table | ADR-075 §D-6 |

Total kickoff effort: ~4 hours hands-on (single shift, single agent OK).

Then: **ADR-075 pair-call** (Stevie + Architect + Developer, ~60 min) reconciles AP-1..AP-6 into a single decision document. After ADR-075 lands, the IMPL plan is authored (~30 min, derived directly from ADR), then executed (Phase 1.3 IMPL itself: 5-7 days unattended per parent plan §1.3).

---

## 4. Cut-points

- AP-2 reveals MRET interpreter bug → Phase 1.3 paused until separate ebpf-MRET fix shift lands. Substrate from Phase 1.2 stays intact.
- AP-1 shows 16-slot budget exceeds 12% gate → fall back to 8 slots, defer multi-process shell demo to Phase 2.
- AP-3 finds neither trapframe variant is verifier-clean → escalate to Round Table; Phase 1.3 deferred a sprint.
- AP-6 final projection >12% even at 4 slots → STUCK; Phase 1.3 IMPL plan must shed scope before authoring.

Each cut-point leaves the Phase 1.2 substrate intact and the dream ladder gated, not lost.

---

## 5. Why a fresh launch for AP-1..AP-6

Each AP-N is bounded and mechanical, so they can run autonomously without pair-call. The substantive decisions (D-1..D-6) are deferred to the ADR-075 pair-call AFTER the AP artifacts land. That mirrors Phase 1.2's working pattern:

1. PRE-WORK plan (this doc)
2. Execute AP-1..AP-6 unattended (4 hours)
3. Pair-call lands ADR-075 (60 min, Stevie + Architect)
4. IMPL plan authored from ADR (30 min)
5. IMPL executed (5-7 days unattended)

---

## 6. Execution mode

- **Marshal-safe autonomous** for AP-1..AP-6 (every step deterministic + verifiable).
- Per `feedback_unattended_churn_with_queued_work.md`: churn through AP-1..AP-6 without per-step check-in. Stop only at AP-2 STUCK or AP-6 budget overshoot.
- Commit cadence: one commit per AP-N (6 commits for the pre-work shift).
- ADR-073 zero-lint ratchet must hold; `--no-gpg-sign` per `feedback_unsigned_commits_when_afk.md`.
- End-of-shift artifact: an updated kickoff doc with AP-1..AP-6 outputs linked + a pre-call dossier for ADR-075.

---

## 7. Out-of-scope (do NOT do in this kickoff)

- Authoring ADR-075 itself — that's the pair-call output, not pre-work.
- Modifying xv6 source — every xv6 edit waits for IMPL.
- Modifying BPF interpreter — except for AP-2 if MRET turns out broken (separate fix shift).
- Adding new syscalls — Phase 1.3 reuses SYS_FORK/EXECVE/WAITPID/EXIT/SCHED_YIELD which all already exist.

---

*Free to use. Free to share.*
