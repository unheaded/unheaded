# S31 DOOM-OVER-IPv6 — Agent Recovery Instructions

**Date:** 2026-02-22
**Status:** Doom stuck in infinite restart loop during R_Init. Root cause: CALLR opcode handler in BPF doesn't save return address in r14 (link register). Every indirect function call (function pointers) loses the return address, causing cascading stack corruption and eventual jump to address 0 → _start → restart.
**Goal:** Get Doom to `D_DoomLoop` (first frame rendered via SYS_DRAW_FRAME)

---

## ⚠️  CRITICAL BUG — CALLR MISSING LINK REGISTER SAVE (READ THIS FIRST)

### Root Cause of Infinite Restart Loop

**Symptom:** Doom restarts D_DoomMain repeatedly during R_Init, consuming ~6MB heap per cycle until heap exhaustion → HALT. Debug buffer shows 5-8 copies of the full init sequence.

**Root cause:** In `monad-cpu-ebpf/src/main.rs`, the `CALLR` opcode handler (indirect function call via register) does NOT save the return address in r14 (link register). The `CALL` opcode (direct call) correctly does `cpu.regs[14] = cpu.pc + 1`, but `CALLR` only jumps without saving.

**Impact:** Every indirect function call (function pointers, vtable dispatch) loses the return address. When the callee executes RET, it reads a stale r14 value from a previous CALL, returning to the wrong address. Stack frames are never cleaned up, cascading corruption eventually produces a return to RV address 0 → RV2MBC_MAP[0] = MBC PC 0 → `_start` → D_DoomMain restarts.

**Why R_Init specifically:** R_InitTextures (called from R_InitData, the first thing R_Init does) processes texture data using function pointers for column drawing setup. These are the first indirect calls in Doom's init path. Earlier init stages (Z_Init, V_Init, W_Init) use only direct calls.

### The Fix (3 LINES in BPF)

In `monad-cpu-ebpf/src/main.rs`, find the CALLR handler. The CALL handler pushes a return address to the MBC call stack (r15-based); CALLR must do the same.

**Look at how CALL works (for reference):**
```rust
} else if opc == op::CALL {
    // Push PC (already incremented = return address) to stack.
    cpu.regs[15] = cpu.regs[15].wrapping_sub(1);
    let sp = cpu.regs[15];
    mem_write_word(sp, cpu.pc);
    let target = insn_word & 0x00FF_FFFF;
    cpu.pc = target;
}
```

**Now fix CALLR to do the same push:**
```rust
// BEFORE (broken — no push, RET will pop garbage):
} else if opc == op::CALLR {
    let rv_addr = cpu.regs[d];
    let rv_word = rv_addr >> 2;
    cpu.pc = match RV2MBC_MAP.get(rv_word) {
        Some(mbc_idx) => *mbc_idx,
        None => { cpu.halted = 1; increment_stat(STAT_ROM_FAULT); break; }
    };
}

// AFTER (fixed — push return address BEFORE jumping):
} else if opc == op::CALLR {
    // Push MBC return address to call stack (same protocol as CALL)
    cpu.regs[15] = cpu.regs[15].wrapping_sub(1);
    let sp = cpu.regs[15];
    mem_write_word(sp, cpu.pc);

    // Indirect jump via RV2MBC address translation
    let rv_addr = cpu.regs[d];
    let rv_word = rv_addr >> 2;
    cpu.pc = match RV2MBC_MAP.get(rv_word) {
        Some(mbc_idx) => *mbc_idx,
        None => { cpu.halted = 1; increment_stat(STAT_ROM_FAULT); break; }
    };
}
```

**Why this works:** CALL pushes MBC return addresses onto a word-addressed call stack via r15 (SP). RET pops from the same stack. CALLR (indirect calls via function pointers) was skipping the push, so RET would pop garbage or the wrong return address. Adding the same push/decrement as CALL makes indirect calls symmetrical with direct calls.

**Note:** JMPR (indirect jump WITHOUT link, used for tail calls and switch tables) should NOT push — it's a jump, not a call. Only CALLR needs the fix.

### Defense-in-Depth: RV2MBC Sentinel

After loading ROM and RV2MBC, set RV2MBC_MAP[0] to a HALT instruction's MBC PC:

```bash
# Find the MBC PC of the HALT opcode (exit function)
# Or just set RV2MBC[0] to a known HALT location
sudo bpftool map update pinned $MAP_DIR/RV2MBC_MAP \
  key 0 0 0 0 value $(python3 -c "
import struct
# Point to MBC address of a HALT instruction
# Or better: use an address past ROM end that decodes as NOP→HALT
print(' '.join(str(b) for b in struct.pack('<I', 0xFFFFFFFF)))
")
```

This converts silent restarts into clean halts, making NULL pointer calls immediately visible.

---

### Previous Lessons (Still Apply)

**When reloading BPF programs, TWO things MUST happen before loading data:**

1. **Verify map fd ownership (Phase 4b):** After `bpftool prog show id $NEW_PROG`, cross-reference `map_ids` with the pinned maps in `/sys/fs/bpf/`. If they don't match, **re-pin** from the program's actual maps. Otherwise the loader writes data into orphaned maps the program can't see.

2. **Zero ROM_MAP before loading (Phase 5d):** BPF array entries are NOT zeroed on creation — they contain whatever kernel memory had. Sparse ROM entries (indexes beyond the .mbc file) will contain random bytes, including `0x40` (SYSCALL). The fix: write `0x00000000` to every ROM_MAP entry BEFORE loading the .mbc. Zeroed entries decode as `NOP` (0x00) which is harmless.

**This prevents the "45,713 phantom SYSCALLs in 320K instructions" bug.**

---

## What Changed (This File Replaces All Previous Session Fixes)

The file `doom/doomgeneric/doomgeneric/libc_monad.c` has been **completely rewritten** with all fixes consolidated. The agent should use this file as-is. **Do not attempt to patch the old file — it has been replaced.**

### Summary of All Fixes in New `libc_monad.c`

| Fix ID | What | Why |
|--------|------|-----|
| **S31-F1** | WAD file I/O: `fread()`, `fseek()`, `ftell()`, `open()`, `read()`, `lseek()` all WAD-aware | Doom needs to actually read WAD data from memory-mapped region |
| **S31-F2** | `_tail_match_doom1_wad()` — inline byte comparison | Eliminates `.rodata` string dependency that was corrupted in RAM_MAP |
| **S31-F3** | Debug buffer expanded to ~128KB (`0x0F0000`–`0x10FFFC`) | Old 4KB buffer filled with config warnings, hiding real errors |
| **S31-F4** | `rewind()`, `feof()`, `fgetc()`, `fgets()`, `ungetc()` WAD-aware | Doom's WAD parser uses multiple FILE operations |
| **S31-F5** | `HEAP_BASE` moved to `0x00520000` | Was `0x00510000`, too close to WAD end at `0x510000` (off by one page) |
| **S31-F6** | `puts()` and `fputs()` route to debug buffer | Previously silently discarded output |

### Key Design Decision: Inline Byte Matching

The old approach used `strcasecmp(base, "doom1.wad")` which loaded the comparison string from `.rodata` at VMA `0x2d770`. After section address shifts during rebuilds, this string was corrupted in RAM_MAP due to the zero-skipping loader bug. The new `_tail_match_doom1_wad()` does character-by-character comparison using only immediate values in the instruction stream — **zero dependency on .rodata**.

---

## Step-by-Step Recovery Procedure

### Phase 1: Rebuild Doom Binary

```bash
cd /home/admin/tmp/unheaded/doom/doomgeneric/doomgeneric

# The libc_monad.c is already updated in the workspace.
# Copy it to the dev VM if needed, or verify it's synced.

# Rebuild the Doom ELF
make clean && make

# Verify the binary
riscv64-unknown-elf-readelf -S doom.elf | grep -E '\.(text|rodata|data|bss)'
# NOTE: Record the NEW section addresses! They WILL shift from the previous build.
```

### Phase 2: Retranslate to MBC

```bash
cd /home/admin/tmp/unheaded/crates/monad-mbc

# Rebuild translator (should already have the R-type clobber fix)
cargo build --release --bin rv32i-to-mbc

# Translate
./target/release/rv32i-to-mbc \
  /home/admin/tmp/unheaded/doom/doomgeneric/doomgeneric/doom.elf \
  -o /home/admin/tmp/unheaded/doom/doomgeneric/doomgeneric/doom.mbc \
  --stats
```

### Phase 3: Fix the BPF SYSCALL Handler

**CRITICAL**: The BPF program `monad-cpu-ebpf/src/main.rs` still has the wrong register mapping for syscalls. Apply this fix:

```rust
// In the SYSCALL handler (around line 618-646):
// CHANGE:
//   let syscall_nr = cpu.regs[0];        // WRONG: r0 = x0 = always zero
// TO:
    let syscall_nr = cpu.regs[1];          // RIGHT: r1 = x17 (a7) = syscall number

// CHANGE all argument/return registers from regs[0]/regs[1] to regs[8]/regs[9]:
//   regs[8] = a0 (x10), first argument and return value
//   regs[9] = a1 (x11), second argument

// SYS_DRAW_FRAME: use fixed framebuffer address, not a register
    let fb_ptr = 0x100000u32;              // SCREEN_BASE from linker script

// SYS_GET_KEY: return in a0/a1
    cpu.regs[8] = (*kv >> 1) & 0x7FFF_FFFF;  // scancode in a0
    cpu.regs[9] = *kv & 1;                    // pressed in a1

// SYS_GET_TICKS: return in a0
    cpu.regs[8] = (now / 1_000_000) as u32;   // ms in a0

// SYS_SLEEP: read from a0
    let ms = cpu.regs[8] as u64;               // a0 = sleep duration ms
```

Then rebuild the BPF program:

```bash
cd /home/admin/tmp/unheaded/ebpf

# Force rebuild (touch to invalidate cache — hardlink issue)
touch monad-cpu-ebpf/src/main.rs

# Build
cargo build --release --target bpfel-unknown-none -p monad-cpu-ebpf
```

### Phase 4: Reload the Ring

**This is the full teardown-and-reload sequence. Do ALL steps — do not skip any.**

```bash
# 1. Kill any existing ebpf-loader holding link FDs
sudo pkill -f ebpf-loader 2>/dev/null; sleep 0.5

# 2. Detach XDP from ALL interfaces in ALL namespaces
for ns_iface in "monad0 veth01" "monad0 veth50p" \
                "monad1 veth01p" "monad1 veth12" \
                "monad2 veth12p" "monad2 veth23" \
                "monad3 veth23p" "monad3 veth34" \
                "monad4 veth34p" "monad4 veth45" \
                "monad5 veth45p" "monad5 veth50"; do
    ns=$(echo $ns_iface | cut -d' ' -f1)
    iface=$(echo $ns_iface | cut -d' ' -f2)
    sudo nsenter --net=/var/run/netns/$ns ip link set $iface xdpgeneric off 2>/dev/null
done

# 3. Verify all clean
for i in 0 1 2 3 4 5; do
    echo "=== monad$i ==="
    sudo nsenter --net=/var/run/netns/monad$i bpftool net show 2>&1
done

# 4. Remove old BPF pins
sudo rm -rf /sys/fs/bpf/unheaded/doom-ring/maps/*

# 5. Load new BPF program on hop0 (veth50p in monad0)
sudo nsenter --net=/var/run/netns/monad0 \
  /home/admin/tmp/unheaded/cmd/ebpf-loader/target/release/ebpf-loader \
  --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps \
  --only monad-cpu \
  --xdp-skb-mode \
  -i veth50p

# 6. Get the new program ID
NEW_PROG=$(sudo bpftool prog show 2>&1 | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
echo "New prog_id: $NEW_PROG"

# 7. Attach same prog to hops 1-5
for ns_iface in "monad1 veth01p" "monad2 veth12p" \
                "monad3 veth23p" "monad4 veth34p" \
                "monad5 veth45p"; do
    ns=$(echo $ns_iface | cut -d' ' -f1)
    iface=$(echo $ns_iface | cut -d' ' -f2)
    sudo nsenter --net=/var/run/netns/$ns \
      bpftool net attach xdpgeneric id $NEW_PROG dev $iface
done

# 8. Verify all 6 hops show same prog_id
for i in 0 1 2 3 4 5; do
    sudo nsenter --net=/var/run/netns/monad$i bpftool net show 2>&1 | grep xdp
done
```

### Phase 4b: Verify Map FD Ownership (CRITICAL — DO NOT SKIP)

**WHY THIS EXISTS:** When the BPF program is reloaded (e.g., prog 175 → prog 191), `aya-ebpf` creates **NEW** BPF maps with new map IDs. If the loader wrote ROM/RAM data into the OLD map fds (belonging to prog 175), the NEW program (191) reads from its own EMPTY maps — full of uninitialized kernel memory that may contain bytes like `0x40` (SYSCALL opcode). This causes phantom SYSCALL counts during what should be pure BSS clearing.

```bash
MAP_DIR=/sys/fs/bpf/unheaded/doom-ring/maps

# 1. Get the NEW program's map IDs
echo "=== Program map IDs ==="
NEW_PROG=$(sudo bpftool prog show 2>&1 | grep monad_cpu | tail -1 | awk '{print $1}' | tr -d ':')
sudo bpftool prog show id $NEW_PROG | grep map_ids
# Example output: map_ids 284,285,286,287,288,289,290,291

# 2. Get the PINNED map IDs
echo "=== Pinned map IDs ==="
for map in ROM_MAP RAM_MAP CPU_MAP STATS RV2MBC_MAP KBD_MAP SCREEN_MAP EVENT_MAP; do
    MAP_ID=$(sudo bpftool map show pinned $MAP_DIR/$map 2>/dev/null | head -1 | awk '{print $1}' | tr -d ':')
    echo "  $map -> map_id $MAP_ID"
done

# 3. COMPARE: Every pinned map ID must appear in the program's map_ids list
#    If ANY pinned map has an ID NOT in the program's list → STALE MAP!
#    Fix: re-pin from the program's actual maps

# 4. If mismatch detected — re-pin all maps:
echo "=== Re-pinning maps from prog $NEW_PROG ==="
PROG_MAPS=$(sudo bpftool prog show id $NEW_PROG | grep -oP 'map_ids \K[0-9,]+')
echo "Program owns maps: $PROG_MAPS"

# Get map names by inspecting each map ID
for MAP_ID in $(echo $PROG_MAPS | tr ',' '\n'); do
    MAP_NAME=$(sudo bpftool map show id $MAP_ID | grep -oP 'name \K\S+')
    echo "  map_id $MAP_ID = $MAP_NAME"
    # Re-pin: remove old pin, create new one
    sudo rm -f $MAP_DIR/$MAP_NAME 2>/dev/null
    sudo bpftool map pin id $MAP_ID $MAP_DIR/$MAP_NAME
done

echo "=== Verify re-pinned maps ==="
ls -la $MAP_DIR/
```

**CHECKPOINT:** After this step, every pinned map MUST belong to the currently loaded program. If `bpftool map show pinned $MAP_DIR/ROM_MAP` returns a map_id that is NOT in the program's `map_ids` list, **STOP and re-pin before proceeding.**

### Phase 5: Load All Data (CRITICAL — Use Correct Addresses)

```bash
MAP_DIR=/sys/fs/bpf/unheaded/doom-ring/maps
DOOM_DIR=/home/admin/tmp/unheaded/doom/doomgeneric/doomgeneric
LOADER=/home/admin/tmp/unheaded/scripts/doom-loader-core.py

# 5a. Get CURRENT section addresses from the NEW ELF
RODATA_ADDR=$(riscv64-unknown-elf-readelf -S $DOOM_DIR/doom.elf | grep '\.rodata' | awk '{print $4}')
DATA_ADDR=$(riscv64-unknown-elf-readelf -S $DOOM_DIR/doom.elf | grep ' \.data ' | awk '{print $4}')
echo "rodata VMA: 0x$RODATA_ADDR  data VMA: 0x$DATA_ADDR"

# 5b. Extract raw sections
cd $DOOM_DIR
riscv64-unknown-elf-objcopy -O binary -j .rodata doom.elf doom.rodata.bin
riscv64-unknown-elf-objcopy -O binary -j .data doom.elf doom.data.bin

# 5c. CLEAR the entire .rodata+.data+.bss region in RAM_MAP first
#     This prevents stale data from corrupting null terminators
RODATA_WORD=$((0x$RODATA_ADDR / 4))
BSS_END_WORD=$((0x8F000 / 4))  # generous upper bound covering BSS
echo "Clearing RAM words $RODATA_WORD to $BSS_END_WORD..."
python3 -c "
import struct, subprocess
MAP='$MAP_DIR/RAM_MAP'
zero = struct.pack('<I', 0)
for w in range($RODATA_WORD, $BSS_END_WORD):
    key = struct.pack('<I', w)
    subprocess.run(['sudo', 'bpftool', 'map', 'update', 'pinned', MAP,
                    'key'] + [str(b) for b in key] +
                   ['value'] + [str(b) for b in zero],
                   capture_output=True)
    if w % 10000 == 0: print(f'  cleared {w - $RODATA_WORD} words...')
print('Done clearing')
"

# 5d. ██ ZERO ROM_MAP BEFORE LOADING ██
#     WHY: Uninitialized BPF array entries contain kernel garbage — random bytes
#     that decode as valid opcodes (including 0x40 = SYSCALL). The ROM is sparse
#     (not every index is written), so unzeroed entries WILL execute as garbage.
#     This is THE root cause of "45,713 phantom SYSCALLs in 320K instructions."
ROM_SIZE=$(sudo bpftool map show pinned $MAP_DIR/ROM_MAP | grep -oP 'max_entries \K\d+')
echo "Zeroing ROM_MAP ($ROM_SIZE entries)..."
python3 -c "
import struct, subprocess, sys
MAP = '$MAP_DIR/ROM_MAP'
zero = struct.pack('<I', 0)
total = int('$ROM_SIZE')
batch = 0
for w in range(total):
    key = struct.pack('<I', w)
    subprocess.run(['sudo', 'bpftool', 'map', 'update', 'pinned', MAP,
                    'key'] + [str(b) for b in key] +
                   ['value'] + [str(b) for b in zero],
                   capture_output=True)
    batch += 1
    if batch % 50000 == 0:
        pct = (batch * 100) // total
        print(f'  zeroed {batch}/{total} ({pct}%)', flush=True)
print(f'Done: zeroed {total} ROM entries')
"

# 5e. NOW load ROM (writes over the zeroed entries with actual MBC instructions)
sudo python3 $LOADER rom $MAP_DIR/ROM_MAP $DOOM_DIR/doom.mbc

# 5f. Verify ROM integrity — spot-check first 8 instructions
echo "=== ROM[0:7] spot check ==="
for i in 0 1 2 3 4 5 6 7; do
    KEY=$(python3 -c "import struct; print(' '.join(str(b) for b in struct.pack('<I', $i)))")
    VAL=$(sudo bpftool map lookup pinned $MAP_DIR/ROM_MAP key $KEY 2>&1 | grep value)
    # Extract opcode byte (bits 31-24 = last byte in little-endian output)
    echo "  ROM[$i]: $VAL"
done
# Expected opcodes for SP init + BSS clear:
#   [0] = 0x0F (MOVI)   — MOVI r15, 0x03F0
#   [1] = 0x0B (SHL)    — SHL r15, 0, 16    (NOT SLT! 0x0B = SHL in MBC)
#   [2] = 0x0F (MOVI)   — MOVI for BSS start
#   [3] = 0x0B (SHL)    — SHL for BSS start
#   ...pattern continues with MOVI/SHL pairs for address loads

# 5g. Load RV2MBC
sudo python3 $LOADER rv2mbc $MAP_DIR/RV2MBC_MAP $DOOM_DIR/doom.rv2mbc

# 5h. Load .rodata at CORRECT address
sudo python3 $LOADER ram $MAP_DIR/RAM_MAP $DOOM_DIR/doom.rodata.bin 0x$RODATA_ADDR

# 5i. Load .data at CORRECT address
sudo python3 $LOADER ram $MAP_DIR/RAM_MAP $DOOM_DIR/doom.data.bin 0x$DATA_ADDR

# 5j. Load WAD at 0x110000
sudo python3 $LOADER ram $MAP_DIR/RAM_MAP \
  /home/admin/tmp/unheaded/doom/doom1.wad 0x110000

# 5k. Reset CPU at instance 0xDE
sudo python3 /tmp/reset_cpu.py
# Verify: SP should be 0x3F00000, PC=0, halted=0
sudo python3 /tmp/read_cpu.py
```

### Phase 6: Post-Load Verification (DO NOT SKIP)

```bash
MAP_DIR=/sys/fs/bpf/unheaded/doom-ring/maps

# Verify heap_ptr is correct (should be 0x00520000 at the .data symbol address)
HEAP_PTR_SYM=$(riscv64-unknown-elf-nm $DOOM_DIR/doom.elf | grep heap_ptr | awk '{print $1}')
HEAP_PTR_WORD=$((0x$HEAP_PTR_SYM / 4))
echo "heap_ptr at word 0x$(printf '%X' $HEAP_PTR_WORD):"
sudo bpftool map lookup pinned $MAP_DIR/RAM_MAP key \
  $(python3 -c "import struct; print(' '.join(str(b) for b in struct.pack('<I', $HEAP_PTR_WORD)))")
# Expected: value 00 00 52 00 (0x00520000 little-endian)

# Verify WAD magic at 0x110000 (word 0x44000)
echo "WAD magic:"
sudo bpftool map lookup pinned $MAP_DIR/RAM_MAP key 0 64 4 0
# Expected: value 49 57 41 44 ("IWAD")

# Verify debug buffer length is zero
echo "Debug buffer length:"
sudo bpftool map lookup pinned $MAP_DIR/RAM_MAP key 255 63 4 0
# Expected: value 00 00 00 00
```

### Phase 7: Inject Packets and Monitor

```bash
# Get MACs
SRC_MAC=$(sudo ip netns exec monad0 ip link show veth01 | awk '/ether/{print $2}')
DST_MAC=$(sudo ip netns exec monad1 ip link show veth01p | awk '/ether/{print $2}')
echo "SRC=$SRC_MAC DST=$DST_MAC"

# Small test: 100 packets = ~408K instructions
sudo ip netns exec monad0 python3 \
  /home/admin/tmp/unheaded/scripts/bulk_inject.py \
  0xDE 100 $SRC_MAC $DST_MAC

# Check CPU state
sudo python3 /tmp/read_cpu.py

# Read debug buffer
sudo python3 /tmp/dump_debug_buf.py

# If Doom is making progress (PC advancing, not halted), inject more:
# 1000 packets = ~4M insns
# 10000 packets = ~40M insns
# 100000 packets = ~408M insns (BSS clearing takes ~60M+)
```

---

## Expected Doom Init Sequence

With all fixes applied, the debug buffer should show:

```
Z_Init: Init zone memory allocation daemon.
zone memory: 0x00XXXXXX, NNNNNN allocated for zone
-iwad not specified, trying a few iwad names
Trying IWAD file:doom2.wad          ← fopen returns NULL (not doom1.wad)
Trying IWAD file:doom.wad           ← fopen returns NULL (not doom1.wad)
Trying IWAD file:doom1.wad          ← fopen returns &_wad_f ✓
W_Init: Init WADfiles.
 adding doom1.wad
...
V_Init: allocate screens
M_LoadDefaults
...
R_Init                               ← Renderer init (texture loading)
P_Init                               ← Playloop init
S_Init                               ← Sound init (will fail gracefully)
D_DoomMain: entering D_DoomLoop      ← THE GOAL
```

### Known Future Halt Points

| Init Stage | Likely Issue | Fix |
|------------|------------|-----|
| **S_Init** (sound) | Tries to open audio device | `I_InitSound` should already be stubbed via doomgeneric; verify |
| **R_Init** (renderer) | ~~Large texture allocations~~ CALLR missing link save | **FIX CALLR HANDLER** — add `cpu.regs[14] = cpu.pc + 1;` before RV2MBC lookup |
| **D_DoomLoop** | Calls `SYS_DRAW_FRAME` | BPF SYSCALL handler must use `regs[8]` for a0 (already fixed above) |
| **D_DoomLoop** | Calls `SYS_GET_TICKS` | BPF must return monotonic ms in `regs[8]` |
| **D_DoomLoop** | Calls `SYS_SLEEP` | BPF sets `sleep_until_ns`, breaks execute loop |

---

## Memory Map (Current — doom1.wad)

```
Address Range          Size    Contents
─────────────────────  ──────  ──────────────────────────
0x000000 – 0x02XXXX   ~184KB  .text (code → MBC ROM, not in RAM_MAP)
0x02XXXX – 0x04XXXX   ~100KB  .rodata (read-only data)
0x04XXXX – 0x05XXXX   ~60KB   .data (initialized globals)
0x05XXXX – 0x08XXXX   ~240KB  .bss (zero-init, cleared at startup)
0x0F0000 – 0x10FFFC   ~128KB  Debug output buffer
0x100000 – 0x10FA00   ~64KB   SCREEN (320×200 framebuffer)
0x110000 – 0x510000   ~4MB    doom1.wad (WAD data)
0x520000 – 0x1520000  16MB    Heap (bump allocator)
0x3F00000             ──      Stack top (grows downward)
```

## Memory Map (Future — DOOM.WAD Full Retail)

If switching to the full DOOM.WAD (12.4MB), update these constants:

```c
// In libc_monad.c:
#define WAD_SIZE  12408292       // was 4196020
#define HEAP_BASE ((char *)0x00D10000)  // was 0x00520000 — must be past WAD end
// WAD end = 0x110000 + 12408292 = ~0xCF4000, so 0xD10000 gives 128KB gap

// In linker script (monad.ld):
// Update stack top if needed (0x3F00000 is fine, plenty of room)
```

Also update the WAD loader command:

```bash
# Load DOOM.WAD instead of doom1.wad
sudo python3 $LOADER ram $MAP_DIR/RAM_MAP \
  /home/admin/tmp/unheaded/doom/DOOM.WAD 0x110000
```

And change `_tail_match_doom1_wad` to `_tail_match_doom_wad` matching "doom.wad" (8 chars) instead of "doom1.wad" (9 chars).

**Recommendation:** Get doom1.wad working first, then upgrade to DOOM.WAD.

---

## Files Modified (Complete List)

| File | Status | Notes |
|------|--------|-------|
| `doom/doomgeneric/doomgeneric/libc_monad.c` | **REWRITTEN** | All fixes consolidated, use as-is |
| `ebpf/monad-cpu-ebpf/src/main.rs` | **NEEDS FIX** | SYSCALL register mapping (Phase 3 above) |
| `doom/doomgeneric/doomgeneric/m_config.c` | **Previously fixed** | `GetDefaultForName` returns NULL under `__MONAD__` |
| `crates/monad-mbc/src/translator.rs` | **Previously fixed** | R-type rd==rs2 clobber fix + `emit_r_type_op` |

---

## Troubleshooting

### "doom2.wad still being accepted"
This should be fixed by the inline byte matching. If it persists, add debug output:
```c
// In _tail_match_doom1_wad, before the return:
dbg_puts("[WAD] checking: "); dbg_puts(p);
dbg_puts(t[4]=='1' ? " -> MATCH\n" : " -> REJECT\n");
```

### "Z_Malloc failed" or "Unable to allocate"
Heap is too small or heap_ptr was corrupted during .data load. Verify heap_ptr symbol address matches the loaded value in RAM_MAP (Phase 6 verification).

### CPU halted immediately (insn_count=0)
Instance ID mismatch. Packets use flow_label=0xDE. CPU must be initialized at key 222 (0xDE). Verify with:
```bash
sudo bpftool map lookup pinned $MAP_DIR/CPU_MAP key 222 0 0 0
```

### Only 64 insns per packet (not 4080)
Packets aren't circulating. With `xdpgeneric` on veth pairs and `XDP_PASS`, the kernel forwards the packet to the next namespace. Verify all 6 hops have the program attached and use real MACs (not fake ones).

### Excessive SYSCALL count (thousands of SYSCALLs during BSS clearing)

**Symptom:** STAT_SYSCALLS (index 7) shows tens of thousands of SYSCALLs in the first few hundred thousand instructions. BSS clearing should produce ZERO SYSCALLs — it's just `ST`, `ADDI`, `CMP`, `JLT` in a tight loop.

**Root cause:** Stale/uninitialized ROM_MAP data. When the BPF program is reloaded (new prog_id), aya creates NEW maps. If:
- (a) The pinned map fds point to the OLD program's maps → loader writes ROM into old map, new program reads empty/garbage map
- (b) The ROM_MAP wasn't zeroed before loading → sparse entries contain kernel garbage bytes, some of which decode as opcode 0x40 (SYSCALL)

**Fix:**
1. Run **Phase 4b** (verify map fd ownership) — ensure pinned maps belong to the current program
2. Run **Phase 5d** (zero ROM_MAP) — ensure every entry is 0x00000000 before loading .mbc
3. Run **Phase 5f** (spot-check ROM[0:7]) — verify opcodes match expected MOVI/SHL/etc. pattern

**How to verify the fix worked:**
```bash
# After reloading and injecting 100 packets:
sudo bpftool map lookup pinned $MAP_DIR/STATS key 7 0 0 0
# SYSCALL count should be 0 (or very small) during BSS clearing phase
# Non-zero means ROM still contains garbage — re-run Phase 4b + 5d
```

**Opcode reference for ROM verification:**
| Byte | Opcode | Notes |
|------|--------|-------|
| 0x00 | NOP | Safe (zeroed entry = NOP = no-op) |
| 0x0B | SHL | Shift left immediate — NOT SLT (SLT doesn't exist in MBC) |
| 0x0F | MOVI | Move immediate to register |
| 0x31 | ST | Store word — used in BSS clearing loop |
| 0x40 | SYSCALL | **The phantom — should NOT appear in BSS clearing** |

### Doom restarts D_DoomMain repeatedly (infinite restart loop)

**Symptom:** Debug buffer shows "R_Init: Init DOOM refresh daemon - Doom Generic 0.1" — the R_Init banner immediately followed by a full restart. Multiple init cycles consume heap until exhaustion → HALT.

**Root cause:** CALLR opcode handler doesn't save return address in r14. See the **CRITICAL BUG** section at the top of this document.

**How to verify:** Check CALLR handler in `monad-cpu-ebpf/src/main.rs`. It must have the same `cpu.regs[15].wrapping_sub(1)` + `mem_write_word(sp, cpu.pc)` stack push as the CALL handler. If the push is missing, that's the bug. Compare CALL and CALLR side by side — they should be identical except CALL uses `insn_word & 0x00FF_FFFF` for the target while CALLR uses `RV2MBC_MAP.get(rv_word)`.

**Evidence that confirms this is the cause:**
- ROM_FAULT = 0 (all indirect jumps resolve — including the bogus jump to addr 0)
- RV2MBC_MAP[0] = 0 (address 0 maps to MBC PC 0 = _start)
- SYSCALLS = 0 (Doom never reaches D_DoomLoop/DG_DrawFrame)
- Zone addresses increment by ~6MB per restart cycle (heap never freed, bump allocator)
- Restart always happens at R_InitData (first place function pointers are called in init)

**After fixing:** Revert HEAP_SIZE back to 16MB (the 32MB was a workaround, not a fix). Doom shareware needs one zone of 6MB + ~4MB overhead = ~10MB total. 16MB heap is plenty for a single init pass.

### BPF verifier rejects program
MAX_INSN_PER_TICK must be 64. Higher values exceed the 8192 jump complexity limit or 1M processed instruction limit.

### Agent misidentifies opcode 0x0B as "SLT"

**Clarification:** There is NO `SLT` opcode in MBC. The opcode 0x0B is `SHL` (shift left by immediate). The full bitwise opcode range is:
- 0x07 = AND
- 0x08 = OR
- 0x09 = XOR
- 0x0A = NOT
- 0x0B = **SHL** (shift left immediate)
- 0x0C = SHR (shift right logical)
- 0x0D = SAR (shift right arithmetic)

The RISC-V `SLT` (set less than) instruction is translated to MBC's `CMP` (0x10) + conditional branch pattern — it does NOT have a dedicated opcode. If the agent labels 0x0B as "SLT", the agent is using a wrong opcode table.
