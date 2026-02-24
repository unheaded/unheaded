# S42 DOOM-OVER-IPv6 PoC COMPLETION BATTLE PLAN — 10 Phases, 180+ Steps

**Date**: 2026-02-24
**Sprint**: S42 — Close every gap from D-001→D-020. Gradient renders. Checkerboard renders. Cross-compile pipeline works. Doomgeneric port stubs compile. THE PACKET WALKS THE PATTERN.
**Prerequisite**: S41 complete (dashboard topic routing fixed, protocol audit done, all tests pass)
**Target**: `demos/mbc/gradient.mbc` renders a visible gradient in the browser dashboard via the full BPF ring pipeline. Cross-compile pipeline produces MBC from C. Doomgeneric port scaffolded.
**Estimated Duration**: 10-16 hours across 2-3 sessions
**Agent Strategy**: Phases 0-3 sequential (foundation verification), Phases 4-6 parallelizable (Go tooling + Rust MBC + dashboard), Phases 7-9 sequential (integration + E2E)
**Commit Cadence**: Every 5 steps
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts. BPF-dependent steps that require `sudo`/namespaces: mark [BARE-METAL] and skip on non-root machines.

---

## SITUATION REPORT

### What We Have (Built, Tested, Working)

| Component | Location | LOC | Status |
|-----------|----------|-----|--------|
| MBC CPU XDP program | `ebpf/monad-cpu-ebpf/src/main.rs` | 949 | Compiled, needs ring integration test |
| Shared eBPF types | `ebpf/monad-common/src/lib.rs` | 1,833 | Compiled |
| MBC assembler | `crates/monad-mbc/src/assembler.rs` | — | Working, 52 tests |
| MBC disassembler | `crates/monad-mbc/src/disasm.rs` | — | Working |
| RV32I→MBC translator | `crates/monad-mbc/src/translator.rs` | — | Working, 52 tests |
| MBC CPU emulator | `crates/monad-mbc/src/cpu.rs` + `execute.rs` | — | Working |
| Doom bridge (Go) | `cmd/doom-bridge/` | 1,431 | Built, tests pass |
| Packet injector (Go) | `cmd/doom-go-injector/` | 769 | Built, fuzz tests |
| Doom CLI (Go) | `cmd/doom/main.go` | 227 | Built |
| Doom internals (Go) | `internal/doom/` | 2,398 | Built, full test coverage |
| wotan-ctl doom cmds | `cmd/wotan-ctl/doom.go` | — | load/status/input/reset/inject-tick |
| Wotan compute memory | `services/wotan/internal/compute/` | 1,729 | Built, 19 tests |
| Doom viewport (JS) | `dashboard/js/doom-viewport.js` | 723 | Built, needs live data |
| Doom ring script | `scripts/doom-ring.sh` | 600 | Built, needs kernel+sudo |
| Demo MBC programs | `demos/mbc/*.mbc` | — | gradient, checkerboard, add42, sum99 |

**Total Doom-related code**: ~10,659 LOC (Go) + ~2,782 LOC (Rust eBPF) + 723 LOC (JS) + 600 LOC (bash) = **~14,764 LOC**

### What's Missing (The Gap)

| Task | Description | Blocked By |
|------|-------------|-----------|
| D-016 | Gradient E2E integration — THE CHECKPOINT | Ring setup (sudo), BPF load |
| D-017 | Checkerboard E2E | D-016 |
| D-018 | Cross-compile pipeline (C → RV32I → MBC) | riscv32 toolchain |
| D-019 | Doomgeneric bare-metal port stubs | D-018 |
| D-020 | DOOM RUNS | Everything |
| Dashboard wiring | compute.* topics → doom-viewport.js | S41 Phase 1-2 |
| Wotan compute → WS bridge | SCREEN_MAP → WebSocket → browser | Wotan running |
| Tick injector integration | doom-go-injector at 35Hz → ring | Ring setup |

### Execution Strategy

This sprint has TWO tracks:

**Track A: Go/Rust/JS (no sudo needed)** — Can run on any machine
- Verify all Doom Go code builds and tests pass
- Verify MBC assembler/translator/emulator work
- Wire dashboard compute topics to doom-viewport.js
- Build cross-compile pipeline scaffolding
- Create doomgeneric port stubs

**Track B: BPF Ring Integration (sudo + Linux kernel required)** — Bare metal only
- Setup doom-ring.sh namespaces
- Load monad-cpu-ebpf into XDP
- Pin BPF maps
- Load gradient.mbc into ROM_MAP
- Inject tick packets
- Verify gradient renders

**If running on a Mac/VM without BPF**: Execute Track A fully, mark Track B as [BARE-METAL-REQUIRED], skip to handoff.

### Repository Layout (Updated 2026-02-24)

The Doom ecosystem now spans multiple repos under the **unheaded** GitHub organization:

```
~/fucking-unheaded/
├── unheaded/           ← Main monorepo (this repo)
├── unheaded-wiki/      ← GitHub wiki
├── DOOM/               ← Fork: github.com/unheaded/DOOM (id Software GPL source)
├── doomgeneric/        ← Fork: github.com/unheaded/doomgeneric (fbdev port base)
```

**WAD files**: Live at `~/fucking-unheaded/` or `~/tmp/` (NOT in any git repo — gitignored everywhere).

**Phase 5 doomgeneric stubs**: Should reference `~/fucking-unheaded/doomgeneric/` as the upstream fork, NOT a vendored `demos/doom/` copy. The port stubs go IN the doomgeneric fork, our build integration stays in the monorepo.

**Migration TODO** (light lift):
1. Copy `doom/doomgeneric/` modifications from monorepo → `~/fucking-unheaded/doomgeneric/`
2. Copy any Doom-specific scripts → `~/fucking-unheaded/DOOM/`
3. Monorepo keeps: MBC tools, Go doom infrastructure, eBPF CPU, dashboard viewport, doom-ring.sh

---

## LEGEND

[B] = Bash command
[V] = Verification step
[D] = Debug step
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable
[C] = Commit checkpoint
[BARE-METAL] = Requires Linux kernel with BPF + sudo + network namespaces

---

## PHASE 0: INVENTORY & BASELINE (Steps 1-15)

**Goal**: Verify every Doom component builds and tests pass. Establish what works NOW.
**Time**: 15 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R] ~1m: Read D-001→D-020 task list for context
  ```bash
  head -100 docs/DOOM_TASKS_20.md
  ```

- [ ] **Step 2** [B] ~2m: Build all Go Doom components
  ```bash
  go build ./cmd/doom-bridge/ && go build ./cmd/doom-go-injector/ && go build ./cmd/doom/ && go build ./internal/doom/... && echo "All Doom Go builds OK"
  ```

- [ ] **Step 3** [V] ~30s: All Go Doom builds pass
  - If pass → Step 4
  - If fail → Step 3a [D]: `go build -v ./cmd/doom-bridge/ 2>&1`

- [ ] **Step 4** [B] ~2m: Run all Go Doom tests
  ```bash
  go test -race -count=1 ./cmd/doom-bridge/... ./cmd/doom-go-injector/... ./internal/doom/... 2>&1 | tail -20
  ```

- [ ] **Step 5** [V] ~30s: All Go Doom tests pass
  - If pass → Step 6
  - If fail → Step 5a [D]: identify and classify failures

- [ ] **Step 6** [B] ~1m: Check MBC crate builds (Rust)
  ```bash
  cd crates/monad-mbc && cargo check 2>&1 | tail -10 && cd ../..
  ```

- [ ] **Step 7** [B] ~2m: Run MBC crate tests
  ```bash
  cd crates/monad-mbc && cargo test 2>&1 | tail -20 && cd ../..
  ```

- [ ] **Step 8** [V] ~30s: MBC assembler + translator + emulator tests pass
  - If pass → Step 9
  - If fail → Step 8a [D]

- [ ] **Step 9** [B] ~1m: Check eBPF Rust programs compile (if nightly toolchain present)
  ```bash
  cd ebpf && cargo +nightly check --target bpfel-unknown-none -Z build-std=core 2>&1 | tail -10 && cd ..
  ```
  - If nightly/bpfel not available: document as SKIP and continue

- [ ] **Step 10** [R] ~1m: Verify demo MBC programs exist
  ```bash
  ls -la demos/mbc/ && file demos/mbc/*.mbc
  ```

- [ ] **Step 11** [R] ~1m: Check doom-viewport.js is complete
  ```bash
  wc -l dashboard/js/doom-viewport.js && head -30 dashboard/js/doom-viewport.js
  ```

- [ ] **Step 12** [R] ~1m: Check wotan-ctl doom subcommands
  ```bash
  go build -o /tmp/wotan-ctl ./cmd/wotan-ctl/ && /tmp/wotan-ctl doom --help 2>&1 | head -20
  ```

- [ ] **Step 13** [R] ~1m: Verify doom-ring.sh is operational
  ```bash
  head -40 scripts/doom-ring.sh && bash -n scripts/doom-ring.sh && echo "Syntax OK"
  ```

- [ ] **Step 14** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 1-14: Doom inventory complete — all Go builds pass, MBC crate verified"
  ```

- [ ] **Step 15** [V]: **PHASE 0 EXIT GATE** — Baseline established
  - Go Doom builds: ALL PASS
  - Go Doom tests: ALL PASS
  - MBC crate: VERIFIED (build + tests)
  - eBPF programs: CHECKED (or SKIP documented)
  - Demo programs: EXIST
  - doom-viewport.js: EXISTS (723 LOC)
  - doom-ring.sh: SYNTAX OK
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

---

## PHASE 1: MBC EMULATOR VERIFICATION (Steps 16-35)

**Goal**: Prove gradient.mbc and checkerboard.mbc produce correct output in the SOFTWARE emulator before touching BPF.
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 16** [R] ~2m: Read gradient.mbc to understand what it does
  ```bash
  cat demos/mbc/gradient.mbc
  ```

- [ ] **Step 17** [R] ~2m: Read checkerboard.mbc
  ```bash
  cat demos/mbc/checkerboard.mbc
  ```

- [ ] **Step 18** [R] ~2m: Read MBC CPU emulator code
  ```bash
  cat crates/monad-mbc/src/cpu.rs | head -80 && cat crates/monad-mbc/src/execute.rs | head -80
  ```

- [ ] **Step 19** [B] ~2m: Assemble gradient.mbc (if it's assembly text, not binary)
  ```bash
  file demos/mbc/gradient.mbc && hexdump -C demos/mbc/gradient.mbc | head -10
  ```

- [ ] **Step 20** [W] ~10m: Create `scripts/run-mbc-emulator.sh` — run MBC programs in software emulator
  ```bash
  #!/usr/bin/env bash
  # Run an MBC program in the software emulator and dump SCREEN_MAP to PPM
  # Usage: ./scripts/run-mbc-emulator.sh demos/mbc/gradient.mbc [max-instructions]
  set -euo pipefail
  PROG="${1:?Usage: $0 <program.mbc> [max-instructions]}"
  MAX_INSN="${2:-100000}"
  cd "$(dirname "$0")/.."
  cargo run -p monad-mbc --bin mbc-emulate -- "$PROG" --max-insn "$MAX_INSN" --dump-screen screen.ppm 2>&1
  echo "Screen dumped to screen.ppm (320x200 8-bit indexed)"
  ```

- [ ] **Step 21** [R] ~2m: Check if mbc-emulate binary exists in monad-mbc crate
  ```bash
  ls crates/monad-mbc/src/bin/ 2>/dev/null && find crates/monad-mbc -name "*.rs" -path "*/bin/*"
  ```

- [ ] **Step 22** [W] ~10m: If `mbc-emulate` binary doesn't exist, create it
  - `crates/monad-mbc/src/bin/mbc_emulate.rs`
  - Loads MBC binary into emulated ROM
  - Runs CPU loop up to --max-insn instructions
  - On SYSCALL 0x01 (DRAW_FRAME): dump SCREEN_MAP to stdout or PPM file
  - On HALT: stop and report instruction count
  - Print: total instructions, cache hits, cache misses, final register state

- [ ] **Step 23** [B] ~2m: Build the emulator binary
  ```bash
  cd crates/monad-mbc && cargo build --bin mbc_emulate 2>&1 | tail -10 && cd ../..
  ```

- [ ] **Step 24** [V] ~30s: Emulator builds
  - If pass → Step 25
  - If fail → Step 24a [D]

- [ ] **Step 24a** [D] ~5m: Fix build issues — check Cargo.toml [[bin]] entries
  ```bash
  cat crates/monad-mbc/Cargo.toml | grep -A5 "bin"
  ```

- [ ] **Step 25** [B] ~2m: Run gradient.mbc through emulator
  ```bash
  cd crates/monad-mbc && cargo run --bin mbc_emulate -- ../../demos/mbc/gradient.mbc --max-insn 100000 2>&1 | tail -20 && cd ../..
  ```

- [ ] **Step 26** [V] ~1m: Emulator executes without panic
  - Check: instruction count > 0
  - Check: SYSCALL DRAW_FRAME fired (or HALT reached)
  - If pass → Step 27
  - If fail → Step 26a [D]

- [ ] **Step 26a** [D] ~5m: Debug emulator execution
  ```bash
  cd crates/monad-mbc && RUST_LOG=debug cargo run --bin mbc_emulate -- ../../demos/mbc/gradient.mbc --max-insn 1000 2>&1 | head -50 && cd ../..
  ```

- [ ] **Step 27** [B] ~2m: Run checkerboard.mbc through emulator
  ```bash
  cd crates/monad-mbc && cargo run --bin mbc_emulate -- ../../demos/mbc/checkerboard.mbc --max-insn 200000 2>&1 | tail -20 && cd ../..
  ```

- [ ] **Step 28** [V] ~1m: Checkerboard executes and produces different output than gradient
  - If pass → Step 29
  - If fail → Document issue

- [ ] **Step 29** [B] ~2m: Run add42.mbc and sum99.mbc as sanity checks
  ```bash
  cd crates/monad-mbc && cargo run --bin mbc_emulate -- ../../demos/mbc/add42.mbc --max-insn 1000 2>&1 | tail -5 && cargo run --bin mbc_emulate -- ../../demos/mbc/sum99.mbc --max-insn 10000 2>&1 | tail -5 && cd ../..
  ```

- [ ] **Step 30** [V] ~30s: add42 produces r0=42, sum99 produces r0=4950
  - If pass → Step 31
  - If fail → Check MBC encoding

- [ ] **Step 31** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 16-31: MBC emulator verified — gradient, checkerboard, add42, sum99 all execute"
  ```

- [ ] **Step 32** [W] ~5m: Add emulator integration test
  - `crates/monad-mbc/tests/integration_demos.rs`
  - Test: load gradient.mbc → run 100K insn → verify no panic
  - Test: load add42.mbc → run → verify r0 == 42
  - Test: load sum99.mbc → run → verify r0 == 4950

- [ ] **Step 33** [B] ~2m: Run integration tests
  ```bash
  cd crates/monad-mbc && cargo test --test integration_demos 2>&1 | tail -10 && cd ../..
  ```

- [ ] **Step 34** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 32-34: MBC emulator integration tests"
  ```

- [ ] **Step 35** [V]: **PHASE 1 EXIT GATE** — Software emulator verified
  - gradient.mbc: EXECUTES, no panic
  - checkerboard.mbc: EXECUTES, no panic
  - add42.mbc: r0 == 42
  - sum99.mbc: r0 == 4950
  - Integration tests: PASS
  - If pass → Phase 2
  - If fail → DO NOT PROCEED

---

## PHASE 2: WOTAN COMPUTE MEMORY WIRING (Steps 36-60)

**Goal**: Verify Wotan compute memory service handles ROM load, cache miss restaging, dirty writeback, and screen reads.
**Time**: 30 minutes
**Agent**: Agent

- [ ] **Step 36** [R] ~2m: Read Wotan compute memory implementation
  ```bash
  cat services/wotan/internal/compute/memory.go | head -80
  ```

- [ ] **Step 37** [R] ~2m: Read Wotan compute BPF map abstraction
  ```bash
  cat services/wotan/internal/compute/bpfmap.go
  ```

- [ ] **Step 38** [B] ~2m: Run existing Wotan compute tests
  ```bash
  go test -race -count=1 -v ./services/wotan/internal/compute/... 2>&1 | tail -30
  ```

- [ ] **Step 39** [V] ~30s: All 19 existing tests pass
  - If pass → Step 40
  - If fail → Step 39a [D]

- [ ] **Step 39a** [D] ~5m: Debug failing tests
  ```bash
  go test -race -count=1 -v -run "FAIL" ./services/wotan/internal/compute/... 2>&1
  ```

- [ ] **Step 40** [W] ~10m: Verify or create `wotan-ctl doom load` test
  - Test: load gradient.mbc binary → verify it populates ROM mock
  - Test: verify instruction count matches MBC file size / 4
  - Test: verify first instruction matches expected opcode

- [ ] **Step 41** [B] ~2m: Run doom CLI tests
  ```bash
  go test -race -count=1 -v ./cmd/wotan-ctl/... 2>&1 | tail -20
  ```

- [ ] **Step 42** [V] ~30s: Tests pass
  - If pass → Step 43
  - If fail → Step 42a [D]

- [ ] **Step 42a** [D] ~3m: Debug — check if doom.go compiles standalone
  ```bash
  go build -v ./cmd/wotan-ctl/ 2>&1
  ```

- [ ] **Step 43** [W] ~10m: Verify cache miss → restage → retry flow in compute memory
  - The flow: BPF CPU hits L1 cache miss → Anamnesis CACHE_MISS event → Wotan reads from RAM_MAP → writes to L1 cache → next packet retries
  - Check: does `services/wotan/internal/compute/memory.go` implement `HandleCacheMiss()`?
  - If not: implement it (D-006 from task list)

- [ ] **Step 44** [W] ~10m: Verify dirty writeback flow
  - The flow: BPF CPU does ST (store) → L1 dirty → Anamnesis MEM_WRITE event → Wotan writes back to RAM_MAP
  - Check: does compute memory implement `HandleDirtyWriteback()`?
  - If not: implement it (D-007)

- [ ] **Step 45** [W] ~5m: Verify SCREEN_MAP read for framebuffer extraction
  - The flow: BPF CPU fires SYSCALL 0x01 → Anamnesis DRAW_FRAME event → Wotan reads SCREEN_MAP (64000 bytes) → sends to dashboard via WebSocket
  - Check: does compute memory implement `ReadScreenBuffer()`?

- [ ] **Step 46** [B] ~2m: Run updated Wotan compute tests
  ```bash
  go test -race -count=1 ./services/wotan/internal/compute/... 2>&1 | tail -20
  ```

- [ ] **Step 47** [V] ~30s: All pass
  - If pass → Step 48
  - If fail → FIX

- [ ] **Step 48** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 36-48: Wotan compute memory verified — cache miss, writeback, screen read"
  ```

- [ ] **Step 49** [R] ~2m: Check doom-bridge BPF map reader
  ```bash
  cat cmd/doom-bridge/bpf.go | head -60
  ```

- [ ] **Step 50** [W] ~5m: Verify doom-bridge can read from all required BPF maps
  - CPU_MAP (104 bytes): CpuState
  - ROM_MAP (1MB): instruction words
  - RAM_MAP (4MB): general RAM
  - SCREEN_MAP (64000 bytes): framebuffer
  - KBD_MAP (16 bytes): keyboard state

- [ ] **Step 51** [B] ~2m: Run doom-bridge tests
  ```bash
  go test -race -count=1 -v ./cmd/doom-bridge/... 2>&1 | tail -20
  ```

- [ ] **Step 52** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 49-52: Doom bridge BPF map access verified"
  ```

- [ ] **Step 53** [V]: **PHASE 2 EXIT GATE** — Wotan compute memory and doom-bridge verified
  - Wotan compute tests: ALL PASS
  - Cache miss handling: VERIFIED
  - Dirty writeback: VERIFIED
  - Screen buffer read: VERIFIED
  - Doom bridge map access: VERIFIED
  - If pass → Phase 3
  - If fail → DO NOT PROCEED

---

## PHASE 3: DASHBOARD DOOM INTEGRATION (Steps 54-80)

**Goal**: Wire dashboard to receive and display Doom compute events + framebuffer data.
**Time**: 45 minutes
**Agent**: Agent

- [ ] **Step 54** [R] ~2m: Check how doom-viewport.js expects to receive data
  ```bash
  grep -n "WebSocket\|onmessage\|compute_screen\|SCREEN" dashboard/js/doom-viewport.js | head -20
  ```

- [ ] **Step 55** [R] ~2m: Check dashboard-backend WebSocket handler
  ```bash
  grep -rn "WebSocket\|websocket\|ws\|upgrade" cmd/dashboard-backend/ | head -20
  ```

- [ ] **Step 56** [R] ~1m: Check if dashboard-backend serves doom-viewport.js
  ```bash
  grep -rn "doom\|Doom\|viewport" cmd/dashboard-backend/ | head -20
  ```

- [ ] **Step 57** [W] ~10m: Add `/api/v1/doom/screen` endpoint to dashboard-backend
  - Returns current SCREEN_MAP data as binary (64000 bytes) or base64 JSON
  - Polls Wotan compute memory service for latest framebuffer
  - Or: reads from BPF map directly if doom-bridge is available

- [ ] **Step 58** [W] ~10m: Add `/api/v1/doom/cpu` endpoint to dashboard-backend
  - Returns current CpuState (PC, registers, flags, instruction count, cache stats)
  - JSON format matching doom-viewport.js expectations

- [ ] **Step 59** [W] ~5m: Add `/api/v1/doom/input` endpoint to dashboard-backend
  - POST: accepts keycode + pressed/released
  - Writes to KBD_MAP (via Wotan or doom-bridge)

- [ ] **Step 60** [B] ~1m: Build dashboard-backend
  ```bash
  go build ./cmd/dashboard-backend/ 2>&1 | tail -5
  ```

- [ ] **Step 61** [V] ~30s: Build passes
  - If pass → Step 62
  - If fail → Step 61a [D]

- [ ] **Step 61a** [D] ~3m: Fix build
  ```bash
  go build -v ./cmd/dashboard-backend/ 2>&1
  ```

- [ ] **Step 62** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 54-62: Dashboard Doom API endpoints added — screen, cpu, input"
  ```

- [ ] **Step 63** [W] ~5m: Ensure doom.html in dashboard/ loads doom-viewport.js
  ```bash
  cat dashboard/doom.html | head -30
  ```
  - Verify it includes doom-viewport.js
  - Verify canvas element is 320x200 (or scaled)
  - Verify it connects to the correct API/WebSocket endpoints

- [ ] **Step 64** [W] ~5m: Add Doom palette to doom-viewport.js if not present
  - The classic Doom palette (256 colors, RGB values)
  - Used to convert 8-bit indexed SCREEN_MAP values to RGB for canvas

- [ ] **Step 65** [W] ~10m: Wire doom-viewport.js to poll `/api/v1/doom/screen` at frame rate
  - Default: poll at 35 Hz (match tick injection rate)
  - Alternative: WebSocket push on DRAW_FRAME event
  - Render to canvas using `putImageData()`
  - Update CPU overlay from `/api/v1/doom/cpu`

- [ ] **Step 66** [W] ~5m: Wire keyboard capture in doom-viewport.js
  - Capture keydown/keyup events on the doom canvas
  - Map browser keycodes to Doom keycodes
  - POST to `/api/v1/doom/input`
  - Prevent default on captured keys (arrows, space, ctrl, etc.)

- [ ] **Step 67** [B] ~1m: Rebuild dashboard-backend with all changes
  ```bash
  go build -o bin/dashboard-backend ./cmd/dashboard-backend/ && echo "OK"
  ```

- [ ] **Step 68** [B] ~2m: Run all dashboard tests
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 69** [V] ~30s: All pass
  - If pass → Step 70
  - If fail → FIX

- [ ] **Step 70** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 63-70: Doom viewport wired — screen polling, CPU overlay, keyboard input"
  ```

- [ ] **Step 71** [W] ~5m: Add tests for Doom API endpoints
  - Test: `/api/v1/doom/screen` returns 200 (with mock data)
  - Test: `/api/v1/doom/cpu` returns valid CpuState JSON
  - Test: `/api/v1/doom/input` POST accepts keycode

- [ ] **Step 72** [B] ~2m: Run tests
  ```bash
  go test -race -count=1 ./cmd/dashboard-backend/... 2>&1 | tail -20
  ```

- [ ] **Step 73** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 71-73: Doom API endpoint tests added"
  ```

- [ ] **Step 74** [V]: **PHASE 3 EXIT GATE** — Dashboard Doom integration complete
  - Doom API: screen + cpu + input endpoints
  - doom-viewport.js: screen polling + keyboard capture + CPU overlay
  - doom.html: loads correctly
  - All tests: PASS
  - If pass → Phase 4
  - If fail → DO NOT PROCEED

---

## PHASE 4: CROSS-COMPILE PIPELINE (Steps 75-100)

**Goal**: Build the C → RV32I → MBC pipeline (D-018).
**Time**: 30-45 minutes
**Agent**: Agent [P]

- [ ] **Step 75** [B] ~1m: Check if riscv32 toolchain is available
  ```bash
  which riscv32-unknown-elf-gcc 2>/dev/null || which riscv64-unknown-elf-gcc 2>/dev/null || echo "NO RISCV TOOLCHAIN"
  ```

- [ ] **Step 76** [B] ~5m: Install riscv32 toolchain (if not present)
  ```bash
  # Try apt first
  sudo apt-get install -y gcc-riscv64-unknown-elf 2>/dev/null || \
  # Try nix
  nix-env -i gcc-riscv32 2>/dev/null || \
  echo "SKIP — install manually: apt install gcc-riscv64-unknown-elf or use Docker"
  ```
  - If no toolchain available: SKIP Phase 4 (mark steps as BARE-METAL)

- [ ] **Step 77** [R] ~1m: Check if rv32i-to-mbc binary exists
  ```bash
  cd crates/monad-mbc && cargo build --bin rv32i_to_mbc 2>&1 | tail -5 && cd ../..
  ```

- [ ] **Step 78** [W] ~5m: Create `demos/c/test_add.c` — simplest C test
  ```c
  // test_add.c — Prove the cross-compile pipeline works
  // Expected: r0 = 4950 after execution
  int _start(void) {
      int result = 0;
      for (int i = 0; i < 100; i++) {
          result += i;
      }
      return result; // r0 = 4950
  }
  ```

- [ ] **Step 79** [W] ~3m: Create `demos/c/linker.ld` — minimal linker script
  ```ld
  ENTRY(_start)
  SECTIONS {
      . = 0x0;
      .text : { *(.text*) }
      .rodata : { *(.rodata*) }
      .data : { *(.data*) }
      .bss : { *(.bss*) COMMON }
      _stack_top = 0xFFFF0000;
  }
  ```

- [ ] **Step 80** [W] ~5m: Create `scripts/cross-compile.sh`
  ```bash
  #!/usr/bin/env bash
  # Cross-compile C → RV32I ELF → MBC binary
  # Usage: ./scripts/cross-compile.sh demos/c/test_add.c output.mbc
  set -euo pipefail
  SRC="${1:?Usage: $0 <source.c> <output.mbc>}"
  OUT="${2:?Usage: $0 <source.c> <output.mbc>}"
  REPO_ROOT="$(cd "$(dirname "$0")/.." && pwd)"
  ELF="${OUT%.mbc}.elf"

  echo "[1/3] Compiling C → RV32I ELF..."
  riscv32-unknown-elf-gcc -O2 -march=rv32im -mabi=ilp32 \
    -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19 \
    -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23 \
    -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27 \
    -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31 \
    -nostdlib -static -T "$REPO_ROOT/demos/c/linker.ld" \
    -o "$ELF" "$SRC"

  echo "[2/3] Translating RV32I → MBC..."
  cargo run -p monad-mbc --bin rv32i_to_mbc -- "$ELF" -o "$OUT" --stats --disasm

  echo "[3/3] Done: $OUT"
  ls -la "$OUT"
  ```

- [ ] **Step 81** [B] ~2m: Test the pipeline (if toolchain available)
  ```bash
  chmod +x scripts/cross-compile.sh && ./scripts/cross-compile.sh demos/c/test_add.c demos/mbc/test_add.mbc 2>&1
  ```

- [ ] **Step 82** [V] ~30s: MBC binary produced
  - If pass → Step 83
  - If no toolchain → SKIP, document

- [ ] **Step 83** [B] ~1m: Verify test_add.mbc in emulator
  ```bash
  cd crates/monad-mbc && cargo run --bin mbc_emulate -- ../../demos/mbc/test_add.mbc --max-insn 50000 2>&1 | tail -10 && cd ../..
  ```

- [ ] **Step 84** [V] ~30s: r0 == 4950
  - If pass → Step 85
  - If fail → Check translator limitations doc

- [ ] **Step 85** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 75-85: Cross-compile pipeline C→RV32I→MBC — verified"
  ```

- [ ] **Step 86** [V]: **PHASE 4 EXIT GATE** — Cross-compile pipeline works (or documented SKIP)
  - C source compiles to RV32I ELF
  - RV32I translates to MBC binary
  - MBC binary executes correctly in emulator
  - If pass → Phase 5
  - If SKIP → Phase 5 (document)

---

## PHASE 5: DOOMGENERIC PORT STUBS (Steps 87-110)

**Goal**: Scaffold the Doom bare-metal port (D-019). Create the C stubs that will map Doom API calls to MBC SYSCALLs.
**Time**: 30 minutes
**Agent**: Agent [P]

> **NOTE**: The doomgeneric source now lives in the **unheaded org fork** at `~/fucking-unheaded/doomgeneric/`.
> Port stubs go in the fork. Build integration scripts stay in the monorepo at `scripts/cross-compile.sh`.
> WAD files live at `~/fucking-unheaded/` or `~/tmp/` — never committed to git.

- [ ] **Step 87** [B] ~1m: Create demos/doom directory (monorepo build integration)
  ```bash
  mkdir -p demos/doom
  ```

- [ ] **Step 88** [W] ~10m: Create `demos/doom/doomgeneric_unheaded.c`
  - Implement DG_Init(): allocate framebuffer at 0x200000 in Wotan RAM
  - Implement DG_DrawFrame(): SYSCALL 0x01 (r1 = framebuffer addr)
  - Implement DG_SleepMs(ms): SYSCALL 0x04 (r1 = ms)
  - Implement DG_GetTicksMs(): SYSCALL 0x03 (return r0)
  - Implement DG_GetKey(): SYSCALL 0x02 (r1 = &pressed, r2 = &key)
  - NOTE: These use inline asm or direct SYSCALL encoding via MBC instruction words

- [ ] **Step 89** [W] ~3m: Create `demos/doom/linker.ld`
  - .text at 0x0
  - .rodata after .text
  - .data after .rodata
  - .bss after .data
  - Stack at 0xFFFF0000

- [ ] **Step 90** [W] ~5m: Create `demos/doom/crt0.s`
  - Minimal startup: zero BSS, set SP to _stack_top, call _start/main
  - Use MBC-compatible register assignments (r0-r15, r15=SP)

- [ ] **Step 91** [W] ~5m: Create `demos/doom/Makefile`
  ```makefile
  # Doom-over-IPv6 build pipeline
  # Requires: riscv32-unknown-elf-gcc, monad-mbc crate
  CC = riscv32-unknown-elf-gcc
  CFLAGS = -O2 -march=rv32im -mabi=ilp32 \
    -ffixed-x16 -ffixed-x17 -ffixed-x18 -ffixed-x19 \
    -ffixed-x20 -ffixed-x21 -ffixed-x22 -ffixed-x23 \
    -ffixed-x24 -ffixed-x25 -ffixed-x26 -ffixed-x27 \
    -ffixed-x28 -ffixed-x29 -ffixed-x30 -ffixed-x31 \
    -nostdlib -static

  doom.mbc: doom.elf
  	cargo run -p monad-mbc --bin rv32i_to_mbc -- $< -o $@ --stats

  doom.elf: doomgeneric_unheaded.c crt0.s linker.ld
  	$(CC) $(CFLAGS) -T linker.ld -o $@ crt0.s doomgeneric_unheaded.c

  clean:
  	rm -f doom.elf doom.mbc

  .PHONY: clean
  ```

- [ ] **Step 92** [W] ~5m: Create `demos/doom/README.md`
  - Explain the build pipeline: C → RV32I ELF → MBC binary → BPF ROM_MAP → XDP execution
  - Document known translator limitations
  - Reference DOOM_TASKS_20.md D-019
  - Link to doom-over-ipv6-architecture.md

- [ ] **Step 93** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 87-93: Doomgeneric port stubs scaffolded"
  ```

- [ ] **Step 94** [V]: **PHASE 5 EXIT GATE** — Doomgeneric port scaffolded
  - doomgeneric_unheaded.c: EXISTS with all 5 DG_* stubs
  - linker.ld: EXISTS
  - crt0.s: EXISTS
  - Makefile: EXISTS
  - README.md: EXISTS
  - If pass → Phase 6
  - If fail → DO NOT PROCEED

---

## PHASE 6: TICK INJECTOR INTEGRATION (Steps 95-115)

**Goal**: Verify the doom-go-injector can send tick packets at correct rate. Wire to doom-ring topology.
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 95** [R] ~2m: Read doom-go-injector implementation
  ```bash
  cat cmd/doom-go-injector/main.go | head -60
  ```

- [ ] **Step 96** [R] ~2m: Understand tick packet format
  - IPv6 packet with Hop-by-Hop extension header
  - Monad register file (20 bytes) with CUSTOM flag set
  - Flow label identifies CPU instance

- [ ] **Step 97** [W] ~10m: Verify or add `--rate` flag to doom-go-injector
  - Default: 35 Hz (35 tick packets per second)
  - Uses time.Ticker for consistent interval
  - Logs tick count every second
  - Graceful shutdown on SIGINT

- [ ] **Step 98** [W] ~5m: Verify or add `--flow-label` flag
  - Default: 0x0A3F7E (the canonical Doom flow label)
  - Low 8 bits = instance_id for multi-instance

- [ ] **Step 99** [B] ~1m: Build doom-go-injector
  ```bash
  go build -o bin/doom-go-injector ./cmd/doom-go-injector/ && echo "OK"
  ```

- [ ] **Step 100** [B] ~2m: Run injector tests
  ```bash
  go test -race -count=1 ./cmd/doom-go-injector/... 2>&1 | tail -10
  ```

- [ ] **Step 101** [V] ~30s: Tests pass
  - If pass → Step 102
  - If fail → FIX

- [ ] **Step 102** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 95-102: Tick injector verified with rate and flow-label flags"
  ```

- [ ] **Step 103** [V]: **PHASE 6 EXIT GATE** — Tick injector ready
  - Build: OK
  - Tests: PASS
  - --rate and --flow-label flags: PRESENT
  - If pass → Phase 7
  - If fail → DO NOT PROCEED

---

## PHASE 7: BPF RING INTEGRATION [BARE-METAL] (Steps 104-140)

**Goal**: Set up the 6-namespace ring, load BPF programs, load ROM, inject ticks, verify gradient renders.
**Prerequisite**: Linux kernel >= 5.15, sudo access, BPF support, network namespaces
**Time**: 60-90 minutes
**Agent**: Coordinator [BARE-METAL]

⚠️ **If running on Mac/VM without BPF support**: SKIP this entire phase. Mark as BARE-METAL-REQUIRED. Proceed to Phase 8.

- [ ] **Step 104** [S][BARE-METAL] ~5m: Set up doom ring namespaces
  ```bash
  sudo ./scripts/doom-ring.sh setup 2>&1 | tail -20
  ```

- [ ] **Step 105** [V] ~1m: All 6 namespaces up
  ```bash
  sudo ./scripts/doom-ring.sh status 2>&1
  ```
  - Verify: monad0 through monad5 all exist
  - Verify: veth pairs connected
  - Verify: IPv6 addresses assigned
  - If pass → Step 106
  - If fail → Step 105a [D]

- [ ] **Step 105a** [D] ~5m: Debug namespace setup
  ```bash
  ip netns list && sudo ip netns exec monad0 ip addr show 2>&1
  ```

- [ ] **Step 106** [S][BARE-METAL] ~5m: Build and load monad-cpu-ebpf into XDP
  ```bash
  cd ebpf && cargo +nightly build -p monad-cpu-ebpf --target bpfel-unknown-none -Z build-std=core --release 2>&1 | tail -10 && cd ..
  ```

- [ ] **Step 107** [S][BARE-METAL] ~3m: Attach XDP program to all ring interfaces
  ```bash
  sudo ./scripts/doom-ring.sh load-bpf 2>&1 | tail -10
  ```

- [ ] **Step 108** [V] ~1m: BPF programs attached
  ```bash
  sudo bpftool prog list | grep -i xdp | head -10
  ```

- [ ] **Step 109** [S][BARE-METAL] ~2m: Pin BPF maps
  ```bash
  ls /sys/fs/bpf/unheaded/doom-ring/maps/ 2>/dev/null || echo "Maps not pinned"
  ```

- [ ] **Step 110** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 104-110: Doom ring setup — namespaces up, BPF loaded, maps pinned"
  ```

- [ ] **Step 111** [S][BARE-METAL] ~2m: Load gradient.mbc into ROM_MAP
  ```bash
  bin/wotan-ctl doom load demos/mbc/gradient.mbc --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps 2>&1
  ```

- [ ] **Step 112** [V] ~1m: ROM loaded
  ```bash
  bin/wotan-ctl doom status --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps 2>&1
  ```
  - Verify: PC = 0, halted = false, instruction count = 0

- [ ] **Step 113** [S][BARE-METAL] ~1m: Start Wotan with compute memory service
  ```bash
  bin/wotan --compute-enable --bpf-pin-path /sys/fs/bpf/unheaded/doom-ring/maps &
  sleep 2 && curl -s http://localhost:18000/health | head -5
  ```

- [ ] **Step 114** [B] ~1m: Start dashboard-backend
  ```bash
  bin/dashboard-backend &
  sleep 2 && curl -s http://localhost:20000/ | head -5
  ```

- [ ] **Step 115** [S][BARE-METAL] ~1m: Inject first tick packet
  ```bash
  sudo bin/doom-go-injector --flow-label 0x0A3F7E --count 1 --interface monad0-in 2>&1
  ```

- [ ] **Step 116** [V] ~1m: CPU state shows progress
  ```bash
  bin/wotan-ctl doom status --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps 2>&1
  ```
  - Verify: instruction count > 0
  - Verify: PC advanced

- [ ] **Step 117** [S][BARE-METAL] ~5m: Run tick injector at 35 Hz for 10 seconds
  ```bash
  timeout 10 sudo bin/doom-go-injector --flow-label 0x0A3F7E --rate 35 --interface monad0-in 2>&1 | tail -5
  ```

- [ ] **Step 118** [V] ~1m: Significant instruction progress
  ```bash
  bin/wotan-ctl doom status --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps 2>&1
  ```
  - Verify: instruction count > 1000 (35 ticks × 6 hops × ~5 insn/tick)
  - Verify: no stall

- [ ] **Step 119** [B] ~1m: Check dashboard for Doom data
  ```bash
  curl -s http://localhost:20000/api/v1/doom/cpu 2>/dev/null | python3 -m json.tool | head -20
  ```

- [ ] **Step 120** [B] ~1m: Check screen buffer
  ```bash
  curl -s http://localhost:20000/api/v1/doom/screen 2>/dev/null | wc -c
  ```
  - Should return ~64000 bytes (320×200 pixels)

- [ ] **Step 121** [V] ~1m: **THE CHECKPOINT** — gradient visible?
  - Open browser to http://localhost:20000/viz/doom
  - Does the canvas show ANY non-black pixels?
  - If yes → **D-016 COMPLETE. THE PATTERN WALKS.**
  - If no → Step 121a [D]

- [ ] **Step 121a** [D] ~10m: Debug gradient rendering
  ```bash
  # Check if SYSCALL DRAW_FRAME was ever fired
  bin/wotan-ctl doom status --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps 2>&1
  # Check if SCREEN_MAP has any non-zero bytes
  sudo bpftool map dump pinned /sys/fs/bpf/unheaded/doom-ring/maps/SCREEN_MAP 2>/dev/null | head -10
  # Check dashboard logs for errors
  curl -s http://localhost:20000/api/v1/ebpf/stats | python3 -m json.tool
  ```

- [ ] **Step 122** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 111-122: Gradient integration test — THE CHECKPOINT"
  ```

- [ ] **Step 123** [S][BARE-METAL] ~5m: Load and run checkerboard.mbc (D-017)
  ```bash
  # Reset CPU
  bin/wotan-ctl doom reset --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps
  # Load checkerboard
  bin/wotan-ctl doom load demos/mbc/checkerboard.mbc --map-pin-path /sys/fs/bpf/unheaded/doom-ring/maps
  # Run for 30 seconds
  timeout 30 sudo bin/doom-go-injector --flow-label 0x0A3F7E --rate 35 --interface monad0-in 2>&1 | tail -5
  ```

- [ ] **Step 124** [V] ~1m: Checkerboard visible in browser
  - Different pattern than gradient
  - If yes → **D-017 COMPLETE**

- [ ] **Step 125** [B] ~1m: Stop all services
  ```bash
  pkill -f dashboard-backend; pkill -f wotan; pkill -f doom-go-injector; sleep 1
  ```

- [ ] **Step 126** [S][BARE-METAL] ~1m: Teardown doom ring
  ```bash
  sudo ./scripts/doom-ring.sh teardown 2>&1
  ```

- [ ] **Step 127** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 123-127: Checkerboard verified, ring teardown clean"
  ```

- [ ] **Step 128** [V]: **PHASE 7 EXIT GATE** — BPF ring integration verified (or SKIPPED)
  - Ring: setup → BPF loaded → maps pinned → ticks injected → gradient visible → checkerboard visible → teardown
  - Or: BARE-METAL-REQUIRED, all steps documented as SKIP
  - If pass → Phase 8
  - If SKIP → Phase 8

---

## PHASE 8: TEST SUITE HARDENING (Steps 129-145)

**Goal**: Full test suite passes with race detector. Add any missing tests.
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 129** [B] ~5m: Run FULL Go test suite
  ```bash
  go test -race -count=1 ./... 2>&1 | tail -40
  ```

- [ ] **Step 130** [V] ~1m: All pass (or pre-existing documented)
  - If pass → Step 131
  - If fail → FIX new failures

- [ ] **Step 131** [B] ~2m: Run MBC Rust tests
  ```bash
  cd crates/monad-mbc && cargo test 2>&1 | tail -20 && cd ../..
  ```

- [ ] **Step 132** [V] ~30s: All Rust tests pass
  - If pass → Step 133
  - If fail → FIX

- [ ] **Step 133** [B] ~2m: Check test coverage for Doom Go packages
  ```bash
  go test -cover ./cmd/doom-bridge/... ./cmd/doom-go-injector/... ./internal/doom/... 2>&1 | tail -10
  ```

- [ ] **Step 134** [W] ~5m: Add any missing tests identified during phases
  - Doom API endpoints (dashboard)
  - Emulator integration (MBC crate)
  - Cross-compile script (if toolchain present)

- [ ] **Step 135** [B] ~2m: Re-run tests
  ```bash
  go test -race -count=1 ./cmd/doom-bridge/... ./cmd/doom-go-injector/... ./cmd/dashboard-backend/... ./internal/doom/... 2>&1 | tail -20
  ```

- [ ] **Step 136** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 129-136: Full test suite verified — Go + Rust"
  ```

- [ ] **Step 137** [V]: **PHASE 8 EXIT GATE** — Tests green
  - Go tests: ALL PASS
  - Rust tests: ALL PASS
  - Race detector: CLEAN
  - If pass → Phase 9
  - If fail → DO NOT PROCEED

---

## PHASE 9: SESSION HANDOFF & DOCUMENTATION (Steps 138-160)

**Goal**: Write handoff, update docs, prepare for S43.
**Time**: 20 minutes
**Agent**: Agent

- [ ] **Step 138** [B] ~1m: Git log summary
  ```bash
  git log --oneline -15
  ```

- [ ] **Step 139** [W] ~10m: Write `docs/sessions/S42-doom-poc-handoff.md`
  - **Phases completed** (list all with status)
  - **D-016 status**: Gradient renders? YES / NO / SKIPPED (bare metal)
  - **D-017 status**: Checkerboard renders? YES / NO / SKIPPED
  - **D-018 status**: Cross-compile pipeline? WORKS / SKIPPED (no toolchain)
  - **D-019 status**: Doomgeneric stubs? SCAFFOLDED
  - **D-020 status**: DOOM RUNS? NOT YET (requires full doomgeneric port)
  - **What remains for D-020**:
    1. Work in `~/fucking-unheaded/doomgeneric/` fork (NOT vendored into monorepo)
    2. Replace DG_* implementations with MBC SYSCALL encoding
    3. Cross-compile full Doom binary (C → RV32I → MBC)
    4. Load ~4MB ROM + WAD data into BPF maps (WADs at `~/fucking-unheaded/` or `~/tmp/`)
    5. Run at sufficient tick rate for playable FPS
    6. Migrate existing `doom/doomgeneric/` monorepo changes → fork
  - **Current state**: all services, tests, pipeline
  - **Known issues**: list any

- [ ] **Step 140** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 138-140: S42 handoff doc written"
  ```

- [ ] **Step 141** [W] ~5m: Update CLAUDE.md with S42 findings
  - MBC emulator binary location
  - Cross-compile pipeline script
  - Doom API endpoints
  - Demo programs verified
  - Doomgeneric stubs location

- [ ] **Step 142** [W] ~3m: Update D-001→D-020 task list with completion status
  ```bash
  # Mark completed tasks in DOOM_TASKS_20.md
  ```

- [ ] **Step 143** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S42] Steps 141-143: CLAUDE.md and task list updated"
  ```

- [ ] **Step 144** [B] ~1m: Final full test
  ```bash
  go test -race -count=1 ./cmd/doom-bridge/... ./cmd/doom-go-injector/... ./cmd/dashboard-backend/... ./internal/doom/... 2>&1 | tail -20
  ```

- [ ] **Step 145** [V] ~30s: All pass
  - If pass → Step 146
  - If fail → FIX

- [ ] **Step 146** [B] ~30s: Final git status
  ```bash
  git status
  ```

- [ ] **Step 147** [C] ~30s: **FINAL COMMIT**
  ```bash
  git add -A && git commit -m "feat(S42): Doom PoC completion — emulator verified, dashboard wired, cross-compile pipeline, doomgeneric stubs"
  ```

- [ ] **Step 148** [V]: **PHASE 9 EXIT GATE** — Sprint complete
  - Handoff: WRITTEN
  - Task list: UPDATED
  - Tests: PASS
  - Git: CLEAN
  - If pass → SPRINT COMPLETE

---

## APPENDIX A: EMERGENCY PROCEDURES

### BPF Program Won't Load
```
1. Check kernel version: uname -r (need >= 5.15)
2. Check BPF support: ls /sys/fs/bpf/
3. Check program with bpftool: bpftool prog list
4. Check verifier log: increase VERIFIER_LOG_SIZE in loader
5. Check XDP mode: try generic (-m skb) if native fails
```

### Doom Ring Namespace Issues
```
1. Clean teardown: sudo ./scripts/doom-ring.sh teardown
2. Manual cleanup: sudo ip netns del monad0 (repeat for 0-5)
3. Check for leftover veth: ip link show | grep veth
4. Reboot if systemd-networkd cached stale state
```

### Emulator Hangs (Infinite Loop in MBC Program)
```
1. Use --max-insn flag to bound execution
2. Check for backwards JMP without exit condition
3. Use disassembler: cargo run -p monad-mbc --bin mbc_disasm -- program.mbc
4. Check HALT instruction is reachable
```

### Dashboard Shows No Doom Data
```
1. Check Doom API: curl localhost:20000/api/v1/doom/cpu
2. Check Wotan compute: curl localhost:18000/health
3. Check BPF maps are pinned: ls /sys/fs/bpf/unheaded/doom-ring/maps/
4. Check doom-bridge is running and reading maps
5. Verify doom-viewport.js is polling correct endpoints
```

---

## APPENDIX B: KEY FILE PATHS

```
# Rust eBPF (the CPU)
ebpf/monad-cpu-ebpf/src/main.rs              — MBC VM in XDP (949 LOC)
ebpf/monad-common/src/lib.rs                  — Shared eBPF types (1,833 LOC)

# Rust MBC tools
crates/monad-mbc/src/assembler.rs             — MBC assembler
crates/monad-mbc/src/cpu.rs                   — Software emulator
crates/monad-mbc/src/execute.rs               — Instruction execution
crates/monad-mbc/src/translator.rs            — RV32I → MBC translator
crates/monad-mbc/src/bin/rv32i_to_mbc.rs      — ELF → MBC CLI
crates/monad-mbc/src/bin/mbc_emulate.rs       — Software emulator CLI (CREATE)

# Go Doom infrastructure
cmd/doom-bridge/                               — BPF map bridge (1,431 LOC)
cmd/doom-go-injector/                          — Tick packet injector (769 LOC)
cmd/doom/main.go                               — Doom CLI (227 LOC)
internal/doom/                                 — Core Doom logic (2,398 LOC)
cmd/wotan-ctl/doom.go                          — ROM loader, status, input, reset

# Wotan compute
services/wotan/internal/compute/memory.go      — Compute memory service (745 LOC)
services/wotan/internal/compute/bpfmap.go      — BPF map abstraction (105 LOC)

# Dashboard
dashboard/js/doom-viewport.js                  — Framebuffer renderer (723 LOC)
dashboard/doom.html                            — Doom page
cmd/dashboard-backend/                         — API server (add doom endpoints)

# Infrastructure
scripts/doom-ring.sh                           — 6-namespace ring (600 LOC)
scripts/cross-compile.sh                       — C → MBC pipeline (CREATE)
scripts/run-mbc-emulator.sh                    — MBC emulator runner (CREATE)

# Demos
demos/mbc/gradient.mbc                         — Gradient test program
demos/mbc/checkerboard.mbc                     — Checkerboard test program
demos/mbc/add42.mbc                            — Arithmetic test (r0=42)
demos/mbc/sum99.mbc                            — Loop test (r0=4950)
demos/c/test_add.c                             — C cross-compile test (CREATE)
demos/doom/                                    — Monorepo build integration stubs (CREATE)

# External Repos (unheaded GitHub org)
~/fucking-unheaded/DOOM/                       — Fork: id Software Doom source (GPL)
~/fucking-unheaded/doomgeneric/                — Fork: doomgeneric fbdev port (our stubs go here)
~/fucking-unheaded/*.wad                       — WAD files (gitignored, never committed)

# Docs
docs/DOOM_TASKS_20.md                          — D-001→D-020 task list
docs/protocol/doom-over-ipv6-architecture.md   — Architecture doc
docs/protocol/doom-over-ipv6-plan.md           — Implementation plan
docs/doom/DOOM-BRIDGE-ARCHITECTURE.md          — Bridge architecture
```

---

## APPENDIX C: BPF MAP LAYOUT

```
Map Name       Type    Key   Value    Entries   Description
─────────────  ──────  ────  ───────  ────────  ──────────────────────────
CPU_MAP        ARRAY   4     104      1         MbcCpuState
ROM_MAP        ARRAY   4     4        262144    1MB instruction ROM
RAM_MAP        ARRAY   4     4        1048576   4MB general RAM
SCREEN_MAP     ARRAY   4     1        64000     320×200 framebuffer
KBD_MAP        ARRAY   4     16       1         Keyboard state

All pinned at: /sys/fs/bpf/unheaded/doom-ring/maps/
```

---

*S42 Battle Plan — Forged 2026-02-24*
*10 Phases. 148 Steps. Track A runs anywhere. Track B needs bare metal.*
*The emulator proves the math. The ring proves the wire. The browser shows the world.*
*THE WIRE IS THE PROCESSOR. WOTAN IS THE RAM. PACKETS ARE THE CPU.*

⚔️🔥🎮🛡️
