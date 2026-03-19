# WARMONGER BATTLE PLAN REFERENCE TEMPLATE
# ═══════════════════════════════════════════
# Canonical format for ALL Unheaded battle plans.
# Derived from S42-S74 exhaustive sprint plans.
# Post-mortem improvements: S74 (10 structural issues resolved).
# Maintained by: The Warmonger (Battle Planner)
# Created: 2026-02-27 | Version: 2.0 | Updated: 2026-03-19

---

# S[N] [TITLE] BATTLE PLAN — [X] Phases, [Y]+ Steps

**Date**: YYYY-MM-DD
**Sprint**: S[N] — [One-line description]
**Prerequisite**: [What must be true before execution starts]
**Target**: [What "done" looks like — measurable, verifiable]
**Commit Cadence**: Every [N] steps (formula: max(3, min(5, total_steps/20)))
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts

### Multi-Agent Time Estimates

| Mode | Agents | Estimated Duration | Critical Path |
|------|--------|--------------------|---------------|
| Solo | 1 agent | [X-Y hours] | [Full sequential chain] |
| Pair | 2 agents | [X-Y hours] | [With parallel savings] |
| Swarm | 4 agents | [X-Y hours] | [Maximum parallelization] |

**Agent Strategy**: [e.g., "Phases 0-3 sequential, 4-6 parallelizable, 7 sequential"]

---

## VARIABLES

```
$PROJECT_ROOT  = $(git rev-parse --show-toplevel)
$SPRINT_ID     = S[N]
$SPEC_DIR      = $PROJECT_ROOT/references
$AUDIT_DOC     = $PROJECT_ROOT/docs/internal/audit/S[N]-audit.md
$HANDOFF_DIR   = $PROJECT_ROOT/docs/handoffs
$BATTLE_PLAN   = $SPEC_DIR/S[N]-BATTLE-PLAN.md
```

> Agents resolve `$PROJECT_ROOT` via `git rev-parse --show-toplevel` in Step 1.
> All paths in this plan are relative to `$PROJECT_ROOT` unless fully qualified.
> ZERO hardcoded absolute paths. If you see `/home/user/project/`, it's a bug.

---

## LEGEND

```
[B]            = Bash command (run directly)
[V]            = Verification step (MUST pass before proceeding)
[D]            = Debug step (only if prior step fails)
[W]            = Write/create file
[R]            = Read/inspect file
[S]            = Sudo/elevated privileges required
[P]            = Parallelizable with other [P] steps in same phase
[P:N]          = Parallelizable with Phase N (cross-phase)
[SEQ]          = MUST be sequential — do NOT parallelize
[C]            = Commit checkpoint
[ENV]          = Environment setup/check
[BUILD]        = Build/compilation step
[TEST]         = Test execution
[CODE]         = Code implementation
[DESIGN]       = Design/planning decision
[STUCK]        = Step skipped via Skip Protocol
[BLOCKED]      = Step blocked by upstream STUCK (NOT for decisions)
[DECIDE]       = Decision point WITH pre-seeded recommendation (agent proceeds autonomously)
[ESCALATE]     = Requires human input — no recommendation possible. STOP.
[PREFLIGHT]    = Hypothesis verification (Phase 0 only)
[REGEN]        = Regenerate derived artifacts (MD→JSON/YAML, XML, etc.)
[AUDIT-UPDATE] = Mark finding resolved in tracking doc
[DOC-UPDATE]   = Update downstream docs (CLAUDE.md, timeline, wiki)
[SECURITY]     = Security review step
[COMPLIANCE]   = License/SBOM/dependency check
[VM-SCAN]      = Requires VM environment (Kali/Lich for scanning)
[BARE-METAL]   = Requires real hardware/kernel/BPF
```

**Key change from v1**: `[BLOCKED]` is NO LONGER used for decision points. It only means "blocked by upstream `[STUCK]`." Decisions use `[DECIDE]` (autonomous) or `[ESCALATE]` (human required, rare).

**Exit Gate Convention**: Every phase ends with a gate. If the gate fails, DO NOT proceed. Debug in-phase (max 2 attempts, then Skip Protocol).

**Commit Convention**: `[C]` steps inserted every [N] steps. Never commit broken state — only after a passing `[V]`.

### [DECIDE] Step Format

```markdown
- [ ] **Step N** [DECIDE] ~1m: **[Decision description]**
  - **RECOMMENDATION**: [The answer]
  - **Rationale**: [Why]
  - **Override ONLY if**: [Specific evidence that contradicts]
  - Agent proceeds with recommendation. No stop.
```

Agent uses the recommendation. No stop. No ask. Override ONLY if the agent observes specific contradicting evidence listed in the step.

### [ESCALATE] Step Format

```markdown
- [ ] **Step N** [ESCALATE]: **[Decision description]**
  - **Context**: [What the agent knows]
  - **Why no recommendation**: [What makes this un-automatable]
  - STOP. Wait for human input.
```

---

## SITUATION REPORT

### What We Have (Built, Tested, Working)

| Component | Location | LOC | Status |
|-----------|----------|-----|--------|
| [Name]    | `$PROJECT_ROOT/path/` | X | ✅ GREEN / ⚠️ DEGRADED / ❌ BROKEN |

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

## PREFLIGHT HYPOTHESES

> **Scientist lens**: Every assumption the plan makes about current state must be verified before execution begins. Catches stale data (e.g., "plan says 45 opcodes but code has 48").

| # | Hypothesis | Verification Command | Expected | Correction if Wrong |
|---|-----------|---------------------|----------|-------------------|
| H1 | [Assumption about state] | `[command]` | [Expected output] | [What to do if wrong] |
| H2 | [Assumption about state] | `[command]` | [Expected output] | [What to do if wrong] |
| H3 | [Assumption about state] | `[command]` | [Expected output] | [What to do if wrong] |

> If ANY hypothesis fails and the correction changes the plan's scope, STOP and re-scope before proceeding. The plan is only valid when its assumptions are valid.

---

## KNOWN FAILURES BASELINE

> Pre-existing test failures captured BEFORE execution. Agents compare this baseline against the final test run. NEW failures = regression = investigate. Known failures = ignore.

| # | Package | Test Name | Failure Mode | Ticket | Since Sprint |
|---|---------|-----------|-------------|--------|-------------|
| F1 | [package] | [TestName] | [timeout/panic/assert] | [#N or N/A] | S[X] |
| F2 | [package] | [TestName] | [timeout/panic/assert] | [#N or N/A] | S[X] |

**Expected fail count at plan start**: [N]
**Rule**: If final fail count > baseline count, NEW failures exist. Investigate before handoff.

---

## PARALLEL MATRIX

> Replaces the old Agent Assignment Matrix. Includes dependency graph, cross-phase parallelization, and critical path per agent mode.

### Dependency Graph

```
Phase 0 ──→ Phase 1 ──→ Phase 3 ──→ Phase 5 ──→ Phase 7
                 │                        │
                 └──→ Phase 2 [P:1]       └──→ Phase 6 [P:5]
                 └──→ Phase 4 [P:1]
```

### Phase Assignment

| Phase | Title | Can Parallel With | Dependencies | Agent | Est. Time |
|-------|-------|------------------|-------------|-------|-----------|
| 0 | Intelligence & Preflight | None | None | Coordinator | 15m |
| 1 | [Title] | None | Phase 0 | Agent A | 45m |
| 2 | [Title] | Phase 1 [P:1] | Phase 0 | Agent B | 30m |
| ... | ... | ... | ... | ... | ... |

### Critical Path by Agent Mode

| Mode | Critical Path | Duration |
|------|--------------|----------|
| 1 agent | 0→1→2→3→4→5→6→7 (all sequential) | [X hours] |
| 2 agents | 0→1→3→5→7 (while B does 2→4→6) | [X hours] |
| 4 agents | 0→{1,2,4}→{3,5,6}→7 | [X hours] |

---

## PHASE 0: INTELLIGENCE & PREFLIGHT VERIFICATION (Steps 1-[N])

**Goal**: Resolve variables. Verify all preflight hypotheses. Establish baseline.
**Prerequisite**: None (this IS the prerequisite phase)
**Time**: ~15-20 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~30s: **Resolve $PROJECT_ROOT**
  ```bash
  PROJECT_ROOT=$(git rev-parse --show-toplevel) && echo "PROJECT_ROOT=$PROJECT_ROOT"
  ```
  - All subsequent commands use `$PROJECT_ROOT`

- [ ] **Step 2** [R] ~1m: **Read prior sprint handoff**
  ```bash
  cat $PROJECT_ROOT/docs/handoffs/S[N-1]-handoff.md | head -50
  ```
  - Note blockers, skipped items, known issues

- [ ] **Step 3** [B] ~30s: **Check git status and recent history**
  ```bash
  cd $PROJECT_ROOT && git status && git log --oneline -10
  ```
  - Expected: Clean working tree, recent commits visible

- [ ] **Step 4** [BUILD] ~30s: **Verify build health**
  ```bash
  cd $PROJECT_ROOT && go build ./... 2>&1 | tail -5
  ```
  - Expected: No errors

- [ ] **Step 5** [V] ~30s: **Verify test health**
  ```bash
  cd $PROJECT_ROOT && go test ./... -count=1 -timeout=120s 2>&1 | tail -20
  ```
  - Expected: All packages pass (or only KNOWN FAILURES BASELINE entries)
  - If fail → Step 5a [D]

- [ ] **Step 5a** [D] ~3m: **Debug test failures**
  ```bash
  cd $PROJECT_ROOT && go test ./... -count=1 -timeout=120s 2>&1 | grep "FAIL"
  ```
  - If pre-existing failures matching baseline: Note in log, continue
  - If NEW failures: STOP. Fix before proceeding.

- [ ] **Step 6** [ENV] ~1m: **Verify required toolchain**
  ```bash
  for tool in [list required tools]; do
    which $tool 2>/dev/null && echo "$tool: $(${tool} --version 2>/dev/null | head -1)" || echo "$tool: MISSING"
  done
  ```
  - If any MISSING → Step 6a [D]

- [ ] **Step 6a** [D] ~5m: **Install missing tools**
  ```bash
  # Platform-specific install commands (see APPENDIX A for fallback chains)
  ```
  - If unresolvable → [STUCK]

- [ ] **Step 7** [PREFLIGHT] ~3m: **Verify all preflight hypotheses**
  ```bash
  # Run each verification command from PREFLIGHT HYPOTHESES table
  # Compare results against Expected column
  # Execute Correction if Wrong for any mismatches
  ```
  - If any hypothesis fails with scope-changing correction → [ESCALATE]
  - If corrections are minor → apply and continue

- [ ] **Step 8** [C] ~30s: **COMMIT CHECKPOINT — Baseline verified**
  ```bash
  cd $PROJECT_ROOT && git add -A && git commit -m "[PLAN $SPRINT_ID] Steps 1-8: Environment verified, baseline established

  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step 9** [V]: **PHASE 0 EXIT GATE** — Environment confirmed
  - [ ] Build: GREEN
  - [ ] Tests: GREEN (or known pre-existing only)
  - [ ] Tools: All present
  - [ ] Git: Clean
  - [ ] Preflight hypotheses: All verified or corrected
  - [ ] Known failures baseline: Recorded
  - If pass → Phase 1
  - If fail → DO NOT PROCEED

### Phase 0 Definition of Done

- [ ] All `[B]` steps executed or `[STUCK]`-logged
- [ ] All `[V]` steps passed
- [ ] Zero NEW test failures (vs baseline)
- [ ] Commit checkpoint created
- [ ] Preflight hypotheses verified
- [ ] Known failures baseline recorded

---

## PHASE [N]: [TITLE IN CAPS] (Steps [X]-[Y])

**Goal**: [One sentence — what this phase achieves]
**Prerequisite**: Phase [N-1] EXIT GATE passed
**Time**: ~[X] minutes
**Agent**: [Coordinator | Agent | Agent [P] | Agent [P:N]]

### [Subsection Title]

- [ ] **Step [X]** [B] ~2m: **[Action description]**
  ```bash
  cd $PROJECT_ROOT && [exact command with relative paths and flags]
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

- [ ] **Step [X+2]** [DECIDE] ~1m: **[Decision description]**
  - **RECOMMENDATION**: [The answer]
  - **Rationale**: [Why]
  - **Override ONLY if**: [Specific evidence that contradicts]
  - Agent proceeds with recommendation. No stop.

- [ ] **Step [X+3]** [W] ~3m: **Create [filename]**
  ```bash
  cat << 'EOF' > $PROJECT_ROOT/path/to/file
  [file contents]
  EOF
  ```
  - Save to: `$PROJECT_ROOT/path/to/file`

- [ ] **Step [X+4]** [TEST] ~2m: **Run tests for [component]**
  ```bash
  cd $PROJECT_ROOT && go test ./pkg/[component]/... -race -count=1 -timeout=60s 2>&1 | tee /tmp/${SPRINT_ID}-phase[P]-tests.log
  grep "ok\|FAIL" /tmp/${SPRINT_ID}-phase[P]-tests.log
  ```
  - Expected: All ok, zero FAIL
  - If fail → Step [X+4]a [D]

- [ ] **Step [X+5]** [C] ~30s: **COMMIT CHECKPOINT**
  ```bash
  cd $PROJECT_ROOT && git add -A && git commit -m "[PLAN $SPRINT_ID] Steps {X}-{X+5}: [achievement summary]

  Phase {P}: {phase name}
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step [Y]** [V]: **PHASE [N] EXIT GATE** — [What must be true]
  - [ ] Check 1: [specific criterion with command]
  - [ ] Check 2: [specific criterion with command]
  - [ ] Check 3: [specific criterion with command]
  - If ALL pass → Phase [N+1]
  - If ANY fail → DO NOT PROCEED. Debug within this phase.

### Phase [N] Definition of Done

- [ ] All `[B]` steps executed or `[STUCK]`-logged
- [ ] All `[V]` steps passed
- [ ] Zero NEW test failures (vs baseline)
- [ ] Commit checkpoint created
- [ ] Audit doc updated (if applicable)
- [ ] No new dependencies without license check
- [ ] Attack surface unchanged or security-reviewed

---

## PHASE [S]: SECURITY REVIEW GATE (Steps [X]-[Y])

> **BlackMage lens**: Every battle plan that modifies network listeners, external inputs, trust boundaries, or cryptographic operations MUST include this phase.

**Goal**: Verify no new attack surface introduced by this sprint's changes.
**Prerequisite**: All implementation phases complete.
**Time**: ~15-20 minutes
**Agent**: Coordinator

- [ ] **Step [X]** [SECURITY] ~2m: **Detect trust boundary changes**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD -- '*.go' '*.rs' | grep -E '(Listen|Serve|Accept|Bind|net\.)' | head -20
  ```
  - If new listeners found → review each for authentication

- [ ] **Step [X+1]** [SECURITY] ~2m: **Detect new network listeners**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD -- '*.go' '*.rs' | grep -E '(http\.ListenAndServe|grpc\.NewServer|net\.Listen)' | head -20
  ```
  - Every new listener MUST have auth middleware

- [ ] **Step [X+2]** [SECURITY] ~2m: **Detect new external input handling**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD -- '*.go' '*.rs' | grep -E '(r\.URL|r\.Body|r\.Header|r\.Form|ParseForm|Decode\()' | head -20
  ```
  - Every new input MUST have validation

- [ ] **Step [X+3]** [SECURITY] ~2m: **Secret leak detection**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD | grep -iE '(password|secret|token|key|credential|apikey|api_key)' | grep -v '_test\.go' | head -20
  ```
  - Any matches in non-test files → investigate

- [ ] **Step [X+4]** [V]: **SECURITY GATE** — Attack surface review
  - [ ] No new unauthenticated endpoints
  - [ ] No weakened cryptography
  - [ ] No secret material in source or logs
  - [ ] All new external inputs validated
  - If pass → next phase
  - If fail → fix before proceeding

---

## PHASE [C]: COMPLIANCE GATE (Steps [X]-[Y])

> **Sentinel lens**: Every battle plan that adds dependencies, creates new source files, or modifies build configs MUST include this phase.

**Goal**: Verify licensing, SBOM, and compliance status of all changes.
**Prerequisite**: Implementation phases complete.
**Time**: ~10-15 minutes
**Agent**: Coordinator

- [ ] **Step [X]** [COMPLIANCE] ~2m: **Detect new dependencies**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD -- go.mod go.sum Cargo.toml Cargo.lock package.json | head -40
  ```
  - If new deps found → license audit required

- [ ] **Step [X+1]** [COMPLIANCE] ~3m: **License audit on new dependencies**
  ```bash
  # For each new dependency:
  # 1. Check license type (MIT/Apache/BSD = OK, GPL = boundary check)
  # 2. Verify no copyleft contamination of core
  cd $PROJECT_ROOT && go list -m -json all 2>/dev/null | grep -A2 '"Path"' | head -40
  ```
  - GPL dependencies MUST NOT link into core binary

- [ ] **Step [X+2]** [COMPLIANCE] ~2m: **SPDX header check on new files**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD --name-only --diff-filter=A -- '*.go' | while read f; do
    head -3 "$f" | grep -q "SPDX-License-Identifier" || echo "MISSING SPDX: $f"
  done
  ```
  - Every new .go file MUST have SPDX header

- [ ] **Step [X+3]** [COMPLIANCE] ~1m: **SBOM impact assessment**
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD -- go.mod | grep '^+[^+]' | wc -l
  ```
  - If new deps added → note for SBOM regeneration in post-execution

- [ ] **Step [X+4]** [V]: **COMPLIANCE GATE** — License and SBOM review
  - [ ] All new dependencies have compatible licenses
  - [ ] GPL boundary intact (zero copyleft in core)
  - [ ] SPDX headers on all new .go files
  - [ ] SBOM impact noted
  - If pass → next phase
  - If fail → fix before proceeding

---

## PHASE [LAST]: POST-EXECUTION (Steps [X]-[Y])

> **Mandatory final phase**. Replaces the old Handoff phase. Includes artifact regeneration, audit updates, doc updates, baseline comparison, and handoff document.

**Goal**: Regenerate artifacts, update docs, compare baselines, write handoff.
**Prerequisite**: All prior EXIT GATEs passed (or STUCK-logged). Security + Compliance gates passed.
**Time**: ~20-30 minutes
**Agent**: Coordinator

### Artifact Regeneration

- [ ] **Step [X]** [REGEN] ~3m: **Regenerate derived artifacts**
  ```bash
  cd $PROJECT_ROOT
  # Regenerate JSON/YAML from markdown sources
  # Regenerate any auto-generated code
  # Rebuild any derived configs
  ```
  - Verify generated files match sources

### Audit & Doc Updates

- [ ] **Step [X+1]** [AUDIT-UPDATE] ~2m: **Mark resolved findings in tracking docs**
  ```bash
  # Update $AUDIT_DOC with resolved items from this sprint
  # Mark any STUCK items as unresolved
  ```

- [ ] **Step [X+2]** [DOC-UPDATE] ~5m: **Update downstream documentation**
  ```bash
  # Update CLAUDE.md if architecture changed
  # Update timeline.md with sprint completion
  # Update wiki if applicable
  ```

### Baseline Comparison

- [ ] **Step [X+3]** [TEST] ~2m: **Final test run**
  ```bash
  cd $PROJECT_ROOT && go test ./... -count=1 -timeout=120s 2>&1 | tee /tmp/${SPRINT_ID}-final-tests.log
  tail -5 /tmp/${SPRINT_ID}-final-tests.log
  ```
  - Compare against KNOWN FAILURES BASELINE
  - NEW failures = regression = MUST investigate

- [ ] **Step [X+4]** [V] ~2m: **Coverage delta on changed packages** (Developer gate)
  ```bash
  cd $PROJECT_ROOT && go test ./pkg/[changed-packages]/... -coverprofile=/tmp/${SPRINT_ID}-cover.out 2>&1 | grep "coverage"
  ```
  - Coverage should not decrease on changed packages

- [ ] **Step [X+5]** [V] ~1m: **Blast radius assessment** (Architect gate)
  ```bash
  cd $PROJECT_ROOT && git diff $SPRINT_ID-start..HEAD --stat | tail -3
  ```
  - Verify changes are scoped to intended packages

### Review Stuck Steps

- [ ] **Step [X+6]** [R] ~2m: **Review stuck steps for handoff accuracy**
  ```bash
  grep -n "STUCK\|BLOCKED" $BATTLE_PLAN
  ```

### Handoff Document

- [ ] **Step [X+7]** [W] ~10m: **Write handoff document**
  ```bash
  cat << 'EOF' > $HANDOFF_DIR/${SPRINT_ID}-handoff.md
  # $SPRINT_ID Sprint Handoff

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

  ## Test Baseline Comparison
  - Preflight failures: [N]
  - Final failures: [N]
  - NEW regressions: [0 or list]

  ## Security Status
  - Security gate: [PASSED | N/A]
  - New endpoints: [list or none]
  - Attack surface change: [none | reviewed]

  ## Compliance Status
  - Compliance gate: [PASSED | N/A]
  - New dependencies: [list or none]
  - SPDX coverage: [100% | list missing]

  ## Next Sprint Should
  1. [Priority action]
  2. [Priority action]

  ## Metrics
  - Steps completed: X/Y (Z%)
  - Time elapsed: Xh Ym
  - Commits: N
  - Stuck steps: N
  EOF
  ```

- [ ] **Step [X+8]** [C] ~30s: **FINAL COMMIT**
  ```bash
  cd $PROJECT_ROOT && git add -A && git commit -m "[PLAN $SPRINT_ID] COMPLETE: Steps {1}-{Y} — [sprint achievement summary]

  Phases: {completed}/{total}
  Steps: {completed}/{total} ({percent}%)
  Stuck: {count} (see handoff)
  Co-Authored-By: Claude Opus 4.6 <noreply@anthropic.com>"
  ```

- [ ] **Step [Y]** [V]: **FINAL EXIT GATE**
  - [ ] Build: GREEN
  - [ ] Tests: GREEN (or baseline-only failures)
  - [ ] Handoff written
  - [ ] All STUCK steps documented
  - [ ] Derived artifacts regenerated
  - [ ] Audit doc updated
  - [ ] Downstream docs updated
  - [ ] Security gate passed (if applicable)
  - [ ] Compliance gate passed (if applicable)
  - [ ] Git log shows clean commit history

### Post-Execution Definition of Done

- [ ] All `[B]` steps executed or `[STUCK]`-logged
- [ ] All `[V]` steps passed
- [ ] Zero NEW test failures (vs baseline)
- [ ] Final commit created
- [ ] Handoff document written with security/compliance status
- [ ] All derived artifacts current

---

## STUCK REPORT (generated at runtime if skips occur)

**Progress**: [X]/[Y] steps completed ([Z]%)
**Stuck Steps**: [list]
**Blocked Steps**: [list of steps dependent on stuck steps]
**Completed After Skip**: [steps that succeeded despite skips]

### Stuck Step Detail

**Step [N]**: [description]
- **Symptom**: [what went wrong]
- **Attempted**: [debug steps tried]
- **Time Burned**: [duration before skip]
- **Downstream Impact**: Steps [A, B, C] blocked
- **Suggested Fix**: [agent's best guess]

### Recommended Intervention Order
1. Fix Step [X] first — unblocks [N] downstream steps
2. Fix Step [Y] next — unblocks [M] downstream steps

---

## APPENDIX A: RESILIENT TOOL INSTALLATION

> Every tool install uses a fallback chain. Tool failure does NOT block the plan.

### Fallback Pattern

```bash
install_tool() {
  local tool="$1"
  local primary="$2"
  local secondary="$3"
  local docker_image="$4"

  # Primary install (5 min timeout)
  timeout 300 bash -c "$primary" && return 0

  # Secondary install
  if [ -n "$secondary" ]; then
    timeout 300 bash -c "$secondary" && return 0
  fi

  # Docker fallback
  if [ -n "$docker_image" ] && command -v docker >/dev/null; then
    echo "$tool: using Docker fallback ($docker_image)"
    return 0
  fi

  # Skip — tool failure doesn't block plan
  echo "WARN: $tool unavailable — skipping dependent steps"
  return 1
}
```

### Common Tool Installs

| Tool | Primary | Secondary | Docker Fallback | Timeout |
|------|---------|-----------|----------------|---------|
| ScanCode | `pip install scancode-toolkit` | `pipx install scancode-toolkit` | `scancode-toolkit/scancode` | 5m |
| ORT | [project-specific] | — | `ort-docker` | 5m |
| bpftool | `apt install linux-tools-$(uname -r)` | `apt install bpftool` | — | 2m |

> `|| true` guards on all installs — a missing optional tool logs a warning but never halts the plan.

---

## APPENDIX B: EMERGENCY PROCEDURES

### B1: [Failure Mode Name]
**Symptom**: [What you see]
**Likely Cause**: [Root cause]
**Recovery**:
```bash
# Step-by-step recovery commands
```

### B2: [Failure Mode Name]
**Symptom**: [What you see]
**Likely Cause**: [Root cause]
**Recovery**:
```bash
# Step-by-step recovery commands
```

---

## APPENDIX C: KEY FILE PATHS

| Category | File | Purpose |
|----------|------|---------|
| Config   | `$PROJECT_ROOT/path/to/config` | [What it configures] |
| Source   | `$PROJECT_ROOT/path/to/source` | [What it implements] |
| Test     | `$PROJECT_ROOT/path/to/test`   | [What it tests] |

---

## APPENDIX D: QUICK REFERENCE

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
# VERSION: 2.0 (2026-03-19)
# CHANGELOG from v1:
#   1. VARIABLES block — eliminates hardcoded paths
#   2. Multi-agent time estimates — 1/2/4 agent columns
#   3. PREFLIGHT HYPOTHESES — verify assumptions before execution
#   4. KNOWN FAILURES BASELINE — distinguish regressions from pre-existing
#   5. PARALLEL MATRIX — replaces Agent Assignment Matrix
#   6. Per-phase Definition of Done — Micromanager gate
#   7. SECURITY REVIEW GATE phase — BlackMage lens
#   8. COMPLIANCE GATE phase — Sentinel lens
#   9. POST-EXECUTION phase — replaces old Handoff
#  10. APPENDIX A: Resilient Tool Installation — fallback chains
#  11. [DECIDE] replaces [BLOCKED] for decision points
#  12. [ESCALATE] for human-required decisions (rare)
#  13. New tags: [P:N], [SEQ], [PREFLIGHT], [REGEN], [AUDIT-UPDATE],
#      [DOC-UPDATE], [SECURITY], [COMPLIANCE], [VM-SCAN]
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
# DECISION PROTOCOL:
# [DECIDE] = agent proceeds with recommendation (default)
# [ESCALATE] = agent stops for human input (rare)
# [BLOCKED] = ONLY for upstream STUCK dependencies (never for decisions)
#
# EVERY [B] gets a [V]. EVERY phase gets an EXIT GATE.
# EVERY phase gets a Definition of Done.
# EVERY plan gets: LEGEND, VARIABLES, SITUATION REPORT,
#   PREFLIGHT HYPOTHESES, KNOWN FAILURES BASELINE,
#   PARALLEL MATRIX, SECURITY GATE, COMPLIANCE GATE,
#   POST-EXECUTION, and FORGE STAMP.
#
# The plan IS the product. Write every command.
# ═══════════════════════════════════════════
