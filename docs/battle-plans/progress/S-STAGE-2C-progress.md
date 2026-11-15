<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2C progress

Plan: `docs/battle-plans/S-STAGE-2C-battle-plan.md`. Evidence: `references/s-stage-2c-log.md`.

Skip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.

## For Stevie
- Answers this plan runs on (Stevie, 2026-11-12, from next.md rounds 2 and 3; recorded here, nothing to decide):
-   A-60, UPC D6: "enable D6 and reject MRET/SRET at priv 3 as the first phase of S-STAGE-2C".
-   A-61, exec .bss zeroing (R-NC) and the atomics skipping translate_address (R-S4): "fix both plus an opcode scan tripwire".
-   A-61, open(O_RDWR) on a regular file (T-F3) and the bad .data sidecar: "fix both (sidecar fatal)".
-   A-62, probes: "all of them".
-   ADR-135 F7/F8: "parked ADRs" (written as ADR-145 and ADR-146, Proposed, not built).
-   ush: "add a cd builtin".
- wiki/ADR-Index.md has no rows for ADR-139 to ADR-144 and its counts were 132 against the canonical 141 before this plan's two rows; not this plan's to fix (S-RESIDUE parked it).
- Step 29's review was done by the lane itself (a fork cannot start a review agent). An independent read of the priv-3 test and D6 is owed; dispositions are in references/s-stage-2c-log.md, Phase 2 review.
- Step 26 [DECIDE]: MRET/SRET at priv 3 are NOPs, not an illegal-instruction trap (the plan's recommendation: a trap needs a kill path from a slot row, which arrives at rung 7; Step 250 revisits).
- Step 30 [DECIDE]: D6 first, as asked (recommendation); the override (rung 3 first) applies only if the D6 count fails the budget.
- The host executor crates/monad-mbc/src/execute.rs still takes MRET/SRET at any privilege (not in a boot; no parity test on it). Bring it in line?
- ascend baseline row now 893507 (net vs 921036: 27529)

