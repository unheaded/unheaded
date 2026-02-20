# Session S19 — Handoff Document

**Date:** 2026-02-20
**Status:** Paused mid-debug

---

## What Was Done This Session

### 1. Recovered from hung `doom-ring.sh setup`
- Previous session left PID 7295 (ebpf-loader) and PID 7296 (doom-ring.sh) stuck
- All 6 namespaces (monad0-5) already existed, and hops 1-5 had XDP attached (prog_id 35)
- Hop 0 (veth50p in monad0) was missing XDP — manually attached:
  ```
  sudo ip netns exec monad0 bpftool net attach xdpgeneric id 35 dev veth50p
  ```

### 2. Reloaded all BPF maps
- ROM: 105,970 MBC instructions loaded
- RV2MBC: 46,169 address translation entries
- Data (.rodata/.data): 32,643 words at 0x2d160
- WAD (doom1.wad): 998,293 words at 0x110000
- CPU initialized: PC=0, SP=0x1000000

### 3. Execution progress
- Injected 15,100 packets total (~61.6M instructions)
- BSS clearing completed successfully (as expected, ~60M insns)
- **Doom got past the WAD discovery / access() gate** (halted=0 at 61.6M insns)
  - This confirms the access() fix from the previous session is working
- Injected 5,000 more packets (~20.4M more instructions, 82M total)
- Throughput: ~6,000 pkt/s sustained

### 4. Write-through cache deployed
The BPF program (already built before the hang) adds write-through for all store instructions:
- `ST` (32-bit): direct RAM_MAP insert after L1 store
- `STB` (8-bit): read-modify-write in RAM_MAP after L1 store
- `STH` (16-bit): read-modify-write in RAM_MAP after L1 store

This ensures data persists beyond L1 cache eviction. Confirmed working:
7.7M cache_hits after BSS, 115 cache_misses.

---

## Where We're Stuck

### PC stuck at 103940 after ~82M instructions

**Symptoms:**
- PC = 103940 (0x19604) — does not change between packet batches
- insn_count increases by exactly the expected amount (20.4M for 5K packets)
- Most registers unchanged between samples except r13 (24,408,900 → 26,958,900)
- halted=0, stalled=0, flags=0x06

**Possible explanations:**
1. **Tight loop**: A loop whose length divides MAX_INSN_PER_TICK (16), so the PC always lands at the same spot when the packet's budget expires
2. **Instruction encoding bug**: See below

### Possible instruction encoding mismatch

**Finding:** When reading the MBC file and interpreting as LE u32, many instructions decode to opcode 0x00 (invalid/NOP). For example:
- First word: bytes `00 01 f0 0f` → LE u32 = 0x0FF00100 → opc = 0x00

**What the translator does:**
1. `MbcInsn::encode(opcode, dst, src, imm)` packs as: `opcode | (dst << 8) | (src << 12) | (imm << 16)`
2. The inner `u32` is pushed into a `Vec<u32>`
3. Written to file via `.to_le_bytes()`

**What the ROM loader does:**
1. Reads raw 4-byte chunks from the MBC file
2. Writes them directly to BPF Array<u32> map via `BPF_MAP_UPDATE_ELEM`
3. On LE system, these 4 bytes become the native u32 value

**What the BPF CPU does:**
1. `let raw = ROM_MAP.get(&pc)` → reads native u32
2. `opc = raw & 0xFF`

**Round-trip should be correct** (translator writes LE → loader reads same bytes → BPF reads LE), BUT many instructions decode to opcode 0x00 regardless. Unknown opcodes are silently treated as NOP (line 621 of main.rs).

**This needs investigation:**
- Run the translator's own disassembler (`--disasm` flag) on the MBC file to see what it thinks the instructions are
- Check if the first RISC-V instruction from CRT0 translates to something with opcode 0x00
- Verify the `mbc_words` Vec<u32> content before writing to file

---

## What Needs To Be Done Next

### Priority 1: Diagnose the PC-stuck issue
1. **Decode what's at PC 103940** — use the translator's disassembler or write a standalone MBC decoder that matches the BPF code's interpretation
2. **Check if it's a real loop** — read the ROM around PC 103940, decode the instructions, trace the control flow. If there's a loop of length 1, 2, 4, 8, or 16, the sampling always hits the same PC.
3. **Add diagnostic output** — consider dumping the last N executed PCs to a ring buffer to see the actual execution trace

### Priority 2: Investigate instruction encoding
1. Run: `cargo run --bin rv32i-to-mbc -- --disasm doom/doomgeneric/doomgeneric/doom.elf /dev/null 2>&1 | head -50`
2. Compare the disassembler's output against the raw file bytes
3. If the encoding is wrong, fix either the translator or the loader

### Priority 3: If encoding is fine, continue execution
1. If the CPU is in a legitimate loop (e.g., a busy-wait or function call), inject many more packets
2. Watch for halted/stalled state changes
3. The next milestone is Doom calling `DG_Init()` and starting to render frames

---

## Current System State

| Component | Status |
|-----------|--------|
| Ring namespaces | monad0-5 UP |
| XDP attachments | All 6 hops attached (prog_id 35) |
| BPF maps | All loaded (ROM, RV2MBC, RAM, CPU) |
| CPU state | PC=103940, insn_count=82M, halted=0 |
| Write-through cache | Deployed, working |
| Packet throughput | ~6,000 pkt/s (~24.5M insns/sec) |

## Key Files Modified (uncommitted)

```
ebpf/monad-cpu-ebpf/src/main.rs   — write-through for ST/STB/STH
scripts/doom-loader-core.py        — minor change
scripts/doom-loader.sh             — data section VMA auto-detection
```

## Commands to Resume

```bash
# Check current CPU state
sudo python3 scripts/doom-cpu-dump.py

# Inject more tick packets
sudo ip netns exec monad0 python3 scripts/doom-tick.py --count 5000

# If ring needs rebuild
sudo scripts/doom-ring.sh teardown
sudo scripts/doom-ring.sh setup
sudo scripts/doom-loader.sh all
```
