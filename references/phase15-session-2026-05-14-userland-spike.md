# Phase 1.5 — Userland MBC loader spike (2026-05-14)

**Status:** Userland enters user mode and executes its prologue cleanly. printf path stalls on a stack-byte read mismatch — diagnosis in hand, fix is Phase 1.6.
**Predecessors:** Phase 1.4 milestone (`e857a3d6`, `2b139a28`) and the user-mode entry session log at `references/phase14-session-2026-05-14-marshal-shift.md`.
**Local commits this spike:**

- `2295a2e3` feat(upc): Phase 1.5 — userland MBC loader + xv6 enters user mode
- `250d42c3` feat(upc): Phase 1.5 spike — minimal xv6 SYS_write (16) on ecall path

Plus the Phase 1.4 chain (10 commits previously): `d7f7b810`, `38a54714`, `dc6842ff`, `7e6bd19d`, `1c8a5ec7`, `6fd7a337`, `699b9758`, `e857a3d6`, `2b139a28`.

## What ships in 2295a2e3

A working userland MBC loader, end-to-end.

1. **`upc-bootctl --userland <path>`** flag (`cmd/upc-bootctl/src/main.rs`). When provided, the .mbc / .rv2mbc / .data sidecars for init are loaded into a dedicated user region:
   - `.mbc` → ROM_MAP slot 0x4000+ (USER_ROM_BASE). Because the MBC translator emits absolute CALL targets within init.mbc's own MBC PC space (starting at slot 0), every CALL is rewritten at load time with `target += USER_ROM_BASE`. JMP / branches use signed PC-relative offsets so they survive the shift untouched. Empirically the patch touches 41 CALL targets in init.mbc.
   - `.rv2mbc` → RV2MBC_MAP slot 0 onward, with each entry shifted by USER_ROM_BASE. So SRET(SEPC=0) → RV2MBC_MAP[0] → USER_ROM_BASE = MBC PC of init's first instruction.
   - `.data` → RAM_MAP at each record's byte address. User's .rodata at RV byte 0xEB4 (= "console", "init: starting sh\n", etc.) lands at RAM_MAP[0xEB4 / 4] etc., well clear of the kernel layout (IVT 0-0x3FF, BootParams 0x100-0x1FF, kernel .text at 0x20000+).

2. **xv6 `kexec` under `UPC_FLAT_TRAMPOLINE`** stops being a panic-suppress stub. It now wires the user-mode entry:
   - Detects non-ELF magic (= MBC bytecode).
   - Sets `p->trapframe->epc = 0` (matches `user/user.ld`'s `. = 0x0` text base).
   - Sets `p->trapframe->sp = 0x500000 - 16` — a 5 MiB-high address well above kernel BSS (~0x114AA0) and below the fs.img ramdisk (0x800000). 1 MiB of user stack, no conflict with anything.
   - Sets `p->trapframe->a0 = 1` (argc), `a1 = 0` (argv NULL — no argv frame yet).
   - Returns `argc` (= 1). forkret writes it into trapframe->a0 too; redundant but xv6-style.

3. **`mmio_putc` / `mmio_puts` promoted** to externally-linkable non-inline functions so exec.c and proc.c can emit per-line bisect markers (`F1`..`F5`) without dragging in printf. Invaluable during the cycle-2-reboot diagnosis.

4. **`.SECONDARY` in `Makefile.mbc-userland`** keeps .elf intermediates around for objdump during debug.

### Boot result with `--userland init.mbc`

The 100-PC tracer armed by SRET captures init's actual user-mode execution:

```
S 4000 4001 4002 4003 4004 4005 4006 4007 4008 4009 400A 400B   ← init main prologue
  41D9 41DA 41DB 41DC 41DD 41DE 41DF 41E0 41E1                    ← open syscall stub
  400C 400D 400E 400F 4010 4011                                    ← bltz check, dup setup
  4218 4219 421A 421B 421C 421D 421E 421F 4220                    ← dup stub #1
  4012 4013 4014                                                    ← back in main
  4218 4219 421A 421B 421C 421D 421E 421F 4220                    ← dup stub #2
  4015 4016 4017 4018                                              ← main prep for printf
  4464 4465 4466 4467 4468 4469 446A 446B 446C 446D 446E 446F     ← printf wrapper
  4470 4471 4472 4473 4474
  42BE 42BF 42C0 42C1 42C2 42C3 42C4 42C5 42C6 42C7 42C8 42C9     ← vprintf body
  42CA 42CB 42CC 42CD 42CE 42CF 42D0 42D1 42D2 42D3 42D4 42D5
  42D6 42D7 42D8 42D9 42DA
  41B5 41B6                                                       ← write stub: li a7,16; ecall
```

Every CALL target landed in user-region MBC (0x4000+). Trace exhausted right at the ecall — but the syscall sentinel (`#` printed at op::SYSCALL entry during debug) confirmed the ecall fires.

## What 250d42c3 adds

A minimal 1-byte-per-call `SYS_write` (xv6 syscall number 16) on the op::SYSCALL chain. Reads byte at `r9` (= a1 = `&c` for printf's putc) and emits to TTY MMIO 0xC001. Returns 1 in r8.

The 1-byte form is intentional. Initial implementations with a 16- or 64-byte inner loop ran the kernel 6.17 BPF verifier past its complexity budget (XDP load fails with `Error: XDP program load`), particularly when combined with the SYS_READ_BLOCK / SYS_WRITE_BLOCK handlers already on this path. printf's `putc(fd, c) { write(fd, &c, 1); }` naturally loops, so 1 byte per call is enough.

## The remaining issue (Phase 1.6)

`SYS_write`'s `mem_read_byte(buf_addr)` returns NUL during init's first printf. Diagnosis collected during the spike:

- Initial test had a fd guard (`if fd == 1 || fd == 2`). Diagnostic showed fd=0 instead of expected 1 — first hypothesis was "va_list / register-allocation issue across vprintf → write".
- Removed the fd guard (write byte regardless). Now we see `<·>` (sentinel-then-NUL) — confirming the syscall fires but the byte at `r9` is NUL.
- Added hex of r9 low byte: `<CB=·>`. Expected r9 = sp+27 = 0x4FFFA0+27 = 0x4FFFBB (low byte 0xBB). Actual r9 low byte: 0xCB. Off by 0x10 bytes.
- MBC disassembly of vprintf around `0x42D2`-`0x42DA` (`LD a0 ← sp+0; ... STB a4 → sp+27; CALL write`) looks structurally correct.
- Hypothesis: the translator's spill-shadow on x17 (a7) — which maps to r1 (= gp) per `crates/monad-mbc/src/translator.rs::map_register` — emits a 4-extra-MBC-insn save sequence (visible in init.mbc at user MBC 0x41B5-0x41BB: `MOVI r0; MOV r1, r0; ADDI r1, r0, 16; MOVI r0, 0x4004; LOAD_IMM32 r0, 0x0006; ST r0, r1, 0; MOVI r0, 0`) that writes the syscall number into RAM at byte 0x64004. This spill **doesn't** clobber the vprintf stack region, but it could be evidence of broader register-allocation issues. Worth a Computermancer + Developer pair pass.
- Alternative: printf's `t1 = sp + 8` va_list scratch may overlap vprintf's stack frame. printf has `addi sp, sp, -28` then `addi t1, sp, 8` — t1 = sp_printf + 8. vprintf's `&c = sp_vprintf + 27 = sp_printf - 40 + 27 = sp_printf - 13`. No overlap on the C side; but the MBC translation of `addi t1, sp, 8` may emit an ADDI sequence that uses r4 (= t1 → x6) in unexpected ways.

Either way, a careful trace of the actual register contents at write's ecall (specifically r9 vs the STB at vprintf::42D9) will isolate it. The cleanest way is a per-instruction PC + reg trace in a wider RAM debug region — not the verifier-budget-bound TTY tracer used here.

## Diagnostic markers still in tree (to strip in Phase 1.6)

| File | Marker | Purpose |
|---|---|---|
| `crates/xv6-mbc/upstream/kernel/proc.c::forkret` | `F1`..`F5` mmio_putc bisect | Confirms forkret → fsinit → kexec sequence |
| `crates/xv6-mbc/upstream/kernel/exec.c::kexec` | `$`, `KX:enter`, `%` | Confirms kexec entry + mmio_puts works |
| `crates/xv6-mbc/upstream/kernel/main.c` | `after consoleinit` etc. | Phase 1.4 init-loop bisect (still useful as boot heartbeat) |
| `crates/xv6-mbc/upstream/kernel/kalloc.c` | `freerange enter/exit`, `kfree-A/B/C` | Phase 1.4 spinlock + page-walk proof |
| `ebpf/monad-cpu-ebpf/src/main.rs::RET` | `<XXXX>` hex print | Confirms RET RV→MBC translation |
| `ebpf/monad-cpu-ebpf/src/main.rs::SRET` | `S` | Confirms M→U transition fires |
| `ebpf/monad-cpu-ebpf/src/main.rs` (top of dispatch loop) | `TRACE_HEAD` armed by SRET, 100 PCs | Per-instruction trace post-SRET |

All are safe to leave in tree under `UPC_FLAT_TRAMPOLINE` / `ascend-linux` — they're diagnostic value during the active spike. Strip in the cleanup commit once userland prints its first real line.

## Verification recipe

```bash
cd ~/tmp/unheaded/ebpf
cargo build --release -p monad-cpu-ebpf --features ascend-linux

cd ../crates/xv6-mbc/upstream
make -f ../adapters/Makefile.mbc clean kernel
rm -f target/fs.img
make -f ../adapters/Makefile.mbc-userland ramdisk

cd ../../../ebpf
sudo /home/govan/tmp/unheaded/cmd/upc-bootctl/target/release/upc-bootctl boot \
  --kernel /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/xv6-mbc.mbc \
  --ramdisk /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/fs.img \
  --userland /home/govan/tmp/unheaded/crates/xv6-mbc/upstream/target/init.mbc \
  --triggers 800000 \
  --instance <pick>
```

Look for: `Userland MBC: patched 41 CALL targets` in the boot log, then the kernel-init markers, then `F1F2 ... F5 S <pc-trace>` in the TTY ascii.

## Cross-references

- `references/phase14-session-2026-05-14-marshal-shift.md` — Phase 1.4 milestone (Stevie-attended + Marshal shift)
- `references/phase14-xv6-init-loop-root-cause-2026-05-13.md` — original init-loop diagnosis (now historical)
- `cmd/upc-bootctl/src/main.rs` — `--userland` flag + USER_ROM_BASE = 0x4000 CALL-target patcher
- `crates/xv6-mbc/upstream/kernel/exec.c` — kexec MBC stub
- `crates/xv6-mbc/upstream/user/user.ld` — userland linker (text at 0x0)
- `crates/monad-mbc/src/translator.rs::map_register` — RV32 → MBC register map (x17 → r1 spill-shadow is the suspect)
