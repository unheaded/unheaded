# Phase 8: Doom Main Loop Entry - Results

## Execution Summary

### BSS Clearing Analysis
- CRT0 byte-clearing loop at PC 103937-103945
- CRT0 word-clearing loop at PC 20-28
- BSS range: 0x00FFFB9C to 0x01FFF738 (16,776,092 bytes = ~16MB)
- Loop is 8 instructions per byte (byte-clear) or 9 per word (word-clear)
- Each complete CRT0 cycle: ~135M instructions

### Critical Finding: RAM_MAP Too Small
- **RAM_MAP capacity**: 2,097,152 entries x 4 bytes = 8,388,608 bytes (8MB)
- **Doom BSS start**: 0x00FFFB9C (byte address ~16MB)
- **Doom BSS end**: 0x01FFF738 (byte address ~32MB)
- **ENTIRE BSS is above RAM_MAP maximum address (0x00800000)**
- All STB/ST writes to BSS addresses silently fail (out-of-range)
- Doom cannot initialize any BSS variables
- D_DoomMain returns immediately due to uninitialized state
- CRT0 re-calls main() in infinite loop

### Ring Circulation Issue
- Packets only execute 1 XDP hop (16 instructions) per injection
- Ring circulation (6 hops x 255 laps = 4,080 insns/pkt) NOT working
- Cause: kernel not forwarding packets after XDP_PASS
- Likely due to xdpgeneric interaction with veth forwarding
- Workaround: fast injection at 800K pkt/s gives ~12.8M insns/sec

### Performance Metrics
- Fast injection rate: ~800K packets/sec (zero-delay)
- Instructions per packet: 16 (single hop)
- Effective throughput: ~12.8M instructions/sec
- BSS clearing time (if RAM worked): ~10.5 seconds per pass

### Instruction Encoding Corrections
- 0x30 = LD (load word), NOT LOAD
- 0x31 = ST (store word), NOT LOADB
- 0x32 = LDB (load byte)
- 0x33 = STB (store byte)
- 0x27 = CALL, NOT 0x28
- 0x28 = RET, NOT CALL
- Branches: PC pre-incremented before offset applied

### D_DoomMain Entry Point
- main() at PC 7250 (confirmed)
- D_DoomMain() at PC 5602 (called from main() at PC 7256)
- __libc_init_array at PC 103061 (called from main() at PC 7255)

## Required Fix for Phase 9
1. **Increase RAM_MAP to ≥32MB** (8,388,608 entries x 4 bytes = 32MB)
   - Or ≥16MB minimum: 4,194,304 entries
   - Current: 2,097,152 entries (8MB)
2. **Fix ring circulation** for 255x throughput improvement
3. Re-run CRT0 with enlarged RAM_MAP
