# Doom on UPC

Running id Software's original 1993 DOOM on the [Unheaded Protocol Computer](UPC-Overview). The computational-completeness proof for the Unheaded Protocol — every Doom frame is rendered by an XDP program executing MBC bytecode that arrived as IPv6 packets through a network namespace ring.

**Status:** PLAYABLE. Menus, movement, shooting, doors all work. Stable beyond 5.9 B instructions executed. The framebuffer streams to a browser canvas over WebSocket; keyboard input flows back the same way.

## Why this exists

To prove the Unheaded Protocol claim that "packet processing IS computation" — and that the BPF runtime is a real CPU, not a toy. Doom is a non-trivial program: 62 C source files, dynamic memory allocation, fixed-point arithmetic, a software 3D renderer (BSP walk + column rasterizer), a finite-state-machine sound system, a custom palette manager, runtime WAD asset loading. If a 1993 game engine runs to completion at 35 fps on the UPC, the substrate is Turing-complete in practice.

## The pipeline

```
~/tmp/projects/DOOM/linuxdoom-1.10/    id Software GPL-2.0 source, 62 .c files, pristine
                  +
demos/doom/                            UPC platform stubs (replacements for i_video, i_sound,
                                       i_net, i_system) + libc_stubs + gcc_runtime + crt0
                  |
                  v
        riscv64-unknown-elf-gcc        -march=rv32i_zicsr_zmmul -mabi=ilp32e
                                       -ffixed-x16 ... -ffixed-x31
                  |
                  v
            doom.elf (RV32I)            16-register subset, no M-extension
                  |
                  v
       crates/monad-mbc/...rv32i-to-mbc  Custom translator
                  |
                  v
       doom.mbc + doom.rv2mbc + doom.data
                  |
                  v
       crates/doom-runner (Aya)        Loads MBC into ROM_MAP, populates RAM_MAP from .data,
                                       installs the rv2mbc table, attaches the XDP program,
                                       wires the framebuffer bridge.
                  |
        +---------+---------+
        |  veth-upc0        |
        v                   |
   +------------+   IPv6/HbH   ... ring of namespaces ...
   |  XDP prog  |---------->
   | monad-cpu- |  256 MBC insns/packet
   |   ebpf     |<----------
   +-----+------+
         |
         v
   SCREEN_MAP (320×200 palettized)
         |
         v
   doom-runner/src/bridge.rs           WebSocket → browser canvas
         |
         v
   Keyboard input ----- 8-slot queue → KBD_MAP ----- back into MBC
```

## Components

| Path | What it does |
|---|---|
| `~/tmp/projects/DOOM/linuxdoom-1.10/` | The pristine id Software DOOM source. GPL-2.0. Lives outside the repo to keep the kingdom's GPL boundary clean. |
| `~/tmp/projects/doom-related/DOOM.WAD` | The retail Steam WAD. 12,408,292 bytes. Not redistributed; user-supplied. |
| `demos/doom/i_video_mbc.c` | Platform glue: back-buffer rendering, `I_FinishUpdate` word-copy to SCREEN_MAP, palette management, keyboard queue. |
| `demos/doom/i_sound_mbc.c` | No-op sound stubs. |
| `demos/doom/i_net_mbc.c` | No-op net stubs (the network IS the CPU, not the game's transport). |
| `demos/doom/i_system_mbc.c` | Syscall-style hooks for `I_Quit`, `I_GetTime`, `I_Error`. |
| `demos/doom/libc_stubs.c` | 111+ libc functions: a JVM-style heap, word-aligned memcpy/memmove, snprintf, etc. |
| `demos/doom/gcc_runtime.c` | Software div / mul helpers (RV32I lacks the M extension; this is faster than libgcc). |
| `demos/doom/crt0.S` | Bare-metal startup. BSS zeroing, stack init, jump to `main`. |
| `demos/doom/w_file_stdc.c` | Static WAD file handles (avoids Z_Malloc corruption). |
| `demos/doom/linker.ld` | Memory regions: ROM, RAM, SCREEN, HEAP, WAD. |
| `crates/doom-runner/src/memory.rs` | **The** UPC memory-layout source-of-truth. |
| `crates/doom-runner/src/bridge.rs` | WebSocket bridge. 8-slot keyboard queue. Bilinear CSS upscale. Dynamic palette. |
| `crates/doom-runner/src/loader.rs` | ELF parser, MBC / WAD / rv2mbc / data file loaders. |
| `crates/doom-runner/src/main.rs` | Launch pipeline (load → verify → serve). |
| `crates/doom-runner/src/tail_calls.rs` | Tail-call chain configuration. |
| `ebpf/monad-cpu-ebpf/src/main.rs` | The XDP executor + MBC dispatch + tail calls. Same binary that runs xv6, gated on `--features ascend-linux` for the L5 opcodes. |

## Memory layout

Single canonical reference: `crates/doom-runner/src/memory.rs`. Key byte addresses:

```
0x00000000   ROM_MAP base (MBC code)
0x00010000   RAM start (loaded from doom.data)
0x00070000   SCREEN_BASE  — 320×200 byte framebuffer
0x00073E80   SCREEN_END   — first byte past framebuffer
0x000C0000   HEAP_BASE
0x00200000   WAD_BASE     — IWAD/PWAD loaded here
0x03F00000   STACK_TOP    — grows down
0x0000C001   TTY MMIO data (one byte per store)
0x0000FFFF   KBD MMIO     — keyboard byte
```

The framebuffer SCREEN_BASE was moved from `0xC000` to `0x70000` on 2026-03-03 (commit `c7831cad`) to free the low-MMIO range for TTY + future MMIO devices. Three monad-mbc test fixtures lagged 65 days behind that move; the regression was caught + closed on 2026-05-09.

## Tail-call cadence

XDP programs can tail-call up to 33 times in kernel 6.x. The doom-runner config (`crates/doom-runner/src/tail_calls.rs`) uses 15 additional rounds — 16 total executions per packet, 16 MBC instructions per round, **256 MBC instructions per packet**.

At 35 fps with ~100 K MBC insns per frame, the CPU needs ~3.5 M insns/sec sustained. Empirically Doom runs faster than realtime when the netns ring is hot; the limiting factor is the kbd-queue scan rate, not raw compute.

## Bug-Of-The-Day journal

The Doom path took a year of hard debugging. Highlights live in [`docs/doom/FINDINGS.md`](../docs/doom/FINDINGS.md). The key ones that informed the broader UPC design:

- **Bug 20**: function-pointer JMPR landing on stale rv2mbc slots. Fix: skip the JMPR rather than halt, log a sentinel at 0xE0000. Phase 1.4 then tightened this with a non-zero guard so user VA 0 stops silently rerouting PC to 0.
- **Bug 24**: word stores were leaking into SCREEN_MAP whenever a corrupted pointer landed in the 0x70000–0x73E80 range. Fix: only byte stores (STB → mem_write_byte) update SCREEN_MAP; word stores (ST) stay out.
- **Bug 32**: x16/x17 spill-shadow on r2/r1 wrote into low RAM colliding with `.rodata` at byte 0. Moved spill region to byte 0x64000 / 0x64004 (in the gap between BSS end and WAD).
- **PC corruption bug** (March 2026): MBC PC randomly walked into NOP regions. Root cause: `__umoddi3` / `__udivdi3` were stubbed too aggressively; some 64-bit divides clobbered the link register through libgcc-style trampolines. Fix in `demos/doom/gcc_runtime.c`.

## Building + running

Full guide: [`docs/doom/DOOM-BUILD-GUIDE.md`](../docs/doom/DOOM-BUILD-GUIDE.md). Short form:

```bash
cd ~/tmp/unheaded
cd demos/doom
make    # builds doom.elf → doom.mbc + doom.rv2mbc + doom.data

cd ../../crates/doom-runner
cargo build --release

# Launches doom-runner, attaches XDP, serves WebSocket on port 16666:
./target/release/doom-runner \
    --mbc ../../demos/doom/doom.mbc \
    --wad ~/tmp/projects/doom-related/DOOM.WAD
```

Then open `http://localhost:16666/` for the browser canvas + keyboard relay.

## Computational-generality proof

Doom-on-Monad is documented as a formal proof of computational generality in [`docs/doom/COMPUTATIONAL_GENERALITY.md`](../docs/doom/COMPUTATIONAL_GENERALITY.md). The argument: if a non-trivial Turing-complete program (Doom) runs to completion on a substrate (the UPC), the substrate is Turing-complete. The proof relies on:

- Doom uses dynamic dispatch (function pointers through the BSP traversal), unbounded recursion (via tail-call elimination + heap allocation), arbitrary memory R/W (the Z_Malloc heap), and conditional branches (the entire FSM-driven enemy AI).
- Every one of those compiles to MBC and dispatches through the XDP program.
- The XDP program is purely a function from `(MBC bytecode, packet input)` to `(MBC bytecode state after 256 insns, packet output)`. No external state, no out-of-band side channels.

## Cross-references

- [UPC Overview](UPC-Overview) — the substrate Doom runs on
- [Linux on UPC](Linux-on-UPC) — the L5 sibling project sharing the same XDP program
- [MBC ISA Reference](MBC-ISA-Reference) — what Doom's compiled instructions are
- [`docs/doom/`](../docs/doom/) — 31 documents covering architecture, build guide, findings, BPF maps, baseline benchmarks, computational-generality proof, and the Doom-Bridge architecture

---

> **Source:** [docs/doom/README.md](../docs/doom/README.md) · [docs/doom/ARCHITECTURE.md](../docs/doom/ARCHITECTURE.md) · [docs/doom/FINDINGS.md](../docs/doom/FINDINGS.md) · [docs/doom/COMPUTATIONAL_GENERALITY.md](../docs/doom/COMPUTATIONAL_GENERALITY.md) · [crates/doom-runner/](../crates/doom-runner/) · [demos/doom/](../demos/doom/)
