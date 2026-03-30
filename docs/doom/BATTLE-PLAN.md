# DOOM BATTLE PLAN -- Choose Your Own Adventure

All paths lead to demons on screen, playable in a browser.

## Current State (Verified 2026-03-30)

| Component | Status |
|-----------|--------|
| doom-runner Aya pipeline | WORKING (loads in 1.3s, maps verified) |
| Integrated WebSocket bridge | WORKING (Firefox connects, canvas renders) |
| Tail calls (256 insns/hop) | WORKING (BTF enabled, self tail-call) |
| sendmmsg injection | WORKING (93K pps) |
| id DOOM linuxdoom-1.10 | PLAYABLE (menus, movement, shooting, doors) |
| Back buffer rendering | WORKING (screens[0]=malloc, word-copy to SCREEN_BASE) |
| 8-slot kbd circular queue | WORKING (prevents key overwrite) |
| Total bugs fixed | 23 |
| Baseline commits | 42bbc34d + 46f36f77 |

**STATUS: PLAYABLE** -- PC corruption was fixed during the id DOOM port.
Doom runs stably at 5.9B+ instructions. All forks below FORK E have been
completed. Current work is optimization and polish.

---

## Flow Diagram

```
                    START
                      |
                 +----v----+
                 |  FORK A  | Fix PC corruption
                 |  (entry) |
                 +----+----+
                      |
              +-------+-------+
              | PC fixed?     |
          YES |               | NO
              v               v
         +--------+     +--------+
         | FORK E |     | FORK B | Simplify binary
         |optimize|     |        |
         +---+----+     +---+----+
             |              |
             |         +----+----+
             |         |renders? |
             |     YES |         | NO
             |         v         v
             |    +--------++--------+
             |    | FORK D || FORK C | Native emulator
             |    |restore ||        |
             |    +---+----++---+----+
             |        |         |
             |        |    +----+----+
             |        |    |native ok|
             |        |YES |         |NO
             |        |    v         v
             |        |+--------++--------+
             |        || FORK F || FORK A | (with knowledge)
             |        ||fix MBC ||        |
             |        |+---+----++--------+
             |        |    |
             |        v    v
             |   +----------+
             +-->|  FORK E  | Optimize fps
                 |          |
                 +----+-----+
                      |
                 +----v----+
                 | FORK G  | Polish & ship
                 |         |
                 +----+----+
                      |
                      v
              +--------------+
              |  PLAYABLE    |
              |    DOOM      |
              +--------------+
```

---

## FORK A: Fix PC Corruption (RECOMMENDED FIRST)

**Entry condition:** Current state. PC goes invalid during init.

**Theory:** An indirect call (function pointer or return address) jumps to
garbage. We need to catch the exact instruction that corrupts PC.

### Steps

1. Add PC bounds check in eBPF executor (`monad-cpu-ebpf/src/main.rs`).
   After each instruction: if `pc > ROM_MAP_SIZE`, set `halted = 1` and
   record `last_valid_pc` in STATS. This catches the EXACT instruction that
   corrupts PC.

2. Rebuild eBPF, reload, inject:
   ```bash
   cd ~/tmp/unheaded/monad-cpu-ebpf
   cargo xtask build-ebpf && cargo build --release
   # reload and inject per doom-runner pipeline
   ```

3. Read `last_valid_pc` from STATS -- this tells us WHICH function jumped
   wrong:
   ```bash
   bpftool map dump name STATS | grep last_valid_pc
   ```

4. Map `last_valid_pc` back to source via `rv2mbc` + `objdump`:
   ```bash
   riscv32-unknown-elf-objdump -d doom.elf | grep -A5 "<address>"
   ```

5. Fix the function pointer / return address corruption in the identified
   function.

6. **Verification gate:** Doom reaches `DG_DrawFrame` (breadcrumb `0x0050`).

### On success --> FORK E (optimize for playable fps)

### On failure --> FORK B (simplify the binary)

---

## FORK B: Simplify the Doom Binary

**Entry condition:** PC corruption persists despite Fork A fixes. The full
Doom binary has too many code paths with potential corruption.

**Theory:** Reduce the binary to minimum viable Doom -- fewer stubs, fewer
features, fewer corruption vectors.

### Steps

1. Disable sound entirely (`S_Init` --> no-op stub).
2. Disable music (`I_InitMusic` --> no-op stub).
3. Disable network (`D_InitNetGame` --> no-op stub).
4. Disable demo recording/playback.
5. Force `-nomonsters -warp 1 1` (skip title screen, go straight to E1M1).
6. Rebuild:
   ```bash
   cd ~/tmp/unheaded/doom
   make clean && make EXTRA_CFLAGS="-DNOMONSTERS -DWARP_E1M1"
   ```
7. Test each simplification independently. Each one removes code paths and
   reduces corruption surface area.

### Verification gate

Doom renders at least one frame (breadcrumb `0x0050` fires).

### On success --> FORK D (add features back one by one)

### On failure --> FORK C (native RV32I emulator for diagnosis)

---

## FORK C: Native RV32I Emulator Diagnosis

**Entry condition:** The BPF VM is too hard to debug. We need to run Doom
natively to isolate whether the bug is in the C code or the BPF executor.

**Theory:** Run `doom.elf` in a RISC-V emulator (qemu-riscv32 or spike) with
the same stubs. If it works natively, the bug is in the BPF VM. If it crashes
natively, the bug is in the C code.

### Steps

1. Build `doom.elf` for qemu-riscv32 (same stubs, same memory layout):
   ```bash
   # Use the same linker script and stubs
   riscv32-unknown-elf-gcc -march=rv32i -mabi=ilp32 \
     -T doom.ld -o doom-qemu.elf ...
   ```

2. Run in qemu with trace enabled:
   ```bash
   qemu-riscv32 -d in_asm,cpu -D trace.log doom-qemu.elf
   ```

3. Identify where the indirect jump happens in the trace. Look for the
   first `jalr` or `jr` that targets an invalid address.

4. Compare behavior: native trace vs BPF VM execution log.

5. Fix whatever differs.

### On success (native works) --> FORK F (bug is in MBC translator or BPF executor)

### On failure (native crashes too) --> FORK A with exact crash knowledge

---

## FORK D: Feature Restoration

**Entry condition:** Simplified Doom from Fork B renders frames. Now add
features back to find the culprit.

### Steps

Binary search method:

1. Add back half the disabled features. Test.
2. If it breaks, the bug is in that half. Subdivide.
3. If it works, add the other half. Test.
4. Repeat until the single culprit feature is identified.
5. Fix the culprit and enable all features.

### Verification gate

Full Doom binary renders frames with all features re-enabled.

### On success --> FORK E (playable fps)

---

## FORK E: Optimize for Playable FPS

**Entry condition:** Doom renders frames. Now make it playable.

**Theory:** Target 35 fps. At ~3.5M insns/frame x 35 fps = 122.5M insns/sec
needed. Current throughput: 23.7M insns/sec. Need roughly 5x improvement.

### Steps

1. **Profile:** How many instructions per frame?
   ```bash
   # Check STAT_FRAME_READY delta vs INSNS delta
   bpftool map dump name STATS
   ```

2. **Optimize hot path in libc stubs:**
   - `memcpy`: word-aligned fast path (4 bytes at a time)
   - `fread`: batch reads, reduce syscall overhead
   - `memset`: word-fill fast path

3. **Increase hops:** 2 --> 3 --> 4 (one at a time, verify each):
   ```bash
   # Each hop = 256 more instructions before returning to userspace
   # Monitor for stack overflow or BTF issues
   ```

4. **Tune injection:** higher pps, larger batches.
   Current: 93K pps. Target: 200K+ pps with batch sendmmsg.

5. **Tune tail call depth:** find the sweet spot between throughput and
   latency. Too deep = input lag. Too shallow = low fps.

6. **Consider XDP_TX turbo mode:** packet bounces on same interface for
   cache-warm re-execution. Eliminates userspace round-trip entirely.

### Verification gate

Doom runs at >= 30 fps sustained (measured via frame counter delta over 10s).

### On success --> FORK G (polish and ship)

---

## FORK F: MBC Translator / BPF Executor Bug

**Entry condition:** Native RV32I (qemu) works but BPF execution does not.

**Theory:** The rv32i-to-mbc translator or the eBPF MBC executor has a bug
for a specific instruction pattern (likely an edge case in indirect jumps,
shift operations, or sign extension).

### Steps

1. Capture native trace (from Fork C) and BPF execution trace.

2. Compare instruction-by-instruction. Script it:
   ```bash
   diff <(head -10000 native-trace.log) <(head -10000 bpf-trace.log)
   ```

3. Find first divergence. This is the buggy instruction translation.

4. Fix the translator (`rv2mbc`) or executor (`monad-cpu-ebpf`).

5. Verify fix with full Doom binary.

### Verification gate

BPF execution trace matches native trace through init sequence.

### On success --> FORK A (retry with fixed executor, should now pass)

---

## FORK G: Polish and Ship

**Entry condition:** Doom is playable at >= 30 fps.

### Steps

1. **Keyboard input:** bridge --> KBD_MAP --> MBC syscall. Map WASD + arrow
   keys + space + enter + escape.

2. **Clean up debug instrumentation:** remove verbose logging, disable
   breadcrumb traces, reduce STATS polling frequency.

3. **NixOS service module:** `doom-runner.nix` with systemd service,
   automatic eBPF loading, bridge startup.

4. **Demo script:** one-command launcher for presentations:
   ```bash
   ./run-doom.sh  # starts everything, opens browser
   ```

5. **Document everything** in `docs/doom/`.

### END STATE

Playable Doom in a browser, running over IPv6 between interfaces.

---

## Recommended Path

**A --> E --> G** (most likely to succeed, least effort)

Fork A (PC bounds check) is a 30-minute fix. It tells us EXACTLY where the
corruption happens. With that information, we fix one function pointer or
return address, and Doom renders. Then optimize.

## Time Estimates

| Path | Description | Estimated Sessions |
|------|-------------|-------------------|
| A --> E --> G | Fix PC, optimize, polish | 2-4 |
| A --> B --> D --> E --> G | Fix PC fails, simplify, restore, optimize | 3-5 |
| A --> B --> C --> F --> A --> E --> G | Full debug cycle through native emulator | 4-6 |

## Decision Log

| Date | Fork | Outcome | Next |
|------|------|---------|------|
| 2026-03-29 | FORK A | PC corruption identified in doomgeneric | Pivoted to id DOOM |
| 2026-03-29 | PIVOT | Switched from doomgeneric to id linuxdoom-1.10 | Build + compile + link |
| 2026-03-29 | Phase 1-3 | id DOOM compiles, links, translates to MBC | Phase 4 (run) |
| 2026-03-29 | Phase 4-5 | WAD loads, bugs 10-20 fixed, DOOM renders | Phase 6 (play) |
| 2026-03-30 | Phase 6 | PLAYABLE! Title, menus, movement, shooting | FORK E (optimize) |
| 2026-03-30 | Polish | Palette, KBD queue, auto-repeat fixed | FORK E (fps) |
