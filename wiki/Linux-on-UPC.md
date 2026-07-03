# Linux on UPC — ASCEND-LINUX

The ASCEND-LINUX campaign brings a real operating system up on the [Unheaded Protocol Computer](UPC-Overview). Phased six-level Dream Ladder summit (L5 → L6); the kernel of choice for L5 is xv6-riscv (vendored at `crates/xv6-mbc/upstream/`), L6 is uClinux then full Linux+MMU.

**Current state (2026-07-03): ASCEND-LINUX Phase 1 COMPLETE.** xv6 runs an **interactive shell** on the UPC — a scripted command line is read from the console, `sh` forks + exec's it, the child runs and exits, `sh` reaps it, and a fresh `$` prompt returns. The full `init → sh → fork/exec/wait` lifecycle works, backed by a real in-BPF filesystem reader over `fs.img`: `ls` lists the root directory with types/inums/sizes, `cat` prints real file contents (including `>12 KiB` files via single-indirect blocks), `echo` and `wc` run with argv, per-pid MMU isolation holds. Gates A (console stdio) → B (`exec` → `$`) → C (interactive sh) → D (FS reader) → D.1 (`wc`) all shipped. **Next: Unheaded Linux** — an own-built minimal OS evolving from this xv6 substrate ([ADR-081](../docs/adr/ADR-081-unheaded-linux-from-scratch.md)); the roadmap is [`docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md`](../docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md).

This page is the cumulative narrative of the campaign. Live session logs and per-phase root-cause docs live under [`references/`](../references/) and the original battle plan is at [`references/battle-plan-ascend-linux-2026-05-08.md`](../references/battle-plan-ascend-linux-2026-05-08.md).

## Phase chronology

### Phase 0 — ISA + ABI freeze (2026-05-08)

Five new MBC opcodes added to support a real kernel: `FENCE`, `MRET`, `SRET`, `LR.W`, `SC.W`. Memory-mapped CSR region at byte `0x000_F000+` (each CSR at `0xF000 + csr_index * 4`). `BootParams v2` spec at byte `0x100`. `MbcCpuState` grows 128 → 136 bytes (adds `priv_level` + `reservation_address`).

All five new opcodes are gated behind `cfg!(feature = "ascend-linux")` so the kernel 6.17 BPF verifier doesn't blow its complexity budget for the Doom build. **Always build the BPF program with `--features ascend-linux`** when running anything ASCEND-LINUX-related, or MRET/SRET silently NOP and the kernel never escapes start_mbc.c.

ADR-067 captures the seven decisions; one shift, 15 commits.

### Phase 1.1 — First boot banner (2026-05-09 → 2026-05-11)

12-iteration super-sprint stood up the rv32i-to-mbc + xv6 toolchain:

- xv6-riscv vendored at `crates/xv6-mbc/upstream/` (commit `5474d4bf`, MIT).
- Adapters in `crates/xv6-mbc/adapters/`: `start_mbc.c` (the M-mode entry that verifies BootParams, sets MEPC, issues `mret`), `console-mmio.c` (replaces upstream `uart.c`), `blk-ramdisk.c`, `syscall_shims.S`, `swtch_mbc.S`, `trampoline_mbc.S`, `kernel-mbc.ld`, `Makefile.mbc`.
- Translator extensions in `crates/monad-mbc/`: CSR opcodes via memory-mapped LD/ST, MRET/SRET/WFI/SFENCE.VMA encoding, register-aliasing x18–x31 best-effort.
- `crates/monad-mbc/src/bin/rv32i_to_mbc.rs` emits `.mbc` + `.rv2mbc` + `.data` sidecars.
- `cmd/upc-bootctl/` (`validate`, `boot`, `console` subcommands; netns + XDP attach; live trigger packet; TTY drain + bridge POST).
- `cmd/upc-tty-bridge/` (Go WebSocket bridge at port 26100, Mode A demo).
- `dashboard/upc-console.html` (browser xterm.js).

**SHIP gate (2026-05-10, `3ac1f684`):** boot advances 4000 instructions, transitions M-mode → S-mode (priv 0→1), emits `xv6 booting...` cleanly to TTY_MAP. BPF verifier under 9% of the 900K budget.

### Phase 1.2 — Per-pid page-table substrate (2026-05-13)

Option A from [ADR-074](../docs/adr/ADR-074-phase12-page-table-model.md): each process gets its own pgd region at a fixed per-pid offset. BPF interpreter, host emulator, and vendored xv6 all updated; `uvmcreate(int pid)` returns `0x00F00000 + pid * 0x1000`. Closure report: [`references/phase12-impl-complete-2026-05-13.md`](../references/phase12-impl-complete-2026-05-13.md).

### Phase 1.3 — Process model (2026-05-13)

[ADR-075](../docs/adr/ADR-075-phase13-process-model.md) accepted. Sub-phases A–D shipped (Steps 1–8): `PROC_TABLE` widened 4 → 8 slots, `SYS_EXIT` ZOMBIE refactor, `LR.W`/`SC.W` reservation-clear (real RISC-V atomicity bug caught and fixed), RV2MBC SHA-256 integrity gate via `UPC_RV2MBC_SHA` env var, userland Makefile scaffold. AP-2 fix (`73834054`) unblocked the headline win: xv6 advances past MRET into `main()` with M→S privilege transition confirmed live. Verifier budget 8.52% of the 900K hard gate.

### Phase 1.4 — Init loop unblocked (2026-05-13 → 2026-05-14)

Two shifts: an overnight Marshal run on 2026-05-13 that landed the substrate work (mkfs + full xv6 userland init/sh/ls/cat/echo/wc + fs.img, `upc-bootctl --ramdisk` + `--triggers`, `PHYSTOP` cap, `UPC_SKIP_KFREE_MEMSET` / `UPC_SKIP_KALLOC_MEMSET` CFLAGS), then a Marshal + attended pass on 2026-05-14 that closed the entire init chain.

Nine architectural fixes shipped this phase:

1. **RET RV-translation**. `op::RET` had treated `cpu.regs[14]` as a literal MBC PC. Fine for compiled RV CALL→RET chains; broken when `r14` is loaded from a struct field set by C with `(uint64)&function` (e.g. `p->context.ra = (uint64)forkret`). Fix: if `r14 >= 0x10000`, look it up in `RV2MBC_MAP` first; else use as MBC PC directly. Commit `1c8a5ec7`.
2. **Zero-slot guards on JMPR / CALLR / MRET / SRET**. BPF `Array::get` returns `Some(&0)` for unset slots, not `None`. Without the guard, an indirect jump to an address outside the populated rv2mbc range silently routes PC to 0 (= start_mbc.c reboots, loop). Guard added: `Some(mbc_idx) if *mbc_idx != 0`; else halt or fall through.
3. **BPF timer interrupt gated**. The XDP-level timer fires `cpu.pc = mem_read_word(IVT[VECTOR_TIMER] * 4)` periodically. xv6 doesn't program our flat IVT, so the timer would route PC to 0. Gated behind `cfg!(not(feature = "ascend-linux"))`. Doom keeps its own scheduler timer.
4. **UPC_SKIP_KVMINIT** (Option A from [`phase14-session-2026-05-14-marshal-shift.md`](../references/phase14-session-2026-05-14-marshal-shift.md)). `vm.c::kvmmake`'s `kvmmap(.., PHYSTOP - (uint64)etext, ..)` underflows under our `PHYSTOP=0x00800000` cap and panics `mappages: size not aligned`. The page table is decorative on UPC (BPF `translate_address` is authoritative), so the fix is to early-return after `kpgtbl = kalloc()`. xv6 still gets a zeroed page that SATP can point at.
5. **Kernel stack on UPC**. With kvminit skipped, `procinit` still set `p->kstack = KSTACK(p) = 0x7FFFE000` — past `RAM_MAP`'s 16 MiB window. Stack `sw` drops silently; subsequent `lw ra, 4(sp)` returns 0; RET PC=0; reboot. Under `UPC_FLAT_TRAMPOLINE`, `procinit` now `kalloc`s a low-PA backing page and uses it directly.
6. **Flat trampoline**. `prepare_return` and `forkret` originally JALR/CALLR'd `TRAMPOLINE + (uservec-trampoline)` / `TRAMPOLINE + (userret-trampoline)` — high VAs that the rv2mbc map doesn't cover. Under `UPC_FLAT_TRAMPOLINE`, both sites use the low link-address of `uservec` / `userret` directly.
7. **Block syscall ABI**. `syscall_shims.S` put the xv6 syscall number in `a0` with a confused mv shuffle; the BPF SYSCALL handler reads `r1` (= RV x17 = a7). Rewrote shim: `li a7, NR; li a2, 0; ecall`. Added `SYS_READ_BLOCK` (200) and `SYS_WRITE_BLOCK` (201) handlers to the `op::SYSCALL` ecall path (previously L4e-only, on INT 0x80).
8. **fs.img filename without `.mbc`**. mkfs was packing each userland binary under its source basename (`init.mbc`, `sh.mbc`); xv6's forkret calls `kexec("/init")` via `namei`. Patched `Makefile.mbc-userland` to `cp init.mbc → init` (etc.) before invoking mkfs.
9. **kexec MBC stub**. xv6's `kexec` panics if the ELF magic check fails; our userland is MBC bytecode, not an ELF. Under `UPC_FLAT_TRAMPOLINE`, kexec detects non-ELF and returns success after wiring `p->trapframe->epc = 0` (matches `user/user.ld` placing `.text` at byte 0) and `p->trapframe->sp = 0x500000 - 16` (1 MiB user stack above kernel BSS and below the ramdisk).

End-of-phase state: every `mmio_puts("after X\n")` marker in `main()` prints. main() returns into `scheduler()`. CPU halts cleanly at the SRET transition (priv=3, halted=1, insn_count ≈ 5.35 M). Detailed in [`references/phase14-session-2026-05-14-marshal-shift.md`](../references/phase14-session-2026-05-14-marshal-shift.md). Commit chain: `d7f7b810` → `2b139a28` (9 commits).

### Phase 1.5 — Userland enters user mode (2026-05-14)

`upc-bootctl --userland <path>` is the new headline flag. It loads init's three sidecars into a dedicated user region of the BPF maps:

- `.mbc` → ROM_MAP slot `0x4000+` (USER_ROM_BASE).
- `.rv2mbc` → RV2MBC_MAP slot `0` onward, with each entry shifted by USER_ROM_BASE. So SRET(SEPC=0) routes to USER_ROM_BASE = MBC PC of init's first instruction.
- `.data` → RAM_MAP at each TLV record's link-time byte address.

**CALL-target patching.** The MBC translator emits CALL with an absolute 24-bit MBC PC, fine when loaded at slot 0. But init.mbc lives at slot 0x4000, so every CALL imm has to be shifted by USER_ROM_BASE or it lands inside the kernel image (consoleintr / consoleread / etc.). Branches (JMP / JZ / JNZ / JN / JP) use signed PC-relative offsets and survive the shift unchanged. Empirically the patch touches 41 CALL targets in init.mbc.

End-of-phase trace (100 user-mode PCs captured by the SRET-armed tracer):

```
S 4000 4001 4002 4003 4004 4005 4006 4007 4008 4009 400A 400B   ← init main prologue
  41D9 41DA 41DB 41DC 41DD 41DE 41DF 41E0 41E1                    ← open syscall stub
  400C 400D 400E 400F 4010 4011                                    ← bltz check, dup setup
  4218 .. 4220                                                     ← dup stub #1
  4012 4013 4014
  4218 .. 4220                                                     ← dup stub #2
  4015 4016 4017 4018
  4464 4465 4466 4467 4468 4469 446A 446B 446C 446D 446E 446F     ← printf wrapper
  4470 4471 4472 4473 4474
  42BE .. 42DA                                                     ← vprintf body
  41B5 41B6                                                        ← write stub: li a7,16; ecall
```

Every CALL target lands in the user region. Every ecall fires correctly. Commit chain: `2295a2e3` (loader), `250d42c3` (`SYS_write` spike), `17a63212` (session log).

### Phase 1.6 — `init: starting sh` (2026-05-14, shipped)

`SYS_write` (xv6 syscall 16) was added to the `op::SYSCALL` ecall path as a minimal 1-byte-per-call handler. printf's `putc(fd, c)` does `write(fd, &c, 1)`, so single-byte is enough; larger writes ran the kernel-6.17 BPF verifier past its budget when combined with the existing SYS_READ_BLOCK / SYS_WRITE_BLOCK handlers.

**The byte-path bug** (turned out NOT to be the spill-shadow on x17): user SP was garbage. trampoline_mbc.S's userret reloads the user register file via `lw sp, 24(a0)` after `li a0, TRAPFRAME`. TRAPFRAME = 0x7FFFE000 (xv6's high VA for the trampoline page). On UPC paging is decorative, translate_address has no entry for 0x7FFFE000, mem_read_word falls through to the out-of-range branch and returns 0. sp gets clobbered to 0; user main's prologue (`addi sp, sp, -12`), printf's (`-28`), vprintf's (`-40`) cumulatively yield sp = -80 = 0xFFFFFFB0. vprintf's `addi a1, sp, 27` yields a1 = 0xFFFFFFCB. mem_read_byte at that address hits the out-of-range RAM_MAP cap (word index > 16 M words) and returns 0. SYS_write writes NUL — no visible output.

**Fix (commit `724d5b06`):**

1. forkret passes `p->trapframe` (low PA from kalloc) as **a1** when calling userret under `UPC_FLAT_TRAMPOLINE`. The trapframe lives at a real RAM_MAP-backed address.
2. userret in trampoline_mbc.S gains an `#ifdef UPC_FLAT_TRAMPOLINE` arm: `mv a0, a1` instead of `li a0, TRAPFRAME`. The subsequent lw / sw sequence is untouched — it's just reading the actual trapframe page now.

After the fix, the boot log goes through to `init: starting sh\n` printed to the host TTY, one byte at a time, through `user/printf.c → user/usys.S write → BPF op::SYSCALL → mem_write_byte(0xC001, byte) → upc-bootctl TTY drain`.

The xv6 L5 user-mode TTY gate is shipped.

### Phase 1.7 — Gates A → D.1: interactive shell + filesystem (2026-06-18 → 2026-07-03)

The syscall fan-out landed as five gates, each with its own root-cause hunt (session logs under `references/`):

- **Gate A — console stdio.** `read(5)` blocks on an empty KBD ring instead of returning EOF; a scripted `--input` pre-fills the ring. sh reads its command line.
- **Gate B — `exec` → `$` (ADR-077).** A feature-gated `MbcCpuState` ABI fork adds a per-process `rv2mbc_base` so each program's indirect branches resolve into its own disjoint RV2MBC region (init and sh both link at RV byte 0 and would otherwise collide at `RV2MBC_MAP[0]`). `exec("sh")` succeeds and sh emits `$`.
- **Gate C — interactive sh.** `fork → exec → wait → prompt` lifecycle. Two root causes behind the post-`$` reboot: MRET added sh's *user* rv2mbc base to a *kernel* return target (fix: per-process base only at `priv==3`); and the user link defaulted sh's entry to `getcmd` not `main` (fix: `crt0_mbc.S` forces `_start` to RV byte 0). Plus parent-filtered `wait()` (fixes grandchild-reap).
- **Gate D — FS reader ([ADR-078](../docs/adr/ADR-078-gate-d-fs-reader-and-exec-argv-abi.md)).** `open`/`read`/`fstat` walk `fs.img` inodes directly from RAM_MAP inside the eBPF CPU (pure `monad_common::fs_walk`, off-target tested), backed by per-fd `{inum, offset}` state. `exec` gains an argv frame (VA `0x600000`). The headline bug: xv6 `uint64` is 4 bytes under `-mabi=ilp32e`, so `struct stat` is 16 bytes with `size` at offset 12 — writing the RV64 24-byte layout smashed `ls`'s buffer (the "blank names + size 0" symptom).
- **Gate D.1 — `wc` + RET tagging ([ADR-079](../docs/adr/ADR-079-ret-address-tagging.md)).** `wc README` runaway root-caused to the RET handler's MBC-vs-RV magnitude floor (wc's ROM base crossed it). Fixed by tagging MBC return addresses with bit 31 at CALL/CALLR — which also unblocks Linux-scale images and, by construction, kills the historical Doom PC-corruption misparse.

Since then: `read(5)` reads `>12 KiB` files via the single-indirect block; the code-store maps grew behind a `large-image` feature (option A for Linux-scale images); the whole thing is guarded by `scripts/upc-regression.sh` (load-tests both the xv6/ascend and Doom/non-ascend builds).

## Reproduction recipe

```bash
# Always build the BPF interpreter with --features ascend-linux:
cd ~/tmp/unheaded/ebpf
cargo build --release -p monad-cpu-ebpf --features ascend-linux

# Build the xv6 kernel + userland ramdisk:
cd ../crates/xv6-mbc/upstream
make -f ../adapters/Makefile.mbc clean kernel
rm -f target/fs.img
make -f ../adapters/Makefile.mbc-userland ramdisk

# Boot. --userland pre-loads init.mbc into the user region.
cd ../../../ebpf
sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/fs.img \
  --userland /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/init.mbc \
  --triggers 800000 \
  --instance 222
```

Expected boot log includes `Userland MBC: patched 41 CALL targets`, then the kernel-init markers (`after consoleinit`, `after kvminit`, ..., `after started=1`), then `sched: entered` → `sched: pre-swtch` → `forkret: entered` → `forkret: lock released` → kexec stub → SRET (`S` marker) → user-mode PC trace.

## What's next

Phase 1 (xv6) is complete. The path forward is **Unheaded Linux** ([ADR-081](../docs/adr/ADR-081-unheaded-linux-from-scratch.md)): rather than vendor uClinux + busybox, build our own minimal OS from scratch on this substrate, evolving from xv6 (own PID 1 → shell → kernel edges → kernel core), scaling toward the Yggdrasil golden image. Long-term. The two-track roadmap is [`docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md`](../docs/battle-plans/UPC-LINUX-MASTER-BATTLE-PLAN.md); near-term Track 1 keeps hardening the UPC substrate (FS stretch, the `upc-api` guest contract, code-store readiness). The stage-1 stub `crates/upc-bootstub/` builds end-to-end and stands ready for a kernel-class guest.

## Demo surface

Mode A (browser xterm) is live: `cmd/upc-tty-bridge/` accepts TTY drain on port 26100, `dashboard/upc-console.html` runs xterm.js against the WebSocket. Mode C (SSH-over-IPv6) is post-Phase-4 work.

## Cross-references

- [UPC Overview](UPC-Overview) — the substrate this runs on
- [UPC Dream Ladder](UPC-Dream-Ladder) — level-by-level gates
- [Doom on UPC](Doom-on-UPC) — the L3 sibling proof
- [MBC ISA Reference](MBC-ISA-Reference) — opcode + encoding reference
- [`references/battle-plan-ascend-linux-2026-05-08.md`](../references/battle-plan-ascend-linux-2026-05-08.md) — the original 10-month phased campaign
- [`references/phase14-session-2026-05-14-marshal-shift.md`](../references/phase14-session-2026-05-14-marshal-shift.md) — Phase 1.4 milestone
- [`references/phase15-session-2026-05-14-userland-spike.md`](../references/phase15-session-2026-05-14-userland-spike.md) — Phase 1.5 spike + Phase 1.6 diagnosis
- ADR-067, ADR-072, ADR-074, ADR-075

---

> **Source:** [crates/xv6-mbc/](../crates/xv6-mbc/) · [ebpf/monad-cpu-ebpf/src/main.rs](../ebpf/monad-cpu-ebpf/src/main.rs) · [cmd/upc-bootctl/](../cmd/upc-bootctl/) · [docs/doom/UPC_BOOT_PROTOCOL_V2.md](../docs/doom/UPC_BOOT_PROTOCOL_V2.md) · [references/battle-plan-ascend-linux-2026-05-08.md](../references/battle-plan-ascend-linux-2026-05-08.md)
