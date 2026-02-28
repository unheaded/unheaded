# WARMONGER BATTLE PLAN REFERENCE TEMPLATE
# ═══════════════════════════════════════════
# Canonical format for ALL Unheaded battle plans.
# Derived from S42-S50 exhaustive sprint plans.
# Maintained by: The Warmonger (Battle Planner)
# Created: 2026-02-27 | Version: 1.0

---

# S[N] [TITLE] BATTLE PLAN — [X] Phases, [Y]+ Steps

**Date**: YYYY-MM-DD
**Sprint**: S[N] — [One-line description]
**Prerequisite**: [What must be true before execution starts]
**Target**: [What "done" looks like — measurable, verifiable]
**Estimated Duration**: [X-Y hours across Z sessions]
**Agent Strategy**: [e.g., "Phases 0-3 sequential, 4-6 parallelizable, 7 sequential"]
**Commit Cadence**: Every [N] steps (formula: max(3, min(5, total_steps/20)))
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

---

## LEGEND

```
[B]     = Bash command (run directly)
[V]     = Verification step (MUST pass before proceeding)
[D]     = Debug step (only if prior step fails)
[W]     = Write/create file
[R]     = Read/inspect file
[S]     = Sudo/elevated privileges required
[P]     = Parallelizable with other [P] steps in same phase
[C]     = Commit checkpoint
[ENV]   = Environment setup/check
[BUILD] = Build/compilation step
[TEST]  = Test execution
[CODE]  = Code implementation
[DESIGN]= Design/planning decision
[STUCK] = Step skipped via Skip Protocol
[BLOCKED]= Step blocked by upstream STUCK
[BARE-METAL] = Requires real hardware/kernel/BPF
```

---

## SITUATION REPORT

### What We Have (Built, Tested, Working)

| Component | Location | LOC | Status |
|-----------|----------|-----|--------|
| [Name]    | `path/`  | X   | ✅ GREEN / ⚠️ DEGRADED / ❌ BROKEN |

### What We Need (Gap Analysis)

| Gap | Blocker | Severity | Estimated Effort |
|-----|---------|----------|-----------------|
| [Missing thing] | [What blocks it] | P0/P1/P2 | S/M/L/XL |

### Prerequisites Verified

- [ ] `go build ./...` passes
- [ ] `go test ./...` passes
- [ ] Git working tree clean
- [ ] Required tools installed: [list]
- [ ] Prior sprint handoff reviewed: [S{N-1} file]

---

## AGENT ASSIGNMENT MATRIX

| Phase | Agent | Parallelizable | Dependencies | Est. Time |
|-------|-------|---------------|-------------|-----------|
| 0     | Coordinator | No | None | 15m |
| 1     | Agent A | No | Phase 0 | 45m |
| 2     | Agent B | Yes [P] | Phase 0 | 30m |
| ...   | ... | ... | ... | ... |

**Critical Path**: Phase 0 → 1 → 3 → 5 → 7 → 9 (X hours)
**Parallelizable Savings**: ~Y hours

---

## PHASE 0: INTELLIGENCE & ENVIRONMENT VERIFICATION (Steps 1-[N])

**Goal**: Confirm starting conditions. Verify tools. Establish baseline.
**Prerequisite**: None (this IS the prerequisite phase)
**Time**: ~15-20 minutes
**Agent**: Coordinator

- [ ] **Step 1** [R] ~1m: **Read prior sprint handoff**
  ```bash
  cat docs/handoffs/S[N-1]-handoff.md | head -50
  ```
  - Note blockers, skipped items, known issues

- [ ] **Step 2** [B] ~30s: **Check git status and recent history**
  ```bash
  cd $(git rev-parse --show-toplevel) && git status && git log --oneline -10
  ```
  - Expected: Clean working tree, recent commits visible

- [ ] **Step 3** [B] ~30s: **Verify build health**
  ```bash
  go build ./... 2>&1 | tail -5
  ```
  - Expected: No errors

- [ ] **Step 4** [V] ~30s: **Verify test health**
  ```bash
  go test ./... -count=1 -timeout=120s 2>&1 | tail -20
  ```
  - Expected: All packages pass
  - If fail → Step 4a [D]

- [ ] **Step 4a** [D] ~3m: **Debug test failures**
  ```bash
  go test ./... -count=1 -timeout=120s 2>&1 | grep "FAIL"
  ```
  - If pre-existing failures: Note in log, continue
  - If NEW failures: STOP. Fix before proceeding.

- [ ] **Step 5** [ENV] ~1m: **Verify required toolchain**
  ```bash
  for tool in go rustc cargo nix docker bpftool ip wg; do
    which $tool 2>/dev/null && echo "$tool: $(${tool} --version 2>/dev/null | head -1)" || echo "$tool: MISSING"
  done
  ```
  - If any MISSING → Step 5a [D]

- [ ] **Step 5a** [D] ~5m: **Install missing tools**
  ```bash
  # Platform-specific install commands
  ```
  - If unresolvable → [STUCK]

- [ ] **Step 6** [C] ~30s: **COMMIT CHECKPOINT — Baseline verified**
  ```bash
  git add -A && git commit -m "[PLAN S{N}] Steps 1-6: Environment verified, baseline established

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 7** [V]: **PHASE 0 EXIT GATE** — Environment confirmed
  - [ ] Build: GREEN
  - [ ] Tests: GREEN (or known pre-existing only)
  - [ ] Tools: All present
  - [ ] Git: Clean
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

---

## PHASE [N]: [TITLE IN CAPS] (Steps [X]-[Y])

**Goal**: [One sentence — what this phase achieves]
**Prerequisite**: Phase [N-1] EXIT GATE passed
**Time**: ~[X] minutes
**Agent**: [Coordinator | Agent | Agent [P]]

### [Subsection Title]

- [ ] **Step [X]** [B] ~2m: **[Action description]**
  ```bash
  [exact command with full paths and flags]
  ```
  - Expected: [What success looks like]

- [ ] **Step [X+1]** [V] ~30s: **Verify [what was just done]**
  ```bash
  [verification command]
  ```
  - If pass → Step [X+2]
  - If fail → Step [X+1]a [D]

- [ ] **Step [X+1]a** [D] ~3m: **Debug — [failure description]**
  ```bash
  [diagnostic command]
  ```
  - If resolved → Step [X+2]
  - If unresolvable after 2 attempts → [STUCK]

- [ ] **Step [X+2]** [W] ~3m: **Create [filename]**
  ```bash
  cat << 'EOF' > path/to/file
  [file contents]
  EOF
  ```
  - Save to: `path/to/file`

- [ ] **Step [X+3]** [TEST] ~2m: **Run tests for [component]**
  ```bash
  go test ./pkg/[component]/... -race -count=1 -timeout=60s 2>&1 | tee /tmp/s{N}-phase{P}-tests.log
  grep "ok\|FAIL" /tmp/s{N}-phase{P}-tests.log
  ```
  - Expected: All ok, zero FAIL
  - If fail → Step [X+3]a [D]

- [ ] **Step [X+4]** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  git add -A && git commit -m "[PLAN S{N}] Steps {X}-{X+4}: [achievement summary]

  Phase {P}: {phase name}
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step [Y]** [V]: **PHASE [N] EXIT GATE** — [What must be true]
  - [ ] Check 1: [specific criterion with command]
  - [ ] Check 2: [specific criterion with command]
  - [ ] Check 3: [specific criterion with command]
  - If ALL pass → Phase [N+1]
  - If ANY fail → DO NOT PROCEED. Debug within this phase.

---

## PHASE [LAST]: HANDOFF & DOCUMENTATION (Steps [X]-[Y])

**Goal**: Document what was done, what remains, prepare next sprint.
**Prerequisite**: All prior EXIT GATEs passed (or STUCK-logged)
**Time**: ~15-30 minutes
**Agent**: Coordinator

- [ ] **Step [X]** [W] ~10m: **Write handoff document**
  ```bash
  cat << 'EOF' > docs/handoffs/S{N}-handoff.md
  # S{N} Sprint Handoff

  ## Status: [COMPLETE | PARTIAL | BLOCKED]

  ## Phases Completed
  - Phase 0: ✅
  - Phase 1: ✅
  - Phase 2: ⚠️ (Step X stuck)
  ...

  ## Deliverables
  - [x] [Deliverable 1]
  - [ ] [Deliverable 2 — blocked by Step X]

  ## Known Issues
  1. [Issue from STUCK steps]

  ## Next Sprint Should
  1. [Priority action]
  2. [Priority action]

  ## Metrics
  - Steps completed: X/Y (Z%)
  - Time elapsed: Xh Ym
  - Commits: N
  EOF
  ```

- [ ] **Step [X+1]** [R] ~2m: **Review stuck steps for handoff accuracy**
  ```bash
  grep -n "STUCK\|BLOCKED" references/S{N}-BATTLE-PLAN.md
  ```

- [ ] **Step [X+2]** [B] ~1m: **Final test run**
  ```bash
  go test ./... -count=1 -timeout=120s 2>&1 | tail -5
  ```
  - Expected: Same or better than baseline

- [ ] **Step [X+3]** [C] ~30s: **FINAL COMMIT**
  ```bash
  git add -A && git commit -m "[PLAN S{N}] COMPLETE: Steps {1}-{Y} — [sprint achievement summary]

  Phases: {completed}/{total}
  Steps: {completed}/{total} ({percent}%)
  Stuck: {count} (see handoff)
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step [Y]** [V]: **FINAL EXIT GATE**
  - [ ] Build: GREEN
  - [ ] Tests: GREEN
  - [ ] Handoff written
  - [ ] All STUCK steps documented
  - [ ] Git log shows clean commit history

---

## APPENDIX A: EMERGENCY PROCEDURES

### A1: [Failure Mode Name]
**Symptom**: [What you see]
**Likely Cause**: [Root cause]
**Recovery**:
```bash
# Step-by-step recovery commands
```

### A2: [Failure Mode Name]
**Symptom**: [What you see]
**Likely Cause**: [Root cause]
**Recovery**:
```bash
# Step-by-step recovery commands
```

---

## APPENDIX B: KEY FILE PATHS

| Category | File | Purpose |
|----------|------|---------|
| Config   | `path/to/config` | [What it configures] |
| Source   | `path/to/source` | [What it implements] |
| Test     | `path/to/test`   | [What it tests] |

---

## APPENDIX C: QUICK REFERENCE

### [Data Structure / Wire Format / CLI Commands]
```
[Reference diagram or cheat sheet]
```

---

*S[N] Battle Plan — Forged YYYY-MM-DD*
*[X] Phases. [Y] Steps. [Evocative one-liner.]*
*[Optional second one-liner.]*

---

# ═══════════════════════════════════════════
# TEMPLATE USAGE NOTES (remove in actual plans)
# ═══════════════════════════════════════════
#
# SCALING:
# Micro  (1-2h exec):  3-5 phases,  30-60 steps,  no appendices
# Small  (2-4h):       5-8 phases,  60-120 steps,  optional appendices
# Medium (4-8h):       8-12 phases, 120-200 steps, recommended appendices
# Large  (8-16h):      12-16 phases, 200-300 steps, required appendices
# Epic   (16-24h+):    16-20 phases, 300-420+ steps, required + extended
#
# COMMIT CADENCE:
# commit_interval = max(3, min(5, total_steps / 20))
#
# STUCK PROTOCOL:
# 3x time estimate OR 2 failed debug attempts → SKIP
# Log: step, attempts, error, downstream impact
# Commit before skip. Resume at next non-blocked step.
#
# EVERY [B] gets a [V]. EVERY phase gets an EXIT GATE.
# EVERY plan gets a LEGEND, SITUATION REPORT, and FORGE STAMP.
#
# The plan IS the product. Write every command.
# ═══════════════════════════════════════════
