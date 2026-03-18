# Session S20 — Handoff Document

**Date:** 2026-02-20
**Status:** Paused — race condition diagnosed, BSS clearing in progress

---

## What Was Done This Session

### 1. Continued execution from S19 state
- CPU state from S19: PC=103940 stuck (tight memset loop), 82M insns
- S19 had already applied memset ROM patch (fix 3→2 operand bug at PC 103934-103936)
- S19 had already reset CPU and reloaded data+WAD
- Resumed at: PC=23, insn_count=102M, r3=0x74434, BSS clearing in progress

### 2. Verified MBC instruction encoding layout
Confirmed the correct bit layout by reading `monad-common/src/lib.rs`:

| Field  | Bits    | Accessor              |
|--------|---------|-----------------------|
| opcode | [31:24] | `(self.0 >> 24) & 0xFF` |
| dst    | [23:20] | `(self.0 >> 20) & 0x0F` |
| src    | [19:16] | `(self.0 >> 16) & 0x0F` |
| imm16  | [15:0]  | `self.0 & 0xFFFF`       |

`encode()` uses the same layout (opcode << 24, dst << 20, src << 16, imm). Encode and decode are consistent.

Branches use 24-bit signed offset: `insn_word & 0x00FFFFFF`, sign-extended from bit 23.
**PC is pre-incremented before execution** (line 283 of main.rs), so `JP +7` at PC 21 goes to PC 22+7 = **29**.

### 3. Fully decoded CRT0 startup sequence (PC 0-34)

```
  PC 0-1:   SP = 0x01000000 (16MB stack)
  PC 2-10:  r3 = __bss_start (0x5428C)
  PC 11-19: r4 = __bss_end (0x8EB50)

  BSS clearing loop (PC 20-28, 9 instructions per word):
    PC 20: CMP  r3, r4
    PC 21: JP   +7 → PC 29       (exit when r3 >= r4)
    PC 22: MOVI r0, 0x0000
    PC 23: ST   [r3+0], r0       (clear 4 bytes)
    PC 24: MOV  r3, r3           (NOP)
    PC 25: MOVI r0, 0x0004
    PC 26: ADD  r3, r0           (r3 += 4)
    PC 27: MOVI r0, 0x0000
    PC 28: JMP  -9 → PC 20       (loop back)

  Post-BSS (PC 29+):
    PC 29-33: r8 = 1 (argc), r9 = 2 (argv setup)
    Then: CALL doomgeneric_Create(1, _argv)
    Then: loop { CALL doomgeneric_Tick }
```

BSS range: 0x5428C → 0x8EB50 = 240,836 bytes = 60,209 words.
At 9 insns/word = 541,881 insns ≈ 133 packets for full BSS clear.

### 4. **CRITICAL: Discovered race condition in packet ring**

**Symptom:** After injecting 100 rapid packets, r3 went *backwards* (0x801F8 → 0x70C54). This is impossible in the monotonically-increasing BSS loop.

**Root Cause:** Multiple tick packets overlapping in the 6-namespace ring simultaneously. Each packet's XDP program instance does:
1. `CPU_MAP.get_ptr_mut(&instance)` — gets raw mutable pointer
2. Executes 16 MBC instructions, modifying state in-place
3. Returns — no locking, no atomics

When packet A is at hop 3 and packet B is at hop 0, both modify the same CPU_MAP entry concurrently. The last writer wins, discarding the other's work. This causes:
- Lost progress (r3 advances on one hop, gets overwritten by stale state from another)
- r3 appearing to go backwards
- insn_count being approximately (not exactly) correct (lost increments cancel somewhat)

**Proof:** With 1 packet at a time and 200ms delay, the behavior is correct and monotonic:
- Iteration 1: r3 went from 0x70C54 → 0x7136C (+0x718 = 1816 bytes = 454 words)
- Iteration 2: 0x7136C → 0x71A80 (+0x714 = 1812 bytes = 453 words)
- Iteration 3: 0x71A80 → 0x72194 (+0x714 = 1812 bytes = 453 words)
- Iteration 4: 0x72194 → 0x728AC (+0x718 = 1816 bytes = 454 words)
- Iteration 5: 0x728AC → 0x72FC0 (+0x714 = 1812 bytes = 453 words)

Each packet processes ~4,080 instructions = ~453 words cleared (4080/9 = 453.3). insn_count increments by exactly 4,080 each time. **Single-packet injection is perfectly correct.**

**Timing analysis:** Each packet traverses 255 hops. Each hop takes ~10-50µs (XDP + kernel routing). Full ring traversal ≈ 3-13ms. Safe injection rate: ~50-80 pkt/s max (one packet completes before the next enters).

---

## Where We're At

### BSS clearing still in progress
- Current: r3 = 0x72FC0, r4 = 0x8EB50
- Remaining: 0x8EB50 - 0x72FC0 = 0x1BB90 = 113,552 bytes = 28,388 words
- Instructions needed: 28,388 × 9 = 255,492
- Packets needed: 255,492 / 4,080 ≈ **63 packets**
- At 50 Hz injection rate: ~1.3 seconds

### After BSS: Doom initialization
- CRT0 calls `doomgeneric_Create(argc=1, argv=_argv)`
- doomgeneric_Create calls D_DoomMain → D_DoomLoop → doomgeneric_Tick
- This will need millions of instructions (calloc, WAD loading, texture setup, etc.)
- With the memset bug fixed, calloc should be MUCH faster (no more 16.8MB clears per allocation)

---

## What Needs To Be Done Next

### Priority 1: Fix the race condition (blocking issue)

The current single-packet injection at ~50 Hz gives only ~204K insns/sec. At this rate, Doom initialization would take minutes to hours. The old rate of ~5,600 pkt/s gave ~23M insns/sec but caused races.

**Options (in order of preference):**

1. **BPF spinlock on CPU_MAP** — Add `bpf_spin_lock` to the CPU state struct and lock around the execute loop. Requires:
   - Add `__u32 lock` field to `MbcCpuState` in `monad-common/src/lib.rs`
   - Use `bpf_spin_lock(&cpu.lock)` / `bpf_spin_unlock(&cpu.lock)` in `main.rs`
   - Rebuild and redeploy BPF program
   - **Pro:** Allows max throughput. **Con:** BPF spin_lock has restrictions (can't call helpers while holding lock — may conflict with map operations inside the execute loop).

2. **Sequence number in CPU state** — Add a sequence counter. Each hop reads seq, executes, writes seq+1. Before executing, check if seq matches expected. If not, skip (stale packet). This is optimistic concurrency control.
   - **Pro:** Simple, no lock restrictions. **Con:** Wasted work on conflicts.

3. **Single-packet injection with rate limiting** — Keep the current approach but optimize:
   - Inject at ~80 pkt/s (estimated safe rate based on ring latency)
   - ~327K insns/sec — still slow but may be adequate for testing
   - **Pro:** No code changes. **Con:** Slow.

4. **Hop-limit = 1 with direct re-injection** — Reduce hop_limit to 1 so each packet only does 16 insns. Re-inject from userspace at max rate. No overlap since each packet dies immediately.
   - **Pro:** No races, simple. **Con:** Max 16 insns per packet, overhead of per-packet injection.

### Priority 2: Continue execution past BSS

Once the race is fixed (or with slow safe injection):
1. Inject ~63 packets to finish BSS clearing
2. Then inject more to push through doomgeneric_Create initialization
3. Watch for: halted state (error), new tight loops, syscall invocations (SYS_DRAW_FRAME etc.)

### Priority 3: Fix translator properly

The 3→2 operand bug in `crates/monad-mbc/src/translator.rs` (~line 421-424) needs a source-level fix for all affected instructions:
- `add rd, rs1, rs2` where rd == rs2 → should emit `ADD rd, rs1` (not `MOV rd, rs1; ADD rd, rd`)
- Same pattern applies to: SUB, AND, OR, XOR, SLL, SRL, SRA, MUL, etc.
- After fixing, retranslate doom.mbc and reload ROM

---

## Current System State

| Component | Status |
|-----------|--------|
| Ring namespaces | monad0-5 UP |
| XDP attachments | All 6 hops attached (prog_id 35) |
| BPF maps | All loaded (ROM with live memset patch, RV2MBC, RAM, CPU) |
| CPU state | PC≈20-28 (BSS loop), r3=0x72FC0, insn_count≈122.9M, halted=0 |
| Write-through cache | Deployed, working |
| Safe packet rate | ~50 Hz (single-packet-at-a-time) |
| Race condition | **CONFIRMED** — must serialize or lock |

## Key Files Modified (uncommitted)

```
ebpf/monad-cpu-ebpf/src/main.rs   — write-through for ST/STB/STH
scripts/doom-loader-core.py        — SP fix
scripts/doom-loader.sh             — data section VMA auto-detection
docs/sessions/S20-handoff.md       — this document
```

## Live ROM Patches (not in source)

These patches are in the BPF ROM_MAP but NOT in doom.mbc source:
```
ROM[103934]: 0x0EA80000 → 0x01A80000  (MOV r10,r8 → ADD r10,r8)
ROM[103935]: 0x01AA0000 → 0x0ED80000  (ADD r10,r10 → MOV r13,r8)
ROM[103936]: 0x0ED80000 → 0x0EDD0000  (MOV r13,r8 → NOP)
```
These fix the memset 3→2 operand bug. The translator source fix is still needed.

## Commands to Resume

```bash
# Check current CPU state
sudo python3 scripts/doom-cpu-dump.py

# Safe single-packet injection (finish BSS: ~63 packets needed)
for i in $(seq 1 63); do
  sudo ip netns exec monad0 python3 scripts/doom-tick.py --count 1
  sleep 0.02
done

# Or use rate-limited injection (untested, try ~50 Hz)
sudo ip netns exec monad0 python3 scripts/doom-tick.py --count 100 --rate 50

# If ring needs rebuild
sudo scripts/doom-ring.sh teardown
sudo scripts/doom-ring.sh setup
sudo scripts/doom-loader.sh all
# Then re-apply live ROM patches (see above)
```
