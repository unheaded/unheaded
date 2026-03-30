# Doom-over-IPv6 -- Next Steps

**Last updated:** 2026-03-30
**Current state:** PLAYABLE (baseline commits 42bbc34d + 46f36f77)

## Priority Order

### P0: Immediate (next session)

1. **Improve browser frame rate**
   - Current: ~2-3 visible fps despite 35 fps internal
   - Root cause: bridge reads 16,000 words from RAM_MAP per frame (BPF map reads are slow)
   - Approaches:
     a. Batch map reads (read contiguous ranges instead of individual lookups)
     b. Frame-diff: only send changed pixels
     c. Shared memory path instead of BPF map reads
     d. Move screen data to a separate pinned map for direct mmap
   - Target: >= 15 fps visible in browser

2. **Record demo video**
   - Needs stable visuals at reasonable frame rate
   - Capture both terminal output and browser gameplay
   - Will serve as proof-of-concept for presentations

### P1: Near-term polish

3. **Investigate weapon sprite flashing**
   - Symptom: hand/weapon sprite appears/disappears rapidly
   - Likely cause: rendering timing between I_FinishUpdate and bridge read
   - The back buffer should fix most tearing; remaining flash may be column draw timing

4. **NixOS service module**
   - `doom-runner.nix` with systemd service definition
   - Automatic eBPF loading, bridge startup, injector management
   - One-command `systemctl start doom` to launch everything

5. **One-command launcher script**
   - `./run-doom.sh` that handles all steps from RUNBOOK
   - Builds if needed, launches doom-runner, attaches XDP, starts injector, opens browser

### P2: Performance optimization

6. **Tune injection rate**
   - Current: sendmmsg with batch 200, ~93K pps
   - Experiment with larger batches, different burst patterns
   - Target: maximize useful instructions per second

7. **Explore XDP_TX turbo mode**
   - Packet bounces on same interface for cache-warm re-execution
   - Eliminates userspace round-trip entirely
   - Could dramatically increase throughput

8. **Profile MBC hot paths**
   - Which libc functions consume the most instructions?
   - memcpy, memset, R_DrawColumn likely dominate
   - Consider MBC-specific optimizations for innermost loops

### P3: Stretch goals

9. **Sound support**
   - Would require audio output hardware in MBC
   - Or: stream audio alongside video over WebSocket
   - Low priority: Doom without sound is still Doom

10. **Multiplayer over Monad transport**
    - id DOOM has built-in IPX/UDP networking
    - Replace with Monad message passing
    - Two UPC instances, each running Doom, connected over IPv6

## Completed (for reference)

- [x] Fix PC corruption (CALL/RET in MBC translator)
- [x] Back buffer rendering (reduces tearing)
- [x] 8-slot keyboard circular queue (prevents key overwrite)
- [x] Browser auto-repeat suppression
- [x] Bilinear CSS upscale
- [x] Word-aligned memcpy fast path
- [x] Dynamic PLAYPAL palette (correct colors)
- [x] Fix palette (203/256 entries were wrong)
- [x] Full JS keyCode -> Doom key mapping
- [x] Soft-float IEEE 754 (25 functions)
- [x] JVM-style dynamic heap (sbrk)
- [x] POSIX fd table for WAD I/O
