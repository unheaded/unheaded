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

