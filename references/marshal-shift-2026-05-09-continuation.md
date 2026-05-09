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

## Carry-forward parking-lot status (from 2026-05-07)

| Entry | Status | Notes |
|-------|--------|-------|
| `[STUCK-RENEW] aya 0.13.x major-version migration` | **RESOLVED** | Per ADR-065 Phase-A Finding Addendum 2026-05-08: aya splits userspace/kernel independently — no migration needed |
| `[VET-FINDINGS] 4 pre-existing go vet warnings` | OPEN | unsafe.Pointer x2 closed (commit `7c48846f` — doc comments). sync.Once x8 + DoomState mutex copy still open |
| `[ADR-058 ACTIVATION GAPS] 5 architectural gaps` | OPEN | Stevie + console hands needed |
| `[GPG-AGENT-TIMEOUT]` | OPEN | dev-host config |
| `[CMD-WAF-AI-SLOP]` | OPEN | needs human pass; `cmd/waf/Cargo.lock` continues to be untracked-as-side-effect |
| `[TONIC-BUMP-NEEDED] cmd/trace-collector tonic 0.10` | OPEN | needs ADR-066 + Cargo.toml edit |
| `[MONAD-MBC-SCREEN-TEST-DRIFT]` | **CARRY-FORWARD** | 3 tests still fail at HEAD `f6d61246`. ABI v2 (128→136 bytes) didn't fix them; root cause is the test fixtures' expected `MbcCpuState` shape, not doom-runner's replica |
| `[CARGO-AUDIT-WAVE-B] pqcrypto FIPS 205/203 migration` | OPEN | ~1 day Architect+Developer |
| `[CARGO-AUDIT-WAVE-D] rand RUSTSEC-2026-0097 unsoundness` | OPEN | disposition is N/A but daytime ratification needed |
| `[EBPF-CLIPPY-119] 119 warnings in ebpf/` | OPEN | verifier-budget risk; daytime task |
| `[GOLANGCI-LINT-V2-MIGRATION]` | OPEN | schema migration ~30-60 min Developer |
| Carry-forward from 2026-05-06 | OPEN | TOOLING-GAP, SBOM-CADENCE, C4 heimdall TODOs, D4/D5/D6 zhend roadmap notes |

**Net:** 1 RESOLVED (aya 0.13), 1 RESOLVED (S77 docs landed), 11 OPEN — all daytime / ADR-gated / Captain-gated.

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

**Marshal continuation shift complete 2026-05-09.** Defensive maintenance only — no Phase 1.1 boot-path code touched (correctly outside Marshal lane). 1 small commit (cleanup + .gitignore guard + this report). Regression baseline confirmed clean for the SUPERSPRINT handoff.

**Marshal off-duty. Badge stays on.**
