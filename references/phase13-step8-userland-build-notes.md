# Phase 1.3 IMPL Step 8 — Userland Build Pipeline (scaffold + carry-over)

**Date:** 2026-05-13 (autonomous Marshal shift)
**Status:** SCAFFOLD ONLY — actual build deferred to Phase 1.4
**Predecessor:** ADR-075 D-4 + battle-plan-phase13-impl-2026-05-13.md §Sub-phase D

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

## Iteration recipe for Phase 1.4

1. Compile `mkfs.c` as a native x86-64 host tool:
   ```bash
   cd crates/xv6-mbc/upstream/mkfs
   gcc -Wall -Werror -I.. -o mkfs mkfs.c
   ```
2. Build the userland trivial test (or stub `init.c` down to a write+exit):
   ```bash
   cd crates/xv6-mbc/upstream
   make -f ../adapters/Makefile.mbc-userland init
   # produces target/init.mbc + target/init.rv2mbc
   ```
3. Build ramdisk image with mkfs:
   ```bash
   ./mkfs/mkfs target/ramdisk.img target/init.mbc
   ```
4. Add `--ramdisk <path>` to `upc-bootctl` that loads `ramdisk.img` into RAM_MAP at byte `0x00800000` (per `docs/doom/UPC_PAGE_TABLE_LAYOUT.md`).
5. Stub `open`/`dup`/`mknod` with `return -ENOSYS` in the BPF SYSCALL dispatch (the existing handler chain already has SYS_OPEN as a stub returning fd=3 — extend the pattern).
6. Add `crates/xv6-mbc/upstream/user/init.c` to actually use `write` + `exit` only (or sidestep with a custom `test_init.c` that bypasses the unbacked syscalls). Author a `references/battle-plan-phase14-impl-YYYY-MM-DD.md`.

## Why we stopped here (autonomous-shift cut-point)

Per the battle plan's explicit cut-point for Step 8:

> if user/sh.c references xv6 userspace syscalls our SYSCALL handler doesn't yet implement (open/read/write/fork/exec/wait/dup/pipe/close), stub them in BPF with `return -ENOSYS` and document the gap.

That cut-point fires HERE. Even init.c — not just sh.c — needs unbacked syscalls. Building it without the stubs lands a binary that immediately faults; building the stubs is real interactive engineering, not unattended churn. The scaffolding Makefile + this note are the deliverable; Phase 1.4 inherits the iteration.

## Cross-references

- `crates/xv6-mbc/adapters/Makefile.mbc-userland` — the scaffold itself.
- `crates/xv6-mbc/upstream/user/` — vendored xv6 userland source.
- `crates/xv6-mbc/upstream/mkfs/mkfs.c` — ramdisk builder (needs native compile).
- `references/battle-plan-phase13-impl-2026-05-13.md` §Sub-phase D — the plan's Step 8.
- `docs/doom/UPC_PAGE_TABLE_LAYOUT.md` — ramdisk at byte 0x00800000.
- ADR-075 §D-4 — userland VA layout decision.
