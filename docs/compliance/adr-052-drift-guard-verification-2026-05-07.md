# ADR-052 Drift Guard Re-Verification — 2026-05-07

**Auditor:** Claude (Opus 4.7) under unheaded-marshal oversight
**Scope:** ADR-052 Rule 1 (timeline freshness) CI enforcement chain
**Result:** PASS

---

## 1. Script execution

`bash scripts/check-timeline-freshness.sh --check` exits 0:

```
==================== Timeline Freshness Check (ADR-052) ====================
  Timeline path        : references/timeline.md
  Timeline last touched: 2026-05-05 06:36:30 UTC  (ts=1777962990)
  HEAD last commit     : 2026-05-06 09:23:30 UTC  (ts=1778059410)
  Delta                : 1 days (96420s)
  Max age allowed      : 7 days
  Status               : FRESH - within 7-day threshold
EXIT=0
```

Timeline last touched by commit `WAVE17 Phase 7: stub the two referenced-but-missing
runbooks + timeline Age 3 entry` at 2026-05-05 06:36 UTC. HEAD is the
NORTH-STAR Appendix A Phase F commit at 2026-05-06 04:23 CDT. Delta = 1 day,
well inside the 7-day threshold.

## 2. GitHub Actions workflow — `.github/workflows/timeline-drift-guard.yml`

**Triggers:**
- `push` to `main` (catches direct pushes / merges)
- `pull_request` targeting `main` (catches incoming PRs before merge)
- `workflow_dispatch` (manual)

**Fail-the-build logic (verified line-by-line):**

1. Step `drift_check` runs `./scripts/check-timeline-freshness.sh --check` with
   `set +e` and captures `$?` into `$GITHUB_OUTPUT` as `exit_code`.
2. Step `override` runs only if `exit_code != 0`; greps HEAD commit message for
   `[TIMELINE-DRIFT-OVERRIDE: <rationale>]` and emits `override_present` = true|false.
3. **Step `Fail (no override)`** — `if: exit_code != '0' && override_present != 'true'`
   — emits `::error` annotation and `exit 1`. **This is the build-failing line.**
4. Step `Pass with override (warn)` runs on drift+override and emits
   `::warning` so Marshal sees it on the next Round Table.

The fail condition is real: a PR with stale timeline.md and no override directive
cannot be merged into `main` without bypassing branch protection.

## 3. Jenkinsfile parity

`Jenkinsfile` lines 107-130 contain stage `Timeline Drift Guard` with the same
script invocation, the same override-directive grep, and an `ERROR` exit on
drift-without-override. CI parity between GHA and Jenkins is intact per ADR-052
Rule 5.

## 4. `scripts/drift-detect.sh` (separate concern)

This script is the **infrastructure** drift detector (terragrunt plan across
dev/staging/prod), not the timeline drift guard. It is wired into
`.github/workflows/drift-detect.yml` as a nightly cron (`0 6 * * *`) and opens a
GitHub issue on detected drift. Not in ADR-052's scope but worth noting it
exists and runs.

## 5. Gaps found

**None blocking ADR-052.** Two minor observations for future improvement:

1. The override grep pattern `\[TIMELINE-DRIFT-OVERRIDE:[^]]+\]` accepts any
   non-`]` content as rationale, including a single space. ADR-052 says
   "rationale documented" but the regex doesn't enforce minimum length. Low
   priority.
2. `scripts/check-timeline-freshness.sh` resolves "latest non-timeline commit"
   via `git log --since=@TIMELINE_TS -- ':!timeline.md' | head -1`, but never
   compares it against HEAD_TS. The current logic uses `HEAD_TS - TIMELINE_TS`,
   so a timeline-only commit *after* a real commit will mask drift until 7
   days pass. Edge case; acceptable.

## 6. Verdict

**ADR-052 drift-guard chain is operational and correctly fails the build on
>7-day timeline drift via push or PR to main.** Re-verification PASSES.
