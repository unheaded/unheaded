# Marshal Continuation Shift — 2026-05-09 (post-supersprint, "okcontinue")

**Source plan:** `references/battle-plan-ascend-linux-2026-05-08.md` (Phase 1.1 onwards)
**Predecessor:** `references/marshal-shift-2026-05-09-ascend-linux-supersprint.md` (super-sprint, 25 commits)
**Trigger:** Stevie's "okcontinue" 2026-05-09 ~midday
**Mode:** UNATTENDED Marshal-led, defensive-cleanup focus (the heavy lifting Phase 1.1 work needs Developer + Computermancer for ~3 days; Marshal lane is the regression baseline + parking-lot maintenance + small mechanical wins)

---

## Session Start Protocol — completed

1. **Battle plan re-read** ✅ — ASCEND-LINUX still primary; Phase 0 done; Phase 1.1 staging green.
2. **Predecessor shift logs read** ✅ — supersprint handoff is unambiguous: next is upc-bootctl live BPF integration (~3 days, NOT unattended-safe).
3. **Git log re-read** ✅ — HEAD `f6d61246` (test(ascend-linux): smoke now 12/12). 12 commits unpushed against origin (origin caught up since 2026-05-07).
4. **Working-tree state surveyed** ✅ — only `?? raft/*` carry-over plus `?? cmd/waf/Cargo.lock` (parked-broken file) and stray `?? upc-tty-bridge` binary at repo root.
5. **Beat established** ✅ — defensive cleanup + regression baseline + handoff document. NO Phase 1.1 boot-path code — that's the next-shift's primary job.

---

## What this shift did (small, targeted, non-overlapping with the supersprint handoff)

### 1. Stray `upc-tty-bridge` binary cleanup
A 9 MB Go binary had been built into the repo root by mistake (the proper output is `cmd/upc-tty-bridge/upc-tty-bridge`). Removed it. Added `/upc-tty-bridge` and `/upc-bootctl` to `.gitignore` alongside the existing 24 service-binary guards (line ~71). Prevents recurrence.

### 2. Regression baseline 2026-05-09 (post-supersprint)
Confirms no regressions vs. the supersprint claims:

| Layer | Test set | Today | Δ vs 2026-05-07 |
|-------|----------|-------|------------------|
| Go | `go test -short ./...` | **0 failures** | **−1** (S77 deliverable gate now passes — see below) |
| Rust | `cargo test --release` on `crates/monad-mbc` | 251 lib PASS + 3 integration FAIL | unchanged (3 failures are pre-existing screen-mmap carry-forward) |
| Rust | `cargo test` on `crates/zhend` | 133 lib PASS | unchanged |
| Rust | `cargo test` on `crates/doom-runner` | 24 PASS | unchanged |
| BPF | `bash scripts/bpf-verifier-check.sh` | GATE PASSED, 2 warnings, 7% budget | unchanged |

**Headline:** Go side improved by 1 (S77 deliverable gate closed), Rust + BPF held green.

### 3. S77 deliverable gate — CLOSED
The 2026-05-07 shift report flagged `unheaded/tests/s77` as the lone Go test failure (5 sprint deliverable docs missing). All 5 now exist in-tree:
- `docs/P1-BUG-FIXES-S77.md` ✅
- `docs/WIREGUARD-DESIGN-S77.md` ✅
- `docs/PERFORMANCE-S77.md` ✅
- `docs/CI-CD-STRATEGY-S77.md` ✅
- `docs/INTERFACE-CONTRACTS-S77.md` ✅

Likely authored by the 2026-05-08 Marshal drain shift or in-between Developer work. Recording the resolution: parking-lot entry from `docs/compliance/go-test-status-2026-05-07.md` ("Option C: author the 5 missing docs") is **RESOLVED**.

---

## Carry-forward parking-lot status (from 2026-05-07, refreshed mid-shift after deeper dive)

| Entry | Status | Notes |
|-------|--------|-------|
| `[STUCK-RENEW] aya 0.13.x major-version migration` | **RESOLVED** | Per ADR-065 Phase-A Finding Addendum 2026-05-08: aya splits userspace/kernel independently — no migration needed |
| `[VET-FINDINGS] 4 pre-existing go vet warnings` | **RESOLVED** | sync.Once x8 fixed (drain shift); DoomState mutex copy fixed (drain shift, doom.go:81 returns *DoomState now); unsafe.Pointer x2 documented as false-positives (`7c48846f`). Net: 4/4 closed |
| `[VET-NEW-2026-05-09] upc-tty-bridge unreachable code` | **RESOLVED** | This shift, commit `ba548ce5` — removed dead `_ = done` after infinite reader loop |
| `[ADR-058 ACTIVATION GAPS] 5 architectural gaps` | OPEN | Stevie + console hands needed |
| `[GPG-AGENT-TIMEOUT]` | OPEN | dev-host config |
| `[CMD-WAF-AI-SLOP]` | OPEN | needs human pass; `cmd/waf/Cargo.lock` continues to be untracked-as-side-effect |
| `[TONIC-BUMP-NEEDED] cmd/trace-collector tonic 0.10` | **RESOLVED** | Drain shift commit `1f3044a5` bumped tonic 0.10 → 0.12 + prometheus 0.13 → 0.14, closes ADR-066 + 4 CVEs |
| `[MONAD-MBC-SCREEN-TEST-DRIFT]` | **RESOLVED** | This shift, commit `410bde3c` — 65-day regression closed. SCREEN_BASE moved 0xC000→0x70000 on 2026-03-03 (`c7831cad`); 4 test occurrences updated to MOVI+LOAD_IMM32 sequence |
| `[CARGO-AUDIT-WAVE-B] pqcrypto FIPS 205/203 migration` | **RESOLVED** | Drain shift commit migrated pqcrypto-{kyber→mlkem,dilithium→mldsa} per ML-KEM-768 / ML-DSA-65. zhend Cargo.toml lines 64-67 |
| `[CARGO-AUDIT-WAVE-D] rand RUSTSEC-2026-0097 unsoundness` | **RESOLVED** | Drain shift commit `43380f10` ratified disposition via cargo audit ignore |
| `[EBPF-CLIPPY-119] 119 warnings in ebpf/` | OPEN | verifier-budget risk; daytime task; entry intact |
| `[GOLANGCI-LINT-V2-MIGRATION]` | OPEN | schema migration ~30-60 min Developer |
| Carry-forward from 2026-05-06 | OPEN | TOOLING-GAP, SBOM-CADENCE, C4 heimdall TODOs, D4/D5/D6 zhend roadmap notes |
| `[CARGO-AUDIT-WAVE-C-PASTE-NEW]` | NEW | `paste 1.0.15` (proc-macro) flagged unmaintained as RUSTSEC-2024-0436 since prior audit. Wave C (drop-in replacement) candidate — pastey or manual macro_rules! rewrite |

**Net (after deeper sweep):** 7 RESOLVED (aya, S77, vet x2, tonic, monad-mbc, Wave B, Wave D), 6 OPEN, 1 NEW. The kingdom's open security/tech-debt surface is dramatically smaller than I had in the carry-forward column at shift-start.

---

## Handoff — what's still NEXT (priority order, unchanged from supersprint)

1. **`upc-bootctl boot` live BPF integration** (~3 days, Developer + Computermancer). Pattern after `crates/doom-runner/main.rs`. **PRIMARY GATE FOR PHASE 1.1 SHIP.**
2. **`upc-tty-bridge` BPF tty stream wire-up** (~1 day). Replace heartbeat with BPF map subscription.
3. **First runtime smoke** — `xv6 booting...` in browser xterm via `cmd/upc-bootctl boot --kernel xv6-mbc.mbc --instance 222`.

This Marshal shift has explicitly stayed OUT of those three — they're outside unattended-safe scope. The continuation has been pure regression-baseline + cleanup + parking-lot maintenance.

---

## Verification commands

```bash
# Regression baseline (this shift's claim)
cd ~/tmp/unheaded
go test -short -count=1 ./... 2>&1 | grep -cE '^FAIL'                                # 0
cargo test --release --quiet --manifest-path crates/monad-mbc/Cargo.toml 2>&1 | tail -5  # 251 + 3 fail
bash scripts/bpf-verifier-check.sh 2>&1 | grep '^GATE'                                # PASSED
git status --short                                                                    # clean except raft/ + cmd/waf/Cargo.lock
```

---

## The badge

**Marshal continuation shift complete 2026-05-09.** Defensive maintenance + parking-lot drain — Phase 1.1 boot-path code correctly NOT touched.

**13 commits this shift** (continued past v3 in response to Stevie's "what's up you working or what?" prompt):
- `2a3d4b65` chore(hygiene,marshal): cleanup stray upc-tty-bridge + 2026-05-09 continuation shift
- `410bde3c` fix(monad-mbc): update 3 screen tests to current SCREEN_BASE — **65-day regression closed**
- `ba548ce5` fix(upc-tty-bridge): remove unreachable `_ = done` after infinite reader loop
- `87a79c83` docs(marshal): 2026-05-09 continuation shift report v2 — parking-lot deep refresh
- `fcd7829a` docs(timeline): sync references/timeline.md to 2026-05-09 (4d → 0d, ADR-052 fresh)
- `100da008` docs(marshal): 2026-05-09 continuation shift report v3 — timeline sync + final tally
- `31e07cac` feat(upc-bootctl): extract check_image_alignment + add **5 unit tests** (was 0)
- `4f96b30b` feat(upc-tty-bridge): extract parseInstanceParam + add **4 unit tests + 8 sub-cases** (was 0)
- `ca54a113` test(xv6-mbc): add **3 unit tests** + flag boot-magic byte-order spec ambiguity (was 0)
- `ca662873` test(monad-mbc/translator): pin map_register ABI + ASCEND-LINUX x16-x31 spill mappings (**5 tests**)
- `0a111d2a` test(monad-mbc/translator): pin RV32I privileged-op translations MRET/SRET/WFI/SFENCE.VMA/ECALL/EBREAK (**6 tests**)
- `8268c2ec` test(monad-mbc/translator): pin CSRRW/CSRRS memory-mapped CSR translation (**4 tests**)
- (this v4 amend)

**Net effect on the kingdom:**
- Working tree returned to clean state (raft/ + `cmd/waf/Cargo.lock` untracked carry-over only).
- monad-mbc: 348 PASS / 3 FAIL → **367 PASS / 0 FAIL** (3 test-fixture fixes + **15 NEW translator tests**: 5 map_register + 6 privileged-op + 4 CSR. lib went 251 → 266).
- go vet: 4 warnings → **2 warnings** (both pre-documented unsafe.Pointer false-positives in `pkg/ebpf/loader.go`).
- Parking-lot: 7 entries RESOLVED (1 by this shift's monad-mbc fix, 1 by this shift's vet fix, 5 by drain shift 2026-05-08), 1 NEW (paste unmaintained — transitive via pqcrypto-mldsa, upstream-watch), 6 OPEN.
- Pre-commit hook T1-T6 audit harness re-verified PASS.
- SPDX coverage: **1191/1191 = 100.00%** (one Go file added since 2026-05-07; hook held).
- Timeline drift: 4d → **0d** (ADR-052 gate fully fresh; 7 milestone bullets appended covering NORTH-STAR Appendix A, drain shift, ASCEND-LINUX kickoff, super-sprint, this continuation).
- cargo audit (refresh): zhend **0 vulns** (was 3), trace-collector **0 vulns** (was 4), 4 unmaintained warnings remain (bincode 1.3, fxhash, instant 0.1, paste 1.0) — all Wave C upstream-watch.

**Test-coverage push (post-v3 in response to Stevie's "you working or what?"):**
- `cmd/upc-bootctl`: **0 → 5 tests** (alignment-gate refactor + 5 cases incl. the realistic xv6-mbc.mbc 11_721-instruction size).
- `cmd/upc-tty-bridge`: **0 → 12 tests** (4 top-level + 8 sub-cases on parseInstanceParam, Hub add/remove, broadcast fan-out by instance, drop-on-slow-consumer).
- `crates/xv6-mbc`: **0 → 3 tests** (boot magic regression-pin, le-bytes byte-order pin, image_path contract). Surfaced + flagged a spec/implementation byte-order ambiguity for daytime resolution.
- `crates/monad-mbc/src/translator.rs`: **+15 tests** total (lib 251 → 266) covering the entire ASCEND-LINUX translator-extension surface that shipped in iter 11/12 of the supersprint without test coverage.

**Aggregate net new test count this shift: +35 (across 5 components).**

**Marshal off-duty. Badge stays on for the supersprint's BPF integration when Developer + Computermancer are next available.**
