<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-IRON-GATE progress log

Plan: `docs/battle-plans/S-IRON-GATE-battle-plan.md`. This file is the session log the plan refers to, and where
`[STUCK]` / `[ESCALATE]` markers live; step ticks live in the plan.

Skip Protocol acknowledgment: "I will skip any step after 3x its estimate or 2 failed debug attempts, mark
it [STUCK] with what I tried, commit known-good state first, continue with the next step that does not depend
on it, and finish with a STUCK/ESCALATE report ordered by how many steps each item unblocks."

## Session S24, 2026-10-27 evening (unattended, Stevie's "churn unattended multi agent")

Base: develop `1f44cc46` (S-CLEAN-SLATE shipped; Go known failures 0). Same pattern as Sprint 1: a coordinator
on develop and worktree agents under one brief (`EXEC-BRIEF-v2`: reset to develop first, never push, no host
changes, no shared-ledger edits, gate exit codes read directly). Phase 0 premises are checked per slice by each
agent, since SITUATION was written at `a7fe3acd` and Sprint 1 changed many of them.

First wave: Phase 1 (pkg/auth core, Steps 29-66) in a worktree agent; next: Phases 2+3 after it merges, then 4; Phases 11 and 12 can run beside them (not beside S-WELL-SPRING Phase 8: both touch every router).

### Phase 12 (Steps 304-328): done by the coordinator on develop, 2026-10-27 evening

Commits 5160faa6, 7a27fd19, 5993ec31, 97ade150 and the docs commit. release.yml builds all 19 binaries, the debs and the eBPF
objects, signs every file with keyless cosign, attests the SBOM, adds GitHub build provenance; `scripts/verify-release.sh` with an
offline test (6/6) run in CI. Scorecard and CodeQL workflows. cosign 3.1.3 pinned. [DECIDE] deviation Step 319: the eBPF size pin
is a build-host promotion gate (toolchain-bound), not a CI step; listed under NEEDS_HARDWARE. First release, Scorecard and
CodeQL runs only happen after Stevie pushes (and tags).
