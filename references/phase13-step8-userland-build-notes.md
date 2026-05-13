# Phase 1.3 IMPL Step 8 — Userland Build Pipeline (scaffold + Phase 1.4 kickoff)

**Date:** 2026-05-13 (autonomous Marshal shift)
**Status:** SCAFFOLD LANDED + BUILD WORKS END-TO-END (mkfs + init.mbc + fs.img).
        Original Phase 1.3 cut-point was over-conservative; init.c compiles
        and translates fine. Runtime path still gates on Phase 1.4 syscall
        backing, but the build chain is unblocked.
**Predecessor:** ADR-075 D-4 + battle-plan-phase13-impl-2026-05-13.md §Sub-phase D

## Build-chain win 2026-05-13 (Phase 1.4 kickoff)

- `crates/xv6-mbc/upstream/mkfs/mkfs` — native x86-64 host tool compiled clean
  (`gcc -Wall -Werror -I. -o mkfs/mkfs mkfs/mkfs.c`).
- `crates/xv6-mbc/upstream/target/init.mbc` — 1616 MBC instructions, 6.4KB,
  + `init.rv2mbc` (3.7KB, 942 entries) + `init.data` (188B).
- `crates/xv6-mbc/upstream/target/fs.img` — 2 MB xv6-format ramdisk with
  `init.mbc` embedded (run `cd target && ../mkfs/mkfs fs.img init.mbc`).
- Makefile.mbc-userland updated: links `adapters/libgcc_stubs.o` to satisfy
  printf.c's transitive `__umoddi3` / `__udivdi3` references (same gap
  the kernel side already handles).

## What landed (scaffold)

## What landed this shift

`crates/xv6-mbc/adapters/Makefile.mbc-userland` — a Makefile scaffold that mirrors `Makefile.mbc` (kernel side) for the userland tree at `crates/xv6-mbc/upstream/user/`. Compiles `init.c`, `sh.c`, `ls.c`, `cat.c`, `echo.c`, `wc.c` and emits `.mbc + .rv2mbc` artifacts via the rv32i-to-mbc translator.

## Why the build doesn't ACTUALLY run yet

xv6's `init.c` uses 8 user-side syscalls: `open`, `mknod`, `dup`, `printf`, `fork`, `exec`, `wait`, `exit`. Of these:

| syscall | Status today |
|---|---|
| `exit` (SYS_EXIT) | ✓ working post Phase 1.3 Step 3 |
| `fork` (SYS_FORK) | ✓ working (PROC_TABLE + page_dir_base + Allocator A1) |
| `exec` (SYS_EXECVE) | ✓ working (entry-point model) |
| `wait` (SYS_WAITPID) | ✓ working (polling, halted_mask) |
| `dup` | ❌ requires Phase 1.4 file-descriptor table |
| `open` / `mknod` | ❌ requires Phase 1.4 filesystem |
| `printf` | partial — internally calls `write` which we have, but printf.c uses formatted output requiring fmt scaffolding |

So a real `init.c` build would compile, but the resulting binary would `open()` and immediately get back `-ENOSYS` (or wherever the unimplemented stub bails). The runtime path needs Phase 1.4's FS work first.

## The compile path WOULD work for a trivial userland test

A hello-world userland program that only does `write(1, "hi\n", 3); exit(0);` would build cleanly against the existing SYSCALL handlers AND run end-to-end through MRET → main() → SYS_WRITE → SYS_EXIT. That's a real Phase 1.4 milestone candidate.

## Iteration recipe for Phase 1.4 (steps 1-3 now DONE 2026-05-13)

1. ✅ Compile `mkfs.c` as a native x86-64 host tool:
   ```bash
   cd crates/xv6-mbc/upstream
   gcc -Wall -Werror -I. -o mkfs/mkfs mkfs/mkfs.c
   ```
2. ✅ Build the xv6 init userland:
   ```bash
   cd crates/xv6-mbc/upstream
   make -f ../adapters/Makefile.mbc-userland init
   # produces target/init.elf + target/init.mbc + target/init.rv2mbc + target/init.data
   ```
3. ✅ Build ramdisk image with mkfs (NOTE: mkfs asserts on '/' in filenames,
   so the input must be invoked with bare basenames, not paths):
   ```bash
   cd crates/xv6-mbc/upstream/target
   ../mkfs/mkfs fs.img init.mbc
   # produces fs.img (2 MB xv6-format with init.mbc embedded)
   ```
4. ⏳ Add `--ramdisk <path>` to `upc-bootctl` that loads `ramdisk.img` into RAM_MAP at byte `0x00800000` (per `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`).
5. ⏳ Stub `open`/`dup`/`mknod` with `return -ENOSYS` in the BPF SYSCALL dispatch (the existing handler chain already has SYS_OPEN as a stub returning fd=3 — extend the pattern). Then back them with the real ramdisk-FS read path.
6. ⏳ Get xv6 kernel to advance past `main()` into `scheduler()` so `userinit()` actually fires and exec's `/init`. This is the bigger Phase 1.4 piece — Phase 1.3 banner-boot stops at insn=4000 mid-`main()`; need to wire the scheduler loop entry properly.

   **2026-05-13 probe finding**: bumped `--triggers 500` → `--triggers 5000`
   (10× insn budget). CPU advanced from insn=4000 to insn=40000 (all 10×
   triggers landed) but PC stayed in the 0x640 vicinity and TTY emitted
   no new output. xv6 is in an UNPRODUCTIVE LOOP in early kernel init —
   not trigger-budget-bound. Likely candidates:

   - Waiting for a timer interrupt that the BPF interpreter doesn't fire
     in trigger-driven mode (xv6 needs the timer to tick for scheduler
     entry from `main()`'s `scheduler()` call to even proceed).
   - Stuck in a `kalloc` loop trying to allocate from a memory pool the
     BootParams didn't size correctly.
   - Spinning on a non-existent CPU coming up (xv6 main() waits for
     started[i] from non-CPU0 hartids; we only have 1 hart).

   First step for the next attended shift: instrument with a `mmio_puts`
   at every major xv6 main() init step, rebuild, see where it actually
   loops. Then fix whatever it's waiting for.

7. ⏳ Author `references/battle-plan-phase14-impl-YYYY-MM-DD.md`.

## Reassessment 2026-05-13 (Phase 1.4 kickoff probe)

The original Step 8 cut-point assumed the BUILD couldn't proceed without
the syscall stubs. Probe revealed that's wrong: `init.c` compiles AND
translates AND embeds in a ramdisk fine — the build chain only needed
`adapters/libgcc_stubs.o` linked in (printf.c pulls in 64-bit divide
helpers freestanding RV32 lacks).

What's actually deferred to Phase 1.4 is the RUNTIME, not the BUILD:
init.mbc would `ecall SYS_OPEN("console")`, the BPF interpreter would
dispatch to the stub returning fd=3, init would then `dup(3)` which is
unimplemented and bail. And before any of that fires, the xv6 kernel
needs to actually reach `userinit()` — currently the BPF interpreter
halts mid-`main()` because the scheduler loop entry isn't wired into the
trigger-packet cadence.

So the carry-overs (numbered 4-7 above) ARE real Phase 1.4 work, but
the build-pipeline carry-over is closed.

## Cross-references

- `crates/xv6-mbc/adapters/Makefile.mbc-userland` — the scaffold itself.
- `crates/xv6-mbc/upstream/user/` — vendored xv6 userland source.
- `crates/xv6-mbc/upstream/mkfs/mkfs.c` — ramdisk builder (needs native compile).
- `references/battle-plan-phase13-impl-2026-05-13.md` §Sub-phase D — the plan's Step 8.
- `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` — ramdisk at byte 0x00800000.
- ADR-075 §D-4 — userland VA layout decision.
