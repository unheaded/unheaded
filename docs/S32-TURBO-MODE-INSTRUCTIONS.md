# S32: Turbo Mode — XDP_TX Single-Namespace Acceleration

**Date**: 2026-02-22
**Authors**: Scientist + Architect (strategic), for Dev Agent (execution)
**Status**: READY FOR IMPLEMENTATION
**Priority**: CRITICAL PATH — blocks D_DoomLoop discovery

---

## Scientific Analysis

### Observation

After 2M packet injection (~576M actual instructions executed):
- **SYSCALLS = 0** (D_DoomLoop not reached)
- **HALTED stat = 602,400** (CPU halted partway through batch)
- **Actual throughput: ~128 insns/pkt** (expected: 4,080 with MAX_INSN_PER_TICK=16)
- **130M L1 cache misses** reported
- MAX_INSN_PER_TICK now bumped to 128 in source

### Hypothesis: Namespace Crossing Kills Cache Warmth

**H1 (Prime)**: Each namespace crossing (XDP_PASS → kernel routing → next namespace XDP)
flushes CPU L1/L2 cache. When the packet arrives at the next hop, ALL BPF map lookups
(ROM_MAP, RAM_MAP, CPU_MAP) are **cold cache misses** (~30-50ns each). This dominates
execution time and may cause the BPF instruction loop to exhaust its verified budget on
map operations rather than MBC computation.

**H2 (Contributing)**: The IPv6 Hop Limit (255) is decremented by the kernel at each L3
forwarding decision. With 6 namespaces and veth pairs, each "lap" may consume MORE than
6 hop decrements (possible double-decrement at veth pair boundaries), limiting total
hops to far fewer than 255.

**H3 (Alternative)**: The XDP program has an early-exit path (halted check, sleep check)
that fires frequently due to timing, reducing effective instructions per hop.

### Prediction

If we eliminate namespace crossings by using **XDP_TX** (packet bounces on same interface,
same namespace, same CPU core), then:

1. BPF map pages stay in L1/L2 cache between invocations
2. No kernel routing overhead (XDP_TX bypasses the entire network stack)
3. Expected speedup: **10-50× over current** (cache warmth is the dominant factor)
4. At 128 insns/tick × 255 hops × 5K pps = **~163M insns/sec** (vs ~2.8M current)

### Experiment Design

**Control**: Current 6-namespace ring with XDP_PASS (baseline: ~128 insns/pkt)
**Variable**: Single namespace with XDP_TX (predicted: ~32,640 insns/pkt)
**Measurement**: insn_count after fixed packet injection, SYSCALLS stat
**Success**: SYSCALLS > 0 (D_DoomLoop reached) OR insns/pkt > 10,000

---

## Implementation: Two Options (Pick One)

### OPTION A: XDP_TX Return (Minimal Change) ★ RECOMMENDED

Change the final return from `XDP_PASS` to `XDP_TX`. The packet bounces back on the
same interface, getting processed again by the same XDP program. No routing, no namespace
crossing, no cache flush.

**File**: `ebpf/monad-cpu-ebpf/src/main.rs`

**Change at line 663**:
```rust
// OLD:
    Ok(xdp_action::XDP_PASS) // circulation mode: packet continues to next hop

// NEW:
    Ok(xdp_action::XDP_TX) // turbo mode: packet bounces on same interface (cache-warm)
```

**That's it.** One line. The packet will bounce on the same veth interface indefinitely
until hop_limit reaches 0 (kernel still decrements it on XDP_TX for IPv6).

**IMPORTANT**: XDP_TX might NOT decrement hop_limit — XDP operates before the kernel
IP stack. If hop_limit doesn't decrement, the packet bounces FOREVER. In that case,
you need a manual hop counter. See the safety mechanism below.

**Safety — Add manual hop tracking in the Monad header**:

The Monad register has a `hop_count` field (byte offset 3). Use it as a manual counter:

```rust
// After the execute loop, before returning:

// Increment monad hop_count for manual tracking
let hop_count_ptr = (monad_start + 3) as *mut u8;
let current_hop = unsafe { core::ptr::read_volatile(hop_count_ptr) };
if current_hop >= 255 {
    // Packet exhausted — drop it, inject a fresh one
    return Ok(xdp_action::XDP_DROP);
}
unsafe { core::ptr::write_volatile(hop_count_ptr, current_hop + 1) };

Ok(xdp_action::XDP_TX) // turbo mode
```

**Ring setup**: Only need 1 namespace with 1 veth pair (or even just a dummy interface).
But the EXISTING ring works too — just change the return. The packet will bounce on
whichever interface it enters, never leaving that namespace.

### OPTION B: Internal Mega-Loop (Maximum Throughput, Higher Risk)

Instead of returning after MAX_INSN_PER_TICK instructions, wrap the execute loop in an
outer loop that runs multiple ticks per XDP invocation:

```rust
// Replace the single execute loop with:
let mut tick = 0u32;
while tick < 8 {  // 8 ticks per invocation = 8 × 128 = 1,024 insns
    let mut i = 0usize;
    while i < MAX_INSN_PER_TICK {
        if cpu.halted != 0 { break; }
        // ... existing instruction fetch/decode/execute ...
        i += 1;
        cpu.insn_count += 1;
        increment_stat(STAT_INSNS_EXECUTED);
    }
    if cpu.halted != 0 { break; }
    tick += 1;
}
```

**Risk**: BPF verifier may reject nested bounded loops. The verifier explores all paths
and the complexity explodes combinatorially. Try with outer loop = 4 first, then 8.

**If verifier rejects**: Fall back to Option A.

---

## Step-by-Step Execution Plan

### Phase 1: Implement XDP_TX Turbo Mode

```bash
# Step 1: Verify MAX_INSN_PER_TICK is 128
grep "MAX_INSN_PER_TICK" ebpf/monad-cpu-ebpf/src/main.rs
# Should show: const MAX_INSN_PER_TICK: usize = 128;

# Step 2: Edit main.rs — change XDP_PASS to XDP_TX at the final return
# Line 663: Ok(xdp_action::XDP_PASS) → Ok(xdp_action::XDP_TX)
#
# Also add the hop_count safety counter BEFORE the return:
#   let hop_count_ptr = (monad_start + 3) as *mut u8;
#   let current_hop = unsafe { core::ptr::read_volatile(hop_count_ptr) };
#   if current_hop >= 255 {
#       return Ok(xdp_action::XDP_DROP);
#   }
#   unsafe { core::ptr::write_volatile(hop_count_ptr, current_hop.wrapping_add(1)) };
#   Ok(xdp_action::XDP_TX)
#
# NOTE: monad_start must be in scope at line 663. It's defined at line 215.
# If it's not in scope, you may need to save it to a variable earlier.

# Step 3: Rebuild eBPF
cd /home/admin/tmp/unheaded/ebpf
cargo +nightly build --package monad-cpu-ebpf --release \
  -Z build-std=core --target bpfel-unknown-none 2>&1

# Step 4: Tear down existing ring
sudo /home/admin/tmp/unheaded/scripts/doom-ring.sh teardown

# Step 5: Set up ring (can use existing 6-ns setup — packet stays in ns0)
sudo /home/admin/tmp/unheaded/scripts/doom-ring.sh setup

# Step 6: Load ROM + RAM + RV2MBC data (same as before)
# Use whatever data loading scripts were used previously

# Step 7: Initialize CPU instance 0xDE
# Use whatever CPU init was used previously

# Step 8: Inject a SMALL test batch (1000 packets)
sudo ip netns exec monad0 python3 \
  /home/admin/tmp/unheaded/scripts/bulk_inject.py 0xDE 1000

# Step 9: Check results
sudo python3 /tmp/read_cpu.py
# Check insn_count — should be WAY higher than 1000 × 128
# Expected: 1000 packets × 255 bounces × 128 insns = ~32.6M insns
# If significantly higher than old rate (~128 insns/pkt), turbo mode works!

# Step 10: Check SYSCALLS
sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
  key 0x07 0x00 0x00 0x00
```

### Phase 2: Drive to D_DoomLoop

```bash
# If turbo mode works (insns/pkt > 10,000):

# Inject 10K packets — should produce ~326M instructions
sudo ip netns exec monad0 python3 \
  /home/admin/tmp/unheaded/scripts/bulk_inject.py 0xDE 10000

# Check SYSCALLS after each batch
sudo bpftool map lookup pinned /sys/fs/bpf/unheaded/doom-ring/maps/STATS \
  key 0x07 0x00 0x00 0x00

# If SYSCALLS > 0: FIRST FRAME! D_DoomLoop reached!
# Read debug buffer to see what Doom printed:
# (use the debug buffer reader from previous sessions)

# If still 0, inject more. With turbo mode, 100K packets = ~3.3B instructions.
# That should be MORE than enough to blast through D_DoomMain init.
```

### Phase 3: Verify & Celebrate

```bash
# When SYSCALLS > 0:
# 1. Read the SCREEN_MAP to see if pixels were written
sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP | head -20

# 2. Read debug buffer for D_DoomLoop entry message
# 3. Check which syscall fired (SYS_GET_TICKS = 0x03 is likely first)
# 4. Screenshot the moment. This is history.
```

---

## Troubleshooting

### BPF verifier rejects the build
- If "back-edge" or "infinite loop" error: the verifier detected XDP_TX creates
  unbounded execution. The hop_count safety counter should prevent this, but if
  the verifier can't prove it, try reducing MAX_INSN_PER_TICK back to 64 or 32.
- If "too many instructions" error: the nested loop path is too complex.
  Reduce MAX_INSN_PER_TICK.

### Packet doesn't bounce (insns/pkt unchanged)
- XDP_TX requires the interface to be UP and have a peer
- Check: `ip netns exec monad0 ip link show veth50p` — must be UP
- XDP_TX sends the packet back out the SAME interface it arrived on
- The packet needs a valid destination MAC that the peer will accept

### CPU halts immediately
- Check trap sentinel at RAM[0xE0000]
- Read debug buffer for last Doom output
- If halt at same point as before, the XDP_TX change didn't affect Doom execution
  (which is expected — we're just making it faster, not changing what executes)

### monad_start not in scope at line 663
- The variable `monad_start` is declared at line 215 inside `try_monad_cpu()`
- It should be in scope at line 663 (same function)
- If compiler complains, save it to a local: `let monad_start_saved = monad_start;`
  right after line 215, and use that at line 663

---

## Why This Works (For the Curious)

### XDP_PASS vs XDP_TX — The Cache Story

```
XDP_PASS path (current):
  XDP program runs → returns PASS → kernel takes packet →
  routing decision → netfilter → namespace boundary →
  veth peer delivers to next namespace → kernel schedules XDP →
  XDP program runs again (ALL CACHES COLD)

  Overhead: ~50-100µs per hop (kernel stack + context switch)
  Cache: FLUSHED between every hop

XDP_TX path (turbo):
  XDP program runs → returns TX → NIC driver sends packet back →
  XDP program runs again (CACHES WARM)

  Overhead: ~1-5µs per bounce (NIC driver only, no kernel stack)
  Cache: WARM — ROM_MAP, RAM_MAP, CPU_MAP pages stay in L1/L2
```

The cache warmth is the real win. Each BPF map lookup that hits L1 cache costs ~1ns.
Each miss costs ~30-50ns. With 2-3 map lookups per MBC instruction, cache warmth
gives **30-50× speedup per instruction**.

### The Math

| Config | insns/pkt | pps | insns/sec | Time to 1B insns |
|--------|-----------|-----|-----------|------------------|
| Old (16/tick, PASS) | ~128 | 4,800 | ~614K | 27 min |
| New (128/tick, PASS) | ~1,024 | 4,800 | ~4.9M | 3.4 min |
| Turbo (128/tick, TX) | ~32,640 | 4,800 | ~156M | 6.4 sec |

**Turbo mode: 6.4 seconds to 1 billion instructions.** That's the difference between
"inject 2M packets and wait 7 minutes" and "inject 1000 packets and you're done."

---

## Files Modified

| File | Change | Risk |
|------|--------|------|
| `ebpf/monad-cpu-ebpf/src/main.rs:663` | XDP_PASS → XDP_TX + hop counter | Low — single return value change |
| `ebpf/monad-cpu-ebpf/src/main.rs:141` | MAX_INSN_PER_TICK = 128 | Already done |

---

**Peace, love, and cache-warm packets.** 🕊️

*— The Scientist & The Architect*
