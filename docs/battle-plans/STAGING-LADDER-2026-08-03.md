<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# STAGING LADDER BATTLE PLAN — 16 Phases, 214 Steps

**Date**: 2026-08-03
**Sprint**: The Staging Ladder — work the static-analysis backlog from `next.md` in
strict ascending risk order, on a `staging` branch built for per-item human review.
**Prerequisite**: `develop` at `b39fb207`, clean tree apart from 2 untracked files.
**Target**: Every backlog item from `next.md` either (a) fixed and committed as one
reviewable unit on `staging`, or (b) explicitly parked in the Stevie Decision Queue
with the decision written down and the options costed.
**Estimated Duration**: 10-16 hours autonomous, across one overnight `/loop`.
**Agent Strategy**: Solo/coordinator throughout. No sub-agents — every phase touches
the same working tree and the same lint baselines, so parallel agents would race.
**Commit Cadence**: One commit per *coherent unit*, not per N steps (see COMMIT POLICY).
**Stuck Protocol**: Skip after 3x time estimate or 2 failed debug attempts.

---

## WHY THIS PLAN EXISTS

`next.md` (2026-08-03) closed out the clippy drain — 348 → 0, ratcheted. What it left
behind is a backlog of four unratcheted linters plus three open judgement calls. The
handoff listed them as flat bullet points. A flat list is the wrong shape for review,
because these findings are *not* equally risky: renaming an unused shell variable and
rewriting 123 blind `except:` handlers are separated by three orders of magnitude of
blast radius, and they were sitting in the same bullet list.

This plan re-sorts the entire backlog along one axis — **probability the change alters
runtime behaviour × blast radius if it does** — and lays it out as a ladder. Stevie
verifies from the bottom rung up, and can stop climbing at any rung without leaving
the tree in a half-fixed state, because every rung is a self-contained commit.

**Governing policy** (do not re-derive, see ADR-073 and
`docs/security/findings-remediation-2026-07-29.md`):

- Fix **all** severities. Never filter a linter by severity to make a number go down.
- Ratchet by **rule-ID exclusion**, never by severity filter or by threshold count.
- A linter lands **non-gating** first so its baseline is measured honestly, then flips
  to **gating** once its backlog is worked.
- The baseline may only ever **shrink**.

---

## MEASURED BASELINE (2026-08-03, this session — not inherited from the handoff)

Every number below was re-measured at `b39fb207` with the CI-pinned tool versions
(`ruff==0.16.0`, `bandit==1.9.4`, distro shellcheck). Where this disagrees with
`next.md`, **this table wins** — `next.md` quoted bandit as "51 MEDIUM, 163 LOW" and
never gave a total; the real total is 212 and the LOW count is 161.

| Linter | Total | Breakdown | Gating today? |
|---|---|---|---|
| **ruff** | **427** | see rule table below | No — `continue-on-error: true` |
| **bandit** | **212** | 0 HIGH / 51 MEDIUM / 161 LOW | No — `continue-on-error: true` |
| **shellcheck** | **350** | 0 error / 98 warning / 252 note | Errors only (and errors are clean) |
| **eslint** | **unmeasured** | no config, no `package.json` found | No — job does not exist |
| clippy | 0 | ratcheted to zero | **Yes — gating** |
| gosec | ratcheted | — | **Yes — gating** |

### ruff 427 by rule

```
123 BLE001   blind-except                     <- Phase 8, the hard one
104 (syntax) invalid-syntax                   <- Phase 2, ALL from ONE notebook cell
 32 EXE001   shebang-not-executable           <- Phase 3
 31 PLW1510  subprocess-run-without-check     <- Phase 5
 30 S110     try-except-pass                  <- Phase 8
 16 DTZ005   call-datetime-now-without-tzinfo <- Phase 6
 10 E722     bare-except                      <- Phase 8
  9 LOG015   root-logger-call                 <- Phase 7
  8 F841     unused-variable                  <- Phase 7
  8 PLW0602  global-variable-not-assigned     <- Phase 7
  7 PIE810   multiple-starts-ends-with        <- Phase 7
  6 RUF059   unused-unpacked-variable         <- Phase 7
  5 ASYNC230 blocking-open-call-in-async      <- Phase 13
  5 SIM115   open-file-with-context-handler   <- Phase 7
  4 ISC004   implicit-str-concat-in-collection<- Phase 7
  3 SIM102   collapsible-if                   <- Phase 7
  3 TRY002   raise-vanilla-class              <- Phase 7
  2 ASYNC210 blocking-http-call-in-async      <- Phase 13
  2 C401     unnecessary-generator-set        <- Phase 7
  2 PERF102  incorrect-dict-iterator          <- Phase 7
  2 PLC0206  dict-index-missing-items         <- Phase 7
  2 RUF013   implicit-optional                <- Phase 7
  2 SIM103   needless-bool                    <- Phase 7
  1 each: ASYNC221 DTZ003 DTZ006 EXE002 F401 F821 PLR1722 SIM101 TRY004 TRY201 UP031
```

**`ruff --fix` has nothing left to give: 0 safe autofixes available.** The 246 easy
ones were already taken in `5b80e288`. All 427 remaining need a human or a careful
agent. 38 have `--unsafe-fixes` available and those are *not* to be trusted blindly —
same lesson as `cargo clippy --fix` in `next.md`.

### The single highest-leverage finding in the whole backlog

All **104** `invalid-syntax` errors come from **one file, one cell**:
`notebooks/02_hypothesis_matrix.ipynb`, cell 6, an unterminated string literal that
cascades the parser into 104 downstream complaints. Fixing one quote drops ruff
427 → 323, a 24% reduction, with **zero** risk to shipped code. That is Phase 2 and
it is the first real work in this plan.

Note this also means `scripts/check-python-syntax.sh` — which is GATING and green —
does not cover notebooks. Worth closing that gap (Phase 2, step 21).

### bandit 212 by rule

```
MEDIUM (51)
 35 B310 urllib urlopen — audit URL schemes (file://, custom) 
 10 B108 hardcoded_tmp_directory
  2 B615 huggingface_unsafe_download (unpinned revision)
  2 B314 xml parsing (defusedxml)
  2 B104 hardcoded_bind_all_interfaces          <- REAL exposure change, Phase 15
LOW (161)
 45 B603 subprocess_without_shell_equals_true
 31 B607 start_process_with_partial_path
 28 B110 try_except_pass  (overlaps ruff S110)
 26 B404 blacklist (subprocess import)
 21 B311 blacklist (random — non-crypto use)
  5 B101 assert_used
  2 B405 blacklist (xml import)
  2 B605 start_process_with_a_shell
  1 B105 hardcoded_password_string              <- MUST triage, no-creds rule
```

### shellcheck 350 by rule (top)

```
 62 SC2034 unused variable (many are false positives in sourced files)
 16 SC2155 declare-and-assign masks the exit code
  4 SC2046 unquoted command substitution word-splits
  3 SC2154 referenced but not assigned
  3 SC2064 trap uses double quotes, expands at set time not signal time
  2 SC2164 cd without || exit
  2 SC2088 tilde in quotes does not expand
  1 each: SC2069 SC2054 SC2043 SC2038 SC2024 SC1090
```

---

## RISK LADDER — the spine of this plan

| Rung | Phase | What | Findings | Why this risk level |
|---|---|---|---|---|
| **R0** | 1 | Untracked file disposition | 2 files | Not code. Nothing imports them. |
| **R0** | 2 | Broken notebook cell | 104 ruff | Notebook, never executed in CI or prod. |
| **R0** | 3 | `EXE001` shebang/mode mismatch | 33 ruff | File mode or comment only. No logic. |
| **R1** | 4 | shellcheck `SC2034`/`SC2155` | ~78 sc | Mechanical. `SC2155` fix *improves* error detection. |
| **R1** | 5 | `PLW1510` explicit `check=` | 31 ruff | `check=False` is byte-for-byte current behaviour. |
| **R1** | 6 | `DTZ*` timezone-aware datetimes | 18 ruff | Well-defined change; only affects naive-vs-aware compare. |
| **R2** | 7 | ruff misc single-digit rules | ~55 ruff | Small but each needs reading the surrounding code. |
| **R2** | 8 | **Exception handling** `BLE001`+`E722`+`S110` | 163 ruff | **The hard one.** Each site is a judgement call about what may be swallowed. |
| **R2** | 9 | bandit MEDIUM: `B310`/`B108`/`B314` | 47 bandit | Changes URL/tempdir/parser behaviour. Needs per-site reasoning. |
| **R2** | 10 | bandit LOW sweep + `B105` triage | 161 bandit | Mostly dispositions; `B105` could be a real credential. |
| **R3** | 11 | eslint from zero | unknown | Unmeasured surface. Config choice cascades. |
| **R3** | 12 | af-xdp ring regression test | 1 test | Refactors a hot path to be testable. Real risk. |
| **R3** | 13 | async blocking-call fixes | 8 ruff | Genuine concurrency semantics change. |
| — | 14 | Ratchet flips + CI gating | — | Locks in everything above. |
| — | 15 | **Stevie Decision Queue** | 5 items | Not churn. Needs a human call. |
| — | 16 | Docs, handoff, wake-up summary | — | Leaves the tree explainable. |

**The ladder is the deliverable.** If the loop only gets through Phase 8, Stevie wakes
to eight independently-reviewable commits and a plan that says exactly where it stopped.

---

## COMMIT POLICY — read this before executing

Stevie owns commits (`feedback_stevie_owns_commits`). This plan makes **one bounded
exception**, and it is the reason the plan exists:

> Stevie asked for "a staging branch where I manually verify each before we pull the
> changes into main." Verifying *each* is only possible if each item is a separate
> commit. A single 400-file uncommitted diff is not reviewable, and it is exactly what
> unattended churn produces without a commit policy.

Therefore, for this plan only:

1. **Branch**: all work lands on `staging`, branched from `develop` at `b39fb207`.
2. **Never** commit to `main` or `develop`. **Never** push. **Never** force anything.
3. One commit per coherent unit — usually one phase, sometimes one rule within a phase
   when the phase is large (Phase 8 commits per-directory).
4. Sign with `E89BB176CC72AAB4FCDDE753C832D4F8283BCE5C` when gpg-agent has the key
   cached. When it does not — which it will not, at 3am — use `--no-gpg-sign` rather
   than stalling on Stevie (`feedback_unsigned_commits_when_afk`). Note which commits
   are unsigned in the wake-up summary so they can be re-signed before promotion.
5. **Never commit red.** A commit fires only after that phase's `[V]` gate passed. If
   a gate fails and the debug branch is exhausted, mark `[STUCK]`, `git stash` or
   revert the partial work, commit nothing, and move to the next independent phase.
6. Commit message format:
   ```
   <type>(<scope>): <what changed>

   <why, and what the risk rung was>
   Ladder rung: R<N> — Phase <P>
   Baseline: <linter> <before> -> <after>
   ```

**Promotion path** (Stevie drives this, not the loop):
`staging` → review each commit → cherry-pick or fast-forward into `develop` → `main`.

---

## LEGEND

```
[B] bash    [V] verify (MUST pass)   [D] debug (only on failure)
[W] write   [R] read                 [S] sudo
[C] commit checkpoint                [SEQ] strictly sequential
[MEASURE] re-measure a baseline      [DECIDE] autonomous, recommendation pre-seeded
[ESCALATE] STOP, human required      [STUCK] skipped via Skip Protocol
[DOC] documentation update           [TRIAGE] classify without changing code
```

## VARIABLES

```bash
PROJECT_ROOT=$(git rev-parse --show-toplevel)     # /home/govan/tmp/unheaded
LINT=/tmp/claude-1000/-home-govan-tmp/7e7ecd76-4e65-4d98-b3c0-685be334a678/scratchpad/lintvenv/bin
RUFF="$LINT/ruff"                                  # pinned 0.16.0, matches CI
BANDIT="$LINT/bandit"                              # pinned 1.9.4, matches CI
BASE=b39fb207                                      # develop HEAD at plan time
```

## PREFLIGHT HYPOTHESES

| # | Hypothesis | How verified | If false |
|---|---|---|---|
| H1 | `develop` is clean but for 2 untracked files | `git status --short` | Re-baseline before touching anything |
| H2 | ruff total is 427 at `$BASE` | `$RUFF check .` | Re-measure, update this doc, continue |
| H3 | All 104 invalid-syntax are one notebook | `ruff --output-format concise \| grep syntax` | Phase 2 grows; re-scope |
| H4 | shellcheck errors are 0 (gating job green) | `shellcheck -S error` | Fix errors FIRST, they gate CI |
| H5 | Test suite is 979 pass / 0 fail | `go test ./...` | Record real baseline; never blame the plan for pre-existing red |
| H6 | No `package.json` → eslint needs config from scratch | `find -name package.json` | Phase 11 shrinks to running existing config |

## KNOWN FAILURES BASELINE

Recorded in Phase 0 **before any edit**. Every later phase compares against this, not
against zero. A test that was red at `$BASE` staying red is not a regression caused by
this plan, and must not be reported as one.

---

## PHASE 0: PREFLIGHT & STAGING BRANCH (Steps 1-14)

**Goal**: Prove the starting state, record the baselines, create `staging`.
**Time**: ~20m · **Agent**: Coordinator · **Risk**: none

- [ ] **Step 1** [B] ~1m: Confirm root and HEAD
  ```bash
  cd "$PROJECT_ROOT" && git rev-parse --short HEAD && git status --short
  ```
- [ ] **Step 2** [V]: HEAD is `b39fb207`, branch is `develop`, only the 2 known
      untracked files present. If HEAD moved → re-read `next.md`, update `$BASE`.
- [ ] **Step 3** [B] ~3m: Record test baseline
  ```bash
  go build ./... && go test ./... 2>&1 | tail -40 > /tmp/baseline-gotest.txt
  ```
- [ ] **Step 4** [B] ~5m: Record Rust baseline
  ```bash
  ./scripts/check-clippy.sh 2>&1 | tail -20 > /tmp/baseline-clippy.txt
  ```
- [ ] **Step 5** [MEASURE] ~2m: Record all four linter baselines to disk
  ```bash
  $RUFF check . --statistics > /tmp/baseline-ruff.txt 2>&1
  $BANDIT -q -r . -x ./llama.cpp,./target,./node_modules -f json \
    -o /tmp/baseline-bandit.json 2>/dev/null
  git ls-files '*.sh' | xargs -r shellcheck -f gcc > /tmp/baseline-shellcheck.txt 2>&1
  ```
- [ ] **Step 6** [V]: **BASELINE GATE** — ruff 427, bandit 212, shellcheck 350, 0 sc
      errors. Deviations recorded in the wake-up summary, not silently absorbed.
- [ ] **Step 7** [B] ~1m: Record the ascend-linux artifact size (the inertness signal)
  ```bash
  cd "$PROJECT_ROOT/ebpf" && cargo build --release -p monad-cpu-ebpf --features ascend-linux
  stat -c %s target/bpfel-unknown-none/release/monad-cpu-ebpf
  ```
- [ ] **Step 8** [V]: Size is **901888**. This plan touches no eBPF, so it must still
      be 901888 at the end. If it drifts, something in this plan was not inert.
- [ ] **Step 9** [B] ~1m: Create the staging branch
  ```bash
  cd "$PROJECT_ROOT" && git checkout -b staging
  ```
- [ ] **Step 10** [V]: `git branch --show-current` is `staging`.
- [ ] **Step 11** [W] ~2m: Write `docs/battle-plans/STAGING-LADDER-PROGRESS.md` — the
      live execution log. One line per completed phase: rung, commit SHA, baseline
      delta, signed/unsigned. This is what Stevie reads first.
- [ ] **Step 12** [DOC] ~1m: Note in the progress log which commits will be unsigned.
- [ ] **Step 13** [C]: Commit the plan + progress scaffold
  ```bash
  git add docs/battle-plans/STAGING-LADDER-*.md && \
  git commit --no-gpg-sign -m "docs(plan): staging ladder — backlog re-sorted by risk

Re-measures every static-analysis baseline at b39fb207 rather than inheriting
next.md's numbers (bandit total was never stated; real total is 212, not 214).
Ladder rung: R0 — Phase 0"
  ```
- [ ] **Step 14** [V]: **PHASE 0 EXIT GATE** — `staging` exists, baselines on disk,
      plan committed. Proceed to Phase 1.

---

## PHASE 1: UNTRACKED FILE DISPOSITION (Steps 15-20) — RUNG R0

**Goal**: Resolve the two untracked files `next.md` flagged. Zero code risk — nothing
in the tree references either.
**Time**: ~15m · **Risk**: R0

- [ ] **Step 15** [R] ~2m: Inspect both
  ```bash
  ls -la db/migrations/init.sh demos/doom/doom.data
  file demos/doom/doom.data && head -c 64 demos/doom/doom.data | xxd | head -4
  ```
- [ ] **Step 16** [B] ~2m: Prove nothing references them
  ```bash
  grep -rn "doom.data" --include='*.go' --include='*.rs' --include='*.py' \
    --include='*.sh' --include='*.js' . | grep -v node_modules | head
  grep -rn "migrations/init.sh" . --exclude-dir=.git | head
  ```
- [ ] **Step 17** [DECIDE]: Disposition.
      **RECOMMENDATION**: `db/migrations/init.sh` is 0 bytes and root-owned — delete
      it; an empty root-owned file in a migrations directory is an artifact of a sudo
      run, not a migration, and leaving it invites someone to fill it in without a
      review. `demos/doom/doom.data` is a 150 KB binary with no referrer — **gitignore
      rather than delete**, because deleting an unreferenced binary you did not create
      destroys the only copy, and 150 KB of ignored file costs nothing.
      *Rationale*: asymmetric cost. Deleting the empty file is reversible (it is empty);
      deleting the binary is not.
      *Override ONLY if* step 16 finds a referrer — then commit it instead.
- [ ] **Step 18** [B] ~3m: Apply
  ```bash
  rm -f db/migrations/init.sh
  grep -q 'demos/doom/doom.data' .gitignore || echo 'demos/doom/doom.data' >> .gitignore
  ```
- [ ] **Step 19** [V]: `git status --short` shows only the `.gitignore` change.
- [ ] **Step 20** [C]: **PHASE 1 COMMIT** — `chore(tree): dispose of two untracked
      files predating this work`. Body records *why* delete-vs-ignore split.

---

## PHASE 2: THE BROKEN NOTEBOOK (Steps 21-30) — RUNG R0

**Goal**: Fix one unterminated string literal; drop ruff 427 → ~323.
**Time**: ~30m · **Risk**: R0 (notebook, not executed by CI or any service)

- [ ] **Step 21** [R] ~3m: Locate the exact cell and line
  ```bash
  $RUFF check notebooks/02_hypothesis_matrix.ipynb --output-format concise | head -20
  ```
- [ ] **Step 22** [R] ~5m: Read cell 6 raw
  ```bash
  python3 -c "
import json
nb=json.load(open('notebooks/02_hypothesis_matrix.ipynb'))
for i,c in enumerate(nb['cells']):
    if i==6: print(''.join(c['source']))
"
  ```
- [ ] **Step 23** [W] ~10m: Repair the literal. **Minimal edit** — close the quote,
      change nothing else. Do not reformat the cell, do not "improve" the code: this
      is a lab notebook and its content is evidence, not product.
- [ ] **Step 24** [V] ~1m: Notebook still parses as JSON
  ```bash
  python3 -c "import json;json.load(open('notebooks/02_hypothesis_matrix.ipynb'));print('JSON OK')"
  ```
- [ ] **Step 25** [MEASURE] ~2m: `$RUFF check . 2>&1 | tail -2`
- [ ] **Step 26** [V]: **RUFF DROP GATE** — total is ~323 and `invalid-syntax` is 0.
      If invalid-syntax survives → more than one broken cell; loop steps 21-24.
- [ ] **Step 27** [R] ~3m: Check whether the gating syntax script covers notebooks
  ```bash
  cat scripts/check-python-syntax.sh
  ```
- [ ] **Step 28** [DECIDE]: Extend `check-python-syntax.sh` to notebooks?
      **RECOMMENDATION**: yes — extend it to `git ls-files '*.ipynb'`, parsing each
      cell's source with `compile()`. *Rationale*: a GATING syntax check that reads
      "green" while a tracked file in the tree cannot be parsed is worse than no check,
      because it is trusted. This is the same class of blind spot as the `pub`-in-a-
      binary-crate dead_code hole from `next.md`.
      *Override ONLY if* the script is shared with a workflow that cannot install
      `nbformat` — it need not, `json` + `compile()` from stdlib suffice.
- [ ] **Step 29** [W][V] ~10m: Extend the script; run it; it must pass now and must
      have failed before the Step 23 fix (prove it by `git stash`-ing the fix once).
- [ ] **Step 30** [C]: **PHASE 2 COMMIT** — `fix(notebooks): close unterminated string
      literal; teach the syntax gate about notebooks`. Body: one cell, 104 findings.

---

## PHASE 3: EXE001 SHEBANG/MODE MISMATCH (Steps 31-40) — RUNG R0

**Goal**: 33 findings (32 `EXE001` + 1 `EXE002`) where a file's shebang and its
executable bit disagree. Resolution is a file mode change or a comment removal —
no logic changes at all.
**Time**: ~40m · **Risk**: R0

- [ ] **Step 31** [B] ~2m: List every offender with its current mode
  ```bash
  $RUFF check . --select EXE --output-format concise | cut -d: -f1 | sort -u \
    | while read -r f; do printf '%s %s\n' "$(stat -c %a "$f")" "$f"; done
  ```
- [ ] **Step 32** [TRIAGE] ~10m: Split into two buckets and write the split down.
      **Bucket A — genuinely executable** (invoked directly, `./script.py`, or named as
      an entrypoint anywhere): `chmod +x`.
      **Bucket B — library/module** (only ever imported, or invoked as `python3 x.py`):
      remove the shebang line.
      Decide per file by grepping for how it is invoked:
  ```bash
  grep -rn "$(basename "$f")" --include='*.sh' --include='*.yml' --include='*.yaml' \
    --include='Makefile' . | grep -v node_modules | head -3
  ```
- [ ] **Step 33** [B] ~5m: Apply Bucket A
- [ ] **Step 34** [B] ~5m: Apply Bucket B
- [ ] **Step 35** [V] ~2m: `$RUFF check . --select EXE` returns 0.
- [ ] **Step 36** [V] ~3m: No file lost its executable bit that a caller depends on
  ```bash
  git diff --summary | grep 'mode change' 
  ```
      Cross-check every `100755 -> 100644` against step 32's Bucket B list.
- [ ] **Step 37** [V] ~3m: Gating syntax check still green
  ```bash
  ./scripts/check-python-syntax.sh
  ```
- [ ] **Step 38** [MEASURE]: ruff total ~290.
- [ ] **Step 39** [V]: **PHASE 3 EXIT GATE** — EXE rules 0, no unintended mode changes.
- [ ] **Step 40** [C]: **PHASE 3 COMMIT** — `chore(python): reconcile shebangs with
      executable bits`. Body must list which files became non-executable and why.

---

## PHASE 4: SHELLCHECK SC2034 + SC2155 (Steps 41-56) — RUNG R1

**Goal**: The two dominant shellcheck rules, 78 of 350 findings.
**Time**: ~90m · **Risk**: R1 — mechanical, but `SC2034` has real false positives.

**Why SC2155 is a fix and not a style nit**: `local x=$(cmd)` makes `local` the command
whose exit status is reported, so a failing `cmd` is silently swallowed. Splitting into
`local x; x=$(cmd)` restores `set -e` and `$?`. These scripts drive boot recipes under
sudo — a swallowed failure there is a real operational bug, not lint noise.

- [ ] **Step 41** [B] ~2m: Enumerate SC2155 sites
  ```bash
  git ls-files '*.sh' | xargs -r shellcheck -f gcc 2>&1 | grep SC2155
  ```
- [ ] **Step 42** [W] ~30m: Fix all 16 — split declaration from assignment. Preserve
      `local`/`export`/`readonly` semantics exactly.
- [ ] **Step 43** [V] ~2m: `shellcheck ... | grep -c SC2155` is 0.
- [ ] **Step 44** [V] ~5m: **BEHAVIOUR GATE** — every touched script still parses and
      its `--help`/dry-run path works
  ```bash
  for f in $(git diff --name-only); do bash -n "$f" && echo "parse OK $f"; done
  ```
- [ ] **Step 45** [C]: Commit SC2155 alone — `fix(scripts): split declare-and-assign so
      command failures are not swallowed`.
- [ ] **Step 46** [B] ~2m: Enumerate SC2034 sites
  ```bash
  git ls-files '*.sh' | xargs -r shellcheck -f gcc 2>&1 | grep SC2034
  ```
- [ ] **Step 47** [TRIAGE] ~25m: **This rule lies in sourced files.** A variable
      assigned in `lib.sh` and consumed in `main.sh` reads as unused to shellcheck,
      which analyses one file at a time. Split into:
      **A — genuinely dead**: delete the assignment.
      **B — consumed by a sourcing script**: add `# shellcheck disable=SC2034` with a
      comment naming the consumer. Do **not** blanket-disable at file top.
      **C — consumed by `eval`/indirect expansion**: same as B, name the mechanism.
      Verify each candidate before deleting:
  ```bash
  grep -rn '\bVARNAME\b' --include='*.sh' . | grep -v node_modules
  ```
- [ ] **Step 48** [D]: If a variable's consumer cannot be found in 2 attempts, treat as
      bucket B with `disable` + `# consumer unknown, kept pending review` — deleting a
      variable you cannot trace is how a boot recipe breaks at 3am on real hardware.
- [ ] **Step 49-52** [W] ~30m: Apply buckets A, B, C; re-run; iterate.
- [ ] **Step 53** [V]: `shellcheck ... | grep -c SC2034` is 0.
- [ ] **Step 54** [V] ~5m: **GATING JOB STILL GREEN** — errors must remain 0
  ```bash
  git ls-files '*.sh' | xargs -r shellcheck -S error -f gcc && echo "ERRORS CLEAN"
  ```
- [ ] **Step 55** [MEASURE]: shellcheck total ~272.
- [ ] **Step 56** [C]: **PHASE 4 COMMIT** — `fix(scripts): resolve SC2034; annotate
      cross-file consumers rather than deleting blind`.

---

## PHASE 5: PLW1510 EXPLICIT subprocess check= (Steps 57-66) — RUNG R1

**Goal**: 31 `subprocess.run()` calls with no `check=`. Python's default is
`check=False`, so **writing `check=False` explicitly is a provably behaviour-preserving
change** — that is what makes this rung R1 and not R2.
**Time**: ~60m · **Risk**: R1

- [ ] **Step 57** [B] ~2m: Enumerate
  ```bash
  $RUFF check . --select PLW1510 --output-format concise
  ```
- [ ] **Step 58** [TRIAGE] ~20m: For each site decide `check=False` (preserve) vs
      `check=True` (fix a latent bug).
      **DEFAULT to `check=False`.** Only choose `check=True` where the very next lines
      already inspect `.returncode`, or where a silent failure would corrupt state.
      *Rationale*: this phase's whole claim to R1 is behaviour preservation. Every
      `check=True` is a behaviour change smuggled into a mechanical phase. Any site
      that wants `check=True` gets **listed in the commit body** so review can find it,
      and if there is more than a handful, they move to their own commit.
- [ ] **Step 59-62** [W] ~25m: Apply.
- [ ] **Step 63** [V] ~2m: `$RUFF check . --select PLW1510` returns 0.
- [ ] **Step 64** [V] ~5m: Every touched file still imports and parses
  ```bash
  for f in $(git diff --name-only -- '*.py'); do python3 -m py_compile "$f" || echo "FAIL $f"; done
  ```
- [ ] **Step 65** [MEASURE]: ruff ~259.
- [ ] **Step 66** [C]: **PHASE 5 COMMIT** — body lists every `check=True` site
      separately from the `check=False` bulk.

---

## PHASE 6: DTZ TIMEZONE-AWARE DATETIMES (Steps 67-78) — RUNG R1

**Goal**: 16 `DTZ005` + 1 `DTZ003` + 1 `DTZ006` = 18 naive `datetime` calls.
**Time**: ~60m · **Risk**: R1 — but read the warning below.

**The one real hazard**: mixing naive and aware datetimes raises `TypeError` on
comparison. So a half-migrated file is *worse* than an unmigrated one. **Migrate whole
files, never individual call sites.** If a file's datetimes flow into something that
persists or compares them (a DB column, a JSON field another service parses, a cache
key), check the consumer before changing the producer.

- [ ] **Step 67** [B] ~2m: Enumerate and group by file
  ```bash
  $RUFF check . --select DTZ --output-format concise | cut -d: -f1 | sort | uniq -c
  ```
- [ ] **Step 68** [TRIAGE] ~20m: Per file, trace where the datetime goes. Mark files
      whose output crosses a process boundary (written to DB/JSON/wire) as **needs
      consumer check**; do those last and note them in the commit body.
- [ ] **Step 69-73** [W] ~25m: `datetime.now()` → `datetime.now(timezone.utc)`,
      `datetime.utcnow()` → `datetime.now(timezone.utc)`,
      `datetime.fromtimestamp(x)` → `datetime.fromtimestamp(x, tz=timezone.utc)`.
      Add the `timezone` import. One file at a time; whole file each time.
- [ ] **Step 74** [V] ~2m: `$RUFF check . --select DTZ` returns 0.
- [ ] **Step 75** [V] ~10m: **NAIVE/AWARE MIX GATE** — grep each touched file for any
      remaining naive constructor and for comparisons against one
  ```bash
  for f in $(git diff --name-only -- '*.py'); do
    grep -n 'datetime\.\(now\|utcnow\|fromtimestamp\)(' "$f" | grep -v 'tz\|timezone' \
      && echo "MIXED: $f"
  done
  ```
- [ ] **Step 76** [V] ~5m: Any test touching these files still passes.
- [ ] **Step 77** [MEASURE]: ruff ~241.
- [ ] **Step 78** [C]: **PHASE 6 COMMIT** — body names any file whose datetimes cross
      a process boundary and what was checked on the consumer side.

---

## PHASE 7: RUFF MISC — THE SINGLE-DIGIT RULES (Steps 79-104) — RUNG R2

**Goal**: ~55 findings across 20 rules. Individually trivial, collectively needs care
because each one requires actually reading the surrounding function.
**Time**: ~2h · **Risk**: R2

Work rule-by-rule, commit in **three** batches so review is tractable.

**Batch A — provably inert** (Steps 79-86): `F401` unused-import (1), `F841`
unused-variable (8), `RUF059` unused-unpacked (6), `PIE810` multiple-startswith (7),
`C401` unnecessary-generator (2), `PERF102` dict-iterator (2), `PLR1722` `exit()`→
`sys.exit()` (1), `UP031` printf-format (1), `ISC004` implicit-concat (4).
- [ ] **Step 79-85** [W]: Apply. `F841` — confirm the variable is not a deliberate
      `_`-style placeholder or a call kept for its side effect **before** deleting the
      assignment; keep the call, drop only the binding.
- [ ] **Step 86** [C]: Commit Batch A.

**Batch B — needs reading the logic** (Steps 87-96): `SIM102` collapsible-if (3),
`SIM103` needless-bool (2), `SIM101` duplicate-isinstance (1), `SIM115`
open-without-context (5), `PLW0602` global-not-assigned (8), `PLC0206` dict-index (2),
`RUF013` implicit-optional (2), `F821` undefined-name (1).
- [ ] **Step 87** [ESCALATE-CHECK] ~5m: `F821` undefined-name is **potentially a live
      bug, not lint** — a name used that does not exist. Investigate it first and on
      its own. If it is reachable code, it is a bug fix and gets its own commit with a
      failing-then-passing test if one can be written cheaply.
- [ ] **Step 88-90** [W]: `SIM115` — convert to `with` blocks. Watch for handles
      deliberately kept open past the block (a long-lived log file); those get a
      `# noqa: SIM115` with the reason, not a forced context manager.
- [ ] **Step 91-95** [W]: Remainder.
- [ ] **Step 96** [C]: Commit Batch B.

**Batch C — logging and exceptions-adjacent** (Steps 97-103): `LOG015` root-logger
call (9), `TRY002` raise-vanilla-class (3), `TRY004` type-check-without-TypeError (1),
`TRY201` verbose-raise (1).
- [ ] **Step 97-99** [W]: `LOG015` — replace `logging.info(...)` with a module logger
      `log = logging.getLogger(__name__)`. This changes which logger handles the record;
      confirm no handler config depends on root before switching.
- [ ] **Step 100-102** [W]: `TRY*` — vanilla `raise Exception(...)` becomes a specific
      type. Where no suitable type exists, define one; do not reach for a broad builtin.
- [ ] **Step 103** [C]: Commit Batch C.
- [ ] **Step 104** [V]: **PHASE 7 EXIT GATE** — ruff ~186, all of Phase 7's rules 0,
      `python3 -m py_compile` clean across every touched file.

---

## PHASE 8: EXCEPTION HANDLING — THE HARD ONE (Steps 105-140) — RUNG R2

**Goal**: `BLE001` blind-except (123) + `S110` try-except-pass (30) + `E722`
bare-except (10) = **163 findings, 38% of the entire ruff backlog.**
**Time**: ~4h · **Risk**: R2, the highest-judgement work in this plan.

**Why this cannot be mechanised.** `except Exception:` is sometimes correct — a
supervisor loop that must not die, a best-effort cache write, a cleanup path. And it is
sometimes a bug that swallows `KeyError` from a typo and returns silently wrong data.
Telling them apart requires reading what the `try` body can actually raise. There is no
autofix and `--unsafe-fixes` must not be used here.

**The discipline**: for each site, name the exceptions the body can raise. Then:
- **Narrow** — replace with the specific types. Preferred outcome.
- **Keep, but log** — supervisor/cleanup paths keep the broad catch and gain a
  `log.exception(...)`. A broad catch that is *silent* is the actual defect;
  a broad catch that is *loud* is a design choice.
- **Keep, annotated** — `# noqa: BLE001` with a one-line reason. Last resort, and the
  reason must say what would break if it were narrowed.

**`S110`/`B110` (`except: pass`) is the worst of the three** — broad *and* silent.
Default to adding logging; a bare `pass` survives only where the exception is genuinely
expected and uninteresting (e.g. best-effort `os.remove` of a temp file), and then the
`pass` gets a comment saying so.

**Commit per directory**, not per rule, so each commit is one coherent area a reviewer
can hold in their head.

- [ ] **Step 105** [B] ~3m: Enumerate and group by directory
  ```bash
  $RUFF check . --select BLE001,S110,E722 --output-format concise \
    | cut -d/ -f1 | sort | uniq -c | sort -rn
  ```
- [ ] **Step 106** [TRIAGE] ~30m: Build the worksheet — one row per site: file:line,
      rule, what the try body calls, what it can raise, chosen disposition. Write it to
      `docs/security/exception-handling-triage-2026-08-03.md`. **This document is a
      deliverable**, not scaffolding: it is the evidence that 163 sites were reasoned
      about rather than bulk-edited.
- [ ] **Step 107** [V]: Worksheet has a row for all 163 sites before any edit is made.
- [ ] **Steps 108-136** [W] ~3h: Work directory by directory, largest first. After each
      directory:
      - `$RUFF check <dir> --select BLE001,S110,E722` → 0
      - `python3 -m py_compile` every touched file
      - any test covering that directory still passes
      - `[C]` commit that directory
- [ ] **Step 137** [V] ~5m: All three rules 0 tree-wide.
- [ ] **Step 138** [V] ~10m: **SWALLOWED-FAILURE GATE** — count how many broad catches
      remain and confirm every one is either logged or `noqa`-annotated with a reason
  ```bash
  grep -rn 'except Exception' --include='*.py' . | grep -v node_modules | wc -l
  grep -rn 'noqa: BLE001' --include='*.py' . | wc -l
  ```
- [ ] **Step 139** [MEASURE]: ruff ~23. bandit `B110` should fall from 28 in step 141.
- [ ] **Step 140** [V]: **PHASE 8 EXIT GATE** — triage doc complete, all sites either
      narrowed, logged, or annotated with a stated reason.

---

## PHASE 9: BANDIT MEDIUM (Steps 141-160) — RUNG R2

**Goal**: 47 of 51 MEDIUM (the 2 `B104` bind-all go to the Decision Queue, Phase 15).
**Time**: ~2.5h · **Risk**: R2

- [ ] **Step 141** [MEASURE] ~3m: Re-run bandit — Phase 8 should already have moved
      `B110` (28 LOW) and possibly some MEDIUM counts.
- [ ] **Step 142** [B] ~2m: Enumerate `B310` (35, the bulk)
  ```bash
  python3 - <<'PY'
import json
d=json.load(open('/tmp/baseline-bandit.json'))
for r in d['results']:
    if r['test_id']=='B310': print(f"{r['filename']}:{r['line_number']}")
PY
  ```
- [ ] **Step 143** [TRIAGE] ~30m: `B310` fires on `urllib.request.urlopen` because the
      URL could be `file://` or a custom scheme. Per site, determine whether the URL is
      **(a)** a hardcoded literal → add `# nosec` with the literal quoted as the reason;
      **(b)** built from config/env → validate the scheme against an allowlist before
      the call; **(c)** derived from anything user-influenced → allowlist **and** treat
      as a genuine finding worth its own note.
- [ ] **Step 144-149** [W] ~50m: Apply. Where a scheme check is added, prefer one small
      shared helper over 35 copies of the same four lines.
      **Note the gosec lesson** (`reference_gosec_nosec_directive`): bandit's `# nosec`
      must be positioned per bandit's own rules — verify by re-running, never assume the
      suppression took.
- [ ] **Step 150** [V] ~3m: `B310` count is 0 or every survivor is deliberately
      suppressed with a reason.
- [ ] **Step 151** [TRIAGE] ~15m: `B108` hardcoded `/tmp` (10). Replace with
      `tempfile.mkdtemp()`/`NamedTemporaryFile` **unless** the path is a documented
      interface another process reads — in which case it is not a bug, it is an API,
      and it gets `# nosec` naming the reader.
- [ ] **Step 152-155** [W] ~30m: Apply `B108`.
- [ ] **Step 156** [W] ~15m: `B314`/`B405` xml (4) — swap to `defusedxml` where the
      input is untrusted; `# nosec` + reason where the XML is a build artifact we
      produced ourselves.
      **`defusedxml` is a new dependency** → Barrister check (license) before adding.
      If that stalls, use `xml.etree` with entity resolution disabled instead and skip
      the dependency entirely. **Prefer the no-new-dependency route.**
- [ ] **Step 157** [V] ~5m: Any test touching XML paths still passes.
- [ ] **Step 158** [MEASURE]: bandit MEDIUM down to 2 (`B104` only).
- [ ] **Step 159** [V]: **PHASE 9 EXIT GATE** — 0 unexplained MEDIUM.
- [ ] **Step 160** [C]: Commit per rule (`B310`, `B108`, `B314`) — three commits.

---

## PHASE 10: BANDIT LOW + B105 TRIAGE (Steps 161-176) — RUNG R2

**Goal**: 161 LOW. Most are dispositions rather than code changes — but `B105`
hardcoded-password gets investigated **first and seriously**.
**Time**: ~2h · **Risk**: R2

- [ ] **Step 161** [ESCALATE-CHECK] ~15m: **`B105` hardcoded_password_string (1).**
      The no-credentials-in-repo rule is absolute (`feedback_no_creds_in_repo`,
      CLAUDE.md). Locate it, read it, and determine whether it is a real credential or
      a variable named `password` holding a non-secret.
  ```bash
  python3 - <<'PY'
import json
d=json.load(open('/tmp/baseline-bandit.json'))
for r in d['results']:
    if r['test_id']=='B105':
        print(r['filename'], r['line_number']); print(r['code'])
PY
  ```
      **If it is a real credential**: STOP this phase. It is not a lint finding, it is
      an incident-shaped problem. Write it up in `docs/security/`, flag it at the very
      top of the wake-up summary, and do **not** quietly fix-and-bury it — gitleaks has
      a shrink-only baseline for exactly this reason and a new secret must not be
      silenced by appending a fingerprint.
      **If it is a false positive** (a field name, a test fixture, a prompt string):
      `# nosec` with the reason, and move on.
- [ ] **Step 162** [TRIAGE] ~30m: `B603`(45) + `B607`(31) + `B404`(26) = 102 findings,
      all "you used subprocess." These are overwhelmingly **dispositions**: this repo
      drives real tooling and subprocess is the correct instrument. The genuine
      questions are (a) is any argument attacker-influenced, and (b) is the binary
      resolved by full path where it matters.
      Fix `B607` (partial path) where the binary is security-relevant or where PATH
      ambiguity could pick the wrong tool; disposition the rest **as a group** in
      `docs/security/`, not with 102 individual `# nosec` comments.
- [ ] **Step 163-167** [W] ~40m: Apply `B607` full-path fixes; write the group
      disposition doc.
- [ ] **Step 168** [TRIAGE] ~15m: `B311` random (21) — non-cryptographic use is fine
      (jitter, sampling, demo data). Confirm **none** feeds a token, key, nonce, or
      session ID. Any that does is a real finding and gets `secrets` instead.
- [ ] **Step 169** [W] ~10m: `B101` assert_used (5) — asserts vanish under `-O`. If any
      guards a security property or validates external input, convert to an explicit
      raise. Test-file asserts are fine.
- [ ] **Step 170** [W] ~10m: `B605` shell=True (2) — highest-severity pattern in the
      LOW bucket despite its rating. Convert to arg-list form unless a shell feature is
      genuinely required.
- [ ] **Step 171** [V] ~5m: Re-run bandit; every survivor has a disposition.
- [ ] **Step 172-175** [C]: Commit in groups — `B105` triage, `B607`, `B311`/`B101`,
      `B605`.
- [ ] **Step 176** [V]: **PHASE 10 EXIT GATE** — bandit total ≤ ~110, all remaining
      findings dispositioned in writing.

---

## PHASE 11: ESLINT FROM ZERO (Steps 177-188) — RUNG R3

**Goal**: Measure the JS surface for the first time. `next.md` says "never measured";
this session confirmed there is **no `package.json` and no eslint config anywhere**.
**Time**: ~1.5h · **Risk**: R3 — unmeasured surface, config choice cascades.

- [ ] **Step 177** [B] ~3m: Size the surface
  ```bash
  git ls-files '*.js' | grep -v node_modules | wc -l
  git ls-files '*.js' | grep -v node_modules | xargs wc -l | tail -1
  ```
- [ ] **Step 178** [R] ~10m: Determine the dialect. Dashboard/kanban JS is vanilla
      browser JS per CLAUDE.md ("no framework overhead") — so the config needs
      `browser` globals, not `node`, and probably `es2022` + `script` (not `module`)
      unless the files use `import`.
  ```bash
  git ls-files '*.js' | grep -v node_modules | xargs grep -l '^import\|^export' | head
  ```
- [ ] **Step 179** [DECIDE]: Config shape.
      **RECOMMENDATION**: flat config (`eslint.config.js`) with `eslint:recommended`
      only — no stylistic plugins, no framework presets, no `package.json` beyond what
      eslint needs. *Rationale*: the whole point of vanilla JS here was avoiding a
      toolchain; introducing a 400-package dev dependency tree to lint 21K LOC inverts
      that trade. `eslint:recommended` catches the real class (undefined vars, unreachable
      code, duplicate keys) without importing an opinion about semicolons.
      *Override ONLY if* the JS turns out to be modules with a build step already.
- [ ] **Step 180** [W] ~15m: Write `eslint.config.js` + a minimal `package.json`
      pinning eslint exactly (same pinning discipline as ruff/bandit — an unpinned
      linter silently moves the baseline).
- [ ] **Step 181** [B] ~10m: **MEASURE THE BASELINE — do not fix anything yet**
  ```bash
  npx --yes eslint . 2>&1 | tail -20
  ```
- [ ] **Step 182** [V]: A number exists. Record it. This is the deliverable of the
      phase — an honest first measurement, per the ratchet policy.
- [ ] **Step 183** [DECIDE]: Fix now or land non-gating?
      **RECOMMENDATION**: land the config **non-gating** with the baseline recorded, and
      fix only findings that are unambiguous bugs (undefined variable, unreachable code,
      duplicate object key). *Rationale*: this is the last rung before the risky
      structural work, at the end of a long unattended run. Bulk-editing browser JS with
      no test coverage at 4am is exactly the kind of change that looks green and breaks
      the dashboard. Measure honestly, fix the real bugs, leave the rest ratcheted.
- [ ] **Step 184-186** [W] ~30m: Fix only the unambiguous-bug subset. Each one gets a
      line in the commit body.
- [ ] **Step 187** [W] ~10m: Add the eslint job to `.github/workflows/static-analysis.yml`
      as `continue-on-error: true`, with a header comment recording the measured
      baseline — matching the existing shellcheck/ruff/bandit job style exactly.
- [ ] **Step 188** [C]: **PHASE 11 COMMIT** — `ci(eslint): measure the JS surface for
      the first time; land non-gating with baseline`.

---

## PHASE 12: AF-XDP RING REGRESSION TEST (Steps 189-200) — RUNG R3

**Goal**: The `ff39131c` wraparound fix is currently **reasoned, not demonstrated** —
`next.md` is explicit that it has no test because the ring types need a live AF_XDP
socket and `CAP_NET_ADMIN`. Close that.
**Time**: ~2h · **Risk**: R3 — refactors a 920 Kpps hot path to be testable.

**The bug being tested**: `CompletionRing::consume` used `producer - consumer` on
free-running `u32` counters. At wraparound that is a debug panic, or in release an
`available` near 4.29e9 fed into `Vec::with_capacity` — a ~34 GB allocation. No attacker
needed, only ~1 hour at 920 Kpps.

- [ ] **Step 189** [R] ~15m: Read `ebpf/af-xdp/src/umem.rs` — both `CompletionRing` and
      the neighbouring `RxRing` (which already had `wrapping_sub` and is the reference
      for what correct looks like).
- [ ] **Step 190** [DECIDE]: Test strategy.
      **RECOMMENDATION**: make the ring constructible from raw pointers behind
      `#[cfg(test)]` (or a `pub(crate)` unsafe constructor), then build a fake ring in
      ordinary heap memory and drive the counters across the `u32` boundary.
      *Rationale*: the privileged-integration-test route needs `CAP_NET_ADMIN` in CI,
      which means the test would be skipped in exactly the environment that is supposed
      to catch the regression. A test that does not run is not a test.
      *Override ONLY if* the ring's layout genuinely cannot be synthesised without a
      kernel-registered UMEM — in which case document why and mark `[STUCK]`.
- [ ] **Step 191** [W] ~30m: Add the test-only constructor. **Do not change the
      production constructor's signature or behaviour.**
- [ ] **Step 192** [W] ~30m: Write the test: set `producer` just below `u32::MAX`, push
      past the boundary, assert `available` is the small true count and never a huge
      number. Include the `RxRing` equivalent as a control that already passes.
- [ ] **Step 193** [V] ~5m: **RED-THEN-GREEN** — revert the `wrapping_sub` fix locally,
      confirm the test **fails**, restore the fix, confirm it **passes**. A regression
      test never observed failing is not known to test anything.
  ```bash
  cd "$PROJECT_ROOT/ebpf" && cargo test -p af-xdp 2>&1 | tail -20
  ```
- [ ] **Step 194** [V] ~5m: Full af-xdp suite green.
- [ ] **Step 195** [V] ~10m: **CLIPPY STILL ZERO** — this touches Rust and clippy is
      gating at 0
  ```bash
  cd "$PROJECT_ROOT" && ./scripts/check-clippy.sh
  ```
- [ ] **Step 196** [V] ~5m: **ARTIFACT INERTNESS** — af-xdp is not `monad-cpu-ebpf`, so
      901888 must be unchanged
  ```bash
  cd "$PROJECT_ROOT/ebpf" && cargo build --release -p monad-cpu-ebpf --features ascend-linux
  stat -c %s target/bpfel-unknown-none/release/monad-cpu-ebpf
  ```
- [ ] **Step 197** [D]: If `scripts/bpf-verifier-check.sh` was run at any point it
      clobbers the ascend-linux build (documented in `next.md`) — rebuild with the
      feature and re-confirm 901888 before trusting step 196.
- [ ] **Step 198** [DOC] ~10m: Update
      `docs/security/af-xdp-completion-ring-wraparound-2026-08-03.md` — it currently
      says the fix is untested. Replace that section with what the test does.
- [ ] **Step 199** [V]: **PHASE 12 EXIT GATE** — test demonstrably red without the fix,
      green with it; clippy 0; artifact 901888.
- [ ] **Step 200** [C]: **PHASE 12 COMMIT** — `test(af-xdp): demonstrate the completion
      ring wraparound the fix prevents`.

---

## PHASE 13: ASYNC BLOCKING CALLS (Steps 201-206) — RUNG R3

**Goal**: `ASYNC230` blocking-open (5) + `ASYNC210` blocking-http (2) + `ASYNC221`
run-process (1) = 8 findings.
**Time**: ~45m · **Risk**: R3 — genuine concurrency semantics change.

**Why R3 despite only 8 findings**: fixing these changes when the event loop yields.
A blocking `open()` inside a coroutine stalls every other task; replacing it with a
thread-pool call means the surrounding code can now interleave where it previously
could not. That can surface latent ordering assumptions.

- [ ] **Step 201** [B] ~2m: `$RUFF check . --select ASYNC --output-format concise`
- [ ] **Step 202** [TRIAGE] ~15m: Per site — is the blocking call on a hot path or in
      startup/config-load code? Startup-path blocking calls are **not worth the risk**:
      annotate with `# noqa` + reason. Hot-path ones are worth fixing.
- [ ] **Step 203** [W] ~20m: Fix hot-path sites (`asyncio.to_thread` for file/process,
      an async HTTP client for `ASYNC210`). **A new async HTTP dependency needs a
      licence check** — if that stalls, `to_thread` around the existing sync client is
      the lower-risk move and needs no new dependency.
- [ ] **Step 204** [V] ~5m: `--select ASYNC` returns 0 or survivors are annotated.
- [ ] **Step 205** [V] ~5m: Any async test still passes.
- [ ] **Step 206** [C]: **PHASE 13 COMMIT** — body states which sites were fixed vs
      annotated and why.

---

## PHASE 14: RATCHET FLIPS & CI GATING (Steps 207-211)

**Goal**: Lock in everything above so it cannot silently regress.
**Time**: ~45m · **Risk**: low, but it changes CI behaviour for every future PR.

- [ ] **Step 207** [MEASURE] ~5m: Final numbers for all four linters.
- [ ] **Step 208** [DECIDE]: Which linters flip to gating?
      **RECOMMENDATION**: flip **ruff** and **shellcheck (warnings)** to gating with
      rule-ID exclusions for anything not driven to zero; leave **bandit** and
      **eslint** non-gating with recorded baselines. *Rationale*: the ratchet policy
      says flip once the backlog is worked. ruff and shellcheck will be at or near zero
      after Phases 2-8; bandit retains a large dispositioned tail (subprocess use that
      is correct and permanent) and eslint was measured for the first time tonight —
      neither has been *worked*, so neither has earned a gate.
      **Exclude by rule ID, never by severity** — that is the standing rule.
- [ ] **Step 209** [W] ~20m: Write `scripts/check-ruff.sh` in the style of
      `check-clippy.sh` (rule-ID exclusion list with a comment per exclusion saying why
      it is there and what would remove it). Wire it into the workflow.
- [ ] **Step 210** [V] ~10m: Run every gating script locally; all green
  ```bash
  ./scripts/check-clippy.sh && ./scripts/check-python-syntax.sh && \
  ./scripts/check-gosec-ratchet.sh && ./scripts/check-secrets-baseline.sh && \
  ./scripts/check-manifest-yaml.sh && ./scripts/check-ruff.sh && \
  git ls-files '*.sh' | xargs -r shellcheck -S error -f gcc && echo "ALL GATES GREEN"
  ```
- [ ] **Step 211** [C]: **PHASE 14 COMMIT** — `ci(ruff,shellcheck): flip to gating with
      rule-ID ratchets`.

---

## PHASE 15: THE STEVIE DECISION QUEUE (Steps 212-213)

**Goal**: These are **not churn**. Each needs a human call. The loop's job is to make
the decision cheap to make — options costed, recommendation stated, consequences named —
**not** to make it.

- [ ] **Step 212** [W] ~30m: Write `docs/battle-plans/STAGING-LADDER-DECISIONS.md` with
      one section per item:

  **D1 — `shield-ebpf` PQC fast path is unwired** (from `next.md`, full options in
  `docs/security/shield-ebpf-pqc-fast-path-unwired-2026-08-03.md`). Nothing calls
  `pqc_fast_path_check`; nothing writes `PQC_SIG_STATUS`. The XDP-layer PQC enforcement
  that appears to exist has never executed. Wiring it changes the verifier budget **and**
  forces a PESSIMISTIC (drop on cache hit) vs OPTIMISTIC (warn) choice. That is a
  live-traffic decision with a real availability/security trade — Stevie's, not mine.

  **D2 — bandit `B104` hardcoded bind-all-interfaces (2 sites).** Changing a bind from
  `0.0.0.0` to a specific interface is a real exposure change that can break a
  deployment. Needs to be decided against the actual topology (WEST/EAST, lxdbr0).

  **D3 — bandit `B615` unpinned HuggingFace download (2 sites).** Pinning a revision is
  correct supply-chain hygiene and changes which weights get fetched. Given the
  Gemma-4 GGUF was already deleted for being the wrong quant, pinning needs to name the
  *right* artifact — which requires knowing which one is wanted.

  **D4 — trivy KSV-0014 (5 HIGH) and KSV-0041/0046 (2 CRITICAL).** Need a live kind
  cluster to verify a fix. Cannot be closed from a laptop at 3am.

  **D5 — GitHub repo settings**: default branch → `develop`, branch protection rules.
  UI-only, needs Stevie's account.

- [ ] **Step 213** [C]: Commit the decision queue.

---

## PHASE 16: HANDOFF (Step 214)

- [ ] **Step 214** [W][C] ~30m: Write the wake-up summary and overwrite `~/tmp/next.md`
      per `feedback_next_md_handoff`. It must contain, in this order:
      1. **Anything alarming first** — especially the `B105` outcome from Step 161.
      2. The commit list on `staging`, in ladder order, each with its rung and which
         are unsigned (and therefore need re-signing before promotion).
      3. Before/after for all four linter baselines.
      4. Exactly where the loop stopped and why.
      5. Every `[STUCK]` marker with its Stuck Report.
      6. The promotion command Stevie runs after reviewing.
      7. A pointer to the Decision Queue.

---

## APPENDIX A: EMERGENCY PROCEDURES

**A1 — A gating check goes red mid-run.**
Stop. Do not commit. `git diff` to find what did it. If it is the current phase's work,
revert that phase's changes (`git checkout -- <files>`) and mark `[STUCK]`. If it was
already red at `$BASE`, it is a pre-existing failure — record it and continue.

**A2 — `monad-cpu-ebpf` is no longer 901888 bytes.**
Almost certainly `scripts/bpf-verifier-check.sh` ran and rebuilt without the feature.
Rebuild with `--features ascend-linux` and re-check before concluding anything broke.

**A3 — gpg-agent has no cached key.**
Expected overnight. Use `--no-gpg-sign` and record which commits are unsigned. Do not
stall waiting for Stevie.

**A4 — A phase's scope turns out much larger than estimated.**
Skip Protocol: 3x the estimate or 2 failed debug attempts. Commit what is verified,
mark `[STUCK]` with a Stuck Report, move to the next independent phase. Phases 1-13 are
independent of each other by construction — only Phase 14 depends on the ones before it.

**A5 — A "lint fix" turns out to be a live bug** (most likely at `F821`, Step 87, or
`B105`, Step 161). Stop treating it as lint. Give it its own commit, its own writeup in
`docs/security/`, and put it at the top of the wake-up summary.

**A6 — Context runs out mid-phase.**
The progress log (Step 11) is the recovery point. It records the last committed phase.
Resume from the next uncommitted step; never re-run a committed phase.

## APPENDIX B: PHASE DEPENDENCY MAP

```
Phase 0 (staging branch)
   |
   +-- 1  R0 untracked files      -- independent
   +-- 2  R0 notebook             -- independent
   +-- 3  R0 shebangs             -- independent
   +-- 4  R1 shellcheck           -- independent
   +-- 5  R1 subprocess check=    -- independent
   +-- 6  R1 datetime tz          -- independent
   +-- 7  R2 ruff misc            -- independent
   +-- 8  R2 exceptions           -- independent (but reduces bandit B110 in 10)
   +-- 9  R2 bandit MEDIUM        -- independent
   +-- 10 R2 bandit LOW           -- reads Phase 8's result, does not require it
   +-- 11 R3 eslint               -- independent
   +-- 12 R3 af-xdp test          -- independent
   +-- 13 R3 async                -- independent
        |
        v
   Phase 14 (ratchet flips) -- REQUIRES 2,3,4,5,6,7,8 complete
        |
        v
   Phase 15 (decisions) + Phase 16 (handoff) -- always run, even on a partial run
```

**Critical path**: 0 → 8 → 14 → 16. Phase 8 alone is ~4h and 38% of the ruff backlog.
**If time runs short**: Phases 15 and 16 run **regardless**. A partial ladder with a
clear handoff is a good outcome; a full ladder with no handoff is not.

## APPENDIX C: QUICK REFERENCE

```bash
# Pinned linters (match CI exactly — never use system versions)
$RUFF check .                              # 427 at baseline
$RUFF check . --statistics                 # by rule
$RUFF check . --select BLE001 --output-format concise
$BANDIT -q -r . -x ./llama.cpp,./target,./node_modules -f json -o out.json
git ls-files '*.sh' | xargs -r shellcheck -f gcc          # all
git ls-files '*.sh' | xargs -r shellcheck -S error -f gcc # GATING subset

# All gating checks
./scripts/check-clippy.sh
./scripts/check-python-syntax.sh
./scripts/check-gosec-ratchet.sh
./scripts/check-secrets-baseline.sh
./scripts/check-manifest-yaml.sh

# The inertness signal
cd ebpf && cargo build --release -p monad-cpu-ebpf --features ascend-linux
stat -c %s target/bpfel-unknown-none/release/monad-cpu-ebpf   # MUST be 901888
```

**Standing traps (from `next.md`, do not rediscover):**
- `scripts/bpf-verifier-check.sh` clobbers the ascend-linux build.
- `cargo clippy --fix` has stripped `cfg(test)` imports and would have silenced a real
  bug by `_`-prefixing live parameters. Same caution applies to `ruff --unsafe-fixes`.
- Workspace discovery is by **tracked `Cargo.lock`** — an untracked lockfile escapes the
  clippy gate entirely.
- The pre-commit hook enforces SPDX headers + rustfmt.
- `zhenai-forge`'s suite cannot run end to end (GGUF deleted 2026-07-31); 28 tests are
  `#[ignore]`d. Do not read those as passing.

---

*Staging Ladder Battle Plan — Forged 2026-08-03*
*16 Phases. 214 Steps. The backlog re-sorted so it can be reviewed one rung at a time.*
*Every rung stands alone. Stop climbing anywhere and the tree is still whole.*
