# LICH Campaigns

The LICH is the automated adversary: fuzz harnesses, offensive campaign modules, and red-team scenarios run against the Kingdom's own code. Attack taxonomy: [[Dark Grimoire|Dark-Grimoire]]. Framework decisions: ADR-062 (fuzz / red team / pentest), ADR-063 (Akira summons the LICH).

## Three parts

| Part | Where | Run |
|------|-------|-----|
| Go fuzz harnesses | `tomb/lich/harnesses/lich_NNN_*_test.go` | `tomb/lich/lich-runner.sh [--harness 001] [--time 300s] [--parallel 4]`; crashes triaged by `crash-triage.sh`, reports in `tomb/lich/results/` |
| Rust fuzz targets | `crates/lich-fuzz/` (LICH-007 to 010) | cargo-fuzz |
| Campaign modules | `cmd/lich-security/campaigns/` D1–D6 | `lich-security` against the running stack |

## Harnesses

| ID | Target |
|----|--------|
| 001 | Monad wire format parser |
| 002 | Monad codec |
| 003 | CRC-16 collisions |
| 004 | MBC parser |
| 005 | Kernel interface (eBPF loader); found a loader panic (2026-09-29) |
| 006 | Lifecycle state machine |
| 007 | MBC in eBPF: 1B+ executions, zero crashes |
| 008 | Wotan L1 cache line races, real ring buffer |
| 009 | Cross-flow composite key collisions |
| 010 | WAL integrity / compaction races |
| 011 | Sophia dictionary hot-swap / sync |
| 012 | PQC signature fuzzing |
| 013 | HTTP APIs |
| 014 | Cross-service flows |

`tomb/lich/LICH-012-config-convergence/` is a separate red-team scenario for Mímir's Law (ADR-043) that reuses the 012 number; older docs also use LICH-001/005 for "network recon" and "credential brute-force" scenarios. The harness file names above are authoritative.

## Campaign modules (lich-security)

| Module | Tests |
|--------|-------|
| D1 | Auth bypass |
| D2 | Injection |
| D3 | Transport security (TLS posture; skips verification on purpose) |
| D4 | Secrets exposure |
| D5 | Privilege escalation |
| D6 | Denial of service |

## Workflow

Pick a grimoire class → write or extend a harness with seeds (`tomb/lich/seeds/`) → run → triage crashes → file a finding in `compliance/findings/register.yaml` → fix with a regression test → harness stays in the suite.

## CI

| Targets | Where | When |
|---|---|---|
| Go `Fuzz*` (every package) | `security.yml` job `lich-fuzz` | every push, 10 s each; nightly |
| Rust libFuzzer (`crates/monad-mbc/fuzz`, `crates/lich-fuzz`) | `lich.yml` via `scripts/lich-cargo-fuzz.sh` | push touching a fuzzed crate, 60 s each; nightly, 1 h each with a cached corpus |

Locally: `scripts/lich-cargo-fuzz.sh --list`, then `scripts/lich-cargo-fuzz.sh --time 60 [target...]` (nightly Rust, no cargo-fuzz needed). A crash fails the run and keeps the input under `<out>/artifacts/<target>/`.

---

> **Sources:** [docs/security/lich-campaigns.md](../docs/security/lich-campaigns.md) · [docs/security/lich-execution.md](../docs/security/lich-execution.md) · `tomb/lich/`
