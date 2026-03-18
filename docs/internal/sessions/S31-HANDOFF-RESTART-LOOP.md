# S31 Handoff: Doom Restart Loop During R_Init

**Date:** 2026-02-22
**Blocker:** Doom stuck in infinite D_DoomMain restart loop — never reaches D_DoomLoop
**Priority:** This is THE blocker preventing first frame render

---

## The Bug

Doom's initialization restarts from D_DoomMain repeatedly. Each cycle runs: Z_Init → V_Init → M_LoadDefaults → IWAD search → W_Init → I_Init → M_Init → starts R_Init → **restarts**. Each cycle consumes ~6MB heap (bump allocator, no free). Eventually heap exhaustion → I_Error → exit() → HALT.

**Debug buffer evidence (5+ init cycles visible):**
```
R_Init: Init DOOM refresh daemon - Doom Generic 0.1    ← restart happens HERE
Z_Init: Init zone memory allocation daemon.
zone memory: 0x00ba5b28, 600000 allocated for zone     ← 600000 is hex = 6MB
...repeats...
Unable to allocate 5 MiB of RAM for zone               ← final halt
```

The restart occurs at the boundary between "R_Init: Init DOOM refresh daemon - " and the next init cycle. R_Init's first action is calling R_InitData() → R_InitTextures(). The crash is somewhere in this call chain.

---

## Verified Facts (from codebase inspection)

### Stats at crash point
```
PACKETS_TOTAL:    102,000     (all received)
INSNS_EXECUTED:   12,800,000  (12.8M, past where 16MB heap halted at 11.6M)
HALTED:           0           (CPU still running with 32MB heap, eventually halts)
SYSCALLS:         0           (never reached D_DoomLoop — no ecalls fired)
ROM_FAULT:        0           (all indirect jumps resolved successfully)
MEM_FAULTS:       0           (all memory accesses in range)
CACHE_HITS:       907,866
CACHE_MISSES:     2,571,967
```

### Call/Return mechanism (VERIFIED CORRECT)
Both CALL and CALLR push return addresses to a word-addressed call stack:
```rust
// CALL (line 455-463): direct call
cpu.regs[15] = cpu.regs[15].wrapping_sub(1);  // SP--
let sp = cpu.regs[15];
mem_write_word(sp, cpu.pc);                     // push MBC return addr
cpu.pc = target;

// CALLR (line 479-495): indirect call — HAS THE PUSH
cpu.regs[15] = cpu.regs[15].wrapping_sub(1);  // SP--
let sp = cpu.regs[15];
mem_write_word(sp, cpu.pc);                     // push MBC return addr
let rv_word = cpu.regs[d] >> 2;
cpu.pc = RV2MBC_MAP[rv_word];

// RET (line 496-500): pop and return
let ret = mem_read_word(cpu.regs[15]);
cpu.regs[15] = cpu.regs[15].wrapping_add(1);  // SP++
cpu.pc = ret;
```
**CALLR already has the stack push.** An earlier theory that CALLR was missing the push was WRONG — the workspace code may differ from dev VM. Verify the dev VM version.

### Memory layout
```
RAM_MAP:    16,777,216 entries (64MB word-addressable)
ROM_MAP:    262,144 entries (1MB of MBC instructions)
Stack:      sp = 0x01000000 (crt0_monad.S line 5) — word addr 0x00FFFFFF in RAM_MAP
Heap:       0x00520000, 16MB (workspace) / 32MB (dev VM after S31 change)
WAD:        0x00110000, ~4MB (doom1.wad)
Debug buf:  0x0F0000 – 0x10FFFC (~128KB)
Screen:     0x00100000 (320×200)
.text:      ~285KB (76,112 MBC instructions)
```

### Dual stack coexistence
CALL/RET treat r15 (SP) as a **word address** directly into RAM_MAP. RISC-V SW/LW treat r15 as a **byte address**, converting to word addr via `>> 2`. These address completely different RAM_MAP regions and don't conflict:
- CALL writes to RAM_MAP[~0x00FFFFFF] (top of array)
- RISC-V SW writes to RAM_MAP[~0x003FFFFF] (middle of array)

### Compilation flags (Makefile.monad)
```makefile
CFLAGS  = -march=rv32im -mabi=ilp32
CFLAGS += -nostdlib -nostdinc -ffreestanding -fno-builtin -Os
CFLAGS += -ffixed-x16 -ffixed-x17 ... -ffixed-x31   # ONLY x0-x15 used!
CFLAGS += -DCMAP256 -DNORMALUNIX -D__MONAD__
LDFLAGS = -nostdlib -static -Wl,--gc-sections -Wl,-T,monad.ld
# NOTE: -mno-relax is NOT set — linker relaxation IS enabled
```

**Critical:** `-ffixed-x16 through -ffixed-x31` means GCC only uses x0-x15 (16 registers), matching MBC's 16-register ISA. x17 (a7) for ecall syscall numbers uses explicit `asm("a7")` binding to override the fixed status.

**Linker relaxation is ON** (no `-mno-relax`). With ~285KB .text, all functions are within JAL's ±1MB range. The linker SHOULD relax all `auipc+jalr` pairs to single `jal` instructions. This means the buggy JALR non-zero-offset code path in the translator (line 897-916) should NOT be exercised.

### Translator JALR non-zero-offset (line 897-916) — SELF-ACKNOWLEDGED BUG
```rust
} else {
    // JALR with non-zero offset — compute target first
    self.emit(op::MOV, 0, mbc_rs1, 0);
    if imm12 > 0 && imm12 <= 0xFFFF {
        self.emit(op::MOVI, mbc_rs1, 0, imm12 as u16);
        self.emit(op::ADD, 0, mbc_rs1, 0);
        self.emit(op::MOV, mbc_rs1, 0, 0); // "actually this is wrong"
    }
    // CALLR/JMPR through r0...
}
```
**Two bugs here:**
1. Only handles positive imm12 (`imm12 > 0`). Negative offsets (common for `%lo` relocations) are SKIPPED entirely — the offset is never added
2. Comment on line 907 admits the register restoration is wrong

**IF linker relaxation converts all auipc+jalr to jal, this code is dead.** But verify with: `riscv64-unknown-elf-objdump -d doom.elf | grep jalr | grep -v 'jr\|ret'`

### SYSCALL register mapping (KNOWN WRONG — separate issue)
```rust
// Current (line 620-621):
let syscall_nr = cpu.regs[0];   // r0 = x0 = ALWAYS ZERO
let fb_ptr = cpu.regs[1];       // r1 = x3 (gp), NOT x17 (a7)

// Should be:
let syscall_nr = cpu.regs[1];   // r1 maps to... actually check register mapping
// The ecall ABI with -ffixed-x17 needs special handling
```
**This only matters AFTER Doom reaches D_DoomLoop.** Not the current blocker.

---

## Disproven Theories

| Theory | Why it's wrong |
|--------|---------------|
| CALLR missing stack push | Agent confirmed CALLR has the push on dev VM (wrapping_sub + mem_write_word) |
| JALR non-zero-offset translator bug | `objdump -d doom.elf | grep jalr` shows ALL jalr have zero offsets. Linker relaxation converted every `auipc+jalr` → `jal`. The buggy handler (line 897-916) is DEAD CODE for this binary. |
| NULL function pointer → addr 0 | Would cause ROM_FAULT (RV2MBC_MAP[0] lookup) — but ROM_FAULT = 0... actually RV2MBC_MAP[0] = 0 IS valid, so no fault. Theory NOT fully disproven |
| Stack/heap collision | RAM_MAP is 64MB. Dual stacks are 12M entries apart. No collision |
| Heap too small | 32MB heap delays but doesn't fix the loop (5 cycles instead of 2) |

---

## Active Theories (investigate these)

### Theory 1 (PRIME SUSPECT): Switch tables via JMPR landing at wrong MBC address

**Evidence:** The binary contains `jr a5` and `jr a3` instructions — these are zero-offset indirect jumps used for GCC switch tables. The translator converts these to JMPR which does RV2MBC_MAP lookup:
```rust
// JMPR (indirect jump, no link save):
let rv_addr = cpu.regs[d];
let rv_word = rv_addr >> 2;
cpu.pc = match RV2MBC_MAP.get(rv_word) { ... };
```

**The problem:** Switch tables compute jump targets as `base + offset*4` where `base` is typically loaded via `auipc` or `lui+addi` at compile time. The RV address stored in the register is a RISC-V text address. The JMPR handler does:
```rust
cpu.pc = match RV2MBC_MAP.get(rv_word) {
    Some(mbc_idx) => *mbc_idx,   // ← if entry exists but == 0, PC = 0 silently!
    None => { cpu.halted = 1; }  // ← only fires if rv_word > 65535 (out of BPF map bounds)
};
```

**THE SMOKING GUN:** `RV2MBC_MAP` is a BPF `Array<u32>` with 65,536 entries, initialized to **0** by the kernel. The translator only writes entries for RV instruction addresses it processes. ANY RV word address in 0..65535 that the translator didn't populate returns `Some(0)`, NOT `None`. So: unmapped address → `*mbc_idx = 0` → `cpu.pc = 0` → `_start` → D_DoomMain restart. NO ROM_FAULT. NO HALT. Silent.

**Why this fits perfectly:**
- Restart happens during R_InitTextures — likely has switch statements for texture type processing
- `jr a5` / `jr a3` confirmed in binary — switch tables ARE present
- ROM_FAULT = 0 — because `RV2MBC_MAP.get()` returns `Some(0)`, never `None`
- Each restart re-runs full init, consuming 6MB heap → eventual OOM → HALT
- The behavior is 100% consistent with a switch table target hitting an unmapped RV2MBC_MAP entry

**How to verify:**
```bash
# 1. Find switch tables near R_InitTextures
riscv64-unknown-elf-objdump -d doom.elf | grep -B5 'jr\s*a[0-9]' | head -40

# 2. Check if any switch target RV addresses map to MBC 0 in RV2MBC_MAP
# Add debug print in JMPR handler:
#   if cpu.pc == 0 { dbg_printf("JMPR→0 from rv_addr=0x%x\n", rv_addr); }

# 3. Verify RV2MBC_MAP coverage — are ALL .text addresses mapped?
# The translator builds RV2MBC_MAP during translation. If switch table
# targets point to addresses between translated instructions (e.g.,
# middle of a multi-instruction expansion), the map entry could be 0.
```

**How to fix:** Either:
- Add a debug trap in the BPF JMPR handler: if `RV2MBC_MAP[rv_word] == 0 && rv_word != 0`, set HALTED or log the bad address
- Ensure the translator populates RV2MBC_MAP for ALL valid .text byte addresses, not just instruction boundaries
- Check if `auipc` used for switch table base addresses is being translated correctly

### Theory 2: RV2MBC_MAP has gaps at switch table target addresses

Related to Theory 1 but more specific. The translator builds RV2MBC_MAP as it translates each RISC-V instruction. Each RV instruction at byte address `X` gets mapped to MBC PC `Y`. But if the translator emits MULTIPLE MBC instructions for one RV instruction (e.g., `lui` expands to `MOVI+ORHI`), only the FIRST byte address gets the mapping. Switch table entries might point to the second RV instruction in a pair that got expanded differently, hitting an unmapped address → RV2MBC_MAP returns 0 → PC = 0 → restart.

**How to verify:**
```bash
# Dump RV2MBC_MAP around known switch table target addresses
# Compare against objdump to see if targets align with instruction boundaries
```

### Theory 3: realloc() stub over-reads
W_AddFile calls realloc to resize lumpinfo. The stub does `memcpy(newp, ptr, size)` where `size` is the NEW size, not the old. This reads past the old allocation's bounds. In Doom's case, the new entries are overwritten by WAD directory data, so this is probably harmless. But if any code path relies on uninitialized-but-zeroed realloc'd memory, the garbage bytes could cause issues.

```c
// libc_monad.c line 54-60:
void *realloc(void *ptr, size_t size) {
    void *newp;
    if (!ptr) return malloc(size);
    if (size == 0) { free(ptr); return NULL; }
    newp = malloc(size);
    if (newp) memcpy(newp, ptr, size);  // BUG: copies 'size' not 'old_size'
    return newp;
}
```
**Likely harmless but worth noting.** The bump allocator doesn't track allocation sizes, so there's no way to know old_size. A proper fix would require size tracking.

### Theory 4: Function pointer in .data section with wrong address
Doom stores function pointers in global tables (e.g., action tables). If .data section addresses shifted during a rebuild but the pointer values weren't updated, function pointer calls go to wrong addresses.

**How to verify:** Check if the .data section base address in the ELF matches what the loader uses. Also verify .rodata VMA matches.

### Theory 5: PC wraparound or instruction fetch from uninitialized ROM
If PC somehow reaches past 76,112 (the loaded MBC instruction count), ROM_MAP entries beyond that are zeroed (NOP). A long NOP slide would increment PC until it wraps to 0. But ROM_MAP has 262,144 entries and NOP doesn't branch, so PC would just increment until ROM_FAULT at entry 262,144.

Actually, ROM entries past 76,112 are NOP (0x00000000). Each NOP increments PC by 1. After 262,144 - 76,112 = 186,032 NOPs, PC hits 262,144 which is out of ROM_MAP bounds → ROM_FAULT → halt. But ROM_FAULT = 0, so this isn't happening.

---

## Reproduction Steps (for new agent)

### Quick reproduction (if ring is still up):
```bash
# 1. Reset CPU state
sudo python3 /tmp/reset_cpu.py

# 2. Zero and reload ROM
sudo python3 /tmp/zero_rom.py
sudo python3 /home/admin/tmp/unheaded/scripts/doom-loader-core.py rom \
  /sys/fs/bpf/unheaded/doom-ring/maps/ROM_MAP /tmp/doom.mbc

# 3. Inject packets
sudo ip netns exec monad0 python3 \
  /home/admin/tmp/unheaded/scripts/bulk_inject.py \
  0xDE 100000 $SRC_MAC $DST_MAC

# 4. Check debug buffer
sudo python3 /tmp/read_dbg.py
# Look for repeated "Doom Generic 0.1" lines = restart loop
```

### Full reproduction (from scratch):
Follow S31-AGENT-INSTRUCTIONS.md Phases 1-7.

---

## Key Files

| File | Location | What |
|------|----------|------|
| BPF program | `ebpf/monad-cpu-ebpf/src/main.rs` | MBC CPU execution engine |
| Translator | `crates/monad-mbc/src/translator.rs` | RV32I → MBC, line 870-916 = JALR handler |
| MBC opcodes | `ebpf/monad-common/src/lib.rs` | Opcode definitions |
| CRT0 | `doom/doomgeneric/doomgeneric/crt0_monad.S` | Boot code, sp init |
| libc stubs | `doom/doomgeneric/doomgeneric/libc_monad.c` | Heap, malloc, WAD I/O |
| Platform | `doom/doomgeneric/doomgeneric/doomgeneric_monad.c` | DG_DrawFrame ecall |
| Makefile | `doom/doomgeneric/doomgeneric/Makefile.monad` | Build flags |
| Linker script | `doom/doomgeneric/doomgeneric/monad.ld` | Memory layout |
| R_Init source | `doom/doomgeneric/doomgeneric/r_data.c` | Where crash happens |

---

## What to Do Next

1. **HIGHEST PRIORITY: Add JMPR→0 trap in BPF CPU**
   In `ebpf/monad-cpu-ebpf/src/main.rs`, line ~470, modify the JMPR handler:
   ```rust
   // CURRENT (line 470-471):
   cpu.pc = match RV2MBC_MAP.get(rv_word) {
       Some(mbc_idx) => *mbc_idx,
       None => { cpu.halted = 1; increment_stat(STAT_ROM_FAULT); break; }
   };

   // CHANGE TO:
   cpu.pc = match RV2MBC_MAP.get(rv_word) {
       Some(mbc_idx) if *mbc_idx != 0 || rv_word == 0 => *mbc_idx,
       Some(_) => {
           // RV2MBC_MAP entry is 0 for non-zero rv_word = UNMAPPED ADDRESS
           // This is our restart loop bug! Log and halt.
           // Write marker + bad RV byte addr to debug buffer
           let dbg_ptr = cpu.regs[13]; // or wherever your debug write pointer is
           mem_write_word(dbg_ptr >> 2, 0xDEAD0001);
           mem_write_word((dbg_ptr >> 2) + 1, rv_addr);
           cpu.halted = 1;
           increment_stat(STAT_ROM_FAULT);
           break;
       }
       None => { cpu.halted = 1; increment_stat(STAT_ROM_FAULT); break; }
   };
   ```
   This will HALT with a `0xDEAD0001` marker + the bad RV address in the debug buffer.

2. **ALSO: Add CALLR→0 trap (same pattern, line ~488)**
   ```rust
   // Same guard on CALLR's RV2MBC_MAP lookup:
   Some(mbc_idx) if *mbc_idx != 0 || rv_word == 0 => *mbc_idx,
   Some(_) => {
       mem_write_word(dbg_ptr >> 2, 0xDEAD0002);  // marker: CALLR
       mem_write_word((dbg_ptr >> 2) + 1, rv_addr);
       cpu.halted = 1;
       increment_stat(STAT_ROM_FAULT);
       break;
   }
   ```

3. **Add targeted debug prints in R_InitData:**
   ```c
   // In r_data.c R_InitData():
   dbg_puts("[R_InitData] enter\n");
   R_InitTextures();
   dbg_puts("[R_InitData] after R_InitTextures\n");
   R_InitFlats();
   dbg_puts("[R_InitData] after R_InitFlats\n");
   ```
   This narrows down WHICH sub-function triggers the restart.

4. **Inside R_InitTextures, add more prints:**
   ```c
   dbg_puts("[R_InitTextures] enter\n");
   // ... before W_GetNumForName("PNAMES"):
   dbg_puts("[R_InitTextures] about to get PNAMES\n");
   int pnames_lump = W_GetNumForName("PNAMES");
   dbg_printf("[R_InitTextures] PNAMES lump=%d\n", pnames_lump);
   ```

5. **If JMPR→0 trap fires:** You now have the RV address. Check:
   ```bash
   # What RV instruction is at that address?
   riscv64-unknown-elf-objdump -d doom.elf | grep -A2 "$(printf '%x' $BAD_ADDR)"

   # Is it in RV2MBC_MAP?
   sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/RV2MBC_MAP \
     key $(printf '%02x %02x %02x %02x' ...)
   ```
   Fix: populate the missing RV2MBC_MAP entries in the translator, OR fix the switch table base address computation.

6. **If JMPR→0 trap does NOT fire:** The restart isn't from JMPR/CALLR. Then look for:
   - Direct `JMP 0` instruction in the MBC (translator emitting wrong target)
   - Stack corruption causing RET to pop 0 → PC = 0
   - Add a PC=0 trap at the top of the main execution loop (before instruction fetch)

7. **After fixing the restart loop:** The SYSCALL register mapping (line 620-621) is also wrong — fix it before testing D_DoomLoop. The ecall ABI with `-ffixed-x17` needs investigation.

---

## Don't Waste Time On

- Adding more heap (32MB, 64MB, etc.) — delays the loop, doesn't fix it
- Investigating CALLR stack push — it's already there, verified
- JALR non-zero-offset translator bug (line 897-916) — ALL jalr in the binary have zero offsets, this code is DEAD
- Looking at eBPF verifier issues — program loads and runs fine
- Optimizing MAX_INSN_PER_TICK — 128 works, not the current blocker
- Sound/music init — Doom shareware handles missing audio gracefully
