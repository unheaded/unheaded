# UPC SANDBOX BATTLE PLAN — 19 Phases, 347 Steps

**Date**: 2026-04-30  
**Sprint**: UPC Sandbox — Free GPL-3.0 untrusted-code execution platform with deterministic replay  
**Prerequisite**: Unheaded monorepo at `/tmp/unheaded`, Rust 1.75+, Go 1.21+, Linux 5.15+ with BPF enabled  
**Target**: Standalone GPL-3.0 UPC Sandbox tool that safely executes arbitrary MBC bytecode with full audit replay  
**Estimated Duration**: 24-32 hours across 4-5 sessions  
**Commit Cadence**: Every 4 steps (87 commits planned)  

---

## LEGEND

- **[B]** = Bash command (run directly)
- **[V]** = Verification step (MUST pass before proceeding)
- **[D]** = Debug step (only if prior step fails)
- **[W]** = Write/create file
- **[R]** = Read/inspect file
- **[S]** = Sudo required
- **[P]** = Parallelizable with other marked steps
- **[C]** = Commit checkpoint
- **[STUCK]** = Step skipped via Skip Protocol
- **[BLOCKED]** = Step blocked by upstream STUCK step

---

## PHASE 0: DOCTRINE VERIFICATION & GPL-3.0 ALIGNMENT (Steps 1-24)

**Goal**: Verify community-first doctrine binding, GPL-3.0 license compliance, zero revenue framing.

**Prerequisite**: CLAUDE.md accessible at `/tmp/unheaded/CLAUDE.md`

**Time**: 45 minutes

**Agent**: Coordinator

### Doctrine Binding Check

- [ ] **Step 1** [R]: Read CLAUDE.md Community-First Doctrine section
- [ ] **Step 2** [V]: Doctrine frozen at c6108fb8, contains "WE DO NOT SELL. WE SHARE."
- [ ] **Step 3** [B]: Search for revenue language in codebase
- [ ] **Step 4** [V]: Revenue language count = 0
- [ ] **Step 5** [B]: Check root LICENSE file exists
- [ ] **Step 6** [V]: License is GPL-3.0

### Extraction Decision

- [ ] **Step 7** [W]: Create extraction strategy memo (EXTRACT not fork)
- [ ] **Step 8** [W]: Create feature checklist (Tier 1-3 features)
- [ ] **Step 9** [V]: **PHASE 0 EXIT GATE** — Doctrine binding frozen, GPL-3.0 verified
- [ ] **Step 10** [C]: **PHASE 0 COMMIT**

---

## PHASE 1: RESOLVE DOOM PC CORRUPTION BLOCKER (Steps 11-60)

**Goal**: Root-cause analysis and fix for PC corruption bug, bounds checking, deterministic behavior proof.

**Prerequisite**: Phase 0 exit gate passed

**Time**: 2 hours

**Agent**: Coordinator

### PC Corruption Analysis

- [ ] **Step 11** [R]: Read docs/doom/FINDINGS.md PC Corruption section
- [ ] **Step 12** [V]: Understand corruption (PC >> 0x3FFFF, infinite NOPs, JALR target corrupted)
- [ ] **Step 13** [B]: Verify ROM_MAP_ENTRIES = 262,144 (max valid PC: 0x3FFFF)
- [ ] **Step 14** [V]: Max valid PC established

### PC Bounds Check Implementation

- [ ] **Step 15** [W]: Create pc_bounds.rs module with check/halt functions
- [ ] **Step 16** [W]: Add 4 unit tests (valid, boundary, over-boundary, huge)
- [ ] **Step 17** [B]: Add module to lib.rs declarations
- [ ] **Step 18** [B]: Build and test PC bounds module
- [ ] **Step 19** [V]: All 4 tests pass
- [ ] **Step 20** [C]: **COMMIT CHECKPOINT**

### eBPF Executor Integration

- [ ] **Step 21** [R]: Read monad-cpu-ebpf main.rs for instruction loop
- [ ] **Step 22** [W]: Document PC check integration points (pseudocode)
- [ ] **Step 23** [W]: Add STATS tracking for pc_faults counter
- [ ] **Step 24** [V]: **PHASE 1 EXIT GATE** — PC bounds tested, eBPF integration planned
- [ ] **Step 25** [C]: **PHASE 1 COMMIT**

---

## PHASE 2: EXTRACT UPC ISA & MBC BYTECODE (Steps 26-72)

**Goal**: Extract Unheaded Protocol Computer ISA, MBC bytecode definitions into standalone UPC Sandbox module.

**Prerequisite**: Phase 1 exit gate passed

**Time**: 1.5 hours

**Agent**: Coordinator

### Create UPC Sandbox Directory Structure

- [ ] **Step 26** [B]: Create sandbox directory hierarchy (runner, isa, verifier, anamnesis, examples)
- [ ] **Step 27** [V]: Directory structure verified

### Extract Core Files

- [ ] **Step 28** [B]: Copy memory.rs to sandbox/runner/
- [ ] **Step 29** [B]: Copy pc_bounds.rs to sandbox/runner/
- [ ] **Step 30** [V]: Core memory files in place

### Create ISA Definition

- [ ] **Step 31** [W]: Create mbc.rs with MbcOpcode enum (30+ opcodes)
- [ ] **Step 32** [W]: Create MbcInsn struct with field extractors
- [ ] **Step 33** [W]: Add 2 unit tests (opcode extraction, register extraction)
- [ ] **Step 34** [B]: Test ISA module
- [ ] **Step 35** [V]: ISA tests passing

### Extract Verifier Gate

- [ ] **Step 36** [B]: Copy bpf-verifier-check.sh to sandbox/verifier/
- [ ] **Step 37** [W]: Create verifier.rs wrapper with VerifierGate struct
- [ ] **Step 38** [C]: **COMMIT CHECKPOINT**

### Extract Anamnesis Event Format

- [ ] **Step 39** [W]: Create anamnesis.rs with event enum and AnamnesisLog struct
- [ ] **Step 40** [V]: **PHASE 2 EXIT GATE** — ISA, memory, verifier, anamnesis extracted
- [ ] **Step 41** [C]: **PHASE 2 COMMIT**

---

## PHASE 3: SPDX + SBOM + GPL BOUNDARY (Steps 42-65)

**Goal**: Tag extracted code with SPDX headers, generate SBOM, verify GPL-3.0 boundary on dependencies.

**Prerequisite**: Phase 2 exit gate passed

**Time**: 1 hour

**Agent**: Coordinator

### SPDX Header Tagging

- [ ] **Step 42** [B]: Add SPDX headers to all .rs files in sandbox/
- [ ] **Step 43** [V]: All files have "SPDX-License-Identifier: GPL-3.0-or-later"
- [ ] **Step 44** [C]: **COMMIT CHECKPOINT**

### Cargo.toml & SBOM

- [ ] **Step 45** [W]: Create Cargo.toml for upc-sandbox (aya, serde, tokio)
- [ ] **Step 46** [W]: Create SBOM.json (name, version, components, GPL boundary clean)
- [ ] **Step 47** [V]: SBOM generated, no GPL-2.0 dependencies

### License & Contributing

- [ ] **Step 48** [B]: Copy root LICENSE to sandbox/
- [ ] **Step 49** [W]: Create CONTRIBUTING.md (SPDX header requirement, free/share doctrine)
- [ ] **Step 50** [V]: **PHASE 3 EXIT GATE** — SPDX 100%, SBOM clean, GPL boundary verified
- [ ] **Step 51** [C]: **PHASE 3 COMMIT**

---

## PHASES 4-13: PARALLEL HARDENING & TOOLING (Steps 52-250) [P]

**Goal**: Verifier hardening, CLI tools, compliance evidence, WASM shim, testing.

**Prerequisite**: Phase 3 exit gate

**Time**: 8 hours (parallelizable after Phase 3)

**Agent**: Developer [P]

### PHASE 4: MBC VERIFIER HARDENING

- [ ] **Step 52** [W]: Create budget.rs (InstructionBudget struct, max_instructions limit)
- [ ] **Step 53** [W]: Create taint.rs (TaintAnalyzer, mark_tainted, check_jump_target)
- [ ] **Step 54** [W]: Create opcodes.rs (ALLOWED_OPCODES whitelist, is_opcode_allowed)
- [ ] **Step 55** [B]: Build and test verifier modules
- [ ] **Step 56** [V]: All verifier tests passing (7+ tests)
- [ ] **Step 57** [C]: **COMMIT CHECKPOINT**

### PHASE 5: CLI TOOLS (mbc-asm, mbc-disasm, upc-replay)

- [ ] **Step 58** [W]: Create src/bin/mbc-asm.rs (stub with clap Args)
- [ ] **Step 59** [W]: Create src/bin/mbc-disasm.rs (stub with verbose flag)
- [ ] **Step 60** [W]: Create src/bin/upc-replay.rs (stub with step, breakpoint args)
- [ ] **Step 61** [V]: All CLI stubs compile
- [ ] **Step 62** [C]: **COMMIT CHECKPOINT**

### PHASE 6: HARDENING BASELINE

- [ ] **Step 63** [W]: Create src/hardening.rs (3-layer: container, VM, BPF verifier)
- [ ] **Step 64** [V]: Hardening baseline documented
- [ ] **Step 65** [C]: **COMMIT CHECKPOINT**

### PHASE 7: AUDIT LOGGING

- [ ] **Step 66** [W]: Create src/audit.rs (AuditEvent, AuditLogger with HMAC chain)
- [ ] **Step 67** [V]: Audit logger created
- [ ] **Step 68** [C]: **COMMIT CHECKPOINT**

### PHASE 8: SEALED CASK REPRODUCIBLE BUILD

- [ ] **Step 69** [W]: Create scripts/build-sealed-cask.sh (RUSTFLAGS, sealed cask, binding rune)
- [ ] **Step 70** [V]: Script created
- [ ] **Step 71** [C]: **COMMIT CHECKPOINT**

### PHASE 9-11: COMPLIANCE EVIDENCE (NIST, ISO, LICH)

- [ ] **Step 72** [W]: Create docs/COMPLIANCE-NIST-SP800-190.md
- [ ] **Step 73** [W]: Create docs/COMPLIANCE-ISO27001-A14.md
- [ ] **Step 74** [W]: Create docs/LICH-CAMPAIGN.md (72h campaign, 4 attack vectors)
- [ ] **Step 75** [V]: Compliance docs complete
- [ ] **Step 76** [C]: **COMMIT CHECKPOINT**

### PHASE 12: INTEGRATION TESTS

- [ ] **Step 77** [W]: Create tests/integration_test.rs (4 test scenarios)
- [ ] **Step 78** [B]: Build integration tests
- [ ] **Step 79** [V]: Tests compile
- [ ] **Step 80** [C]: **COMMIT CHECKPOINT**

### PHASE 13: WASM-COMPAT SHIM

- [ ] **Step 81** [W]: Create src/wasm_bridge.rs (stub, future feature marker)
- [ ] **Step 82** [V]: WASM bridge skeleton in place
- [ ] **Step 83** [C]: **COMMIT CHECKPOINT**

---

## PHASES 14-19: SEQUENTIAL TESTING & RELEASE (Steps 84-180)

**Goal**: Integrated testing, benchmarks, documentation, public release.

**Time**: 6 hours (sequential)

**Agent**: Developer → Librarian → Captain

### PHASE 14-15: INTEGRATED TESTING & BENCHMARKING

- [ ] **Step 84** [B]: Run full test suite (cargo test)
- [ ] **Step 85** [V]: Core tests pass
- [ ] **Step 86** [W]: Create docs/PERFORMANCE.md (35 fps target, opcode latencies)
- [ ] **Step 87** [V]: Performance targets documented
- [ ] **Step 88** [C]: **COMMIT CHECKPOINT**

### PHASE 16-17: PUBLIC DOCUMENTATION

- [ ] **Step 89** [W]: Create README.md (features, quick start, 3-layer security model)
- [ ] **Step 90** [W]: Create GOVERNANCE.md (community-first, decision process)
- [ ] **Step 91** [W]: Create CHANGELOG.md (v0.1.0 features list)
- [ ] **Step 92** [V]: Documentation complete
- [ ] **Step 93** [C]: **COMMIT CHECKPOINT**

### PHASE 18: DEMO & EXAMPLES

- [ ] **Step 94** [W]: Create examples/doom-demo.sh (build, run, replay, verify)
- [ ] **Step 95** [W]: Create examples/wasm-demo.sh (stub for future WASM bridge)
- [ ] **Step 96** [V]: Demo scripts in place
- [ ] **Step 97** [C]: **COMMIT CHECKPOINT**

### PHASE 19: PUBLIC RELEASE

- [ ] **Step 98** [W]: Create RELEASE-NOTES.md (v0.1.0 feature summary)
- [ ] **Step 99** [W]: Create INSTALL.md (source build, prerequisites, verify)
- [ ] **Step 100** [V]: **PHASE 19 EXIT GATE — Release documentation complete**
- [ ] **Step 101** [B]: Create final v0.1.0 commit
- [ ] **Step 102** [B]: Git tag -a v0.1.0
- [ ] **Step 103** [B]: Count UPC-SANDBOX commits (verify 87+ commits)
- [ ] **Step 104** [C]: **FINAL COMMIT**

---

## APPENDIX A: EMERGENCY PROCEDURES

### E1: PC Corruption Detected

**Symptom**: PC sample shows value >> 0x3FFFF (e.g., 0xDEADBEEF)

**Steps**:
1. Capture CPU_MAP state
2. Extract last valid PC before corruption
3. Check CALLR instruction at that PC (indirect jump)
4. Review register state (which register has garbage?)
5. Trace back: malloc corruption? Stack overflow? vtable corruption?

**Workaround**: If blocker, implement PC breakpoint at last_valid_PC + 1K instructions.

### E2: BPF Verifier Rejects Program

**Symptom**: `bpftool prog load` fails with verifier rejection

**Steps**:
1. Extract verifier output
2. Look for: back-edge, unbounded recursion, pointer arithmetic
3. Add explicit bounds check or taint gate
4. Recompile and retry

### E3: Memory Corruption in Heap

**Symptom**: malloc() returns corrupted pointers, allocations overlap

**Steps**:
1. Check heap bounds (0x1C0000 <= ptr < 0x1BC0000)
2. If OOB: heap_ptr corrupted (apply Phase 1 fix)
3. Add fence bytes (0x00FE) around heap
4. Run under Lich fuzzer with heap instrumentation

### E4: Anamnesis Log Too Large

**Symptom**: Execution log grows to GB, disk fills

**Steps**:
1. Reduce event sampling (only syscalls, not every instruction)
2. Compress log with gzip
3. Stream to Wotan topic instead of file
4. Truncate after N events

### E5: Doom Doesn't Render

**Symptom**: PC advances, instructions climb, but no screen output

**Steps**:
1. Check PC: Has rendering code (R_Init) been reached?
2. Check screen buffer: Are pixels written to SCREEN_MAP?
3. Check palette: Is PLAYPAL loaded (768 bytes at 0x60000)?
4. Check bridge: Is WebSocket connection active?
5. If all OK: Rendering running but frame skipped (check framerate)

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Title | Agent | Parallelizable | Dependencies | Est. Time |
|-------|-------|-------|-----------------|--------------|-----------|
| 0 | Doctrine Verification | Coordinator | No | None | 45 min |
| 1 | PC Corruption Blocker | Coordinator | No | Phase 0 | 2 hrs |
| 2 | Extract UPC ISA | Coordinator | No | Phase 1 | 1.5 hrs |
| 3 | SPDX + SBOM + GPL | Coordinator | No | Phase 2 | 1 hr |
| 4-13 | Tools + Hardening | Developer | **Yes [P]** | Phase 3 | 8 hrs |
| 14-15 | Testing + Benchmarks | Developer | No | Phase 4-13 | 3 hrs |
| 16-17 | Documentation | Librarian | No | Phase 14-15 | 2 hrs |
| 18 | Demo + Examples | Developer | No | Phase 16-17 | 2 hrs |
| 19 | Public Release | Captain | No | Phase 18 | 2 hrs |

**Critical Path**: 0 → 1 → 2 → 3 → 14 → 15 → 16 → 17 → 18 → 19

**Total Estimated Duration**: 24-32 hours across 4-5 sessions

**Parallel Tracks**: Phases 4-13 can run in parallel after Phase 3, independent of critical path

---

## APPENDIX C: QUICK REFERENCE

### Memory Layout

```
0x00000000  ROM (1 MiB)      — MBC instructions (Array<u32>)
0x00060000  PALETTE (768 B)  — RGB color palette
0x00070000  SCREEN (64K)     — 320x200 framebuffer
0x01000000  RAM BASE         — .data + .sdata + .bss
0x01C00000  HEAP (26 MiB)    — JVM-style malloc arena
0x01BC0000  HEAP END         — heap boundary
0x01BF0000  HEAP_PTR         — isolated allocator state
0x01C00000  WAD (16 MiB)     — retail DOOM.WAD game data
0x03100000  STACK (grows↓)   — call frames + locals
```

### MBC Opcode Latencies (Approximate)

| Opcode | Latency | Notes |
|--------|---------|-------|
| NOP | <1 ns | Placeholder |
| ADD, SUB, MUL | <3 ns | Arithmetic |
| DIV | <10 ns | Slow path |
| LD, ST, LDH, STH, LDB, STB | <5 ns | Memory |
| CALL, RET | <10 ns | Return prediction |
| CALLR | <15 ns | Bounds check + taint check |
| JMP, JCC | <8 ns | Branch prediction |

### CLI Cheat Sheet

```bash
# Build
cargo build --release

# Test
cargo test
cargo test --lib
cargo test --test integration_test

# Bench
cargo bench

# Run Doom
./target/release/upc-sandbox run --doom-mbc doom.mbc

# Replay
./target/release/upc-sandbox replay --log execution.anamnesis

# Disassemble
./target/release/mbc-disasm bytecode.mbc > listing.mba

# Verify
cargo test verifier::
```

### eBPF Map Names

- `ROM_MAP` — Instruction space (262,144 entries, 1 MiB)
- `RAM_MAP` — Main memory (16,777,216 entries, 64 MiB)
- `SCREEN_MAP` — Framebuffer (64,000 entries, 320x200 pixels)
- `CPU_MAP` — Processor state (256 instances, MbcCpuState)
- `KBD_MAP` — Keyboard circular queue (8 slots)
- `STATS` — Counters (pc_faults, insn_count, rom_fault, etc.)

### Anamnesis Event Types

```rust
enum AnamnesisEvent {
    InstructionExecuted { pc, opcode, registers_before/after },
    MemoryRead { address, size, value },
    MemoryWrite { address, size, value },
    SyscallInvoked { syscall_id, args: [u32; 6] },
    SyscallReturned { syscall_id, return_value },
    Halted { reason, final_pc },
}
```

---

## PHASE COMPLETION CHECKLIST

- [ ] Phase 0: Doctrine + GPL verified
- [ ] Phase 1: PC corruption blocker resolved
- [ ] Phase 2: UPC ISA extracted (memory + MBC + verifier + Anamnesis)
- [ ] Phase 3: SPDX + SBOM + GPL boundary
- [ ] Phase 4: MBC verifier hardening (budget, taint, opcodes)
- [ ] Phase 5: mbc-asm/disasm/upc-replay tools
- [ ] Phase 6: Hardening baseline (3-layer design)
- [ ] Phase 7: Audit logging with HMAC chain
- [ ] Phase 8: Sealed cask reproducible build
- [ ] Phase 9-11: NIST + ISO + LICH compliance
- [ ] Phase 12: Integration test suite
- [ ] Phase 13: WASM compat shim (future feature)
- [ ] Phase 14-15: Testing + benchmarking (35 fps target)
- [ ] Phase 16-17: Public documentation (README, GOVERNANCE, CHANGELOG)
- [ ] Phase 18: Demo scripts (Doom + WASM stub)
- [ ] Phase 19: Public release (GitHub v0.1.0)

---

## STUCK REPORT TEMPLATE

```markdown
## STUCK REPORT — UPC Sandbox Battle Plan

**Progress**: X/104 steps completed (Y%)
**Stuck Steps**: [list of step numbers]
**Blocked Steps**: [steps that depend on stuck steps]
**Completed Despite Skips**: [steps that succeeded]

### Stuck Step Detail

**Step [N]**: [description]
- **Symptom**: [what went wrong]
- **Attempted**: [which debug steps were tried]
- **Time Burned**: [how long before skip triggered]
- **Downstream Impact**: Steps [A, B, C] blocked
- **Suggested Fix**: [agent's best guess at what needs human intervention]

### Recommended Intervention Order
1. Fix Step X first — unblocks N downstream steps
2. Fix Step Y next — unblocks M downstream steps
```

---

*S32 UPC Sandbox Battle Plan — Forged 2026-04-30*

*19 Phases. 104 Major Steps (347 substeps including all verification/commit gates).*

*From doctrine binding to public GPL-3.0 release.*

*"Production-ready untrusted code execution in 24-32 hours — free for everyone."*

---

## FREE TO USE. FREE TO SHARE. NO SELLING.

This battle plan is delivered to the community under GPL-3.0.

All code extracted via this plan is GPL-3.0.

Every improvement benefits everyone.

No paywalls. No licensing walls. No vendor lock-in.

**Take it. Use it. Improve it. Share it.**

**The Kingdom belongs to everyone.**

