# Marshal Shift Log — 2026-05-06 (Overnight Unattended)

**Source plan:** `references/battle-plan-NORTH-STAR-2026-05-05.md` (Appendix A)
**Marshal plan:** `~/.claude/plans/battle-plan-tmp-unheaded-references-batt-eventual-puppy.md`
**Mode:** UNATTENDED — Stevie sleeping
**Shift start:** 2026-05-06 01:00 CDT (UTC-5)

---

## Session Start Protocol

1. **Battle plan read** ✅ — North Star + Appendix A
2. **Timeline read** ✅ — `references/timeline.md` (79 lines, current)
3. **Git log read** ✅ — last commit `e24f6c26` (battle plan + Appendix A)
4. **Beat established** ✅ — Phases A → B → C → D
5. **Rules posted** ✅ — see plan file

---

## Baseline (session-start state)

**Host:** Darwin 25.4.0 arm64 (Stevies-MacBook-Air)
**HEAD:** `e24f6c26 docs(battle-plan): North Star + Appendix A overnight sprint candidates`
**Branch:** `main` (clean working tree except `.claude/skills/` untracked)
**Other local branches:** `claude/migrate-packages-github-V2Ctr`, `docs/s73-public-launch-planning`, `public-release-cleanup` (the 3 stale branches per A2)

### Build baseline — `go build ./...`

**Status:** EXIT=1 with **pre-existing Linux-only failures** (NOT a regression).

Affected packages (all use `unix.SYS_BPF` / `unix.AF_PACKET` / `unix.SockaddrLinklayer` / `unix.ETH_P_IPV6` — Linux-only kernel surface, no darwin equivalents, no `//go:build linux` tag):
- `unheaded/internal/bpf`
- `unheaded/internal/bpfmap`
- `unheaded/cmd/doom-go-injector`
- `unheaded/cmd/wotan-ctl` (only `bpf_maps.go`)
- `unheaded/cmd/trace-collector-go` (eBPF symbol references)

**S1 finding:** This is the documented Darwin-vs-Linux split. CLAUDE.md says "`go build ./...` passes" — that claim implicitly assumes Linux/CI. The macOS dev environment can never build these packages. **Documented as baseline; not in tonight's scope to fix.** Fingerprint saved to `/tmp/marshal-build-baseline.txt`.

**Tooling availability:**
- syft: ❌ not installed
- scancode: ❌ not installed
- cyclonedx: ❌ not installed
- `scripts/build-sealed-cask.sh`: ✅ present
- `.git/hooks/`: empty (no custom pre-commit)
- `.pre-commit-config.yaml`: ❌ not present at repo root

---

## Live progress log

(Appended chronologically as steps complete or get parked.)

### 2026-05-06 01:00 — Marshal on duty
Plan posted. Tasks created. Build baseline pinned (Linux-only eBPF surface fails on darwin — pre-existing, not regression).

### 2026-05-06 01:10 — Phase A1 SBOM regen
- ScanCode/syft/cyclonedx not installed locally → cannot run full regen tonight.
- Generated **delta-SBOM** instead: `docs/sbom/2026-05-06-sbom-delta.md`.
- Findings: Go deps unchanged since 2026-03-15 SBOM (100 vs 99 modules — `+1` is `unheaded` itself). Rust workspaces stable (zhend 337 transitives, zhenai-forge 60, etc.). All lockfile SHA-256s captured.
- Re-aggregated 2026-04-04 ScanCode JSON: GPL-3.0+/MIT/GPL-2.0/Apache/BSD breakdown clean. **13 AGPL "hits" verified as false positives** (all are docs *discussing* AGPL, not AGPL-licensed).
- **3 real bugs detected in `scripts/verify-gpl-boundary.sh`** — see Phase A.5.

**A1 status:** ✅ done (delta-SBOM + parked-bug list).

### 2026-05-06 01:15 — CAPTAIN PIVOT (Stevie message inbound)
Stevie: *"append plan - compliance enforcement audit tooling, some of this is scoped"*

Marshal accepts plan amendment. Inserted **Phase A.5 — Compliance Enforcement Audit Tooling** (CE1–CE5) between Phase A and Phase B, formalizing the bugs that emerged from A1 plus extending the audit surface (SPDX coverage, CI gate inventory, pre-commit). Plan file updated. 5 new tasks created. Continuing with A2.


