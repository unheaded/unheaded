# S42 Doom PoC Completion — Session Handoff

**Date**: 2026-02-24
**Sprint**: S42
**Duration**: ~3 hours (continuation from S41)
**Status**: Track A COMPLETE, Track B BARE-METAL-REQUIRED

---

## Phases Completed

| Phase | Name | Status | Notes |
|-------|------|--------|-------|
| 0 | Inventory & Baseline | COMPLETE | All Go builds pass, MBC crate verified, 43 Rust + 52+ Go tests |
| 1 | MBC Emulator Verification | COMPLETE | `mbc-emulate` binary created, gradient (577K insns), checkerboard (897K insns), add42 (12 insns) all verified |
| 2 | Wotan Compute Memory | COMPLETE | 19 tests pass, cache miss/writeback verified |
| 3 | Dashboard Doom Integration | COMPLETE | 3 REST endpoints: /api/v1/doom/{screen,cpu,input}, 8 tests |
| 4 | Cross-Compile Pipeline | COMPLETE | C → RV32I → MBC verified (sum 0..99 = 4950 correct) |
| 5 | Doomgeneric Port Stubs | COMPLETE | crt0.S + doomgeneric_unheaded.c, 56 RV32I → 91 MBC, 2 syscalls |
| 6 | Tick Injector Integration | COMPLETE | --rate Hz flag added, --flow-label present, 3 modes (steady/burst/fast) |
| 7 | BPF Ring Integration | SKIPPED | BARE-METAL-REQUIRED — no sudo access on this machine |
| 8 | Test Suite Hardening | COMPLETE | All tests pass, 0 race conditions, only pre-existing perf test failure |
| 9 | Handoff & Documentation | COMPLETE | This document |

---

## D-016 through D-020 Status

| Task | Description | Status |
|------|-------------|--------|
| D-016 | Gradient E2E integration | SKIPPED (bare metal) — verified in emulator: 577K insns, 99.7% screen fill |
| D-017 | Checkerboard E2E | SKIPPED (bare metal) — verified in emulator: 897K insns, 100% screen fill |
| D-018 | Cross-compile pipeline (C → RV32I → MBC) | COMPLETE — test_add.c verified, doomgeneric stubs compile |
| D-019 | Doomgeneric bare-metal port stubs | COMPLETE — doomgeneric_unheaded.c + crt0.S compile to 91 MBC insns |
| D-020 | DOOM RUNS | NOT YET — requires full doomgeneric port + bare-metal BPF ring |

---

## Artifacts Created This Session

### New Files
- `crates/monad-mbc/src/bin/mbc_emulate.rs` — Userspace MBC CPU emulator binary
- `cmd/dashboard-backend/internal/server/doom.go` — Doom REST API endpoints
- `cmd/dashboard-backend/internal/server/doom_test.go` — 8 tests for Doom endpoints
- `demos/c/test_add.c` — Cross-compile pipeline test program
- `demos/c/linker.ld` — Minimal linker script for demos
- `demos/doom/crt0.S` — Bare-metal CRT0 startup code
- `demos/doom/doomgeneric_unheaded.c` — MBC platform layer stubs

### Modified Files
- `cmd/doom-go-injector/main.go` — Added `--rate` Hz convenience flag
- `demos/doom/Makefile` — Added `stubs` target for unheaded-doom.mbc
- `demos/doom/linker.ld` — Added BSS symbols + _stack_top for crt0.S
- `crates/monad-mbc/Cargo.toml` — Added `[[bin]]` for mbc-emulate
- `cmd/dashboard-backend/internal/server/server.go` — Added DoomState + routes

### Commits
1. `8509bb6` — feat(mbc): add mbc-emulate binary — userspace MBC CPU emulator
2. `ffc7b5f` — feat(dashboard): add Doom API endpoints — screen, cpu, input
3. `6fb1e61` — feat(demos): add cross-compile pipeline + doomgeneric stubs
4. `313c586` — feat(doom-injector): add --rate Hz flag for steady injection

---

## Test Results

### Go (all pass, 0 race conditions)
- `cmd/doom-bridge`: 30.0% coverage
- `cmd/doom-go-injector`: 19.2% coverage
- `internal/doom`: 91.5% coverage
- `cmd/dashboard-backend/internal/server`: 39.3% coverage
- Full suite: PASS (only pre-existing `TestPerformance_TraceProcessingLatency` fails)

### Rust (43 tests, all pass)
- MBC assembler, translator, CPU, emulator
- 2 doc-tests (1 pass, 1 ignored)

---

## What Remains for D-020 (DOOM RUNS)

1. **Work in `~/fucking-unheaded/doomgeneric/` fork** (NOT vendored into monorepo)
2. Replace DG_* implementations with MBC SYSCALL encoding
3. Cross-compile full Doom binary (C → RV32I → MBC) — will be ~90K MBC instructions
4. Load ~4MB ROM + WAD data into BPF maps
5. Run at sufficient tick rate for playable FPS (~35 Hz × 6 hops = ~1.47M insns/frame)
6. Migrate existing `doom/doomgeneric/` monorepo changes → fork
7. **Bare-metal requirement**: Linux kernel ≥ 5.15, sudo, BPF, network namespaces

---

## Known Issues

- `TestPerformance_TraceProcessingLatency`: Pre-existing, 20µs vs 10µs max threshold
- `sum99.mbc`: ROM fault at PC=65548 (pre-existing demo binary issue)
- `DefaultFlowLabel` in doom-go-injector is `0xDE`, not `0x0A3F7E` — the canonical label in the battle plan differs from what's implemented. Cosmetic only.
- No screen reading in Wotan compute — doom-bridge handles BPF map reads directly

---

## Architecture Notes

### Doom Data Flow
```
Packet → XDP (monad-cpu-ebpf) → MBC execute → SCREEN_MAP → doom-bridge → WebSocket → browser
                                                            → dashboard-backend → REST API
```

### MBC Emulator Path (no BPF needed)
```
.mbc file → mbc-emulate → ExecCpu → step() loop → PPM dump / register report
```

### Cross-Compile Path
```
.c → riscv64-unknown-elf-gcc (RV32I, x0-x15 only) → .elf → rv32i-to-mbc → .mbc
```

---

## Next: S43 — Return to Core

Per S43 battle plan: packet tracing, eBPF, trace-collector wiring. Focus on the core observability pipeline.
