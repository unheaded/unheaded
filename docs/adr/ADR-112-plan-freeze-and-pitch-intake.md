<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-112: Plan freeze and pitch intake: new ideas become ADRs, not battle plans

**Status:** Accepted (Stevie, 2026-10-30).
**Date:** 2026-10-30
**Related:** ADR-089 (develop → staging → main), ADR-098 (findings register), Round Table 2026-10-30 (`docs/battle-plans/ROUND-TABLE-2026-10-30-state-direction-drift.md`).

## Context

The 2026-10-30 Round Table found that code output is well ahead of delivery:
- Eight battle plans are open, and S-LOG-HARVEST added 214 more steps the same day.
- The compose stack has been down since the 10-28 reboot.
- `develop` is 1,498 commits past `staging`.
- `next.md` held 115 requests for a decision.

Every new plan adds escalations for Stevie, and his attention is the bottleneck. Ideas still arrive and are worth keeping. They need a place to wait that costs no execution time and creates no decision debt.

## Decision

1. **No new battle plans while the freeze holds.** "New" means any sprint plan, Warmonger plan, phase plan or plan amendment that adds scope. Work continues only on plans that already exist, in the order of the decision queue (`next.md`, "DECISION QUEUE").

2. **Every pitch from Stevie becomes an ADR.** Whatever the size, it goes into `docs/adr/ADR-NNN-<slug>.md` with:
   - status `Proposed (pitch, parked under ADR-112)`;
   - the pitch in Stevie's words, quoted;
   - context: what exists today, with file paths and the commits checked;
   - the open questions;
   - what it would unblock or depend on;
   - a row in `ADR-INDEX.md`.

   No plan, branch or code is started for it.

3. **The freeze lifts on Stevie's word.** The Round Table's suggested exit is that the push and merge, stack-up (E1) and attended firewall apply are done, and `staging` has moved once. Parked pitch ADRs then compete for the next plan slot like any other proposal.

4. **Exceptions:**
   - Fixes for live security exposure, such as the chaos-controller guard (`07021f66`).
   - Broken CI.
   - Data loss or backup failure.
   - Anything Stevie explicitly asks to be done now.

   None of these is a new plan. Each is a commit with its own evidence.

## Consequences

- Pitches are captured, and none are lost or acted on early.
- Execution time goes to making existing work run, get reviewed and get promoted.
- The ADR index grows by `Proposed` entries. That is cheap, and it is searchable, which `next.md` is not.
- Risk: a long freeze turns pitches stale. Each parked ADR records the commit it was grounded on, so it can be re-checked when it is picked up.

## Decisions 2026-11-03 (decision pack, round 2)

From `docs/battle-plans/DECISION-PACK-2026-11-03.md` (DP ids in parentheses).

- Stevie, 2026-11-04: once the decision pack is answered, the work rolls into a series of warmonger battle plans. That instruction lifts this freeze for exactly the plans that roll from `docs/battle-plans/DECISION-PACK-2026-11-03.md`; pitch intake as parked ADRs continues.
