# Linux on UPC — ASCEND-LINUX

The ASCEND-LINUX campaign brings a real operating system up on the [Unheaded Protocol Computer](UPC-Overview). Phased six-level Dream Ladder summit (L5 → L6); the kernel of choice for L5 is xv6-riscv (vendored at `crates/xv6-mbc/upstream/`), L6 is uClinux then full Linux+MMU.

**Current state (2026-05-14):** xv6 boots end-to-end on the UPC. Kernel completes init, scheduler dispatches the init proc, kexec wires the user trapframe, SRET transitions to user mode (`priv=3`), and init's `main()` runs through `open` / `dup×2` / `printf` / `vprintf` to the `write` syscall stub. Phase 1.6 work is the byte-path through `SYS_write`.

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

### Phase 1.6 — SYS_write byte-path (in progress)

`SYS_write` (xv6 syscall 16) was added to the `op::SYSCALL` ecall path as a minimal 1-byte-per-call handler. printf's `putc(fd, c)` does `write(fd, &c, 1)`, so single-byte is enough; larger writes ran the kernel-6.17 BPF verifier past its budget when combined with the existing SYS_READ_BLOCK / SYS_WRITE_BLOCK handlers.

The remaining bug: `mem_read_byte(r9)` returns NUL during init's first printf. r9 should be `sp + 27` (the byte slot where vprintf's STB stores `c` immediately before calling write). The MBC disassembly of vprintf around 0x42D2–0x42DA (`LD a0 ← sp+0; ... STB a4 → sp+27; CALL write`) is structurally correct; empirically the buf_addr at SYS_write entry lands ~0x10 bytes off from where STB wrote.

Working hypothesis: the translator's spill-shadow on x17 (a7 → r1, aliased onto gp via fixed-address RAM at byte `0x64004`) interacts badly somewhere. Full diagnosis in [`references/phase15-session-2026-05-14-userland-spike.md`](../references/phase15-session-2026-05-14-userland-spike.md). Next attended shift: Computermancer + Developer pair audit of the rv32i-to-mbc register allocation around printf → vprintf → putc → write.

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

Three independent threads from here:

1. **Phase 1.6 SYS_write fix.** The byte-path NUL. One translator-audit shift unblocks the first printf byte.
2. **Phase 1.7 syscall fan-out.** Once the byte path works, wire `SYS_open` / `SYS_close` / `SYS_dup` / `SYS_fork` / `SYS_exec` / `SYS_wait` / `SYS_exit` against the fs.img reader and the per-pid PROC_TABLE. The xv6 init proc loops `printf` → `fork` → `exec("sh", ...)` → `wait`. Each blocker is one syscall handler.
3. **Phase 2 uClinux bring-up.** Real Linux (uClinux first, no MMU) on top of the Phase 1 substrate. Stage-1 stub `crates/upc-bootstub/` is already scaffolded. ADR-067's Phase-2 path expects the BootParams + MEPC handoff to behave identically across xv6 and uClinux.

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
