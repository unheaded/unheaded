# Computational Generality: Beyond DOOM

## Hypothesis

If DOOM (a complex 1993 real-time game with a full 3D engine) successfully runs compiled-to-RV32I inside BPF maps via the Monad CPU, then any software that compiles to RV32I can run in the same substrate.

This document explores what applications beyond DOOM become feasible as proof-of-concept executables for the Unheaded platform, demonstrating that the BPF compute substrate is not specialized for DOOM but genuinely general-purpose.

---

## Candidate 1: SNES Emulator (bsnes-classic or snes9x)

### Overview

The Super Nintendo Entertainment System (SNES) is a 16-bit console from 1990-2003 with significant computational complexity. Emulating it requires:
- CPU emulation (65c816 processor)
- GPU emulation (Mode 7 graphics, sprite rendering)
- Audio synthesis (8 channels, pitch bending, envelope)
- Input handling (controller polling)

### Key Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| ROM size | ~4 MB | Super Mario World; varies by game (32 KB – 8 MB) |
| RAM needed | 128 KB SRAM | Main working memory |
| VRAM needed | 64 KB | Tile/sprite graphics memory |
| Total RAM | ~200 KB | Well within RAM_MAP capacity |
| Codebase | ~150K LOC | bsnes-classic C++ → RV32I cross-compile |
| MBC size estimate | ~3–5 MB | Reasonable for ROM_MAP |

### Implementation Path

1. **Emulator Choice**:
   - **bsnes-classic** (150K LOC, cycle-accurate, modern C++): Harder to cross-compile but most faithful
   - **snes9x-mini** (80K LOC, event-driven, C): Easier cross-compile, good balance
   - **SNES9X-2002 libretro core** (50K LOC, minimal, C): Easiest but less accurate

2. **Cross-Compilation**: C++ → RV32I requires:
   - GCC 13+ with RV32I support
   - Standard library (newlib or musl)
   - No floating-point: All emulation is integer arithmetic
   - No dynamic memory allocation: Preallocate ROM, RAM, VRAM statically

3. **Syscalls Needed**:
   - `SCREEN_WRITE`: 256x224 @ 60 Hz framebuffer (existing)
   - `KBD_READ`: Joypad buttons and D-pad (existing)
   - `AUDIO_WRITE` (NEW): 8 channels, 16-bit PCM, ~44 KHz (not yet implemented)
   - Optional: `TIMER_READ` for sound synchronization

4. **Audio Challenge**:
   - SNES generates ~705,600 samples/sec (44100 Hz stereo)
   - Monad CPU @ 2 GHz can theoretically compute ~500 MHz / sample
   - Sufficient for simple mixing, borderline for synthesis
   - May require real-time sample generation (no buffering)

### Verdict: **FEASIBLE WITH AUDIO_WRITE SYSCALL**

Adding audio output syscall is straightforward. Audio synthesis is CPU-bound but tractable. SNES9X-2002 is recommended for fastest path to playable demo.

**Estimated Effort**:
- 2 weeks: Cross-compile snes9x-mini, debug RV32I issues
- 1 week: Implement AUDIO_WRITE syscall in kernel BPF program
- 1 week: Integrate and test with popular ROM (e.g., Super Mario World)

---

## Candidate 2: Unix v4 (1973) Operating System

### Overview

The original Unix, written in PDP-11 assembly for the DEC PDP-11 computer (1973). Only ~50 KB of code—trivially small. A complete working operating system demonstrating:
- Process management (fork/exec)
- File system abstraction
- Shell interpreter (sh)
- System calls (read, write, open, close, kill, etc.)

### Key Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| Kernel size | ~10 KB | Tiny by modern standards |
| User space tools | ~40 KB | sh, cat, ls, cp, mv, rm, etc. |
| Total ROM | ~50 KB | Comfortably fits ROM_MAP |
| RAM needed | ~16 KB | Original Unix could run on 8 KB |
| Language | PDP-11 asm + C | Available from Unix Heritage Society |
| MBC size estimate | ~50 KB | Single ROM_MAP chunk |

### Critical Challenge: Architecture Mismatch

**Unix v4 is PDP-11 code. Monad runs RV32I.**

To run Unix v4 on Monad, we must **emulate a PDP-11 inside RV32I**.

This is meta-emulation:
```
PDP-11 Binary → (PDP-11 Emulator in RV32I) → Monad RV32I CPU
```

### Implementation Path

1. **PDP-11 Emulator in C**:
   - **pdp11-js** (~2K LOC, JavaScript reference): Convert to C, compile to RV32I
   - **SIMH PDP11** (~10K LOC, full-featured): Overkill but portable
   - Custom lightweight emulator: ~3K LOC for basic instruction set

2. **Emulator Overhead**:
   - Each PDP-11 instruction → 50–100 RV32I instructions (rough estimate)
   - Wall-clock slowdown: ~100–1000x (depends on instruction frequency)
   - PDP-11 @ 1 MHz → Monad @ 2 GHz → effective ~0.1–1 MHz PDP-11
   - Acceptable for running early Unix shell sessions

3. **I/O Virtualization**:
   - PDP-11 Unix expects disk (RK05, RP04) and terminal (TTY)
   - Map Unix syscalls to Unheaded syscalls:
     - `read(0, buf)` (terminal) → KBD_READ
     - `write(1, buf)` (terminal) → SCREEN_WRITE
     - File operations → Stub (emulated RAM filesystem)

4. **Minimal ROM Approach**:
   - Strip unused drivers (magnetic tape, printer)
   - Use in-RAM filesystem (only /tmp, /bin, /etc)
   - No disk I/O needed for demo

### Verdict: **FEASIBLE VIA META-EMULATION**

Running Unix v4 itself (PDP-11 assembly) is impractical. But running it inside a PDP-11 emulator compiled to RV32I is completely feasible—and demonstrates the generality more dramatically than a native port.

**Estimated Effort**:
- 3 days: Port lightweight PDP-11 emulator (simH or custom) to C, compile to RV32I
- 2 days: Wire up Unix syscalls to Unheaded syscalls
- 1 week: Debug and test shell operations
- 1 day: Create demo script (login, run commands, exit)

**Impact**: Demonstrates that Unheaded can host *historical* software, not just modern games.

---

## Candidate 3: Game Boy Emulator (SameBoy or Gambatte)

### Overview

The Nintendo Game Boy (1989–2008) is a handheld console with a Z80-like processor and 160x144 LCD display. Emulating it is **far simpler** than an SNES because:
- Simpler CPU (Sharp LR35902, derived from Z80)
- Monochrome or simple color palette (10-bit per pixel)
- No audio synthesis (only sound effects, square waves)
- Minimal RAM (8 KB)

### Key Metrics

| Metric | Value | Notes |
|--------|-------|-------|
| ROM size | 32 KB – 512 KB | Tetris = 32 KB, Super Mario Land = 64 KB |
| RAM needed | 8 KB | Game Boy SRAM; cartridge SRAM varies |
| VRAM needed | 8 KB | Tile maps and sprite data |
| Total RAM | ~32 KB | Trivial allocation |
| Emulator codebase | 20K–50K LOC | SameBoy (C), Gambatte libretro (C++) |
| MBC size estimate | ~100 KB | Emulator + bundled ROM |

### Implementation Path

1. **Emulator Choice**:
   - **SameBoy** (~20K LOC, accurate, C): Best choice for Unheaded
   - **Gambatte** (~30K LOC, highly accurate, C++): Larger but well-tested
   - **TinyGBEmu** (~5K LOC, minimal, C): Fastest port

2. **CPU Emulation**: Z80 → RV32I
   - Z80 instruction set: 256 instructions (vs. RV32I's 40 core instructions)
   - Straightforward 1:1 instruction mapping
   - Registers: A, B, C, D, E, F, H, L, PC, SP → 8–16 RV32I registers

3. **GPU Emulation**:
   - Game Boy screen: 160x144 @ 60 Hz = ~1.4 MB/sec frame rate
   - Framebuffer: 160x144 = 23 KB per frame
   - Compatible with existing SCREEN_WRITE syscall

4. **Audio**:
   - Game Boy has 4 channels: 2 square, 1 wave, 1 noise
   - Simple synthesis, negligible CPU cost
   - Can route to AUDIO_WRITE (once implemented)
   - Or silent emulation (games still playable without sound)

### Verdict: **MOST FEASIBLE NEXT TARGET**

Game Boy emulation is the sweet spot: realistic complexity, complete implementability, and **immediate playability**. Tetris, Mario, Zelda, all run. Sound can be added later.

**Estimated Effort**:
- 1 week: Port SameBoy to RV32I (minimal changes, mostly C)
- 3 days: Integrate with SCREEN_WRITE and KBD_READ
- 2 days: Test with 3–5 public-domain ROMs (hello.gb, Tetris, etc.)
- 1 day: Optimize and profile

**Win Condition**: Full playability of Tetris or Game Boy version of Mario Land within Monad, proof that emulator targets work.

---

## Comparison Matrix

| Target | Feasibility | Effort | Impact | Notes |
|--------|-------------|--------|--------|-------|
| SNES | Moderate | 4–5 weeks | High (AAA graphics) | Requires AUDIO_WRITE |
| Unix v4 | High | 2 weeks | Very High (historical OS) | Via PDP-11 emulation |
| Game Boy | **Very High** | 1–2 weeks | High (iconic games) | **Recommend next** |

---

## Recommendation: Pursue Game Boy Emulator First

### Rationale

1. **Shortest path to success**: Minimal dependencies, straightforward C port
2. **Immediate validation**: Playable games within 1–2 weeks
3. **Building block**: Once Game Boy works, SNES emulator can reuse infrastructure
4. **Proof of concept**: Demonstrates Unheaded is a genuine general-purpose compute platform
5. **Community engagement**: Game Boy emulation has vibrant hobbyist ecosystem

### Roadmap

```
Phase 1 (Current): DOOM — establish Unheaded's viability
Phase 2 (2 weeks): Game Boy Emulator — prove generality
Phase 3 (1 month): SNES + Audio Syscall — ambitious multimedia
Phase 4 (ongoing): Port other RV32I targets (retro CPUs, minimal OSes, etc.)
```

---

## Conclusion

The hypothesis holds: **Unheaded's BPF compute substrate is general-purpose.**

DOOM is not a special case—it's the first data point on a continuum. With DOOM validated, the next priorities are:

1. **Game Boy** (quickest win, highest impact)
2. **SNES** (more ambitious, requires new syscall)
3. **Unix v4** (historical validation, meta-emulation showcase)

Each successive target strengthens the claim that arbitrary RV32I binaries can run inside BPF maps, positioning Unheaded as a novel computing platform for security research, emulation, and exotic workloads.

### Open Questions

- Can we achieve interactive frame rates for SNES @ 60 FPS on Monad?
- How much CPU overhead does PDP-11 meta-emulation impose?
- What is the maximum ROM size BPF maps can reasonably handle (8 MB? 16 MB)?
- Can we run multiple emulator instances simultaneously in separate BPF programs?

These questions will guide Phase 3+ development.
