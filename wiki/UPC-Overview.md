# UPC — Unheaded Protocol Computer

The **UPC** is a virtual CPU implemented entirely in the eBPF runtime. It treats the IPv6 Hop-by-Hop Options header as a register file, an XDP program as the instruction dispatcher, and BPF maps as ROM / RAM / page tables / TTY / screen / scheduler state. Every Monad packet that arrives is a clock tick.

This page is the canonical UPC reference. Implementation-deep specs live in [`docs/doom/ARCHITECTURE.md`](../docs/doom/ARCHITECTURE.md) and the Internet-Drafts (MBC-ISA-00, Shim-00) under [`docs/protocol/`](../docs/protocol/).

## Big picture

```
+-----------+   IPv6/HbH    +-----------+   ... ring of namespaces ...
| veth-upc0 |-------------->|  XDP prog |
+-----------+               | monad-cpu-|
                            |   ebpf    |
                            +-----+-----+
                                  |
   +------------------------------+------------------------------+
   |              |              |              |               |
   v              v              v              v               v
ROM_MAP       RAM_MAP        RV2MBC_MAP    CPU_MAP / PROC_TABLE  TTY_MAP / SCREEN_MAP
(MBC code)    (data, stack,  (RV byte addr  (per-instance        (output sinks
              heap, BSS,     -> MBC PC)     register file)        + framebuffer)
              MMIO regs)
```

Each XDP invocation tail-calls itself up to 15 times, executing up to 16 MBC instructions per round = 256 MBC instructions per packet. That cadence is what gives "the network IS the CPU" its concrete meaning.

## Components

| BPF map | Size | Purpose |
|---|---|---|
| `ROM_MAP` | 262,144 u32 words | MBC bytecode. xv6 kernel at slot 0; userland at slot 0x4000+. Read-only at runtime (loaded by `upc-bootctl`). |
| `RAM_MAP` | 16,777,216 u32 words (16 MiB byte-addressable / 64 MiB word-addressable) | Working RAM. Stack, BSS, `.data`, ramdisk at 0x800000, framebuffer at SCREEN_BASE, MMIO TTY at 0xC001, CSR shadow at 0xF000+. |
| `RV2MBC_MAP` | 65,536 u32 | RV byte-addr → MBC PC translation table. Populated by the rv32i-to-mbc translator; consumed by JMPR / CALLR / MRET / SRET to land function pointers / EPCs on the right MBC slot. |
| `CPU_MAP` | HashMap<u32, MbcCpuState>, 256 entries | Per-UPC-instance register file + flags + privilege level + reservation address + interrupt state. 136 bytes per instance. |
| `PROC_TABLE` | 8 slots × 32 u32 | Phase 1.2/1.3 per-pid process state. `pgd_base_for_pid()` carves the high pgd region per process. |
| `TLB_MAP` | 64 entries | Direct-mapped TLB for `translate_address`. Only active when `cpu.mmu_enabled = 1` (Doom-mode keeps it flat). |
| `TTY_MAP` | 4096 bytes | Circular byte ring. Bytes written to MMIO 0xC001 land here; `upc-bootctl` drains over the BPF ring buffer. |
| `SCREEN_MAP` | 320×200 bytes | Doom framebuffer (palettized). Byte stores to `SCREEN_BASE` write here AND to RAM_MAP for read consistency. |
| `STATS` | HashMap | Per-instance counters: insns executed, syscalls, halts, ROM faults, frame ready ticks. |

The `MbcCpuState` struct (in `crates/doom-runner/src/memory.rs` — the source-of-truth) packs 16 GPRs (r0–r15), PC, SP, flags, halted/stalled, sleep_until_ns, insn_count, cache_hits/misses, interrupt state, tick_counter, program_break, exit_code, current_pid, num_processes, mmu_enabled, page_dir_base, priv_level, and reservation_address into 136 bytes.

## The MBC ISA

MBC ("Monad Bytecode") is the UPC's native instruction set. It is a custom 32-bit fixed-width ISA designed for fast eBPF dispatch and easy translation from RV32I. Encoding:

```
 31     24 23 20 19 16 15            0
+---------+-----+-----+----------------+
| opcode  | dst | src |     imm16      |
+---------+-----+-----+----------------+
```

Opcodes are 8 bits → 256 slots. Implemented set covers:

- Arithmetic: ADD / SUB / MUL / DIV / MOD / NEG / AND / OR / XOR / NOT / SHL / SHR / SAR, with register-register and immediate variants (MOVI, ADDI, SHLR, SHRR, SARR, MULH, MULHU)
- Memory: LD / ST (word), LDB / STB (byte), LDH / STH (halfword), each going through `translate_address` for MMU support
- Control flow: CALL (24-bit absolute), JMP (24-bit PC-relative signed), JZ/JNZ/JN/JP (conditional relative branches), CALLR / JMPR (indirect via register, with RV2MBC_MAP lookup), RET (jumps to r14 or rv2mbc[r14] depending on r14 size)
- Privilege transitions: MRET (0x47), SRET (0x48) — read MEPC/SEPC from memory-mapped CSR region, translate via RV2MBC_MAP, set `cpu.priv_level` from MSTATUS.MPP / SSTATUS.SPP
- Atomics: LR.W (0x49) / SC.W (0x4A) — RISC-V load-reserved / store-conditional, used by xv6 spinlocks
- Memory barrier: FENCE (0x3F) — no-op on single-CPU UPC
- System: SYSCALL (0x40 — RV32I `ecall` translation), INT 0x80 (Linux-compat syscall path), HALT (0xFF)

The five privilege/atomic opcodes (FENCE / MRET / SRET / LR.W / SC.W) are gated behind `cfg!(feature = "ascend-linux")`. Without the feature flag they short-circuit to NOPs so the BPF verifier on kernel 6.17 doesn't blow its complexity budget for the Doom build.

Full ISA reference: [`docs/protocol/draft-bellis-unheaded-mbc-isa-00.md`](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md).

## The translator (rv32i-to-mbc)

Real programs are written in C, compiled to RV32I ELF with `riscv64-unknown-elf-gcc -march=rv32i_zicsr_zmmul -mabi=ilp32e`, then translated to MBC by `crates/monad-mbc/src/bin/rv32i_to_mbc.rs`.

Key constraints:

- **`-ffixed-x18` through `-ffixed-x31`** pin the compiler off the upper half of the RV register file. MBC has 16 GPRs (r0–r15); RV has 32 (x0–x31). The translator's `map_register` covers x0–x15 directly plus a spill-shadow for x16 (a6, → r2 / tp) and x17 (a7, → r1 / gp) backed by RAM[byte 0x64000] / RAM[byte 0x64004].
- **CALL / branch targets**: CALL imm is the absolute 24-bit MBC PC (within the source `.mbc` file's own zero-based stream). JMP / JZ / JNZ etc. use signed PC-relative 24-bit offsets, which survive any load-time shift unchanged. Userland binaries get their CALL targets rewritten by `upc-bootctl --userland` to add USER_ROM_BASE (= 0x4000) — see [Linux on UPC](Linux-on-UPC) for the why.
- **Sidecars**: every `.mbc` emits two sidecars next to it.
  - `.rv2mbc` — flat u32 LE array, index = RV word offset, value = MBC PC. Loaded into `RV2MBC_MAP` (with a `text_rv_word_base` shift so absolute RV byte addresses index correctly).
  - `.data` — TLV-format dump of every ALLOC PROGBITS section that lacks SHF_EXECINSTR (`.rodata`, `.srodata`, `.data`, `.sdata`). Loaded into `RAM_MAP` at each record's link-time byte address by `upc-bootctl`.

The translator lives at `crates/monad-mbc/`. The rv2mbc table is consulted at runtime by JMPR / CALLR / MRET / SRET to map RV byte addresses (function pointers, EPC values, switch tables, etc.) onto MBC slots.

## Boot Protocol v2

`upc-bootctl boot` sets up the BPF maps and dispatches a Gjallarhorn trigger packet. The sequence (per [`docs/doom/UPC_BOOT_PROTOCOL_V2.md`](../docs/doom/UPC_BOOT_PROTOCOL_V2.md), ADR-067 / ADR-072):

1. Allocate `MbcCpuState` slot for the requested instance ID (CPU_MAP key = low byte).
2. Zero IVT region (byte 0x0000–0x03FF).
3. Zero CSR region (byte 0x000_F000–0x000_F0FF).
4. Write a default HALT trap handler at byte 0x0400.
5. Write `BootParams v2` (256 bytes) at byte 0x0100 — magic `0x554E4844` ("UNHD" canonical / "DHNU" LE wire), version 2, kernel/ramdisk/cmdline addrs, 32-byte random seed.
6. Load `mbc_words` into ROM_MAP starting at slot 0 (Phase 1.1 xv6 path; the bootstub flow uses a different layout).
7. Optionally load `.rv2mbc` into RV2MBC_MAP, `.data` into RAM_MAP, ramdisk into RAM_MAP at 0x00800000, and a userland MBC image into ROM_MAP at slot 0x4000+ with CALL-target patching.
8. Optionally enforce a SHA-256 integrity gate on the rv2mbc bytes via `UPC_RV2MBC_SHA` env var (Phase 1.3 Step 6).
9. Initialise `MbcCpuState`: PC=0, SP=0x03F00000, priv_level=0 (M-mode), reservation_address=0xFFFFFFFF, no MMU.
10. Attach the XDP program to `veth-upc0` and dispatch Monad trigger packets.

The `upc-bootctl` CLI (`cmd/upc-bootctl/`) is the canonical boot dispatcher.

## Privilege model

Three levels, mapped to the BPF interpreter's `cpu.priv_level`:

| priv | RISC-V analogue | Notes |
|---|---|---|
| 0 | M-mode | Reset state. `start_mbc.c` runs here, sets up MEPC + MSTATUS.MPP, issues `mret`. |
| 1 | S-mode | xv6 kernel. Most boot work happens here. `kvminithart` writes SATP but the BPF interpreter doesn't actually consult it (translate_address is its own substrate when `mmu_enabled == 0`). |
| 3 | U-mode | xv6 init proc. Reached via the userret trampoline + `sret`. Phase 1.5 confirmed live (2026-05-14). |

Transitions are *only* through the MRET / SRET opcodes, both gated behind `ascend-linux`. Each reads MEPC / SEPC from the memory-mapped CSR shadow, looks up the target in RV2MBC_MAP, and falls back to `cpu.halted = 1` on an unmapped slot — this catches the "PC silently rerouted to 0" class of bug.

## Demo surfaces

UPC output goes to **MMIO byte 0xC001** (one byte per store). The XDP program intercepts writes to 0xC000–0xC003 and pushes them to `TTY_MAP` (a 4096-byte ring). Surfaces:

- **Mode A**: `cmd/upc-tty-bridge/` accepts TTY drain over HTTP, serves an xterm.js client at `dashboard/upc-console.html` over WebSocket (port 26100).
- **Mode B**: direct host pty (skeleton).
- **Mode C** (planned, post-Phase-4): SSH-over-IPv6 session.

The framebuffer at `SCREEN_MAP` (320×200 palettized) is the Doom path; the doom-runner Rust binary streams it to a browser canvas over WebSocket.

## Dream Ladder

The full UPC build-out is the **Dream Ladder** — a six-level ascent from packet stamping to running Linux:

| Level | Goal | Status |
|---|---|---|
| L1 | Basic eBPF: XDP attach, packet parse, flow table | ✅ shipped (alpha era) |
| L2 | Monad header stamping, CRC-16 valid | ✅ shipped |
| L3 | Framebuffer rendering at 35 fps | ✅ shipped (Doom-on-Monad) |
| L4 | OS primitives: syscalls, interrupts, scheduler, MMU | ✅ shipped through L4f |
| L5 | xv6 kernel boots on UPC | ✅ Phase 1.4/1.5 (2026-05-14): xv6 kernel boots, enters user mode |
| L6 | Linux boots on UPC | ⏳ Phase 2+ (uClinux first, then full Linux with MMU) |

Per-level detail: [UPC Dream Ladder](UPC-Dream-Ladder). The current frontier is [Linux on UPC](Linux-on-UPC). The L3 ascent is documented at [Doom on UPC](Doom-on-UPC).

## Why this exists

The UPC is the Kingdom's computational-completeness proof. The Unheaded Protocol claims that packet processing IS computation; the UPC is what makes that claim falsifiable. Doom-on-Monad ran a real Turing-complete program inside the eBPF runtime. xv6-on-UPC closes the loop: a real OS, with real privilege levels, real syscalls, real user processes, all dispatched by the same XDP program that handles a forwarded HTTP request.

The longer-term play is that everything Unheaded does — gateway routing, drift detection, scheduler decisions, observability event correlation — eventually compiles down to MBC and runs on the UPC. The eBPF dispatch path is the only path. There is no separate userspace coordinator. The kernel ring 0 IS the application.

## Cross-references

- [Linux on UPC](Linux-on-UPC) — ASCEND-LINUX status, Phase 0 through Phase 1.5
- [Doom on UPC](Doom-on-UPC) — the L3 computational-completeness proof
- [UPC Dream Ladder](UPC-Dream-Ladder) — level-by-level gates
- [MBC ISA Reference](MBC-ISA-Reference) — opcode + encoding reference
- [Protocol Foundation](Protocol-Foundation) — the Monad wire format the UPC dispatches on
- ADR-067 — MBC ISA v2 + UPC ABI v1
- ADR-072 — BOOT_MAGIC byte-ordering convention
- ADR-074 — Phase 1.2 page-table model (per-pid pgd, Option A)
- ADR-075 — Phase 1.3 process model

---

> **Source:** [docs/doom/ARCHITECTURE.md](../docs/doom/ARCHITECTURE.md) · [docs/doom/UPC_BOOT_PROTOCOL_V2.md](../docs/doom/UPC_BOOT_PROTOCOL_V2.md) · [docs/protocol/draft-bellis-unheaded-mbc-isa-00.md](../docs/protocol/draft-bellis-unheaded-mbc-isa-00.md) · [ebpf/monad-cpu-ebpf/src/main.rs](../ebpf/monad-cpu-ebpf/src/main.rs) · [crates/doom-runner/src/memory.rs](../crates/doom-runner/src/memory.rs) · [cmd/upc-bootctl/](../cmd/upc-bootctl/)
