# Phase 1.6 — `init: starting sh` prints from user mode (2026-05-14)

**Status:** xv6 init runs in user mode and prints to the host TTY through the BPF `SYS_write` handler. The L5 user-mode TTY gate is shipped. Phase 1.7 work is the rest of init's syscalls (fork / exec / wait / open / close) to get an actual shell prompt up.

**Headline commit:** `724d5b06` feat(upc): Phase 1.6 — user mode prints "init: starting sh" on UPC
**Companion:** `79883496` SYS_fork (-1) stub + SYS_exit handler
**Doc ripples:** `7886c3ee` L5 status updated across Linux-on-UPC, Dream-Ladder, UPC-Overview, Home, CLAUDE.md

## What the bug actually was

It wasn't the spill-shadow on x17 (the Phase 1.5 hypothesis). It was the **userret trampoline reading user SP from an unmapped high VA**.

xv6's `trampoline_mbc.S::userret` does:

```asm
li a0, TRAPFRAME    # a0 = 0x7FFFE000 (high VA where trampoline page is mapped)
lw ra, 20(a0)
lw sp, 24(a0)       # ← key line: sp from trapframe
...
sret               # transitions to user with sp from above
```

On xv6 normally this works because the kernel page table maps the trampoline page (low PA from kalloc) to the high VA `TRAMPOLINE`. Under `UPC_FLAT_TRAMPOLINE` paging is decorative — `translate_address` in `monad-cpu-ebpf` has no entry for 0x7FFFE000. Every `lw` from `TRAPFRAME` hits `RAM_MAP::get(0x7FFFE000/4 = 0x1FFFF800)` which is out of the 16 M-entry RAM_MAP, falls through to the `None => 0` branch, returns 0.

So sp at `sret` = 0. User-mode `main` runs `addi sp, sp, -12` → sp = -12 = 0xFFFFFFF4. printf does `-28` → 0xFFFFFFD8. vprintf does `-40` → 0xFFFFFFB0. vprintf's `addi a1, sp, 27` for the byte slot in putc yields a1 = 0xFFFFFFCB.

When the `write` syscall stub's `ecall` reaches the BPF `SYS_write` handler, the handler reads `r9` (= a1 = 0xFFFFFFCB) and calls `mem_read_byte(0xFFFFFFCB)`. word_addr = 0x3FFFFFF2 — out of RAM_MAP bounds. mem_read_word returns 0. mem_read_byte returns 0. `mem_write_byte(0xC001, 0)` writes NUL. TTY ring has a NUL byte. No visible output.

## The fix

Two files, ~14 lines total.

**`crates/xv6-mbc/upstream/kernel/proc.c::forkret`** — under `UPC_FLAT_TRAMPOLINE`, pass `p->trapframe` (low PA from kalloc) as a second arg to userret:

```c
((void (*)(uint64, uint64))(uint64)userret)(satp, (uint64)p->trapframe);
```

**`crates/xv6-mbc/adapters/trampoline_mbc.S::userret`** — under `UPC_FLAT_TRAMPOLINE`, replace `li a0, TRAPFRAME` with `mv a0, a1`:

```asm
#ifdef UPC_FLAT_TRAMPOLINE
        mv a0, a1            # a1 = p->trapframe low PA, set by forkret
#else
        li a0, TRAPFRAME     # vanilla xv6: high VA mapped via paging
#endif
```

The rest of userret's `lw ra, 20(a0); lw sp, 24(a0); ...; sret` is untouched. It just reads from a real RAM_MAP-backed address now.

## Diagnosis path (what fooled us in Phase 1.5)

The Phase 1.5 session log put the suspicion on the translator's x17 spill-shadow. That spill-shadow IS a real thing (`crates/monad-mbc/src/translator.rs` writes x17's value to RAM byte 0x64004 around every use), and the 7-MBC-instruction `li a7, 16` expansion is a real pattern in the write stub. But it doesn't touch r9 or the user stack, so it can't be what's clobbering the byte read in SYS_write.

The real giveaway was the diagnostic `<CB=·>` (buf_addr low byte = 0xCB, byte at buf_addr = NUL). 0xCB = 0xB0 + 0x1B (the `+27` offset). 0xB0 felt like a kernel kstack tail value, but kernel-side sp at the time of writing was nowhere near 0xB0. Re-running the arithmetic from the sret transition with sp=0 finally clicked: `0 - 12 - 28 - 40 + 27 = -53 = 0xFFFFFFCB`. **Low byte CB.** That matched. Hypothesis: user sp = 0. Verified by re-reading userret's `lw sp, 24(a0)` with TRAPFRAME unmapped.

## What ships, what doesn't

Ships:
- xv6 boots end-to-end on the UPC.
- init enters user mode (priv=3) and runs main → open → dup×2 → printf.
- printf's first iteration emits "init: starting sh\n" to the host TTY one byte per ecall via `op::SYSCALL`'s `SYS_write == 16` branch.
- SYS_fork stub returns -1, taking init's `if (pid < 0)` branch.
- SYS_exit halts the CPU cleanly.

Doesn't ship (Phase 1.7+ work):
- Real `SYS_fork` — needs trapframe duplication, per-pid pgd setup, scheduler enqueue of the child. The Phase 1.2 / 1.3 PROC_TABLE substrate is already in place; just needs the actual fork handler.
- `SYS_exec` — load a different `.mbc` into ROM_MAP at runtime, repatch CALL targets, swap trapframe. Currently a single binary is pre-loaded by `upc-bootctl --userland`.
- `SYS_wait` — parent blocks on child exit. Requires scheduler reentry from blocked state.
- `SYS_open` / `SYS_close` / `SYS_dup` — currently silent-ignore (a0 passes through unchanged). init proceeds because open's return value is a positive address (= the "console" string), not a real fd. Works for now; fragile.

The honest path to a shell prompt is exec — the rest mostly fall out once exec can replace the user image at runtime.

## Reproduction recipe

```bash
cd ~/tmp/unheaded/ebpf && cargo build --release -p monad-cpu-ebpf --features ascend-linux
cd ../crates/xv6-mbc/upstream && make -f ../adapters/Makefile.mbc clean kernel && rm -f target/fs.img && make -f ../adapters/Makefile.mbc-userland ramdisk
cd ../../../ebpf && sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/fs.img \
  --userland /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/init.mbc \
  --triggers 1000000 --instance 222
```

Look for `init: starting sh` in the TTY ascii dump.

## Cross-references

- `references/phase14-session-2026-05-14-marshal-shift.md` — Phase 1.4 milestone (kernel init through SRET)
- `references/phase15-session-2026-05-14-userland-spike.md` — Phase 1.5 spike + the x17-spill-shadow red herring
- `wiki/Linux-on-UPC.md` — full Phase 0→1.6 narrative
- `wiki/UPC-Dream-Ladder.md` — L5 status
- ADR-067 (MBC ISA v2 + UPC ABI v1), ADR-074 (Phase 1.2 page-table model), ADR-075 (Phase 1.3 process model)
