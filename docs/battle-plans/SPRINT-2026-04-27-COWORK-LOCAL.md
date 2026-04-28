# SPRINT 2026-04-27 — COWORK LOCAL BATTLE PLAN (Macbook scope)

**Date**: 2026-04-27
**Sprint**: Round-Table follow-through, **local-only scope** for Cowork-on-Macbook execution
**Supersedes**: `docs/battle-plans/SPRINT-2026-04-27-LANES-A-H.md` (full version preserved for when you're at WEST/EAST or any Linux dev box with the full toolchain)
**Prerequisite**: Round Table convened 2026-04-27; `battle-plan.md` at repo root; full plan archived alongside this one
**Target (local)**: All doc, ADR, script, and plan deliverables ready to ship; every remote-execution item has a packaged handoff (script + checklist + verification recipe) ready to copy-paste on a Linux box
**Estimated Duration**: 4–6 wall-clock hours, single-session-friendly
**Agent Strategy**: Solo (Claude+Stevie at the Macbook). No GPU. No Linux kernel work. No service builds.
**Commit Cadence**: Every 5 steps + every phase exit gate. `--no-gpg-sign` per AFK policy.
**Stuck Protocol**: 3× time / 2 failed debug attempts → STUCK marker + Stuck Report at end.

---

## ENVIRONMENT REALITY CHECK

What this Cowork-on-Macbook session **CAN** do:
- Read/Write/Edit any file in `/Users/govan/home 2/govan/tmp/unheaded`
- Run a Linux sandbox shell (python3, git, jq, curl, basic GNU tools) — repo mounted read-write
- Static checks: grep, find, line counts, YAML/JSON syntax validation
- Git read-only ops: `git log`, `git diff`, `git branch -a`, `git show`
- Git write ops on the repo (commit/tag) — the user pushes from their normal workflow
- Markdown / shell-script authoring + dry-run

What it **CANNOT** do:
- Build Go (`go` not in sandbox)
- Build Rust with HIP/ROCm kernels (no GPU, no AMD stack)
- Run forge `generate-gemma4` (needs Gemma-4 GGUF + LoRA + GPU on a Linux box)
- Touch WEST or EAST bare metal
- Run real CI / wait for green builds
- Run live syft/cargo-deny/go-licenses scans (toolchains absent)
- Live `curl` to internal services (only public internet)
- Test eBPF programs (requires Linux kernel + privileges)

So: **every Lane gets split into LOCAL work (done now) and REMOTE work (handoff packet prepared now, executed when you're at WEST/EAST or your Linux dev box).**

---

## LEGEND

```
[B]    Bash command (run in Linux sandbox)
[V]    Verification step (MUST pass)
[D]    Debug step (only on prior failure)
[W]    Write/create file
[R]    Read/inspect file
[C]    Commit checkpoint
[L]    LOCAL-only step (Cowork-on-Macbook safe)
[R/H]  REMOTE HANDOFF — packaged for Linux/GPU execution; not run here
[STUCK]/[BLOCKED] Skip Protocol markers
```

---

## CRITICAL PATH (LOCAL)

```
Phase 0 (preflight) → Phase 1 (Lane A drift) → Phase 2 (Lane B prep + handoff) →
Phase 3 (Lane C audits — docs only) → Phase 4 (Lane D Track call docs) →
Phase 5 (Lane G ADR-052 + script) → Phase 6 (Lane E compliance framework) →
Phase 7 (Lane F spec docs) → Phase 8 (handoff packet + sprint exit)
```

All ~100 steps run on the Macbook. Three remote-execution packets queue up for the next time you're at a Linux box.

---

## PHASE 0: PREFLIGHT (Steps 1–8) — LOCAL

**Goal**: Confirm sandbox is healthy and repo state matches the Round Table.
**Time**: 5 min
**Agent**: Solo

- [ ] **Step 1** [B][L] ~5s: HEAD check
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log -1 --oneline
  ```
- [ ] **Step 2** [V][L] ~5s: HEAD must match `5d413699` (WAVE13 Phase 1) or descendant
- [ ] **Step 3** [B][L] ~5s: Working tree clean
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git status -sb
  ```
- [ ] **Step 4** [V][L] ~5s: Clean tree (no uncommitted edits — root `battle-plan.md` may already be uncommitted; that's fine, will be folded into a checkpoint).
- [ ] **Step 5** [B][L] ~5s: Off-tree WAVE13 plan exists
  ```bash
  test -f ~/.claude/plans/synthetic-stirring-pudding.md && echo OK || echo MISSING
  ```
- [ ] **Step 6** [B][L] ~5s: Sandbox basics
  ```bash
  for t in python3 git jq curl awk sed; do which $t 2>/dev/null && echo "$t: OK" || echo "$t: MISSING"; done
  ```
- [ ] **Step 7** [V][L] ~5s: All required local tools present (Go/Cargo are *not* required for this scope).
- [ ] **Step 8** [V][L] ~5s: **PHASE 0 EXIT GATE** — sandbox healthy, HEAD aligned, off-tree plan in reach. → Phase 1.

---

## PHASE 1: LANE A — DRIFT & SOURCE-OF-TRUTH (Steps 9–32) — ALL LOCAL

**Goal**: Eliminate the two citations issued by Marshal (33-day timeline drift, off-tree WAVE13 plan).
**Time**: 90 min
**Agent**: Solo (Librarian + Timeguru hats)

### A1: Import accepted WAVE13 plan in-tree

- [ ] **Step 9** [B][L] ~5s: Make sure target dir exists
  ```bash
  mkdir -p /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans
  ```
- [ ] **Step 10** [B][L] ~5s: Copy off-tree plan in-tree
  ```bash
  cp ~/.claude/plans/synthetic-stirring-pudding.md /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE13-INFERENCE.md
  ```
- [ ] **Step 11** [V][L] ~5s: File present and non-empty
  ```bash
  test -s /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE13-INFERENCE.md && wc -l /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE13-INFERENCE.md
  ```
- [ ] **Step 12** [W][L] ~3m: Prepend canonical header (3 lines) to in-tree copy declaring it the source of truth per ADR-052 (forthcoming).

### A2: Fix SUPERSEDED note in old draft

- [ ] **Step 13** [R][L] ~10s: Check current SUPERSEDED text
  ```bash
  head -12 /sessions/funny-kind-hopper/mnt/unheaded/crates/zhenai-forge/notes/wave13-inference-battle-plan.md
  ```
- [ ] **Step 14** [W][L] ~3m: Edit the file: change pointer from `~/.claude/plans/synthetic-stirring-pudding.md` to `docs/battle-plans/WAVE13-INFERENCE.md`.
- [ ] **Step 15** [V][L] ~5s: Confirm new pointer present
  ```bash
  grep -n "docs/battle-plans/WAVE13-INFERENCE.md" /sessions/funny-kind-hopper/mnt/unheaded/crates/zhenai-forge/notes/wave13-inference-battle-plan.md
  ```
- [ ] **Step 16** [V][L] ~5s: Confirm old off-tree pointer is gone (or at most a one-line "history" mention)
  ```bash
  grep -c "synthetic-stirring-pudding" /sessions/funny-kind-hopper/mnt/unheaded/crates/zhenai-forge/notes/wave13-inference-battle-plan.md
  ```
- [ ] **Step 17** [C][L] ~10s: **CHECKPOINT 1**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/battle-plans/WAVE13-INFERENCE.md crates/zhenai-forge/notes/wave13-inference-battle-plan.md && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 9-17: WAVE13 plan in-tree + supersede pointer fixed" --no-gpg-sign
  ```

### A3: Re-sync timeline.md

- [ ] **Step 18** [R][L] ~30s: Read current timeline (top 60 lines)
  ```bash
  head -60 /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md
  ```
- [ ] **Step 19** [B][L] ~5s: Backup before rewrite
  ```bash
  cp /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md.pre-2026-04-27.bak
  ```
- [ ] **Step 20** [W][L] ~30m: Rewrite `references/timeline.md`:
  - Header: "LAST UPDATED: 2026-04-27" + reference HEAD `5d413699`
  - Age 5 contradiction (✅ + 📋 same line) → resolve to one canonical state
  - Age 3: enumerate sub-items (WAVE10F ✅, WAVE11 ✅, WAVE12 ✅ Apr 23, WAVE13 Phase 1 ✅, Phase 2 in progress)
  - Add Mímir's Law / Gleipnir Phase 0 entry (Apr 11 real-metal validated)
  - Add ADR-051 + ADR-052 placeholders
- [ ] **Step 21** [V][L] ~5s: New timestamp present
  ```bash
  grep "2026-04-27" /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md
  ```
- [ ] **Step 22** [V][L] ~5s: No contradictory ✅+📋 same-line entries
  ```bash
  ! grep -E "✅ COMPLETED.*📋 PLANNED|📋 PLANNED.*✅ COMPLETED" /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md
  ```
- [ ] **Step 23** [D][L] ~5m: If contradictions remain, re-edit. STUCK after 2 attempts.

### A4: ADR-INDEX placeholders

- [ ] **Step 24** [R][L] ~10s: Read current index
  ```bash
  cat /sessions/funny-kind-hopper/mnt/unheaded/docs/adr/ADR-INDEX.md
  ```
- [ ] **Step 25** [W][L] ~5m: Append rows for ADR-051 (WAVE13 generate path) status DRAFT, ADR-052 (drift policy) status DRAFT
- [ ] **Step 26** [V][L] ~5s:
  ```bash
  grep -E "ADR-05[12]" /sessions/funny-kind-hopper/mnt/unheaded/docs/adr/ADR-INDEX.md
  ```

### A5: Wiki sync

- [ ] **Step 27** [B][L] ~5s: Find stale wiki refs
  ```bash
  grep -lE "Age 3.*85%|Age 5.*COMPLETED" /sessions/funny-kind-hopper/mnt/unheaded/wiki/*.md 2>/dev/null || echo "no stale refs"
  ```
- [ ] **Step 28** [W][L] ~10m: If any flagged, edit each to match new timeline. If none, skip.
- [ ] **Step 29** [V][L] ~5s: Re-grep, expect zero matches
- [ ] **Step 30** [B][L] ~5s: Confirm timeline mtime
  ```bash
  stat -c '%y %n' /sessions/funny-kind-hopper/mnt/unheaded/references/timeline.md
  ```
- [ ] **Step 31** [C][L] ~10s: **CHECKPOINT 2**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add references/timeline.md docs/adr/ADR-INDEX.md wiki/ && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 18-31: timeline re-sync + ADR-INDEX placeholders + wiki" --no-gpg-sign
  ```
- [ ] **Step 32** [V][L] ~5s: **PHASE 1 EXIT GATE** — both Marshal citations cleared. Off-tree plan eliminated; timeline drift = 0 days. → Phase 2.

---

## PHASE 2: LANE B — WAVE13 PHASE 2 PREP + REMOTE HANDOFF (Steps 33–48)

**Goal**: Cannot run WAVE13 Phase 2 quality call locally (needs GPU + base model + LoRA on Linux). Prepare *everything* the remote run needs, plus the result-template and ADR-051 skeleton, so the human + remote box just execute and fill in.
**Time**: 60 min
**Agent**: Solo (Developer + Scientist hats)

### B1: Build the remote-execution packet

- [ ] **Step 33** [W][L] ~20m: Create `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md`
  - Section 1: Prerequisite checks (base model path, LoRA path, eval JSONL path, gemma4-venv path) — copy-paste-ready commands
  - Section 2: Build forge release (`cargo build --release` in `crates/zhenai-forge`)
  - Section 3: Pick 8 prompts (`shuf -n 8 /tmp/24h-kingdom-eval.jsonl`)
  - Section 4: Generate base + LoRA outputs (full bash loop, identical to full plan Steps 54+56)
  - Section 5: Decode tokens via `gemma4-venv`
  - Section 6: Result-capture template — 8 rows, qualitative tag, win-rate
  - Section 7: Decision template — SHIP / RETRAIN / RANK-UP / DATA-FIX
- [ ] **Step 34** [V][L] ~5s: File contains all 7 sections
  ```bash
  grep -cE "^## (Section|Prereq|Build|Pick|Generate|Decode|Result|Decision)" /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md
  ```

### B2: Result-capture skeleton

- [ ] **Step 35** [W][L] ~15m: Create `crates/zhenai-forge/notes/wave13-phase2-quality.md` as a SKELETON:
  - 8 empty `### Prompt N` rows with placeholders
  - Win-rate field (numeric, blank)
  - Decision section (verdict blank)
  - "Pending remote run" banner at top
- [ ] **Step 36** [V][L] ~5s:
  ```bash
  grep -cE "^### Prompt [0-9]" /sessions/funny-kind-hopper/mnt/unheaded/crates/zhenai-forge/notes/wave13-phase2-quality.md
  ```

### B3: ADR-051 skeleton

- [ ] **Step 37** [W][L] ~15m: Create `docs/adr/ADR-051-wave13-generate-path.md` with structure:
  - Status: DRAFT (becomes ACCEPTED after Phase 2 verdict)
  - Context: WAVE13 Phase 1 generate path; Phase 2 quality finding *(blank — fill from remote run)*
  - Decision: *(blank — fill in)*
  - Consequences: KV-cache deferral; downstream WAVE14 implications (these we know)
  - "Pending Phase 2 verdict" banner
- [ ] **Step 38** [V][L] ~5s:
  ```bash
  test -s /sessions/funny-kind-hopper/mnt/unheaded/docs/adr/ADR-051-wave13-generate-path.md
  ```

### B4: Update root battle-plan.md to reflect deferral

- [ ] **Step 39** [R][L] ~10s: Read root battle-plan.md task queue
- [ ] **Step 40** [W][L] ~10m: Annotate Lane B items with "[REMOTE — PACKET READY at docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md]"

### B5: Mark Phase 2 closed-locally

- [ ] **Step 41** [W][L] ~3m: Add summary block to top of `WAVE13-PHASE2-REMOTE-PACKET.md`:
  - Estimated remote wall-clock: 2–4h
  - Depends on: GPU dev box, gemma-4 GGUF, kingdom LoRA, eval JSONL
  - When to run: as soon as you're at WEST/EAST/dev box
  - Success criteria: result-skeleton + ADR-051 filled in, then resume Phases 4+ here
- [ ] **Step 42** [C][L] ~10s: **CHECKPOINT 3**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md crates/zhenai-forge/notes/wave13-phase2-quality.md docs/adr/ADR-051-wave13-generate-path.md battle-plan.md && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 33-42: WAVE13 Phase 2 remote-handoff packet + skeletons" --no-gpg-sign
  ```

### B6: Carry-forward note

- [ ] **Step 43** [V][L] ~5s: Confirm Phase-2-blocking work is bounded — Phases 4 (Track call) and 5 (ADR-052) and 7 (specs) **DO NOT** depend on the verdict. Phase 4's Track decision benefits from the verdict but can proceed with both branches drafted (A-if-yes / A-if-no).
- [ ] **Step 44** [W][L] ~5m: In `battle-plan.md`, add a note in the Open Questions section: "Captain Track call may need to be drafted in two variants pending WAVE13 Phase 2 verdict — see Phase 4 below."
- [ ] **Step 45** [R][L] ~5s: Sanity-read root battle-plan.md
- [ ] **Step 46** [V][L] ~5s: Annotation present
- [ ] **Step 47** [C][L] ~10s: **CHECKPOINT 4**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27 LOCAL] Steps 43-47: battle-plan.md annotated for remote dependency" --no-gpg-sign || echo "no diff"
  ```
- [ ] **Step 48** [V][L] ~5s: **PHASE 2 EXIT GATE** — Phase 2 work locally complete (packet + skeletons + ADR-051 stub + battle-plan annotated). Remote box can pick this up cold. → Phase 3.

---

## PHASE 3: LANE C — BRANCH AUDITS (DOCS ONLY) (Steps 49–66) — LOCAL

**Goal**: Write the 3 branch audits using `git log` / `git diff` (read-only). Decision (merge/kill) recorded; **execution deferred to Linux box where build+tests can verify safety.**
**Time**: 45 min
**Agent**: Solo (Developer + Marshal hats)

### C1: Audit `claude/migrate-packages-github-V2Ctr`

- [ ] **Step 49** [B][L] ~5s: Commit list
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log main..claude/migrate-packages-github-V2Ctr --oneline 2>&1 | head -30
  ```
- [ ] **Step 50** [B][L] ~5s: File diff stat
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git diff main...claude/migrate-packages-github-V2Ctr --stat 2>&1 | tail -20
  ```
- [ ] **Step 51** [W][L] ~10m: Write `docs/branch-audits/2026-04-27-claude-migrate-packages-V2Ctr.md`:
  - Branch age (first commit date), commit count, file count
  - Diff topic (likely package-path migration to github.com/unheaded/...)
  - Risk: AI-authored → CLA implication on merge
  - Verdict: PROPOSED (requires Linux build before final merge/kill)
  - Remote-handoff: build+test commands the human must run before executing the verdict

### C2: Audit `docs/s73-public-launch-planning`

- [ ] **Step 52** [B][L] ~5s: Same query
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log main..docs/s73-public-launch-planning --oneline 2>&1 | head -30
  ```
- [ ] **Step 53** [W][L] ~10m: Write `docs/branch-audits/2026-04-27-s73-public-launch.md` (same structure)

### C3: Audit `public-release-cleanup`

- [ ] **Step 54** [B][L] ~5s:
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log main..public-release-cleanup --oneline 2>&1 | head -30
  ```
- [ ] **Step 55** [W][L] ~10m: Write `docs/branch-audits/2026-04-27-public-release-cleanup.md`

### C4: Verdict summary + remote handoff

- [ ] **Step 56** [W][L] ~10m: Create `docs/branch-audits/2026-04-27-summary.md` with:
  - Verdict matrix (3 rows × {MERGE, KILL, DEFER, build-required})
  - Linux-side execution checklist:
    - For each MERGE: `git checkout main && git merge --no-ff <branch> && go build ./... && go test ./... -short`
    - For each KILL: `git tag archived/<branch>-2026-04-27 <branch> && git branch -D <branch> && git push origin :<branch>`
  - Sign-off line for Stevie/Marshal at the Linux box
- [ ] **Step 57** [V][L] ~5s: Summary file exists with all 3 branches enumerated
  ```bash
  grep -cE "claude/migrate|s73-public|public-release-cleanup" /sessions/funny-kind-hopper/mnt/unheaded/docs/branch-audits/2026-04-27-summary.md
  ```
- [ ] **Step 58** [V][L] ~5s: Should equal 3
- [ ] **Step 59** [C][L] ~10s: **CHECKPOINT 5**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/branch-audits/ && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 49-59: 3 branch audits + summary (verdicts pending Linux build)" --no-gpg-sign
  ```

### C5: Confirm Cowork can't safely merge

- [ ] **Step 60** [V][L] ~5s: Document the constraint in summary file: "merges deferred — `go build` cannot run in this Cowork sandbox; Linux dev box required for safety verification."
- [ ] **Step 61** [B][L] ~5s: Capture branch state baseline for diff later
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git branch -a > /tmp/sprint-branches-baseline.txt && head /tmp/sprint-branches-baseline.txt
  ```
- [ ] **Step 62** [W][L] ~3m: Append baseline path to the summary so the Linux session can diff before/after
- [ ] **Step 63** [V][L] ~5s: Three audit docs + summary all present
  ```bash
  ls /sessions/funny-kind-hopper/mnt/unheaded/docs/branch-audits/2026-04-27-*.md | wc -l
  ```
- [ ] **Step 64** [V][L] ~5s: Should equal 4
- [ ] **Step 65** [C][L] ~10s: **CHECKPOINT 6**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git diff --quiet || git commit -am "[PLAN SPRINT-04-27 LOCAL] Steps 60-65: branch baseline captured + sandbox limitation noted" --no-gpg-sign || echo "no diff"
  ```
- [ ] **Step 66** [V][L] ~5s: **PHASE 3 EXIT GATE** — 3 branches audited, verdict matrix written, Linux handoff documented. Execution deferred. → Phase 4.

---

## PHASE 4: LANE D — TRACK CALL (DOCS) (Steps 67–80) — LOCAL

**Goal**: Captain drafts A/B/C analysis. **Stevie's actual decision is human work** — we just put the options on the table here.
**Time**: 60 min
**Agent**: Solo (Captain hat) → Stevie

### D1: Options doc

- [ ] **Step 67** [W][L] ~45m: Create `docs/decisions/2026-04-29-track-call-options.md`
  - **Track A (forge-first)**: cost, time-to-launch impact, depends on WAVE13 Phase 2 verdict
  - **Track B (launch-first)**: pause-cost on forge, demo-script readiness gap, README polish
  - **Track C (twin-track)**: coordination cost, Stevie bandwidth model, parallel work feasibility
  - Captain's recommendation (default)
  - Conditional branches: "if Phase 2 verdict = SHIP, recommend X; if RETRAIN, recommend Y"
- [ ] **Step 68** [V][L] ~5s: All 3 tracks present
  ```bash
  grep -cE "^## Track [ABC]" /sessions/funny-kind-hopper/mnt/unheaded/docs/decisions/2026-04-29-track-call-options.md
  ```
- [ ] **Step 69** [V][L] ~5s: Should equal 3
- [ ] **Step 70** [V][L] ~5s: Conditional-branch text exists
  ```bash
  grep -cE "if.*verdict|conditional" /sessions/funny-kind-hopper/mnt/unheaded/docs/decisions/2026-04-29-track-call-options.md
  ```

### D2: Decision template

- [ ] **Step 71** [W][L] ~10m: Create `docs/decisions/2026-04-29-track-call.md` as TEMPLATE:
  - "Chosen track: ____ (A / B / C)"
  - "Rationale: <fill in>"
  - "Locked: <date>"
  - Banner: "Stevie fills this in after WAVE13 Phase 2 verdict (or sooner if confident)"
- [ ] **Step 72** [V][L] ~5s: Template exists with placeholders

### D3: Conditional propagation prep

- [ ] **Step 73** [W][L] ~5m: In the options doc, append a "Propagation Checklist" — 3 sub-lists of doc-edits to perform once each track is chosen:
  - Track A → which timeline rows change, which Lane I items lift to P1
  - Track B → same
  - Track C → same
  Lists are *executable when the track is locked*.
- [ ] **Step 74** [V][L] ~5s: Three propagation sub-lists present
  ```bash
  grep -cE "^### Propagation: Track [ABC]" /sessions/funny-kind-hopper/mnt/unheaded/docs/decisions/2026-04-29-track-call-options.md
  ```

### D4: WAVE14 stub (if Track A or C)

- [ ] **Step 75** [W][L] ~15m: Create `docs/battle-plans/WAVE14-STUB.md`:
  - Banner: "Activates if Track A or C is chosen"
  - Scope: BackwardScratch (per ADR-050) + KV-cache for serving
  - Phase outline (no numbered steps yet — that's a future Warmonger pass)
  - Estimated wall-clock per phase

### D5: Demo script outline (if Track B or C)

- [ ] **Step 76** [W][L] ~15m: Create `docs/launch/2026-04-29-demo-script-v0.md`:
  - Banner: "Activates if Track B or C is chosen"
  - 5–8 minute demo flow (Kingdom self-hosting + dashboard + eBPF tracing)
  - Recording requirements (resolution, audio, scenes)
  - Owner: Captain + Micromanager
- [ ] **Step 77** [V][L] ~5s: Both stubs present
  ```bash
  ls /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE14-STUB.md /sessions/funny-kind-hopper/mnt/unheaded/docs/launch/2026-04-29-demo-script-v0.md
  ```
- [ ] **Step 78** [C][L] ~10s: **CHECKPOINT 7**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/decisions/ docs/battle-plans/WAVE14-STUB.md docs/launch/ && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 67-78: Track A/B/C options + decision template + WAVE14/demo stubs" --no-gpg-sign
  ```
- [ ] **Step 79** [V][L] ~5s: Both branch stubs are clearly marked conditional
  ```bash
  grep -lE "Activates if Track" /sessions/funny-kind-hopper/mnt/unheaded/docs/battle-plans/WAVE14-STUB.md /sessions/funny-kind-hopper/mnt/unheaded/docs/launch/2026-04-29-demo-script-v0.md
  ```
- [ ] **Step 80** [V][L] ~5s: **PHASE 4 EXIT GATE** — options drafted, decision template ready, both conditional stubs in place. Stevie can lock the track at his desk. → Phase 5.

---

## PHASE 5: LANE G — ADR-052 + DRIFT-GUARD SCRIPT (Steps 81–96) — LOCAL

**Goal**: Encode "drift ≤ 7 days" policy + write the CI guard script. **CI wiring** is local-write + remote-validate (we don't run Jenkins from a Macbook).
**Time**: 60 min
**Agent**: Solo (RFC-Editor + Marshal + Developer hats)

### G1: ADR-052

- [ ] **Step 81** [W][L] ~30m: Create `docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md`
  - Status: ACCEPTED
  - Context: 33-day timeline drift + off-tree plan caught at 2026-04-27 Round Table
  - Decision: timeline.md ≤ 7 days from HEAD when HEAD has new commits; battle plans of record live in-tree under `docs/battle-plans/`; off-tree plans are working drafts only
  - Consequences: CI gate (Step 84+); Marshal-citable
- [ ] **Step 82** [V][L] ~5s: ADR exists, status ACCEPTED
  ```bash
  grep -E "^Status:.*ACCEPTED" /sessions/funny-kind-hopper/mnt/unheaded/docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md
  ```
- [ ] **Step 83** [W][L] ~3m: Update ADR-INDEX: ADR-052 DRAFT → ACCEPTED

### G2: Drift-guard script

- [ ] **Step 84** [W][L] ~25m: Create `scripts/check-timeline-freshness.sh`
  - Two modes: `--check` (CI, exits 1 on drift), `--report` (info only, exits 0)
  - Logic: `git log -1 --format=%ct -- references/timeline.md` vs `git log -1 --format=%ct HEAD`; if HEAD has commits since timeline last touch AND delta > 7d, fail
- [ ] **Step 85** [B][L] ~5s: Make executable
  ```bash
  chmod +x /sessions/funny-kind-hopper/mnt/unheaded/scripts/check-timeline-freshness.sh
  ```
- [ ] **Step 86** [B][L] ~10s: Smoke test in sandbox (works in pure-bash; no go/cargo needed)
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && ./scripts/check-timeline-freshness.sh --report
  ```
- [ ] **Step 87** [V][L] ~5s: Script outputs a sane age in days, exits 0 (timeline was just refreshed in Phase 1)
- [ ] **Step 88** [B][L] ~5s: Run `--check` mode — should also exit 0 since drift = 0
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && ./scripts/check-timeline-freshness.sh --check; echo exit=$?
  ```
- [ ] **Step 89** [V][L] ~5s: Exit 0

### G3: CI wiring (file-write only)

- [ ] **Step 90** [R][L] ~3m: Read existing GHA workflows
  ```bash
  ls /sessions/funny-kind-hopper/mnt/unheaded/.github/workflows/ 2>/dev/null
  ```
- [ ] **Step 91** [W][L] ~15m: Create `.github/workflows/drift-guard.yml` (or add a job to an existing workflow):
  - Trigger: push (main), pull_request
  - Step: `./scripts/check-timeline-freshness.sh --check`
- [ ] **Step 92** [B][L] ~10s: Validate YAML syntax in sandbox
  ```bash
  python3 -c "import yaml,sys; yaml.safe_load(open('/sessions/funny-kind-hopper/mnt/unheaded/.github/workflows/drift-guard.yml')); print('YAML OK')"
  ```
- [ ] **Step 93** [V][L] ~5s: Output "YAML OK"

### G4: Jenkinsfile addition (file-only — execution remote)

- [ ] **Step 94** [R][L] ~5s: Check current Jenkinsfile mention
  ```bash
  grep -n "check-timeline-freshness\|drift-guard" /sessions/funny-kind-hopper/mnt/unheaded/Jenkinsfile || echo "needs-add"
  ```
- [ ] **Step 95** [W][L] ~10m: If `needs-add`, add a Jenkins stage `Drift Guard` invoking the script
- [ ] **Step 96** [V][L] ~5s: **PHASE 5 EXIT GATE** — ADR-052 ACCEPTED, script runnable in sandbox, GHA + Jenkinsfile wired. Live CI confirmation deferred to first push. Commit:
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md docs/adr/ADR-INDEX.md scripts/check-timeline-freshness.sh .github/workflows/drift-guard.yml Jenkinsfile && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 81-96: ADR-052 + drift-guard script + CI wiring" --no-gpg-sign
  ```
  → Phase 6.

---

## PHASE 6: LANE E — COMPLIANCE FRAMEWORK + REMOTE PACKET (Steps 97–110)

**Goal**: We can't run live syft / cargo-deny / go-licenses on Macbook. Write the framework + packaged commands; run *only* what's safely local (CISA KEV pull via curl, SPDX coverage via grep).
**Time**: 45 min
**Agent**: Solo (MoatGhost + Sentinel + Barrister hats)

### E1: SPDX coverage check (LOCAL — pure grep)

- [ ] **Step 97** [B][L] ~30s: Find Go files lacking SPDX
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && grep -L "SPDX-License-Identifier:" $(find . -name "*.go" -not -path "./vendor/*" -not -path "./.git/*") 2>/dev/null > /tmp/spdx-missing-go.txt; wc -l /tmp/spdx-missing-go.txt
  ```
- [ ] **Step 98** [V][L] ~5s: Should be 0. If > 0, list them.
- [ ] **Step 99** [B][L] ~30s: Same for Rust
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && grep -L "SPDX-License-Identifier:" $(find . -name "*.rs" -not -path "./target/*" -not -path "./.git/*") 2>/dev/null > /tmp/spdx-missing-rs.txt; wc -l /tmp/spdx-missing-rs.txt
  ```
- [ ] **Step 100** [W][L] ~5m: Write `docs/security/spdx-coverage-2026-04-27.md` with the two counts + lists if non-zero.

### E2: CISA KEV pull (LOCAL — curl works)

- [ ] **Step 101** [B][L] ~30s: Pull last-7-days CISA KEV
  ```bash
  curl -sSL https://www.cisa.gov/sites/default/files/feeds/known_exploited_vulnerabilities.json | jq '[.vulnerabilities[] | select(.dateAdded >= "2026-04-20")]' > /tmp/cisa-kev-recent.json && jq 'length' /tmp/cisa-kev-recent.json
  ```
- [ ] **Step 102** [B][L] ~10s: Filter to relevant tech
  ```bash
  jq -c '.[] | select((.product // "") | test("Linux|kernel|eBPF|HIP|ROCm|llama"; "i"))' /tmp/cisa-kev-recent.json > /tmp/cisa-kev-relevant.json && wc -l /tmp/cisa-kev-relevant.json
  ```
- [ ] **Step 103** [W][L] ~15m: Update or create `docs/security/threat-register.md` — append 2026-04-27 row with the relevant CVEs (or "no relevant CVEs this week" if filter empty).
- [ ] **Step 104** [V][L] ~5s: Date stamp present
  ```bash
  grep "2026-04-27" /sessions/funny-kind-hopper/mnt/unheaded/docs/security/threat-register.md
  ```

### E3: Remote-execution packet for SBOM/licenses

- [ ] **Step 105** [W][L] ~15m: Create `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md`:
  - Section A: `syft dir:. -o spdx-json > docs/legal/sbom-2026-04-27.spdx.json`
  - Section B: `cargo deny check licenses advisories`
  - Section C: `go-licenses report ./...`
  - Section D: where to write the snapshot file post-run
  - Section E: verification checklist (numeric thresholds)
  - Header banner: "Run on Linux dev box; commits SBOM + license artifacts back to repo"

### E4: Compliance snapshot skeleton

- [ ] **Step 106** [W][L] ~10m: Create `docs/security/compliance-snapshot-2026-04-27.md` SKELETON:
  - SPDX coverage (filled from Step 97-99)
  - Threat register: # new rows (filled from Step 103)
  - SBOM: PENDING (remote run)
  - License scan: PENDING (remote run)
  - SOC 2 Y1 trajectory: 1-line note
  - Banner: "Three sections complete locally; SBOM + license-scan rows pending Linux run"
- [ ] **Step 107** [V][L] ~5s: Skeleton exists with PENDING markers
  ```bash
  grep -c "PENDING" /sessions/funny-kind-hopper/mnt/unheaded/docs/security/compliance-snapshot-2026-04-27.md
  ```
- [ ] **Step 108** [V][L] ~5s: ≥ 2

### E5: Commit

- [ ] **Step 109** [C][L] ~10s: **CHECKPOINT 8**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/security/ && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 97-109: SPDX coverage + CISA KEV pull + remote-handoff packet for SBOM/licenses" --no-gpg-sign
  ```
- [ ] **Step 110** [V][L] ~5s: **PHASE 6 EXIT GATE** — 3 of 4 compliance signals fresh locally; remote packet ready. → Phase 7.

---

## PHASE 7: LANE F — SPEC STATUS SWEEP (Steps 111–124) — ALL LOCAL

**Goal**: Read existing specs; write status docs; ship-or-defer ADRs. **Pure read+write — perfect for Cowork.**
**Time**: 60 min
**Agent**: Solo (RFC-Editor + Architect hats)

### F1: Inventory specs

- [ ] **Step 111** [B][L] ~10s: Locate Sophia + Wotan + Anamnesis sources
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && find . -path ./.git -prune -o -type f \( -iname "*sophia*" -o -iname "*wotan*" -o -iname "*anamnesis*" \) -print 2>/dev/null | grep -E "\.(md|xml|txt)$" | head -40
  ```
- [ ] **Step 112** [R][L] ~10m: Read top 80 lines of each spec doc identified.

### F2: Sophia status

- [ ] **Step 113** [W][L] ~15m: Write `docs/specs/2026-04-27-sophia-status.md`:
  - Current draft version
  - Gap to draft-03 target
  - Recommendation: SHIP or DEFER (with rationale)
- [ ] **Step 114** [V][L] ~5s: File present with explicit recommendation
  ```bash
  grep -E "^Recommendation:.*(SHIP|DEFER)" /sessions/funny-kind-hopper/mnt/unheaded/docs/specs/2026-04-27-sophia-status.md
  ```

### F3: Wotan status

- [ ] **Step 115** [W][L] ~15m: Write `docs/specs/2026-04-27-wotan-status.md` (same structure)
- [ ] **Step 116** [V][L] ~5s: Recommendation present

### F4: Anamnesis status

- [ ] **Step 117** [W][L] ~10m: Write `docs/specs/2026-04-27-anamnesis-status.md`
- [ ] **Step 118** [V][L] ~5s: File present

### F5: Defer ADRs (only if any DEFER)

- [ ] **Step 119** [W][L] ~15m: For any spec with DEFER recommendation, draft `ADR-053-...-defer.md` (e.g., `ADR-053-sophia-draft-03-defer.md`) — only if defer chosen.
- [ ] **Step 120** [W][L] ~5m: Update `ADR-INDEX.md` if any ADR-053+ created.

### F6: IANA + Monad freeze sanity check

- [ ] **Step 121** [B][L] ~10s: Confirm Monad v0x01 untouched since freeze
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log --oneline --since="2026-02-23" -- 'docs/specs/*monad*' 'crates/*monad*' 'pkg/*monad*' 2>&1 | head -10
  ```
- [ ] **Step 122** [W][L] ~10m: Write `docs/specs/2026-04-27-iana-snapshot.md` — list 12 registries (per CLAUDE.md S67), each with status note + Monad-freeze confirmation
- [ ] **Step 123** [V][L] ~5s: All three status docs + IANA snapshot present
  ```bash
  ls /sessions/funny-kind-hopper/mnt/unheaded/docs/specs/2026-04-27-*.md | wc -l
  ```
- [ ] **Step 124** [C][L] ~10s: **CHECKPOINT 9**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add docs/specs/ docs/adr/ && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 111-124: spec status sweep (Sophia/Wotan/Anamnesis) + IANA snapshot + Monad-freeze confirmed" --no-gpg-sign
  ```
- [ ] **Step 125** [V][L] ~5s: **PHASE 7 EXIT GATE** — three spec status docs landed, IANA snapshot fresh, Monad freeze re-confirmed. → Phase 8.

---

## PHASE 8: SPRINT EXIT — LOCAL ROUND TABLE + REMOTE HANDOFF SUMMARY (Steps 126–140) — LOCAL

**Goal**: Mini Round Table (covers what we *did* locally), produce Stuck Report if any, package the three remote-handoff packets into a single index.
**Time**: 30 min
**Agent**: Solo (Round Table convener)

### H1: Local sprint state snapshot

- [ ] **Step 126** [B][L] ~10s: Commits this sprint
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log --oneline --since="2026-04-27" 2>&1 | head -30 && git log --oneline --since="2026-04-27" 2>&1 | wc -l
  ```
- [ ] **Step 127** [V][L] ~5s: ≥ 8 commits expected (one per checkpoint)
- [ ] **Step 128** [B][L] ~5s: Drift-guard self-check (re-run script)
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && ./scripts/check-timeline-freshness.sh --report
  ```
- [ ] **Step 129** [V][L] ~5s: Drift = 0 days

### H2: Local deliverable verification

- [ ] **Step 130** [B][L] ~30s: All P0 local artifacts exist
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded
  for f in \
    docs/battle-plans/WAVE13-INFERENCE.md \
    docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md \
    crates/zhenai-forge/notes/wave13-phase2-quality.md \
    docs/adr/ADR-051-wave13-generate-path.md \
    docs/adr/ADR-052-timeline-and-battleplan-source-of-truth.md \
    docs/decisions/2026-04-29-track-call-options.md \
    docs/decisions/2026-04-29-track-call.md \
    docs/branch-audits/2026-04-27-summary.md \
    scripts/check-timeline-freshness.sh \
    .github/workflows/drift-guard.yml \
    docs/security/spdx-coverage-2026-04-27.md \
    docs/security/threat-register.md \
    docs/security/compliance-snapshot-2026-04-27.md \
    docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md \
    docs/specs/2026-04-27-sophia-status.md \
    docs/specs/2026-04-27-wotan-status.md \
    docs/specs/2026-04-27-anamnesis-status.md \
    docs/specs/2026-04-27-iana-snapshot.md; do
    test -s "$f" && echo "PRESENT: $f" || echo "MISSING: $f"
  done
  ```
- [ ] **Step 131** [V][L] ~5s: Zero MISSING — if any, STUCK Report.

### H3: Remote-handoff index

- [ ] **Step 132** [W][L] ~15m: Create `docs/sprints/2026-04-27-remote-handoff-index.md`:
  - **Packet 1**: WAVE13 Phase 2 quality run → `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md` (~2-4h on GPU box)
  - **Packet 2**: Branch hygiene execution → `docs/branch-audits/2026-04-27-summary.md` (Linux box, post-build verification)
  - **Packet 3**: SBOM + cargo-deny + go-licenses → `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md` (Linux dev box)
  - For each: prerequisites, est. wall-clock, success criteria, follow-up commit message template
- [ ] **Step 133** [V][L] ~5s: Index lists all 3 packets
  ```bash
  grep -cE "^### Packet [123]" /sessions/funny-kind-hopper/mnt/unheaded/docs/sprints/2026-04-27-remote-handoff-index.md
  ```
- [ ] **Step 134** [V][L] ~5s: Should equal 3

### H4: Local Round Table mini-doc

- [ ] **Step 135** [W][L] ~10m: Append a "Local Sprint Mini-Round-Table" block to root `battle-plan.md`:
  - Wins (cited from this sprint's commits)
  - Citations cleared (timeline drift → 0d, off-tree plan → in-tree)
  - Citations remaining (none new this sprint locally)
  - Pending verdicts: WAVE13 Phase 2 (remote), Captain Track call (Stevie), 3 branch dispositions (remote)
  - Next session trigger: any of the 3 remote packets completing OR Fri 2026-05-01 sprint-exit Round Table

### H5: Stuck Report (if needed)

- [ ] **Step 136** [W][L] ~5m: If any STUCK markers fired in Phases 0-7, write `docs/sprints/2026-04-27-stuck-report.md` with prioritized intervention list. Otherwise write a 1-line "zero stuck" log.
- [ ] **Step 137** [V][L] ~5s: Either stuck-report or zero-stuck log present
  ```bash
  ls /sessions/funny-kind-hopper/mnt/unheaded/docs/sprints/2026-04-27-*.md 2>&1
  ```

### H6: Final commit

- [ ] **Step 138** [C][L] ~10s: **CHECKPOINT 10 (FINAL)**
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git add -A && git commit -m "[PLAN SPRINT-04-27 LOCAL] Steps 126-138: sprint exit — local Round Table + remote-handoff index + stuck report" --no-gpg-sign
  ```
- [ ] **Step 139** [B][L] ~5s: Final state snapshot
  ```bash
  cd /sessions/funny-kind-hopper/mnt/unheaded && git log --oneline --since="2026-04-27" | wc -l && git status -sb
  ```
- [ ] **Step 140** [V][L] ~5s: **PHASE 8 / SPRINT EXIT GATE** — all local P0 artifacts present, 3 remote-handoff packets indexed, drift = 0d, working tree clean. **LOCAL SPRINT COMPLETE. <3**

---

## APPENDIX A: EMERGENCY PROCEDURES (LOCAL)

### EP-1: timeline.md re-sync introduces contradictions (Step 22)
1. Diff backup: `diff references/timeline.md.pre-2026-04-27.bak references/timeline.md`
2. Locate ✅+📋 same-line: `grep -nE "✅.*📋|📋.*✅" references/timeline.md`
3. Hand-edit; re-verify.
4. After 2 attempts: STUCK; restore backup; mark Lane A3 incomplete.

### EP-2: SUPERSEDED edit corrupts the file
1. `git diff crates/zhenai-forge/notes/wave13-inference-battle-plan.md`
2. If wedged: `git checkout -- crates/zhenai-forge/notes/wave13-inference-battle-plan.md` and re-edit minimally (only the pointer line).

### EP-3: CISA KEV curl fails (Step 101)
1. Check connectivity: `curl -sI https://www.cisa.gov/`
2. If blocked → STUCK on Step 101 only; Step 103 records "feed unreachable on 2026-04-27, retry remote".
3. Do NOT skip threat register entirely — document the gap.

### EP-4: YAML validation fails on drift-guard.yml (Step 92)
1. `python3 -c "import yaml; print(yaml.safe_load(open('.github/workflows/drift-guard.yml')))"` — surfaces the parse error
2. Fix indentation (tabs vs spaces is the usual culprit).
3. Re-validate.

### EP-5: SPDX scan finds untagged Go files (Step 98)
1. List them: `cat /tmp/spdx-missing-go.txt`
2. Author SPDX backfill — single sed pass per file:
   `for f in $(cat /tmp/spdx-missing-go.txt); do sed -i '1i // SPDX-License-Identifier: GPL-3.0-or-later\n' "$f"; done`
3. Re-run Step 97; if still > 0 after 2 passes → STUCK.

### EP-6: Branch audit can't fetch remote (Steps 49/52/54)
1. Branches may be local-only — audit against `main` is still valid; note "no remote ref" in audit doc.
2. No STUCK; just lower confidence on the "currently pushed" question.

---

## APPENDIX B: AGENT ASSIGNMENT MATRIX (LOCAL)

| Phase | Lane | Time | Locally Doable | Remote Packet |
|------:|:-----|-----:|:---------------|:--------------|
| 0 | Preflight | 5m | 100% | — |
| 1 | A — Drift | 90m | 100% | — |
| 2 | B — WAVE13 prep | 60m | Skeletons + packet | **YES** (run on GPU box) |
| 3 | C — Branch audits | 45m | Audit docs | **YES** (Linux build/test/merge) |
| 4 | D — Track call | 60m | Options + template + stubs | Decision = Stevie (anywhere) |
| 5 | G — ADR-052 + script | 60m | Script + YAML | Live CI run on first push |
| 6 | E — Compliance | 45m | SPDX + KEV + framework | **YES** (SBOM + license scans) |
| 7 | F — Specs | 60m | 100% | — |
| 8 | H — Sprint exit | 30m | 100% | — |

**Critical path (local)**: 0 → 1 → 2 → 4 → 5 → (6 ‖ 7) → 8 ≈ **5 hours** of pure local work.
**Total wall-clock with breaks**: ~5–6 hours, single Cowork session.
**Remote work that follows**: 3 packets, est. 4–6 hours total on Linux/GPU box.

---

## APPENDIX C: QUICK REFERENCE (LOCAL)

### Cowork-on-Macbook path mapping
```
Macbook (Read/Write/Edit):  /Users/govan/home 2/govan/tmp/unheaded
Linux sandbox (bash):       /sessions/funny-kind-hopper/mnt/unheaded
```
Same files, two namespaces. Use Macbook path for Read/Write/Edit tools; Linux path for `mcp__workspace__bash`.

### Local-safe commands
```
git log / git diff / git status / git branch -a   ✓
git add / git commit / git tag                    ✓
git push                                          ✓ (Stevie does — sandbox may not have credentials)
python3 / jq / curl / awk / sed / grep / find    ✓
yaml.safe_load smoke-test                         ✓
```

### Remote-only commands (do NOT attempt locally)
```
go build / go test                                ✗ (no Go in sandbox)
cargo build / cargo test (with HIP)               ✗ (no GPU, no AMD stack)
syft / cargo-deny / go-licenses                   ✗ (toolchains absent)
bpftool / ip link / ip netns                      ✗ (need Linux + sudo on real host)
zhenai-forge generate-gemma4                      ✗ (needs GGUF + LoRA + GPU)
```

### Three remote-handoff packets
1. `docs/battle-plans/WAVE13-PHASE2-REMOTE-PACKET.md`
2. `docs/branch-audits/2026-04-27-summary.md`
3. `docs/security/COMPLIANCE-REMOTE-PACKET-2026-04-27.md`

### Wins tracker (local)
- [ ] WAVE13 plan in-tree (citation #2 cleared)
- [ ] Timeline drift → 0d (citation #1 cleared)
- [ ] ADR-051 + ADR-052 drafted (one ACCEPTED, one DRAFT pending Phase 2)
- [ ] Drift-guard script + CI YAML written
- [ ] 3 branch audits + Linux execution checklist
- [ ] Track A/B/C options + decision template
- [ ] WAVE14 stub + demo script outline (conditional)
- [ ] CISA KEV refresh + SPDX coverage check
- [ ] 3 spec status docs + IANA snapshot
- [ ] 3 remote-handoff packets indexed
- [ ] Stuck Report (zero stuck or otherwise)

---

*SPRINT 2026-04-27 LOCAL Battle Plan — Forged 2026-04-27 from Cowork on Macbook*
*9 Phases. 140 Steps. Pure docs/scripts/specs scope. Three remote packets queued for the Linux box.*
*Soldiers fight, not think. The Macbook session ships paper; the Linux box ships steel. <3 KGLW <3 Peace and Love <3*
