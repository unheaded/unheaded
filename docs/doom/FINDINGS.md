# Doom Findings — Session 2026-03-29

## CRITICAL FINDING: PC Corruption

**24 billion instructions executed, zero frames rendered.**

PC sampling over 5 seconds shows values like:
- 0x2E6EB7AB, 0x365E9BDD, 0x931664E9

ROM_MAP only has 104,218 entries (max valid PC = ~0x19700).
These PCs are millions of times larger. **The program counter is corrupted.**

Doom jumped to an invalid address early in init. ROM_MAP returns 0 for
out-of-bounds indices (default Array value). MBC opcode 0 = NOP.
So Doom executes infinite NOPs, incrementing PC into the void.
Instructions climb. Nothing useful happens.

## Root Cause (Suspected)

A function pointer call (indirect jump) with a corrupted pointer.
The MBC `CALL` instruction jumps to an address in a register.
If the register contains garbage, PC goes to garbage.

Candidates:
1. `Z_Malloc`'s zone management uses function pointers (PU_* callbacks)
2. `W_Read` dispatches through `wad_file->file_class->Read` (we fixed this with static handles, but maybe not all paths)
3. A vtable or callback array in Doom's init code
4. Stack corruption overwriting a return address

## What This Means

Every "fix" that increased instruction count was just getting Doom
further before it hit the PC corruption. The billions of instructions
after corruption are wasted NOPs. Doom never reached R_Init.

## Next Steps

1. **Find WHEN PC goes bad** — add a PC range check in the eBPF executor.
   If PC > ROM_MAP size, halt immediately and record the last valid PC.
2. **Trace the jump** — the last valid PC before corruption tells us
   which function made the bad jump.
3. **Fix the indirect call** — either fix the pointer or add a guard.

## Lesson Learned

"More instructions" ≠ "more progress". Always verify PC is in valid
range. 24B instructions of NOPs looks like progress from the stats
but is actually a crash that nobody detected.
