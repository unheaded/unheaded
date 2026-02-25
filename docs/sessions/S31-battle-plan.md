# Battle Plan: S31 — Doom Packet Ring Integration Sprint

## Convened: 2026-02-21 | Reason: S30 Complete — Next Phase Planning
## Kingdom State: Age 1.5 (Protocol Awakening), Doom Emulator GREEN, BPF Verified

---

### Situation Report

S30 delivered the full Doom-over-IPv6 software stack: 43-opcode MBC emulator (Rust + BPF), Go-side loader/CLI, 3 fuzz targets, 43 integration tests, browser-based viewer, and comprehensive documentation. The BPF verifier accepts monad-cpu-ebpf. The LICH-007 72-hour fuzz campaign is running with zero crashes across 1B+ executions.

**What we have**: A verified emulator, a working assembler, a Go control plane, and a BPF program the kernel accepts.

**What we need**: To connect them. The packet ring (6 namespaces, XDP forwarding, shared BPF maps) is the last integration gap between "Doom compiles and runs in userspace" and "Doom runs over IPv6 packets."

---

### Priority Stack

#### P0: LICH-007 Campaign Results (XS — 30 min)

**Prerequisite**: Campaign finishes (~2026-02-24 03:38 UTC)

- [ ] Collect final stats from `/tmp/lich007-{decode,execute,roundtrip}.log`
- [ ] Check `crates/monad-mbc/fuzz/artifacts/` for crashes (expect empty)
- [ ] Run `cargo +nightly fuzz coverage fuzz_mbc_execute` for coverage report
- [ ] Write `docs/sessions/S30-lich007-results.md` with duration, exec/s, coverage %, corpus size
- [ ] Commit results doc

**Exit gate**: Results documented. If crashes found, fix and re-run affected target for 24h.

---

#### P1: Packet Ring Assembly (L — 4-6 hours)

**Goal**: Set up the 6-namespace packet circulation ring and verify packets flow.

This is the physical infrastructure that turns IPv6 packets into CPU clock ticks.

- [ ] **Step 1**: Review and update `scripts/doom-ring.sh` for current kernel/iproute2
  - Verify per-link /64 prefixes: `fd00:3f:75:${i}::1/64` (NOT same /64)
  - Verify destination address outside connected subnets: `fd00:dead::1`
- [ ] **Step 2**: Create 6 network namespaces (monad0-5) with veth pairs
- [ ] **Step 3**: Configure IPv6 forwarding and default routes in each namespace
- [ ] **Step 4**: Verify ping traverses the full ring (monad0 → monad1 → ... → monad5 → monad0)
- [ ] **Step 5**: Measure baseline packet throughput without XDP (`nping --rate 1000`)
- [ ] **Step 6**: Verify no NDP issues (the per-link /64 prefix lesson from previous sessions)

**Exit gate**: `ping6 -c 100 fd00:dead::1` completes 100/100 from monad0 through all 6 hops.

**Known pitfalls** (from MEMORY.md):
- Same /64 across veth pairs causes NDP ambiguity → packet drops
- Destination must be outside connected subnets or kernel attempts NDP instead of forwarding

---

#### P2: BPF Map Pinning & Shared State (M — 2-3 hours)

**Goal**: Pin BPF maps to filesystem, load monad-cpu-ebpf once, attach to all 6 hops.

- [ ] **Step 1**: Create pin directory: `/sys/fs/bpf/unheaded/doom-ring/maps/`
- [ ] **Step 2**: Load monad-cpu-ebpf via `cmd/ebpf-loader/` on hop0 with map pinning
- [ ] **Step 3**: Verify maps pinned: `ls /sys/fs/bpf/unheaded/doom-ring/maps/`
  - Expect: ROM_MAP, CPU_MAP, RAM_MAP, SCREEN_MAP, KBD_MAP, L1_CACHE, RV2MBC_MAP, COMPUTE_EVENTS
- [ ] **Step 4**: Attach same program to hops 1-5 via `bpftool net attach xdpgeneric`
- [ ] **Step 5**: Verify all 6 hops share same prog_id: `bpftool net show`
- [ ] **Step 6**: Write and load a trivial 3-instruction ROM: `MOVI r0, 42; MOVI r1, 1; ADD r0, r1`
- [ ] **Step 7**: Initialize CPU_MAP with default state via Go CLI: `doom reset`
- [ ] **Step 8**: Verify CPU state readable: `doom status`

**Exit gate**: All 6 hops running same BPF program, maps pinned and accessible from Go CLI.

**Known pitfall** (from MEMORY.md): aya-ebpf 0.1.x `EbpfLoader::map_pin_path()` does NOT reuse pinned legacy maps. Load once, attach everywhere via bpftool.

---

#### P3: First Packet Execution (L — 4-6 hours)

**Goal**: Send one packet through the ring and observe CPU state change.

This is the "Hello World" moment — a single IPv6 packet executes 16 MBC instructions.

- [ ] **Step 1**: Load trivial ROM via Go CLI: `doom load trivial.mbc`
- [ ] **Step 2**: Record initial CPU state: `doom status` (expect PC=0, r0=0)
- [ ] **Step 3**: Inject one packet via `scripts/doom-tick.py` (or `doom inject-tick`)
- [ ] **Step 4**: Read CPU state after packet: `doom status`
  - Expect: PC advanced, r0=42 (MOVI executed), insn_count=3
- [ ] **Step 5**: If state unchanged, debug:
  - Check XDP return code: `bpftool prog tracelog`
  - Check packet actually reached XDP: `ip netns exec monad0 tc -s qdisc show`
  - Verify flow label matches CPU_MAP key
- [ ] **Step 6**: Inject 10 packets rapidly, verify insn_count increments by ~16 per packet
- [ ] **Step 7**: Inject packet with HALT in ROM, verify `halted=1` in CPU state

**Exit gate**: Single packet causes CPU state mutation. PC, registers, insn_count all update correctly.

---

#### P4: Doom ROM Loading & BSS Init (XL — 6-10 hours)

**Goal**: Load doom.mbc into the ring and survive BSS clearing.

- [ ] **Step 1**: Verify doom.mbc exists and is well-formed: `doom load --stats doom/doomgeneric/doom.mbc`
- [ ] **Step 2**: Check ROM size vs ROM_MAP capacity (262,144 entries)
- [ ] **Step 3**: Load doom.mbc into BPF maps
- [ ] **Step 4**: Load RV2MBC translation table into RV2MBC_MAP
- [ ] **Step 5**: Initialize CPU state (SP=0xFFFF_0000, PC=0)
- [ ] **Step 6**: Begin packet injection at maximum rate
- [ ] **Step 7**: Monitor BSS clearing progress via insn_count
  - BSS is ~6MB, cleared byte-by-byte (~6 insns/byte)
  - Estimated: ~60M instructions = ~14,700 packets at 4,080 insns/pkt
  - At 8,600 pkt/s max speed: ~1.7 seconds wall time
- [ ] **Step 8**: Monitor for HALT (indicates Doom reached exit/error)
  - Known issue: access() returns -1 → IWAD discovery fails → exit()
  - Fix: make access() return 0 for ".wad" paths in libc_monad.c
- [ ] **Step 9**: If HALT at exit(), patch libc stubs and rebuild
- [ ] **Step 10**: Monitor for first frame write to SCREEN_MAP

**Exit gate**: Doom survives BSS init, enters main loop, writes at least one frame to screen buffer.

**Known pitfalls** (from MEMORY.md):
- BSS clearing takes ~60M instructions (~14,700 packets)
- Doom halts at exit() after 59.8M insns if access() returns -1
- JMPR/CALLR need RV2MBC_MAP lookups for indirect jumps

---

#### P5: Dashboard Live Rendering (M — 2-3 hours)

**Goal**: See Doom frames in the browser.

- [ ] **Step 1**: Wire dashboard handlers to real BPF maps (replace mock accessor)
- [ ] **Step 2**: Serve `dashboard/doom.html` from Go HTTP server
- [ ] **Step 3**: Implement `/doom/screen` endpoint reading SCREEN_MAP (320x200, 8-bit indexed color)
- [ ] **Step 4**: Implement `/doom/input` endpoint writing KBD_MAP
- [ ] **Step 5**: Open browser, verify canvas updates at target 30 FPS
- [ ] **Step 6**: Test keyboard input: arrow keys → Doom responds
- [ ] **Step 7**: Measure end-to-end latency: packet injection → screen render → browser display

**Exit gate**: Doom gameplay visible in browser. Keyboard input works. Sub-100ms latency target.

---

#### P6: Performance Profiling (S — 1-2 hours)

**Goal**: Measure and document the Doom-over-IPv6 pipeline performance.

- [ ] **Step 1**: Measure packet throughput: pkt/s through full 6-hop ring
- [ ] **Step 2**: Measure instruction throughput: insns/sec at max packet rate
- [ ] **Step 3**: Measure frame rate: frames/sec reaching SCREEN_MAP
- [ ] **Step 4**: Profile bottleneck: is it packet injection, BPF execution, or map reads?
- [ ] **Step 5**: Document results in `docs/protocol/doom-performance-baseline.md`
- [ ] **Step 6**: Compare against targets:
  - Packet throughput: >8,000 pkt/s
  - Instruction throughput: >30M insns/sec
  - Frame rate: >10 FPS (Doom target is 35 FPS)

**Exit gate**: Performance documented. Bottleneck identified.

---

### Agent Assignment Strategy

```
Phase 0 (results)  → Solo, when campaign finishes
Phase 1 (ring)     → Coordinator (requires sudo, namespace ops)
Phase 2 (BPF maps) → Coordinator (requires sudo, BPF ops)
Phase 3 (first pkt) → Coordinator (debugging, iterative)
Phase 4 (doom ROM)  → Coordinator + Agent (libc patches if needed)
Phase 5 (dashboard)  → Agent (Go HTTP, independent of ring)
Phase 6 (perf)      → Agent (measurement, independent of dashboard)
```

Phases 1-3 are strictly sequential (each depends on prior). Phases 5-6 can run in parallel once Phase 3 succeeds.

---

### Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| Packet ring NDP issues | Medium | High | Per-link /64 prefixes (already documented) |
| BPF map pinning fails with aya loader | Low | High | Fallback: manual bpftool map create + pin |
| Doom hits unimplemented syscall | High | Medium | Stub returns sensible defaults, fix iteratively |
| access() returns -1, Doom exits | Known | High | Patch libc_monad.c access() stub |
| Frame rate too low (<10 FPS) | Medium | Medium | Increase MAX_INSN_PER_TICK, reduce hops |
| BPF verifier rejects under real load | Low | High | Already verified in S30 — unlikely to regress |

---

### Success Criteria

- [ ] Packet ring operational (6 namespaces, full circulation)
- [ ] BPF maps shared across all hops
- [ ] Single packet mutates CPU state correctly
- [ ] doom.mbc loads and survives BSS init
- [ ] At least one Doom frame reaches SCREEN_MAP
- [ ] Browser displays Doom frames
- [ ] Keyboard input reaches Doom
- [ ] Performance baseline documented

**If all criteria met**: Protocol Awakening Phase 1 COMPLETE. Doom runs over IPv6.

---

### S30 Exit Criteria Checklist (Updated)

From the original S30 plan — current status:

- [x] All 43 MBC opcodes decode without panic on any input
- [x] All ALU operations handle overflow/underflow with wrapping semantics
- [x] Division by zero saturates to 0xFFFFFFFF
- [x] Memory access OOB returns 0/false
- [x] Cycle budget prevents infinite loops in XDP
- [x] BPF verifier ACCEPTS monad-cpu-ebpf
- [x] Minimal test ROM executes correctly
- [x] SYS_DRAW_FRAME writes to screen buffer
- [x] SYS_GET_KEY reads from KBD_MAP
- [x] LICH-007 running with 0 crashes (1B+ executions)
- [x] All unit tests pass (293 Rust + full Go)
- [x] Go/Rust struct parity verified (104 bytes)
- [x] doom load / status / input / reset commands work
- [x] doom.mbc ROM loads without crash
- [x] Userspace emulator matches BPF dispatch
- [x] ISA reference document complete
- [x] Architecture document complete
- [ ] **Packet ring integration** — S31 P1-P3
- [ ] **Live dashboard rendering** — S31 P5
- [ ] **Performance baseline** — S31 P6

---

*Forged after S30's 120-step sprint.*
*The emulator is proven. The BPF is verified. The fuzz campaign holds.*
*S31 connects the wires. Doom meets IPv6 for real.*
