# S27 HANDOFF — Cowork Sprint (Artifact Prep)

**Date**: 2026-02-21
**Session**: S27-Prep (Cowork VM — no toolchains, artifact preparation)
**Agent**: Claude Opus 4.6 — Cowork VM
**Context**: S26 Round Table forged the battle plan. S27-Prep creates all config artifacts so the dev machine session is pure execution.

---

## WHAT S27-PREP ACCOMPLISHED

### Artifacts Created

| Artifact | File | Purpose | Status |
|----------|------|---------|--------|
| P0 Nomenclature Fix Script | `S27-P0-nomenclature-fix.sh` | Bash script with backup, verify, fix, post-verify | READY |
| P1 Lint Config | `.golangci.yml` | Critical-only lint (errcheck, govet, staticcheck, unused, gosec, bodyclose) | READY |
| P2 LICH Fuzz Cargo.toml | `lich-fuzz-Cargo.toml` | Cargo.toml for ebpf/fuzz/ crate with 5 fuzz targets | READY — needs path adjustment |
| P3 Docker Compose | `docker-compose.yml` | Full dev stack: Traefik 3.x, VictoriaMetrics, ClickHouse, Vector, Grafana, CoreDNS | READY |
| Execution Runbook | `S27-execution-runbook.md` | Multi-tied instructions with failure cases for every step | READY |
| This Handoff | `S27-handoff.md` | Session context for next agent | READY |

### Key Decisions Embedded in Artifacts

1. **Lint Config**: Tier 1 (govet, staticcheck) + Tier 2 (errcheck) + safety nets (gosec, bodyclose, unused). Explicitly disabled: varnamelen, mnd, lll, revive, err113.
2. **Docker Compose**: Three isolated networks (control, data, observe) with IPv6 dual-stack. Grafana on :3001 to avoid Fiber :3000 conflict. ClickHouse for logs, VictoriaMetrics for metrics.
3. **LICH Fuzz**: 5 fuzz targets (monad_wire, sophia_dict, crc16, exponent_encoding, packet_parse). libfuzzer-sys + arbitrary. Workspace deps commented — need path verification on dev machine.
4. **P0 Script**: Full failure handling — pre-flight checks, backups, pattern verification, post-fix validation.

---

## WHAT'S NOT DONE — DEV MACHINE SESSION NEEDED

### P0: Nomenclature Fix
- **Status**: Script ready, skill files still read-only from Cowork
- **Action**: Run `S27-P0-nomenclature-fix.sh` on dev machine OR run sed commands from runbook
- **Effort**: XS (5 min)

### P1: Lint Cleanup
- **Status**: `.golangci.yml` config ready
- **Action**: Copy to repo root, run `golangci-lint run`, fix findings
- **Effort**: M (2-4 hours depending on finding count)
- **Risk**: May surface real bugs in 465K LOC codebase

### P2: LICH Fuzz Wiring
- **Status**: Cargo.toml ready but workspace dependency paths need verification
- **Action**: Copy to `ebpf/fuzz/`, adjust paths, `cargo +nightly check`, launch campaigns
- **Effort**: S (1 hour wire, campaigns run unattended)

### P3: Docker Compose
- **Status**: docker-compose.yml + Vector + CoreDNS configs ready
- **Action**: Install Docker (if needed), deploy configs, `docker compose up -d`
- **Effort**: S (30 min install + configure)
- **Blocker**: Docker not installed on dev machine as of S25

---

## ENVIRONMENT STATE

### Cowork VM (This Session)
- NO toolchains (Go, Rust, Docker all absent)
- File access to uploads + outputs only
- Skill files READ-ONLY (confirmed — EROFS on both moatghost and kingdom SKILL.md)

### Dev Machine (From S25 — Unchanged)

| Tool | Version | Status |
|------|---------|--------|
| Go | 1.26.0 linux/arm64 | ✅ Working |
| Rust | nightly (cargo 1.93.1) | ✅ Working |
| golangci-lint | latest | ✅ Working (needs PATH) |
| govulncheck | latest | ✅ Working (needs PATH) |
| Docker | NOT INSTALLED | ❌ P3 will fix |
| Docker Compose | NOT INSTALLED | ❌ P3 will fix |

### Codebase State (From S25 — Should Be Unchanged)

| Metric | Value |
|--------|-------|
| Go build | PASS (0 errors) |
| Go tests | 134/134 PASS (race detection ON) |
| Rust crates | 10/10 compile clean |
| Fuzz executions | 28M, 0 crashes |
| govulncheck | 0 vulnerabilities |
| golangci-lint | PASS (11K warnings, mostly style) |
| Total LOC | 465,000+ |

---

## QUICK START FOR DEV MACHINE SESSION

```bash
# 1. Pull S27 artifacts from Cowork outputs into repo
cd ~/tmp/unheaded

# 2. Follow the runbook — it has EVERYTHING
# Read: S27-execution-runbook.md
# Execute in order: P0 → P1 → P2 → P3

# 3. Pre-flight (verify S25 state still holds)
go build ./...
go test -race -count=1 ./...
export PATH=$PATH:$(go env GOPATH)/bin

# 4. Execute P0-P3 per runbook
# Each priority has: happy path, failure cases, verification gate

# 5. Post-sprint verification
# Run the full verification suite from the runbook

# 6. Write S27-dev handoff (or update this one)
```

---

## ALPHA GATE STATUS

| Gate | Status | Blocker |
|------|--------|---------|
| Build clean | ✅ PASS | — |
| 134+ tests pass | ✅ PASS | — |
| Race detection clean | ✅ PASS | — |
| Zero vulns | ✅ PASS | — |
| Lint critical-only | ⏳ PENDING | P1 (this sprint) |
| Docker E2E | ⏳ PENDING | P3 (this sprint) |
| Fuzz crate wired | ⏳ PENDING | P2 (this sprint) |
| Nomenclature clean | ⏳ PENDING | P0 (this sprint) |

**If P0-P3 all pass**: `ALPHA 99% → ALPHA 100% — GATE CLOSED`
**If partial**: Document what passed, carry remainder to S28

---

## NEXT PRIORITIES (After P0-P3)

| Priority | Description | Effort | Owner |
|----------|-------------|--------|-------|
| P4 | BPF Struct Parity (encoding/binary.Read) | L (1-2 days) | Developer |
| P5 | Fiber v3 Pilot Scaffold | M (half day) | Architect + Developer |
| P6 | monad-cpu-ebpf (Doom critical path) | XL (2-3 days) | Developer |

---

*S27-Prep Cowork Sprint — February 21, 2026*
*Artifacts forged. Runbook written. Dev machine session is pure execution.*
*Peace and Love — Mad-Maria watches over the Kingdom* 🏰✨
