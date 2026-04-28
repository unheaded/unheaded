# SPRINT 2026-04-27 BATTLE PLAN — Lanes A–H from Round Table

**Date**: 2026-04-27
**Sprint**: Round-Table Follow-Through — close P0 items from `battle-plan.md` (Lanes A–H)
**Prerequisite**: Round Table convened 2026-04-27, 19 seats reported, `battle-plan.md` committed at repo root with task queue
**Target**: By Fri 2026-05-01 EOD: timeline re-synced, off-tree plan imported, WAVE13 Phase 2 verdict logged, branches cleaned, Captain Track call published, ADR-051 + ADR-052 merged, SBOM/license/threat refreshed, spec-status swept, sprint-exit Round Table convened
**Estimated Duration**: 16–22 wall-clock hours across 5 working days (Mon–Fri)
**Agent Strategy**: Coordinator-led; Lanes B (training), C (branch audit), E (compliance scan) parallelizable; Lanes A→D→G→H critical path
**Commit Cadence**: Every 5 steps + every phase exit gate (formula: max(3, min(5, 200/20)) = 5)
**Stuck Protocol**: Skip after 3× time estimate or 2 failed debug attempts; preserve known-good state before skip; log STUCK marker; produce Stuck Report at sprint exit if any unresolved.

---

## LEGEND

```
[B] = Bash command (run directly)
[V] = Verification step (MUST pass before proceeding)
[D] = Debug step (only if prior step fails)
[W] = Write/create file
[R] = Read/inspect file
[S] = Sudo required
[P] = Parallelizable with other marked steps
[C] = Commit checkpoint (git commit at prescribed interval)
[STUCK] = Step skipped via Skip Protocol (needs human intervention)
[BLOCKED] = Step blocked by upstream STUCK step
```

---

## CRITICAL PATH

```
Phase 0 (Preflight) → Phase 1 (Lane A: Drift) → Phase 2 (Lane B: WAVE13 Q-call) →
Phase 4 (Lane D: Track call) → Phase 5 (Lane G: ADR-052 + CI guard) →
Phase 8 (Lane H: Sprint-exit Round Table)

Parallelizable side-paths:
  Phase 3 (Lane C: branch hygiene) parallel with Phase 2
  Phase 6 (Lane E: compliance) parallel with Phase 7 (Lane F: specs)
```

---

## PHASE 0: PREFLIGHT & INTEL GATHERING (Steps 1–15)

**Goal**: Verify ground truth before any change. Confirm HEAD, branch state, tool availability.
**Prerequisite**: Round Table battle-plan.md present at repo root.
**Time**: 30 minutes
**Agent**: Coordinator

- [ ] **Step 1** [B] ~10s: Confirm HEAD is WAVE13 Phase 1
  ```bash
  cd /home/govan/tmp/unheaded && git log -1 --oneline
  ```
- [ ] **Step 2** [V] ~5s: HEAD must contain `[PLAN W13] Phase 1` — if not, STOP, sync remote first
- [ ] **Step 3** [B] ~5s: Confirm working tree clean
  ```bash
  cd /home/govan/tmp/unheaded && git status -sb
  ```
- [ ] **Step 4** [V] ~5s: Working tree must be clean (no uncommitted changes). If dirty → stash or commit.
- [ ] **Step 5** [B] ~5s: Confirm Round Table battle-plan.md exists
  ```bash
  test -f /home/govan/tmp/unheaded/battle-plan.md && wc -l /home/govan/tmp/unheaded/battle-plan.md
  ```
- [ ] **Step 6** [V] ~5s: battle-plan.md must exist and be > 200 lines
- [ ] **Step 7** [B] ~5s: List branches, capture state
  ```bash
  cd /home/govan/tmp/unheaded && git branch -a > /tmp/sprint-branches-before.txt && cat /tmp/sprint-branches-before.txt
  ```
- [ ] **Step 8** [B] ~5s: Toolchain check
  ```bash
  for t in go cargo python3 jq syft cargo-deny go-licenses git; do which $t 2>/dev/null && $t --version 2>&1 | head -1 || echo "MISSING: $t"; done
  ```
- [ ] **Step 9** [V] ~5s: `go`, `cargo`, `python3`, `git` MUST be present. `syft`, `cargo-deny`, `go-licenses` install on demand in Phase 6.
- [ ] **Step 10** [D] ~3m: If `syft` missing, install: `curl -sSfL https://raw.githubusercontent.com/anchore/syft/main/install.sh | sh -s -- -b /usr/local/bin`
- [ ] **Step 11** [B] ~5s: Confirm accepted WAVE13 plan exists off-tree
  ```bash
  test -f ~/.claude/plans/synthetic-stirring-pudding.md && wc -l ~/.claude/plans/synthetic-stirring-pudding.md
  ```
- [ ] **Step 12** [V] ~5s: Off-tree plan MUST exist — that's the source for Lane A1
- [ ] **Step 13** [B] ~5s: Confirm timeline last-modified date
  ```bash
  stat -c '%y %n' /home/govan/tmp/unheaded/references/timeline.md
  ```
- [ ] **Step 14** [V] ~5s: Note the staleness (expected ≥ 30 days). Captures before/after metric.
- [ ] **Step 15** [V] ~5s: **PHASE 0 EXIT GATE** — HEAD clean at WAVE13 Phase 1, all critical tools present, off-tree WAVE13 plan exists. If pass → Phase 1.

---

## PHASE 1: LANE A — DRIFT & SOURCE-OF-TRUTH (Steps 16–40) — MON

**Goal**: Eliminate timeline drift; bring off-tree WAVE13 plan in-tree; update ADR-INDEX placeholders.
**Prerequisite**: Phase 0 GATE pass.
**Time**: 2 hours
**Agent**: Coordinator (Librarian + Timeguru)

### A1: Import WAVE13 plan in-tree

- [ ] **Step 16** [B] ~5s: Create target dir if missing
  ```bash
  mkdir -p /home/govan/tmp/unheaded/docs/battle-plans
  ```
- [ ] **Step 17** [B] ~5s: Copy accepted plan into repo
  ```bash
  cp ~/.claude/plans/synthetic-stirring-pudding.md /home/govan/tmp/unheaded/docs/battle-plans/WAVE13-INFERENCE.md
  ```
- [ ] **Step 18** [V] ~5s: Verify file in-tree and non-empty
  ```bash
  test -s /home/govan/tmp/unheaded/docs/battle-plans/WAVE13-INFERENCE.md && wc -l /home/govan/tmp/unheaded/docs/battle-plans/WAVE13-INFERENCE.md
  ```
- [ ] **Step 19** [W] ~5m: Add header to in-tree plan noting it's the canonical version (3 lines at top of file)
  - File: `docs/battle-plans/WAVE13-INFERENCE.md`
  - Prepend: `> **In-tree canonical WAVE13 plan.** Imported 2026-04-27 from off-tree draft.\n> Marshal-citable per ADR-052 (forthcoming): battle plans of record live here.\n\n`
- [ ] **Step 20** [C] ~10s: **COMMIT CHECKPOINT 1**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/battle-plans/WAVE13-INFERENCE.md && git commit -m "[PLAN SPRINT-04-27] Steps 16-20: import accepted WAVE13 plan in-tree" --no-gpg-sign
  ```

### A2: Update SUPERSEDED note in old draft

- [ ] **Step 21** [R] ~10s: Read existing SUPERSEDED note
  ```bash
  head -12 /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-inference-battle-plan.md
  ```
- [ ] **Step 22** [W] ~3m: Edit `crates/zhenai-forge/notes/wave13-inference-battle-plan.md` — change off-tree path reference to point to `docs/battle-plans/WAVE13-INFERENCE.md`
- [ ] **Step 23** [V] ~5s: Confirm pointer updated
  ```bash
  grep -n "docs/battle-plans/WAVE13-INFERENCE.md" /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-inference-battle-plan.md
  ```

### A3: Re-sync timeline.md

- [ ] **Step 24** [R] ~30s: Read current timeline state
  ```bash
  head -60 /home/govan/tmp/unheaded/references/timeline.md
  ```
- [ ] **Step 25** [B] ~5s: Backup before rewrite
  ```bash
  cp /home/govan/tmp/unheaded/references/timeline.md /home/govan/tmp/unheaded/references/timeline.md.pre-2026-04-27.bak
  ```
- [ ] **Step 26** [W] ~30m: Rewrite `references/timeline.md`:
  - Fix Age 5 contradictory flags (✅ COMPLETED + 📋 PLANNED on same line)
  - Mark Age 3 actual completion percentage based on remaining work in CLAUDE.md
  - Add Age 3 sub-items: WAVE10F (✅), WAVE11 (✅), WAVE12 (✅ Apr 23), WAVE13 (Phase 1 ✅, Phase 2 in progress)
  - Add "LAST UPDATED: 2026-04-27" header
  - Add reference to HEAD commit `5d413699`
- [ ] **Step 27** [V] ~10s: Confirm timeline updated
  ```bash
  head -10 /home/govan/tmp/unheaded/references/timeline.md && grep -c "2026-04-27" /home/govan/tmp/unheaded/references/timeline.md
  ```
- [ ] **Step 28** [V] ~5s: No "✅ COMPLETED" and "📋 PLANNED" on the same line
  ```bash
  ! grep "✅ COMPLETED.*📋 PLANNED\|📋 PLANNED.*✅ COMPLETED" /home/govan/tmp/unheaded/references/timeline.md
  ```
- [ ] **Step 29** [D] ~5m: If contradictions remain, re-edit. Skip Protocol after 2 attempts.
- [ ] **Step 30** [C] ~10s: **COMMIT CHECKPOINT 2**
  ```bash
  cd /home/govan/tmp/unheaded && git add references/timeline.md crates/zhenai-forge/notes/wave13-inference-battle-plan.md && git commit -m "[PLAN SPRINT-04-27] Steps 21-30: timeline.md re-sync + supersede pointer fix" --no-gpg-sign
  ```

### A4: Update ADR-INDEX with placeholders

- [ ] **Step 31** [R] ~10s: Read current ADR-INDEX
  ```bash
  cat /home/govan/tmp/unheaded/docs/adr/ADR-INDEX.md
  ```
- [ ] **Step 32** [W] ~5m: Append placeholder rows for ADR-051 (WAVE13 generate path) and ADR-052 (drift policy), status DRAFT
- [ ] **Step 33** [V] ~5s: Confirm both ADRs listed
  ```bash
  grep -E "ADR-05[12]" /home/govan/tmp/unheaded/docs/adr/ADR-INDEX.md
  ```
- [ ] **Step 34** [B] ~5s: Confirm SPDX header still present on ADR-INDEX.md (if .md SPDX policy applies — many repos skip)
  ```bash
  head -3 /home/govan/tmp/unheaded/docs/adr/ADR-INDEX.md
  ```
- [ ] **Step 35** [C] ~10s: **COMMIT CHECKPOINT 3**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/adr/ADR-INDEX.md && git commit -m "[PLAN SPRINT-04-27] Steps 31-35: ADR-INDEX placeholders for 051+052" --no-gpg-sign
  ```

### A5: Wiki sync check

- [ ] **Step 36** [B] ~5s: Identify wiki entries referencing old timeline state
  ```bash
  grep -l "Age 3.*85%\|Age 5.*COMPLETED" /home/govan/tmp/unheaded/wiki/*.md 2>/dev/null || echo "no stale wiki refs"
  ```
- [ ] **Step 37** [W] ~10m: If any wiki files flagged, update them to match new timeline. If none, skip.
- [ ] **Step 38** [V] ~5s: Re-run grep, expect zero hits
- [ ] **Step 39** [B] ~5s: List timeline.md mtime delta (was 33 days, should now be 0)
  ```bash
  stat -c '%y' /home/govan/tmp/unheaded/references/timeline.md
  ```
- [ ] **Step 40** [V] ~5s: **PHASE 1 EXIT GATE** — Off-tree plan imported, supersede pointer fixed, timeline re-synced (no contradictions, today's date), ADR-INDEX updated, wiki consistent. If pass → Phase 2 (or Phase 3 in parallel).

---

## PHASE 2: LANE B — WAVE13 PHASE 2 QUALITY CALL (Steps 41–75) — MON/TUE

**Goal**: Run base-vs-LoRA side-by-side on held-out Kingdom prompts; produce a number, not a vibe.
**Prerequisite**: Phase 1 GATE pass; `raft/kingdom-w12/kingdom.lora.gguf` present; `/var/zhen/models/gemma-4-E2B-it.gguf` present; `/tmp/24h-kingdom-eval.jsonl` present (re-tokenize if missing per accepted plan Appendix A1).
**Time**: 2–4 hours (1h forge build + warmup, 1h generation, 30m analysis)
**Agent**: Coordinator (Developer + Scientist)

### B1: Prerequisite assets

- [ ] **Step 41** [B] ~5s: Check Kingdom LoRA on disk
  ```bash
  ls -lh /home/govan/tmp/unheaded/raft/kingdom-w12/kingdom.lora.gguf
  ```
- [ ] **Step 42** [V] ~5s: File exists and is non-zero
- [ ] **Step 43** [B] ~5s: Check base model
  ```bash
  ls -lh /var/zhen/models/gemma-4-E2B-it.gguf
  ```
- [ ] **Step 44** [V] ~5s: File exists ~9.3GB
- [ ] **Step 45** [B] ~5s: Check held-out eval set
  ```bash
  ls -lh /tmp/24h-kingdom-eval.jsonl && head -1 /tmp/24h-kingdom-eval.jsonl | jq -c 'keys'
  ```
- [ ] **Step 46** [D] ~10m: If missing, re-tokenize per accepted plan Appendix A1
  ```bash
  cd /home/govan/tmp/unheaded && /home/govan/tmp/gemma4-venv/bin/python scripts/tokenize-kingdom-for-gemma4.py --eval-out /tmp/24h-kingdom-eval.jsonl
  ```

### B2: Build forge with WAVE13 generate-gemma4

- [ ] **Step 47** [B] ~3m: Build release forge
  ```bash
  cd /home/govan/tmp/unheaded/crates/zhenai-forge && cargo build --release 2>&1 | tail -20
  ```
- [ ] **Step 48** [V] ~5s: Build success, binary exists
  ```bash
  test -x /home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge
  ```
- [ ] **Step 49** [D] ~10m: If build fails, capture error, run `cargo build 2>&1 | grep "^error"`. Skip Protocol after 2 attempts.
- [ ] **Step 50** [C] ~10s: **COMMIT CHECKPOINT 4** (no source changes — checkpoint is informational; skip if `git diff` empty)
  ```bash
  cd /home/govan/tmp/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27] Steps 46-50: forge build artifacts" --no-gpg-sign || echo "no changes to commit"
  ```

### B3: Pick 5–10 held-out prompts

- [ ] **Step 51** [B] ~5s: Sample 8 random prompts from held-out
  ```bash
  shuf -n 8 /tmp/24h-kingdom-eval.jsonl > /tmp/wave13-phase2-prompts.jsonl && wc -l /tmp/wave13-phase2-prompts.jsonl
  ```
- [ ] **Step 52** [V] ~5s: Confirm 8 prompts captured
- [ ] **Step 53** [W] ~2m: Create scratch dir for results
  ```bash
  mkdir -p /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality && cd /home/govan/tmp/unheaded
  ```

### B4: Generate base + LoRA outputs (loop x8)

- [ ] **Step 54** [B] ~30m: Generate BASE outputs for all 8 prompts (no LoRA flag)
  ```bash
  i=0; while IFS= read -r line; do
    i=$((i+1)); echo "$line" | jq -r '.tokens | tostring' > /tmp/p$i.tokens.json
    /home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge generate-gemma4 \
      --model /var/zhen/models/gemma-4-E2B-it.gguf \
      --tokens /tmp/p$i.tokens.json \
      --max-new-tokens 80 --temperature 0.0 \
      > /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality/base-p$i.txt 2>&1
  done < /tmp/wave13-phase2-prompts.jsonl
  ```
- [ ] **Step 55** [V] ~10s: 8 base output files exist
  ```bash
  ls /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality/base-p*.txt | wc -l
  ```
- [ ] **Step 56** [B] ~30m: Generate LoRA outputs (with `--lora` flag)
  ```bash
  i=0; while IFS= read -r line; do
    i=$((i+1));
    /home/govan/tmp/unheaded/crates/zhenai-forge/target/release/zhenai-forge generate-gemma4 \
      --model /var/zhen/models/gemma-4-E2B-it.gguf \
      --lora /home/govan/tmp/unheaded/raft/kingdom-w12/kingdom.lora.gguf \
      --tokens /tmp/p$i.tokens.json \
      --max-new-tokens 80 --temperature 0.0 \
      > /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality/lora-p$i.txt 2>&1
  done < /tmp/wave13-phase2-prompts.jsonl
  ```
- [ ] **Step 57** [V] ~10s: 8 LoRA output files exist
  ```bash
  ls /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality/lora-p*.txt | wc -l
  ```
- [ ] **Step 58** [D] ~15m: If any generation hangs > 5min, kill and skip that prompt; log STUCK on that prompt only.
- [ ] **Step 59** [C] ~10s: **COMMIT CHECKPOINT 5**
  ```bash
  cd /home/govan/tmp/unheaded && git add crates/zhenai-forge/notes/wave13-phase2-quality/ && git commit -m "[PLAN SPRINT-04-27] Steps 54-59: WAVE13 Phase 2 raw base+LoRA outputs (8 prompts)" --no-gpg-sign
  ```

### B5: Decode tokens to text

- [ ] **Step 60** [B] ~5m: Decode all outputs through gemma4-venv tokenizer
  ```bash
  cd /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality && for f in base-p*.txt lora-p*.txt; do
    /home/govan/tmp/gemma4-venv/bin/python /home/govan/tmp/unheaded/scripts/gemma4-decode-tokens.py < $f > ${f%.txt}.decoded.txt 2>&1 || echo "decode-failed: $f"
  done
  ```
- [ ] **Step 61** [V] ~5s: 16 decoded files exist
  ```bash
  ls /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality/*.decoded.txt | wc -l
  ```

### B6: Analysis — produce a NUMBER

- [ ] **Step 62** [W] ~30m: Create `crates/zhenai-forge/notes/wave13-phase2-quality.md` with:
  - For each of 8 prompts: prompt-id, prompt-text-snippet, base-output-snippet, lora-output-snippet
  - Per-prompt qualitative tag: {coherent, mode-collapsed, multilingual-noise, gibberish, kingdom-relevant}
  - Aggregate: count of "kingdom-relevant" base vs LoRA
  - **Win-rate** number: (LoRA-better count) / 8
  - **Mean qualitative shift** vs WAVE12 CE delta (−14.32) — does qualitative match the CE drop?
- [ ] **Step 63** [V] ~10s: Document contains an explicit win-rate number
  ```bash
  grep -E "win-rate|Win-rate|WIN-RATE" /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality.md
  ```
- [ ] **Step 64** [V] ~10s: Document contains all 8 prompt rows
  ```bash
  grep -cE "^### Prompt [0-9]" /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality.md
  ```

### B7: DECIDE node (Phase 3 of WAVE13)

- [ ] **Step 65** [W] ~15m: Append `## Decision` section to `wave13-phase2-quality.md`:
  - Verdict: SHIP / RETRAIN / RANK-UP / DATA-FIX
  - Rationale: cite numbers from B6
  - Owner of next action
- [ ] **Step 66** [V] ~5s: Decision section exists and verdict is one of the four canonical values
  ```bash
  grep -E "Verdict:.*(SHIP|RETRAIN|RANK-UP|DATA-FIX)" /home/govan/tmp/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality.md
  ```
- [ ] **Step 67** [C] ~10s: **COMMIT CHECKPOINT 6**
  ```bash
  cd /home/govan/tmp/unheaded && git add crates/zhenai-forge/notes/wave13-phase2-quality.md crates/zhenai-forge/notes/wave13-phase2-quality/ && git commit -m "[PLAN SPRINT-04-27] Steps 60-67: WAVE13 Phase 2 quality verdict" --no-gpg-sign
  ```

### B8: Draft ADR-051 with the verdict

- [ ] **Step 68** [W] ~20m: Create `docs/adr/ADR-051-wave13-generate-path.md`
  - Status: ACCEPTED / PROPOSED based on Step 65 verdict
  - Context: WAVE13 Phase 1 generate path; Phase 2 quality finding
  - Decision: cite Step 65 verdict
  - Consequences: KV-cache deferral; downstream WAVE14 implications
- [ ] **Step 69** [V] ~5s: ADR-051 file exists, has SPDX header (if convention applies), references HEAD commit
  ```bash
  test -s /home/govan/tmp/unheaded/docs/adr/ADR-051-wave13-generate-path.md && head -3 /home/govan/tmp/unheaded/docs/adr/ADR-051-wave13-generate-path.md
  ```
- [ ] **Step 70** [W] ~3m: Update ADR-INDEX.md: ADR-051 status changed from DRAFT → ACCEPTED/PROPOSED
- [ ] **Step 71** [V] ~5s: Index reflects new status
- [ ] **Step 72** [B] ~30s: Run forge regression tests (sanity)
  ```bash
  cd /home/govan/tmp/unheaded/crates/zhenai-forge && cargo test --release --lib 2>&1 | tail -10
  ```
- [ ] **Step 73** [V] ~5s: All tests pass (zero failures)
- [ ] **Step 74** [C] ~10s: **COMMIT CHECKPOINT 7**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/adr/ADR-051-wave13-generate-path.md docs/adr/ADR-INDEX.md && git commit -m "[PLAN SPRINT-04-27] Steps 68-74: ADR-051 WAVE13 generate path" --no-gpg-sign
  ```
- [ ] **Step 75** [V] ~5s: **PHASE 2 EXIT GATE** — wave13-phase2-quality.md has win-rate number + verdict; ADR-051 drafted; tests green. If pass → Phase 3 (already in progress in parallel) and Phase 4.

---

## PHASE 3: LANE C — BRANCH HYGIENE (Steps 76–100) — TUE [P with Phase 2]

**Goal**: Audit and resolve 3 stale branches: merge or kill, with rationale.
**Prerequisite**: Phase 1 GATE pass; can run in parallel with Phase 2.
**Time**: 1.5 hours
**Agent**: Coordinator (Developer + Marshal)

### C1: Audit `claude/migrate-packages-github-V2Ctr`

- [ ] **Step 76** [B] ~10s: Diff branch vs main
  ```bash
  cd /home/govan/tmp/unheaded && git log main..claude/migrate-packages-github-V2Ctr --oneline | head -30
  ```
- [ ] **Step 77** [B] ~10s: File-level diff summary
  ```bash
  cd /home/govan/tmp/unheaded && git diff main...claude/migrate-packages-github-V2Ctr --stat | tail -20
  ```
- [ ] **Step 78** [W] ~10m: Write summary at `docs/branch-audits/2026-04-27-claude-migrate-packages-V2Ctr.md`
  - Branch age, commit count, file count
  - Diff topic (package migration to github.com/unheaded/*)
  - Verdict: MERGE / KILL / DEFER (with rationale)
- [ ] **Step 79** [V] ~5s: Audit doc exists
  ```bash
  test -s /home/govan/tmp/unheaded/docs/branch-audits/2026-04-27-claude-migrate-packages-V2Ctr.md
  ```

### C2: Audit `docs/s73-public-launch-planning`

- [ ] **Step 80** [B] ~10s: Diff vs main
  ```bash
  cd /home/govan/tmp/unheaded && git log main..docs/s73-public-launch-planning --oneline | head -30 || echo "no remote diff"
  ```
- [ ] **Step 81** [W] ~10m: Write `docs/branch-audits/2026-04-27-s73-public-launch.md`
  - Verdict: MERGE / KILL / DEFER

### C3: Audit `public-release-cleanup`

- [ ] **Step 82** [B] ~10s: Diff vs main
  ```bash
  cd /home/govan/tmp/unheaded && git log main..public-release-cleanup --oneline | head -30
  ```
- [ ] **Step 83** [W] ~10m: Write `docs/branch-audits/2026-04-27-public-release-cleanup.md`
  - Verdict: MERGE / KILL / DEFER
- [ ] **Step 84** [C] ~10s: **COMMIT CHECKPOINT 8** (audits committed before any branch action)
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/branch-audits/ && git commit -m "[PLAN SPRINT-04-27] Steps 76-84: 3 branch audit reports written" --no-gpg-sign
  ```

### C4: Execute decisions

- [ ] **Step 85** [R] ~2m: Read all three verdicts
  ```bash
  for f in /home/govan/tmp/unheaded/docs/branch-audits/2026-04-27-*.md; do echo "==> $f"; grep -E "^Verdict:" $f; done
  ```
- [ ] **Step 86** [B] ~10s: For any MERGE verdict, ensure tests green BEFORE merge
  ```bash
  cd /home/govan/tmp/unheaded && git fetch --all 2>&1 | tail -5
  ```
- [ ] **Step 87** [B] ~varies: Execute merges (only branches with MERGE verdict)
  ```bash
  # Per-branch: cd /home/govan/tmp/unheaded && git checkout main && git merge --no-ff <branch> -m "merge: <branch> per audit 2026-04-27" && git push
  ```
  *(Step performs only declared merges; non-MERGE branches skipped to Step 88)*
- [ ] **Step 88** [B] ~10s: For KILL verdicts, archive branch ref before delete
  ```bash
  # Per-branch: cd /home/govan/tmp/unheaded && git tag archived/<branch>-2026-04-27 <branch> && git branch -D <branch> && git push origin :<branch> 2>/dev/null || true
  ```
- [ ] **Step 89** [V] ~5s: Confirm post-state branch list
  ```bash
  cd /home/govan/tmp/unheaded && git branch -a > /tmp/sprint-branches-after.txt && diff /tmp/sprint-branches-before.txt /tmp/sprint-branches-after.txt || true
  ```
- [ ] **Step 90** [C] ~10s: **COMMIT CHECKPOINT 9**
  ```bash
  cd /home/govan/tmp/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27] Steps 85-90: branch hygiene executed" --no-gpg-sign || echo "merge-only changes pushed"
  ```

### C5: Verify build still green post-merge

- [ ] **Step 91** [B] ~5m: Full Go build
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... 2>&1 | tail -20
  ```
- [ ] **Step 92** [V] ~5s: Build exits zero. If non-zero → revert merges (`git reset --hard ORIG_HEAD`) and STUCK.
- [ ] **Step 93** [B] ~10m: Go tests
  ```bash
  cd /home/govan/tmp/unheaded && go test ./... -short -timeout 5m 2>&1 | tail -30
  ```
- [ ] **Step 94** [V] ~5s: Test exits zero. Same revert protocol if not.
- [ ] **Step 95** [B] ~3m: Rust workspace test (forge + critical crates only — skip slow ones)
  ```bash
  cd /home/govan/tmp/unheaded && cargo test --release --workspace --exclude doom-runner -- --test-threads 4 2>&1 | tail -20
  ```
- [ ] **Step 96** [V] ~5s: Cargo tests pass
- [ ] **Step 97** [D] ~10m: If any test fails, identify owner crate. Skip Protocol after 2 fixes.
- [ ] **Step 98** [B] ~5s: Final branch state
  ```bash
  cd /home/govan/tmp/unheaded && git branch -a
  ```
- [ ] **Step 99** [V] ~5s: Branch count reduced from 4 to ≤2 active (main + at most 1 long-lived). Tags `archived/*` retain history of killed branches.
- [ ] **Step 100** [V] ~5s: **PHASE 3 EXIT GATE** — All 3 audited branches resolved, build green, tests green, archived tags present for any kills. If pass → Phase 4.

---

## PHASE 4: LANE D — CAPTAIN TRACK CALL (Steps 101–120) — WED

**Goal**: Stevie picks Track A (forge-first), B (launch-first), or C (twin-track). Decision documented and propagated.
**Prerequisite**: Phase 2 verdict + ADR-051 in hand; Phase 3 branches resolved.
**Time**: 2 hours (mostly thinking + drafting; minutes of execution)
**Agent**: Captain → Stevie → Micromanager

### D1: Captain drafts A/B/C analysis

- [ ] **Step 101** [W] ~45m: Create `docs/decisions/2026-04-29-track-call-options.md`
  - Track A (forge-first): cost, time-to-launch, customer impact, dependency on Phase 2 verdict
  - Track B (launch-first): cost, forge-pause cost, demo-readiness, README polish gap
  - Track C (twin-track): coordination cost, parallel work feasibility, Stevie bandwidth
  - Recommendation (Captain): default position
- [ ] **Step 102** [V] ~5s: File exists, contains all 3 tracks + recommendation
  ```bash
  grep -cE "^## Track [ABC]" /home/govan/tmp/unheaded/docs/decisions/2026-04-29-track-call-options.md
  ```
- [ ] **Step 103** [V] ~5s: Should equal 3
- [ ] **Step 104** [C] ~10s: **COMMIT CHECKPOINT 10**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/decisions/ && git commit -m "[PLAN SPRINT-04-27] Steps 101-104: Track A/B/C analysis drafted" --no-gpg-sign
  ```

### D2: Stevie reviews + decides

- [ ] **Step 105** [R] ~30m: Stevie reads options doc, considers
- [ ] **Step 106** [W] ~10m: Stevie writes decision at `docs/decisions/2026-04-29-track-call.md`
  - Chosen track: A / B / C
  - Rationale (1-2 paragraphs)
  - Date locked
- [ ] **Step 107** [V] ~5s: File exists, exactly one of A/B/C named
  ```bash
  grep -E "Chosen track:.*[ABC]" /home/govan/tmp/unheaded/docs/decisions/2026-04-29-track-call.md
  ```

### D3: Propagate to sprint backlog

- [ ] **Step 108** [W] ~20m: Micromanager updates `references/timeline.md` with track-aligned next sprint backlog
- [ ] **Step 109** [W] ~15m: Update `battle-plan.md` (Round Table doc) with track-aligned Lane I priorities
- [ ] **Step 110** [V] ~5s: Confirm timeline + battle-plan reference Track decision
  ```bash
  grep -l "Track [ABC]\|2026-04-29-track-call" /home/govan/tmp/unheaded/references/timeline.md /home/govan/tmp/unheaded/battle-plan.md
  ```
- [ ] **Step 111** [C] ~10s: **COMMIT CHECKPOINT 11**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/decisions/ references/timeline.md battle-plan.md && git commit -m "[PLAN SPRINT-04-27] Steps 105-111: Track call locked + propagated" --no-gpg-sign
  ```

### D4: WAVE14 plan stub if Track A or C

- [ ] **Step 112** [R] ~5s: Read decision again
  ```bash
  grep -E "Chosen track:" /home/govan/tmp/unheaded/docs/decisions/2026-04-29-track-call.md
  ```
- [ ] **Step 113** [W] ~15m: If A or C, draft `docs/battle-plans/WAVE14-STUB.md` covering BackwardScratch + KV-cache scope. If B, skip to Step 116.
- [ ] **Step 114** [V] ~5s: WAVE14 stub exists OR explicit "deferred per Track B" log entry exists
- [ ] **Step 115** [C] ~10s: **COMMIT CHECKPOINT 12**
  ```bash
  cd /home/govan/tmp/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27] Steps 112-115: WAVE14 stub or Track-B defer" --no-gpg-sign
  ```

### D5: Demo/README scope gate if Track B or C

- [ ] **Step 116** [R] ~5s: Re-read decision
- [ ] **Step 117** [W] ~10m: If B or C, draft `docs/launch/2026-04-29-demo-script-v0.md` outline (Lane I1). If A, defer.
- [ ] **Step 118** [V] ~5s: Outline exists OR explicit "deferred per Track A" log
- [ ] **Step 119** [C] ~10s: **COMMIT CHECKPOINT 13**
  ```bash
  cd /home/govan/tmp/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27] Steps 116-119: demo script outline or Track-A defer" --no-gpg-sign
  ```
- [ ] **Step 120** [V] ~5s: **PHASE 4 EXIT GATE** — Track call locked, propagation complete, conditional WAVE14 + demo stubs handled. If pass → Phase 5.

---

## PHASE 5: LANE G — ADR-052 + CI DRIFT GUARD (Steps 121–140) — WED

**Goal**: Encode "drift ≤ 7 days" as policy + automated CI check.
**Prerequisite**: Phase 4 GATE pass.
**Time**: 1.5 hours
**Agent**: Coordinator (RFC-Editor + Marshal + Developer)

### G1: Draft ADR-052

- [ ] **Step 121** [W] ~30m: Create `docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md`
  - Status: ACCEPTED
  - Context: 33-day timeline drift detected 2026-04-27 Round Table; off-tree plan SOP violation
  - Decision: timeline.md + active battle plans must be ≤7 days from HEAD if HEAD has new commits; battle plans of record live in-tree under `docs/battle-plans/`
  - Consequences: CI gate (Step 124+); Marshal-citable
- [ ] **Step 122** [V] ~5s: ADR-052 exists, status ACCEPTED
  ```bash
  grep -E "^Status:.*ACCEPTED" /home/govan/tmp/unheaded/docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md
  ```
- [ ] **Step 123** [W] ~3m: Update ADR-INDEX status DRAFT → ACCEPTED for ADR-052

### G2: CI drift-guard script

- [ ] **Step 124** [W] ~30m: Create `scripts/check-timeline-freshness.sh`
  - Inputs: TIMELINE_PATH=references/timeline.md, MAX_AGE_DAYS=7
  - Logic:
    - HEAD_DATE = `git log -1 --format=%ct HEAD`
    - TIMELINE_DATE = `git log -1 --format=%ct -- $TIMELINE_PATH`
    - If HEAD_DATE - TIMELINE_DATE > 7*86400 AND HEAD has commits since TIMELINE last touch → exit 1 with citation message
  - Mode `--check` (CI) and `--report` (info-only)
- [ ] **Step 125** [B] ~10s: Make executable
  ```bash
  chmod +x /home/govan/tmp/unheaded/scripts/check-timeline-freshness.sh
  ```
- [ ] **Step 126** [B] ~10s: Local smoke test
  ```bash
  cd /home/govan/tmp/unheaded && ./scripts/check-timeline-freshness.sh --report
  ```
- [ ] **Step 127** [V] ~5s: Script outputs current age in days, exits 0 (timeline fresh after Phase 1)

### G3: Wire into CI

- [ ] **Step 128** [R] ~3m: Read existing GHA workflow files
  ```bash
  ls /home/govan/tmp/unheaded/.github/workflows/ 2>&1
  ```
- [ ] **Step 129** [W] ~15m: Add drift-guard step to existing CI workflow (or create `.github/workflows/drift-guard.yml`)
  - Trigger: pull_request, push to main
  - Step: `./scripts/check-timeline-freshness.sh --check`
- [ ] **Step 130** [V] ~5s: YAML syntax-valid
  ```bash
  python3 -c "import yaml; yaml.safe_load(open('/home/govan/tmp/unheaded/.github/workflows/drift-guard.yml'))" 2>&1 || echo "OR" && python3 -c "import yaml; [yaml.safe_load(open(f)) for f in __import__('glob').glob('/home/govan/tmp/unheaded/.github/workflows/*.yml')]"
  ```
- [ ] **Step 131** [B] ~5m: Equivalent in Jenkinsfile (if Jenkins active)
  ```bash
  grep -n "check-timeline-freshness\|drift-guard" /home/govan/tmp/unheaded/Jenkinsfile || echo "needs-add"
  ```
- [ ] **Step 132** [W] ~10m: If `needs-add`, add Jenkinsfile stage
- [ ] **Step 133** [V] ~5s: Both CIs reference the script
- [ ] **Step 134** [C] ~10s: **COMMIT CHECKPOINT 14**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md docs/adr/ADR-INDEX.md scripts/check-timeline-freshness.sh .github/workflows/ Jenkinsfile && git commit -m "[PLAN SPRINT-04-27] Steps 121-134: ADR-052 + CI drift guard" --no-gpg-sign
  ```

### G4: Pre-commit hook (optional, local)

- [ ] **Step 135** [R] ~10s: Check existing pre-commit hooks
  ```bash
  ls /home/govan/tmp/unheaded/.git/hooks/pre-commit 2>&1 || echo "no hook"
  ```
- [ ] **Step 136** [W] ~10m: If absent or doesn't include drift check, add invocation of drift-guard script with `--report` (warn only locally; CI is the hard gate)
- [ ] **Step 137** [V] ~5s: Hook present and executable
  ```bash
  test -x /home/govan/tmp/unheaded/.git/hooks/pre-commit
  ```
- [ ] **Step 138** [B] ~10s: Smoke-test hook
  ```bash
  cd /home/govan/tmp/unheaded && .git/hooks/pre-commit 2>&1 | tail -10
  ```
- [ ] **Step 139** [V] ~5s: Hook runs without erroring on the current clean tree
- [ ] **Step 140** [V] ~5s: **PHASE 5 EXIT GATE** — ADR-052 ACCEPTED, drift-guard script runnable, CI references script, optional local hook installed. If pass → Phase 6 (parallelizable with Phase 7).

---

## PHASE 6: LANE E — COMPLIANCE REFRESH (Steps 141–165) — THU [P with Phase 7]

**Goal**: Re-run SBOM, license scan, threat-register pull, SPDX coverage check.
**Prerequisite**: Phase 5 GATE pass.
**Time**: 2 hours
**Agent**: Coordinator (MoatGhost + Sentinel + Barrister)

### E1: Syft SBOM regen

- [ ] **Step 141** [B] ~5m: Run Syft scan
  ```bash
  cd /home/govan/tmp/unheaded && syft dir:. -o spdx-json > /tmp/sbom-2026-04-27.spdx.json 2>&1
  ```
- [ ] **Step 142** [V] ~5s: SBOM file non-empty, valid JSON
  ```bash
  jq 'has("packages")' /tmp/sbom-2026-04-27.spdx.json
  ```
- [ ] **Step 143** [B] ~10s: Count packages
  ```bash
  jq '.packages | length' /tmp/sbom-2026-04-27.spdx.json
  ```
- [ ] **Step 144** [V] ~5s: Compare to CLAUDE.md "553 deps" baseline; flag any net new
- [ ] **Step 145** [W] ~5m: Save SBOM to repo at `docs/legal/sbom-2026-04-27.spdx.json`
  ```bash
  cp /tmp/sbom-2026-04-27.spdx.json /home/govan/tmp/unheaded/docs/legal/sbom-2026-04-27.spdx.json
  ```

### E2: License scan

- [ ] **Step 146** [B] ~3m: cargo-deny advisories + licenses
  ```bash
  cd /home/govan/tmp/unheaded && cargo deny check licenses advisories 2>&1 | tail -40 > /tmp/cargo-deny-2026-04-27.txt
  ```
- [ ] **Step 147** [V] ~5s: Zero "error[]" lines (warnings ok)
  ```bash
  ! grep -E "^error\[" /tmp/cargo-deny-2026-04-27.txt
  ```
- [ ] **Step 148** [B] ~3m: Go license check
  ```bash
  cd /home/govan/tmp/unheaded && go-licenses report ./... > /tmp/go-licenses-2026-04-27.csv 2>&1 || echo "go-licenses not present, skip"
  ```
- [ ] **Step 149** [V] ~10s: If go-licenses ran, no GPL deps in non-GPL dir; if it didn't, log STUCK on E2 and continue.
- [ ] **Step 150** [C] ~10s: **COMMIT CHECKPOINT 15**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/legal/sbom-2026-04-27.spdx.json && git commit -m "[PLAN SPRINT-04-27] Steps 141-150: SBOM + license scan refresh" --no-gpg-sign
  ```

### E3: Threat register refresh

- [ ] **Step 151** [B] ~3m: Pull last-7-days CISA KEV deltas
  ```bash
  curl -sSL https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json | jq '.vulnerabilities[] | select(.dateAdded >= "2026-04-20")' > /tmp/cisa-kev-recent.json
  ```
- [ ] **Step 152** [B] ~10s: Filter to kernel/eBPF/HIP/llama.cpp relevant
  ```bash
  jq -c 'select(.product | test("Linux|kernel|eBPF|HIP|ROCm|llama"; "i"))' /tmp/cisa-kev-recent.json | tee /tmp/cisa-kev-relevant.json | wc -l
  ```
- [ ] **Step 153** [W] ~15m: Update `docs/security/threat-register.md` with relevant rows + date stamp 2026-04-27
- [ ] **Step 154** [V] ~5s: Threat register has 2026-04-27 timestamp
  ```bash
  grep "2026-04-27" /home/govan/tmp/unheaded/docs/security/threat-register.md
  ```

### E4: SPDX coverage verification

- [ ] **Step 155** [B] ~30s: Find Go files without SPDX header
  ```bash
  cd /home/govan/tmp/unheaded && grep -L "SPDX-License-Identifier:" $(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*") 2>/dev/null | tee /tmp/spdx-missing-go.txt | wc -l
  ```
- [ ] **Step 156** [V] ~5s: Count must be 0
- [ ] **Step 157** [D] ~30m: If count > 0, run SPDX-add script to backfill (mirror existing `scripts/add-spdx.sh` pattern). Skip Protocol after 2 attempts.
- [ ] **Step 158** [B] ~30s: Same for Rust files
  ```bash
  cd /home/govan/tmp/unheaded && grep -L "SPDX-License-Identifier:" $(find . -name "*.rs" -not -path "./target/*" -not -path "./.git/*") 2>/dev/null | tee /tmp/spdx-missing-rs.txt | wc -l
  ```
- [ ] **Step 159** [V] ~5s: Note count (Rust SPDX policy may differ; if zero needed, fine)

### E5: Compliance summary

- [ ] **Step 160** [W] ~15m: Write `docs/security/compliance-snapshot-2026-04-27.md`
  - SBOM: count, delta vs baseline
  - License scan: cargo-deny status, go-licenses status
  - Threat register: # new rows, severity distribution
  - SPDX coverage: Go count, Rust count
  - SOC 2 Y1 trajectory note
- [ ] **Step 161** [V] ~5s: Snapshot exists
  ```bash
  test -s /home/govan/tmp/unheaded/docs/security/compliance-snapshot-2026-04-27.md
  ```
- [ ] **Step 162** [B] ~10s: Confirm all compliance artifacts ≤ 7 days old
  ```bash
  for f in docs/legal/sbom-2026-04-27.spdx.json docs/security/threat-register.md docs/security/compliance-snapshot-2026-04-27.md; do
    cd /home/govan/tmp/unheaded && stat -c '%y %n' $f
  done
  ```
- [ ] **Step 163** [V] ~5s: All three artifacts have today's date
- [ ] **Step 164** [C] ~10s: **COMMIT CHECKPOINT 16**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/security/ docs/legal/ && git commit -m "[PLAN SPRINT-04-27] Steps 151-164: threat register + SPDX + compliance snapshot" --no-gpg-sign
  ```
- [ ] **Step 165** [V] ~5s: **PHASE 6 EXIT GATE** — SBOM regen, license scan, threat refresh, SPDX coverage all green and committed. If pass → Phase 8 (or run Phase 7 in parallel if didn't already).

---

## PHASE 7: LANE F — SPEC STATUS SWEEP (Steps 166–185) — THU [P with Phase 6]

**Goal**: Confirm Sophia draft-03, Wotan draft-03, Anamnesis spec-00 statuses; ship-or-defer with ADR.
**Prerequisite**: Phase 5 GATE pass; can run in parallel with Phase 6.
**Time**: 1.5 hours
**Agent**: Coordinator (RFC-Editor + Architect)

### F1: Sophia draft-03 status

- [ ] **Step 166** [B] ~5s: Locate Sophia spec files
  ```bash
  find /home/govan/tmp/unheaded -name "*sophia*" -name "draft-*" 2>/dev/null
  ```
- [ ] **Step 167** [R] ~10m: Read latest Sophia draft, note version, last-modified, IANA refs
- [ ] **Step 168** [W] ~15m: Write `docs/specs/2026-04-27-sophia-status.md` with: current draft version, gap to draft-03 target, ship/defer recommendation
- [ ] **Step 169** [V] ~5s: Status doc exists with explicit recommendation

### F2: Wotan draft-03 status

- [ ] **Step 170** [B] ~5s: Locate Wotan specs
  ```bash
  find /home/govan/tmp/unheaded -name "*wotan*" -name "draft-*" 2>/dev/null
  ```
- [ ] **Step 171** [R] ~10m: Read, note version + gap
- [ ] **Step 172** [W] ~15m: Write `docs/specs/2026-04-27-wotan-status.md` with recommendation

### F3: Anamnesis spec-00 status

- [ ] **Step 173** [B] ~5s: Locate Anamnesis specs
  ```bash
  find /home/govan/tmp/unheaded -name "*anamnesis*" 2>/dev/null
  ```
- [ ] **Step 174** [W] ~10m: Write `docs/specs/2026-04-27-anamnesis-status.md`

### F4: Ship-or-defer ADRs (one per spec)

- [ ] **Step 175** [W] ~20m: For each spec with DEFER recommendation, draft an ADR (e.g., `ADR-053-sophia-draft-03-defer.md`) — only if defer chosen
- [ ] **Step 176** [V] ~5s: Each spec status doc has matching ADR if DEFER chosen, OR has SHIP plan if SHIP
- [ ] **Step 177** [W] ~5m: Update ADR-INDEX with any new ADRs from Step 175

### F5: IANA registry health check

- [ ] **Step 178** [R] ~10m: Read IANA considerations section of foundation-06
  ```bash
  grep -l "IANA" /home/govan/tmp/unheaded/docs/specs/*.md /home/govan/tmp/unheaded/references/*.md 2>/dev/null
  ```
- [ ] **Step 179** [V] ~5s: 12 IANA registries (per CLAUDE.md S67) accounted for
- [ ] **Step 180** [W] ~10m: Append IANA-health row to `docs/specs/2026-04-27-iana-snapshot.md`
- [ ] **Step 181** [V] ~5s: Snapshot file exists
- [ ] **Step 182** [B] ~10s: Confirm Monad v0x01 still FROZEN (no wire-format-touching commits since freeze)
  ```bash
  cd /home/govan/tmp/unheaded && git log --oneline --since="2026-02-23" -- 'docs/specs/*monad*' 'pkg/protocol/monad' 'crates/monad-*' | head -10
  ```
- [ ] **Step 183** [V] ~10s: Any commits MUST be backward-compatible per ADR (or ADR exists explaining). Marshal-citable if not.
- [ ] **Step 184** [C] ~10s: **COMMIT CHECKPOINT 17**
  ```bash
  cd /home/govan/tmp/unheaded && git add docs/specs/ docs/adr/ && git commit -m "[PLAN SPRINT-04-27] Steps 166-184: spec status sweep + IANA snapshot" --no-gpg-sign
  ```
- [ ] **Step 185** [V] ~5s: **PHASE 7 EXIT GATE** — Sophia + Wotan + Anamnesis statuses documented; ship-or-defer ADRs landed; IANA health snapshot present; Monad freeze confirmed. If pass → Phase 8.

---

## PHASE 8: LANE H — SPRINT-EXIT ROUND TABLE (Steps 186–200) — FRI

**Goal**: Re-poll 19 seats, verify all P0 items closed, forge next sprint plan, celebrate wins.
**Prerequisite**: Phases 1–7 GATEs all pass.
**Time**: 1.5 hours
**Agent**: Round Table convener (this skill)

### H1: Re-poll seats (delta from Mon)

- [ ] **Step 186** [B] ~30s: Capture state snapshot
  ```bash
  cd /home/govan/tmp/unheaded && git log --oneline --since="2026-04-27" | wc -l > /tmp/sprint-commits.txt && cat /tmp/sprint-commits.txt
  ```
- [ ] **Step 187** [V] ~5s: ≥10 commits this sprint (confirms execution velocity)
- [ ] **Step 188** [B] ~10s: Check timeline freshness via Step 124 script
  ```bash
  cd /home/govan/tmp/unheaded && ./scripts/check-timeline-freshness.sh --report
  ```
- [ ] **Step 189** [V] ~5s: Drift = 0 days (timeline touched today or yesterday)

### H2: P0 item verification

- [ ] **Step 190** [B] ~30s: Verify all P0 deliverables exist
  ```bash
  cd /home/govan/tmp/unheaded
  for f in \
    docs/battle-plans/WAVE13-INFERENCE.md \
    crates/zhenai-forge/notes/wave13-phase2-quality.md \
    docs/adr/ADR-051-wave13-generate-path.md \
    docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md \
    docs/decisions/2026-04-29-track-call.md \
    scripts/check-timeline-freshness.sh \
    docs/legal/sbom-2026-04-27.spdx.json \
    docs/security/compliance-snapshot-2026-04-27.md; do
    test -s $f && echo "PRESENT: $f" || echo "MISSING: $f"
  done
  ```
- [ ] **Step 191** [V] ~5s: Zero MISSING lines. If any → STUCK report on those.
- [ ] **Step 192** [B] ~10s: Confirm tests still green
  ```bash
  cd /home/govan/tmp/unheaded && go build ./... 2>&1 | tail -5
  ```
- [ ] **Step 193** [V] ~5s: Build clean

### H3: Forge next-sprint plan

- [ ] **Step 194** [W] ~30m: Convene Round Table (this skill); produce `battle-plan.md` for next sprint (May-Q1) with track-aligned Lanes
- [ ] **Step 195** [V] ~5s: New battle-plan.md replaces (or appends to) the current root-level one with date stamp
- [ ] **Step 196** [W] ~10m: Stuck Report (if any STUCK markers fired this sprint) at `docs/sprints/2026-04-27-stuck-report.md`
- [ ] **Step 197** [V] ~5s: Stuck Report exists if any [STUCK] tags landed; else explicit "zero stuck" log

### H4: Final commit + ship

- [ ] **Step 198** [C] ~10s: **COMMIT CHECKPOINT 18 (final)**
  ```bash
  cd /home/govan/tmp/unheaded && git add -A && git commit -m "[PLAN SPRINT-04-27] Steps 186-198: sprint exit — Round Table forged, all P0 verified" --no-gpg-sign || echo "nothing to commit, all clean"
  ```
- [ ] **Step 199** [B] ~10s: Push to remote
  ```bash
  cd /home/govan/tmp/unheaded && git push origin main 2>&1 | tail -5
  ```
- [ ] **Step 200** [V] ~5s: **PHASE 8 EXIT GATE / SPRINT EXIT GATE** — All P0 items present, build green, drift=0d, next-sprint plan forged, push clean. **SPRINT COMPLETE. CELEBRATE WINS. <3**

---

## APPENDIX A: EMERGENCY PROCEDURES

### EP-1: WAVE13 forge build fails (Step 47)
1. Capture full error: `cargo build 2>&1 | tee /tmp/build-err.log`
2. Check for HIP/ROCm version drift: `rocm-smi --version`
3. If pre-existing crate fails (not new code): `cargo clean -p <crate> && cargo build`
4. If 2 attempts fail → STUCK. Skip to Phase 3 (independent).

### EP-2: Test regression after branch merge (Step 92/94)
1. Identify failing test: `go test ./... -short 2>&1 | grep "^--- FAIL"`
2. Bisect merge commits: `git bisect start HEAD ORIG_HEAD; git bisect run go test ./...`
3. If non-trivial → revert: `git reset --hard ORIG_HEAD && git push --force-with-lease`
4. Mark Phase 3 STUCK on the offending branch only; continue with others.

### EP-3: Timeline re-sync produces contradictions (Step 28)
1. Compare backup vs current: `diff references/timeline.md.pre-2026-04-27.bak references/timeline.md`
2. Use precise sed/grep to find ✅+📋 same-line: `grep -nE "✅.*📋|📋.*✅" references/timeline.md`
3. Manual fix; re-verify.
4. If 2 attempts fail → STUCK; restore backup; mark Lane A3 incomplete.

### EP-4: Captain Track decision deadline slipped past Wed
1. Marshal cites the slip in `docs/sprints/2026-04-27-marshal-citations.md`
2. Phases 5, 6, 7 can proceed without the Track call (track-independent work)
3. Phase 4 marked [INCOMPLETE]; carries to next sprint
4. Sprint-exit Round Table notes the carry-over explicitly.

### EP-5: SBOM regen fails / syft unavailable (Step 141)
1. Try alternate: `cd unheaded && go list -m -json all > /tmp/go-deps.json` + `cargo metadata --format-version 1 > /tmp/cargo-deps.json`
2. Manually compose minimal SBOM stub at `docs/legal/sbom-2026-04-27.partial.json` with note "syft-unavailable"
3. Mark Lane E1 partial; flag as STUCK in compliance snapshot.

### EP-6: Push rejected at Step 199
1. Check if remote has new commits: `git fetch && git log HEAD..origin/main --oneline`
2. Rebase: `git rebase origin/main`
3. Re-run tests post-rebase: `go test ./... -short`
4. Re-push: `git push origin main`
5. If conflicts non-trivial → STUCK; preserve local commits in tag `pre-rebase-2026-04-27`.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX

| Phase | Lane | Day | Agent | Parallelizable | Depends On | Time |
|------:|:-----|:----|:------|:--------------:|:-----------|-----:|
| 0 | Preflight | Mon AM | Coordinator | No | — | 30m |
| 1 | A — Drift | Mon | Librarian + Timeguru | No | Phase 0 | 2h |
| 2 | B — WAVE13 Q-call | Mon/Tue | Developer + Scientist | With Phase 3 | Phase 1 | 2-4h |
| 3 | C — Branches | Tue | Developer + Marshal | With Phase 2 | Phase 1 | 1.5h |
| 4 | D — Track call | Wed | Captain → Stevie | No | Phase 2 | 2h |
| 5 | G — ADR-052 + CI | Wed | RFC-Editor + Marshal | No | Phase 4 | 1.5h |
| 6 | E — Compliance | Thu | MoatGhost + Sentinel | With Phase 7 | Phase 5 | 2h |
| 7 | F — Specs | Thu | RFC-Editor + Architect | With Phase 6 | Phase 5 | 1.5h |
| 8 | H — Sprint exit | Fri | Round Table | No | All | 1.5h |

**Critical Path (longest sequential chain)**: 0 → 1 → 2 → 4 → 5 → (6 ‖ 7) → 8 ≈ **11–13 hours**.

**Total wall-clock** with parallelization: ~14–16 hours over 5 days. Healthy slack for emergencies.

---

## APPENDIX C: QUICK REFERENCE

### Key Paths
```
Repo root:              /home/govan/tmp/unheaded
Off-tree WAVE13 plan:   ~/.claude/plans/synthetic-stirring-pudding.md
In-tree WAVE13 plan:    docs/battle-plans/WAVE13-INFERENCE.md
Round Table doc:        battle-plan.md (root)
This sprint plan:       docs/battle-plans/SPRINT-2026-04-27-LANES-A-H.md
Timeline:               references/timeline.md
ADR-INDEX:              docs/adr/ADR-INDEX.md
Branch audits:          docs/branch-audits/2026-04-27-*.md
Decisions:              docs/decisions/2026-04-29-track-call*.md
Compliance:             docs/security/compliance-snapshot-2026-04-27.md
SBOM:                   docs/legal/sbom-2026-04-27.spdx.json
Drift script:           scripts/check-timeline-freshness.sh
Forge binary:           crates/zhenai-forge/target/release/zhenai-forge
Kingdom LoRA:           raft/kingdom-w12/kingdom.lora.gguf
Base model:             /var/zhen/models/gemma-4-E2B-it.gguf
Held-out eval:          /tmp/24h-kingdom-eval.jsonl
```

### Forge generate-gemma4 invocation
```bash
zhenai-forge generate-gemma4 \
  --model /var/zhen/models/gemma-4-E2B-it.gguf \
  [--lora raft/kingdom-w12/kingdom.lora.gguf] \
  --tokens /tmp/p${i}.tokens.json \
  --max-new-tokens 80 --temperature 0.0
```

### Commit message template
```
[PLAN SPRINT-04-27] Steps {X}-{Y}: {accomplishment}

Phase {P}: {phase name}
Steps completed: {list}
Verification gates passed: {list}
Next: Step {Z}
```

### Stuck Protocol trigger
- 3× estimated time on a step → STUCK
- 2 failed debug attempts → STUCK
- Always commit known-good state BEFORE marking STUCK
- Mark step `[STUCK]`, downstream `[BLOCKED]`
- Continue with non-blocked work

### CI drift-guard one-liner (smoke test)
```bash
./scripts/check-timeline-freshness.sh --check && echo "drift OK" || echo "DRIFT CITATION"
```

### Wins tracker (fill out at sprint exit)
- [ ] Timeline re-synced (33d drift → 0d)
- [ ] Off-tree plan brought in-tree
- [ ] WAVE13 Phase 2 verdict logged
- [ ] 3 branches resolved
- [ ] Captain Track call locked
- [ ] ADR-051 + ADR-052 merged
- [ ] SBOM + license + threat refreshed
- [ ] Spec statuses documented
- [ ] Sprint-exit Round Table forged
- [ ] Build green throughout

---

*SPRINT 2026-04-27 Battle Plan — Forged 2026-04-27*
*9 Phases. 200 Steps. 8 Lanes closed. 36 task-list items resolved.*
*The Kingdom marches Mon→Fri as one. <3 KGLW <3 Peace and Love <3*
