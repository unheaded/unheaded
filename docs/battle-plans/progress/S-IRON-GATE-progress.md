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

### Phase 1 (Steps 29-66): merged ed46d54f

Ed25519 service tokens; trust.d with `sub` bound to the `.pub` file name (a service cannot mint another's identity); caller
allowlist answers 403; JWKS-verified user tokens; `alg` exactly `EdDSA` (none/HS256/lowercase refused before key lookup), 15-minute
cap, `crit` refused; HS256 test-only with a test that fails if production code opts in. ADR-103 gained measurements (west: Ed25519
verify 34.1 µs, ML-DSA-65 11.7 µs; token 308 B vs ~4,640 B). Full suite 0 FAIL before and after.

Phase 0 corrections: Step 17 now 11 of 12 services open (akira, architect, captain, cuirass, dashboard-backend, kanban-app,
micromanager, monad, sophia, timeguru, wotan), not 8; Step 19's compose hit is a commented example (effectively still true).

Hand-offs to Phase 6: switch each service from `SetupMiddleware` (now fails closed when misconfigured) to `NewServiceMiddleware`
(returns errors so a service can refuse to start); dashboard-backend's own HS256 authenticators now refuse JWTs (API keys still
work; compose runs it with auth off, so no live impact) and need `NewServiceAuthenticator`; `raft/start-zhen.sh` and
`cmd/sword/main.go` still mention `AUTH_JWT_KEY`; `pkg/auth/README.md` says "Stub" (Phase 16). Phase 3 (API keys) touches
`pkg/auth/setup.go` next; golangci-lint panics under Go 1.27.3 (tool mismatch, not code).
