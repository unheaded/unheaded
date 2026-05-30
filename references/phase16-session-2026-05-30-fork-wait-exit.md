# Phase 1.6 session — RV32I fork / wait / exit, and the memory-isolation wall

**Date:** 2026-05-30
**Area:** `ebpf/monad-cpu-ebpf/src/main.rs` (RV32I `ecall` dispatch chain, `opc == op::SYSCALL`)
**Goal:** advance xv6-on-UPC from the Phase 1.6 banner (`init: starting sh`) toward a
visible shell prompt by implementing the syscalls in init.c's loop:
`fork() → exec("sh") → wait()`.

## What shipped this session (kept in tree)

All in the RV32I ecall path; the proven `INT 0x80` (Level 4c) handlers were the template.

1. **`SYS_fork` (#1)** — replaced the `return -1` stub with a real fork: snapshot the
   parent's MBC register file into `PROC_TABLE[num_processes]`, child resumes after the
   ecall with `a0 = 0`, parent gets `child_pid`. RISC-V ABI (`a0` = `regs[8]`). Verified:
   init no longer prints `init: fork failed` and no longer halts — it takes the
   `fork() > 0` parent branch into the wait loop.
2. **`SYS_exit` (#2) rework** — was "halt the whole CPU". Now marks the caller a ZOMBIE
   (`halted_mask` bit) and context-switches to a runnable process; only halts when nothing
   else can run. Mirror of `INT 0x80 SYS_EXIT` (ADR-075 D-5).
3. **`SYS_wait` (#3)** — new. Returns an unreaped zombie child's pid (and records it in a
   new **`reaped_mask`**, `SCHED_STATE[4]`, so a later `wait()` won't return the same pid);
   otherwise context-switches to a runnable child and re-executes the ecall on resume.
   PC is decremented **before** the switch so the *parent's* saved PC lands back on the
   wait ecall (PC is pre-incremented at fetch, `main.rs:514`).
4. **`SCHED_STATE`** grown `4 → 5` entries for `reaped_mask`.

Build/boot recipe unchanged (see CLAUDE.md ASCEND-LINUX section). Boot still reaches the
Phase 1.6 banner — no regression to the milestone.

## The wall (root cause, confirmed)

fork + wait + exit are individually correct, and wait **does** schedule the child (PC
moves from the wait-spin `0x419F` into child execution). But the child never makes useful
progress — execution diverges into an infinite loop near the userland entry (`PC≈0x4024`),
no further TTY output, `halted=0`.

Root cause: **all processes share one flat address space.** `translate_address`
(`main.rs:2239`) returns `vaddr` unchanged whenever `cpu.mmu_enabled == 0`, and xv6 boots
with the MMU off (kvminit is skipped via `UPC_SKIP_KVMINIT`; userland never calls
`SYS_ENABLE_MMU`). The per-pid pgd (`page_dir_base`, Phase 1.2 Option A) is saved/restored
across context switches but is **decorative** — nothing translates through it.

Consequently fork's child shares the parent's stack. init.c does `pid = fork();` and spills
the return value to a stack slot; parent and child alias that slot, so the child reads a
non-zero `pid`, skips the `if (pid == 0)` exec branch, and behaves like a second parent.
Register-level `a0 = 0` is necessary but not sufficient — the C code immediately writes it
to shared memory.

## Implication for the roadmap

A visible shell prompt is **not** reachable by syscall handlers alone. It requires genuine
per-process memory isolation:
- `cpu.mmu_enabled = 1` for userland, and
- `translate_address` honoring each pid's page table so the same virtual stack/heap maps
  to different physical `RAM_MAP` pages per process, and
- fork giving the child a real (copied or COW) address space; exec building a fresh one.

That is the **Phase 1.2 "page tables for real" work** — the pair-call that was explicitly
deferred pending Stevie (per `battle-plan-roundtable-2026-05-11-postcheck.md` and
ADR-074). The fork/wait/exit substrate landed here is the consumer that needs it.

## Suggested next step

Convene the Phase 1.2 pair-call (Architect + Computermancer + Developer + Marshal): turn
the decorative per-pid pgd into a live translation path (enable MMU for userland, populate
per-pid page tables, make `translate_address` authoritative), then revisit fork-copy /
exec-fresh-address-space on top of the handlers shipped here.
