# WS3 Phase 2: BPF Bounce Timestamp Instrumentation Design

## Overview

This document describes the proposed STATS map extension for nanosecond-precision
bounce cycle measurement in the monad-cpu-ebpf XDP program.

## Current STATS Map (keys 0-10)

From `ebpf/monad-cpu-ebpf/src/main.rs`:

```
 0: PACKETS_TOTAL    -- Total packets received
 1: CPU_TICKS        -- Tick packets executed
 2: INSNS_EXECUTED   -- Total MBC instructions completed
 3: HALTED           -- Halts due to SYSCALL
 4: SLEEPING         -- CPU sleep state
 5: NO_STATE         -- Packets for uninitialized instance
 6: MEM_FAULTS       -- Invalid RAM_MAP access
 7: SYSCALLS         -- SYSCALL instructions executed
 8: ROM_FAULT        -- Out-of-bounds PC or invalid PC
 9: MEM_STORES       -- Memory write operations (was CACHE_HITS)
10: MEM_LOADS        -- Memory read operations (was CACHE_MISSES)
```

## Proposed Extension (keys 11-15)

```rust
// Timestamp tracking for bounce cycle measurement
const STAT_LAST_BOUNCE_NS:    u32 = 11;  // Last XDP_TX bounce timestamp (ns)
const STAT_FIRST_BOUNCE_NS:   u32 = 12;  // First bounce in current batch (ns)
const STAT_BOUNCE_COUNT:      u32 = 13;  // Bounces in current batch
const STAT_MAX_BOUNCE_NS:     u32 = 14;  // Max single bounce time (ns)
const STAT_AVG_BOUNCE_NS:     u32 = 15;  // Average bounce time (ns, stored as u64)
```

## Implementation Location

In `ebpf/monad-cpu-ebpf/src/main.rs`, around line 320-340 where XDP_TX is returned,
add before the return statement:

```rust
// Record bounce timestamp if this is a tick packet
if is_tick_packet {
    let now = unsafe { bpf_ktime_get_ns() };

    // Get previous timestamp
    let last = match STATS.get(&STAT_LAST_BOUNCE_NS) {
        Some(ts) => *ts,
        None => 0,
    };

    if last > 0 {
        let delta_ns = now - last;
        // Update max bounce time
        if let Some(max_ptr) = STATS.get_mut(&STAT_MAX_BOUNCE_NS) {
            if delta_ns > *max_ptr {
                *max_ptr = delta_ns;
            }
        }
        // Increment bounce count
        if let Some(count_ptr) = STATS.get_mut(&STAT_BOUNCE_COUNT) {
            *count_ptr += 1;
        }
    } else {
        // First bounce: record as first timestamp
        STATS.insert(&STAT_FIRST_BOUNCE_NS, &now, 0);
    }

    // Update last bounce timestamp
    STATS.insert(&STAT_LAST_BOUNCE_NS, &now, 0);
}
```

## Expected Measurements

Based on the current 6-hop ring with XDP_TX forwarding:

- **Bounce cycle time:** ~1.3ms per full ring traversal (estimated)
- **Max bounce time:** ~2.1ms (worst case with kernel scheduling)
- **Min bounce time:** ~0.5ms (best case)

These measurements will validate hypothesis H1: whether the 3ms inter-packet
delay compensates for Python overhead (yes) or for XDP ring limits (no).

## Reading the Timestamps

Use `scripts/ws3/read-stats.py --bounce-only` to read just keys 11-15.

## Rebuild Instructions

```bash
cd /home/admin/tmp/unheaded
cargo build --release -p monad-cpu-ebpf 2>&1 | tee /tmp/ebpf-build.log

# Reload BPF program
sudo scripts/cleanup.sh 2>/dev/null || true
sudo scripts/setup.sh 2>&1 | tee /tmp/bpf-load.log
```

## Status

- [x] Design documented
- [ ] Implementation applied to monad-cpu-ebpf (requires eBPF build environment)
- [ ] Measurements collected
- [ ] H1 validated with ground truth data
