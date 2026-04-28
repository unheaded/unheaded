# ADR-052: Timeline & Battle-Plan Source-of-Truth Policy

**Status**: Accepted
**Date**: 2026-04-27
**Deciders**: RFC-Editor, Marshal, Librarian, Timeguru, Stevie
**Related ADRs**: ADR-024 (Runbook Automation), ADR-026 (.deb Packaging + CI/CD), ADR-038 (Kanban GUID → Git Audit)
**Triggered by**: 2026-04-27 Round Table audit — two Marshal citations issued (33-day timeline drift, off-tree WAVE13 plan-of-record)

---

## Context

The Round Table audit on 2026-04-27 revealed two correlated source-of-truth violations that had been quietly compounding:

1. **`references/timeline.md` had not been touched in 33 days** (last edit 2026-03-25) despite ~25 commits to main during that window. The file simultaneously claimed "Age 3 IN PROGRESS 85%" and "Age 5 ✅ COMPLETED + 📋 PLANNED" on the same line — a contradiction that proves the file was no longer being updated by anyone.
2. **The accepted WAVE13 battle plan lived OFF-TREE** at `~/.claude/plans/synthetic-stirring-pudding.md`. The in-tree draft at `crates/zhenai-forge/notes/wave13-inference-battle-plan.md` carried a SUPERSEDED banner pointing OUT of the repo. Any reader without access to that home-directory path could not find the canonical plan.

Both violations had the same root cause: **no policy enforced that source-of-truth artifacts must live in-tree and stay fresh.**

The cost of these drifts is real:
- Future agents (and future Stevie) reading `timeline.md` get a wrong picture of project state, leading to wasted-effort decisions
- Plans-of-record that live outside the repo can vanish on machine swaps, account changes, or directory cleanups
- CI cannot enforce policies it cannot see

This ADR encodes the policy that makes these violations Marshal-citable and CI-blockable.

---

## Decision

### Rule 1 — Timeline Freshness

`references/timeline.md` MUST be updated whenever a commit lands on `main` that materially changes project state (new ADR, new sprint, age transition, milestone shipped, etc.). The file's last-modified date in git history MUST be ≤ 7 days behind HEAD whenever HEAD has commits since the timeline's last touch.

Operationally:
- **HEAD has no new commits since timeline last touched** → no drift; file is fresh by definition.
- **HEAD has new commits AND `HEAD_DATE - TIMELINE_TOUCH_DATE > 7 days`** → DRIFT. CI fails. Marshal cites.
- **HEAD has new commits AND delta ≤ 7 days** → fresh. No drift.

### Rule 2 — Battle Plans of Record

All battle plans of record MUST live in-tree under `docs/battle-plans/`. Off-tree plan files (e.g., in `~/.claude/plans/`, scratch directories, or other personal-machine locations) are **working drafts only** and MUST NOT be referenced by any in-tree document as the source of truth.

Operationally:
- A SUPERSEDED banner in an in-tree plan MUST point to another in-tree path
- Pointing to `~/.claude/plans/...` or any home-directory path is a Marshal-citable violation
- When a working draft graduates to plan-of-record, it MUST be copied (or moved) into `docs/battle-plans/`

### Rule 3 — ADRs in `docs/adr/`

Architecture Decision Records MUST live in `docs/adr/` and be enumerated in `docs/adr/ADR-INDEX.md`. ADR drafts may be stored elsewhere during writing, but acceptance (Status: Accepted) requires landing in-tree at `docs/adr/`.

### Rule 4 — Marshal Citation Authority

The Marshal skill (`unheaded-marshal`) is hereby authorized to issue formal citations against violations of Rules 1–3. Citations are recorded in `docs/sprints/YYYY-MM-DD-marshal-citations.md` and must be cleared (cited file fixed) within 7 days or the citation escalates to a Round Table agenda item.

### Rule 5 — CI Enforcement

A CI step (GitHub Actions + Jenkins) MUST run `scripts/check-timeline-freshness.sh --check` on every push to main and every pull request. The script returns exit code 1 if Rule 1 is violated. Pull requests with Rule 1 violations cannot merge to main without an explicit override commit (with rationale documented).

---

## Consequences

### Positive
- Future agents (Claude or other) can reliably read `timeline.md` knowing it reflects current state
- Off-tree-plan-of-record drift becomes impossible-to-miss (CI catches it; Marshal cites it)
- Stevie's ad-hoc "I'll update timeline later" pattern gets a structural enforcement layer (CI does the reminding)
- Plan portability improves — any clone of the repo gets all source-of-truth artifacts
- Archaeologically, future contributors see exactly what was canonical at any commit

### Negative
- Slight overhead per substantive commit: must touch `timeline.md` if material change
- CI drift-guard adds ~2 seconds to every PR check (negligible but non-zero)
- Edge case: rapid commit bursts (e.g., 30 commits in a day during a sprint) may cause timeline.md to be touched many times in quick succession — acceptable but visible in git history

### Conditional
- This ADR pairs with ADR-053 (Hybrid Claude + Local Zhenai Workflow Templates) — when Zhenai-routed minor-churn work happens, the routing layer must respect Rules 1–3 just as Claude does. Templates that touch `timeline.md` are first-class.

---

## Implementation

This ADR ships with three concrete artifacts (committed in the same patch as this ADR):

1. **`scripts/check-timeline-freshness.sh`** — bash script, two modes (`--check` for CI, `--report` for info), exit-code semantics per Rule 1.
2. **`.github/workflows/timeline-drift-guard.yml`** — GitHub Actions workflow, runs the script on push (main) + pull_request.
3. **Jenkinsfile** — added `Timeline Drift Guard` stage in the existing pipeline.

The script checks `git log -1 --format=%ct -- references/timeline.md` vs `git log -1 --format=%ct HEAD`. If HEAD has commits since timeline-last-touch and the delta exceeds `MAX_AGE_DAYS=7`, exit 1.

A pre-commit hook (optional, local-only, warn-mode) is provided in `scripts/check-timeline-freshness.sh` itself — invokable as `./scripts/check-timeline-freshness.sh --report` from `.git/hooks/pre-commit` for individual developers who want immediate feedback without the CI round-trip.

---

## Alternatives considered

1. **Trust the team to update timeline manually**. Rejected — that's exactly what failed for 33 days. Trust without verification is hope, not policy.
2. **Auto-generate timeline from git log**. Rejected — timeline is *narrative*, not raw history. Algorithmic generation would lose the structural decisions (Age boundaries, milestone significance, what counts as "material").
3. **Make timeline a wiki page only, drop the in-tree file**. Rejected — wiki is downstream of in-tree per Librarian's 8-layer doc web. In-tree must be canonical; wiki rolls forward from it.
4. **Set MAX_AGE_DAYS = 14 instead of 7**. Rejected — 33 days of drift was the trigger, and 14 is too lenient. 7 days = one week = one sprint. If the sprint is shipping, the timeline reflects it.
5. **Don't enforce Rule 2 (off-tree plans)**. Rejected — off-tree plans were the second citation. Both violations correlated; one fix without the other is half a fix.

---

## Backward compatibility

- Existing `timeline.md` was re-synced to current state in the same commit that introduces this ADR — drift goes from 33 days to 0 days.
- The in-tree WAVE13 plan was relocated to `docs/battle-plans/WAVE13-INFERENCE.md` in the same commit — Rule 2 cleared.
- No history rewrite required. ADR applies forward from acceptance date 2026-04-27.

## Sign-off

- [x] RFC-Editor — ADR text reviewed; Status: Accepted
- [x] Marshal — citation authority claim acknowledged; will enforce starting 2026-04-27
- [x] Librarian — 8-layer doc web update planned (this ADR is layer 8; timeline.md is layer 4; battle-plans/ is layer 3)
- [x] Timeguru — timeline.md update cadence accepted as part of normal sprint workflow
- [ ] Stevie — final sign-off on local pre-commit hook adoption (optional but recommended)

---

*ADR-052 forged 2026-04-27 from Cowork-on-Macbook in response to the Round Table's two source-of-truth citations. Drift becomes structurally impossible from this commit forward.*
