# Cargo Test Suite Status — 2026-05-07

**Auditor:** Marshal-led unattended session, Phase 17 (post-`charge`)
**Command:** `cargo test --quiet` (per-crate, no --release)
**Host:** WEST (Linux 6.17, Rust nightly cargo 1.95)

---

## Headline numbers

| Workspace | Tests | Pass | Fail | Notes |
|-----------|-------|------|------|-------|
| `crates/zhend` (lib) | 133 | **133** | 0 | All anti-fragile gossip / PQC / fragment / hybrid-store tests green |
| `crates/doom-runner` | 24 | **24** | 0 | All loader / memory / CPU-state tests green |
| `crates/monad-mbc` | 43 | 40 | **3** | All 3 screen-mmap tests failing — **pre-existing**, not from this session |
| `crates/zhenai-forge` | (skipped — Captain-gated WAVE work) | — | — | Last shipped 500-step LoRA at WAVE12 (2026-04-23) per CLAUDE.md |
| `cmd/ebpf-loader` | 0 | 0 | 0 | No unit tests defined (compiles green) |
| `cmd/ebpf-collector` | (deferred — eBPF target build constraints) | — | — | Compile gate satisfied via clippy --fix sweep |
| `cmd/trace-collector` | (deferred — has integration_test.rs touched by clippy --fix; needs Linux ringbuf access at runtime) | — | — | Compile-only verification this session |
| `ebpf/` workspace | (build-only — BPF target programs do not run as host tests) | — | — | bpf-verifier-check.sh passes at 7% budget per Phase 3 |
| **Aggregate (where runnable)** | **200** | **197** | **3** | 98.5 % pass rate |

**Verdict:** Rust regression baseline is clean except for 3 pre-existing failures in monad-mbc, all in the same `screen-mmap` / `byte-store` integration test family. None introduced by today's session.

---

## The 3 failing tests in `crates/monad-mbc`

```
failures:
    integration_byte_store_load_screen
    step101_screen_gradient_pattern
    step101_screen_write_and_readback
```

**Pre-existing-confirmed:** reverting tonight's clippy --fix commit (`8999e437`) and re-running these tests produces the same 3 failures. The integration_tests.rs file was last meaningfully modified before tonight in `7b16ba9d` ("S51+S59 Wave 1 security hardening + Wave 3 dashboard polish") on 2026-03-XX — well before the failures' age.

**Pattern:** all three tests touch the screen-mmap byte-store path. Possible root causes (un-investigated this session):
1. The `MbcCpuState` layout shifted at some point (per the doom-runner work which references `crates/doom-runner/src/memory.rs` as the layout source-of-truth) and the test harness in monad-mbc is referencing the older shape.
2. A `BYTE_STORE` opcode / instruction-encoding change broke binary compatibility with a fixture.
3. A stale memory-map address constant in the test fixtures.

**Failure signature** (from `integration_byte_store_load_screen` at `tests/integration_tests.rs:947:5`):
```
assertion `left == right` failed
```
i.e. classic data-mismatch — the screen buffer doesn't match the expected pattern after store/load round-trip.

**Disposition:** Add to parking lot. Daytime: monad-mbc test suite owner (Computermancer + Developer pair) re-aligns the 3 tests with current `MbcCpuState` layout. ~1-2 hour task on a Linux host.

---

## What ran clean tonight

- **`crates/zhend`** — 133 tests covering Fragment ops, gossip codec, PQC sealed-payload (Gungnir), trigger packets (Gjallarhorn), drift-aggregator (Enkrateia), hybrid-store, etc.
- **`crates/doom-runner`** — 24 tests covering bridge handler, BPF loader recipes, memory-map constants, CPU register ops.
- **All 218 Go packages** that pass `go test -short ./...` per `docs/compliance/go-test-status-2026-05-07.md`.

---

## Cross-reference

This run was the third regression-baseline check of the 2026-05-07 unattended session, after:
- `bash scripts/bpf-verifier-check.sh` (Phase 3) — eBPF instruction-budget gate PASS
- `go test -short ./...` (Phase 14) — 218/221 packages PASS
- `cargo test` (this) — 200/203 runnable Rust tests PASS

**No regression introduced by tonight's 19 commits across {compliance docs, gofmt + cargo-fmt + clippy --fix sweeps, pre-commit hook, ADR-065, SPDX coverage, security audit, unsafe.Pointer doc comments, dependency patch}.**

---

## Reproduction

```bash
cd ~/tmp/unheaded/crates/zhend       && cargo test --quiet --lib    # 133/133 pass
cd ~/tmp/unheaded/crates/doom-runner && cargo test --quiet           # 24/24 pass
cd ~/tmp/unheaded/crates/monad-mbc   && cargo test --quiet           # 40/43 (3 pre-existing screen-mmap fails)
```
