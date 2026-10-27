<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# YYYY-MM-DD: one-line title naming the symptom, not a person

Blameless postmortem (ADR-100 section 5). Name systems, defaults and missing
checks; never a person. Copy this file to `docs/incidents/YYYY-MM-DD-<slug>.md`,
keep every `##` section (write "None." rather than delete one), and link the
file from the register finding or Kanban card it produced.

**Status:** draft | reviewed (by whom, when)
**Services:** which services were affected

## Summary

Two or three sentences: what broke, for how long, and why it went unnoticed
(or did not).

## Impact

- **Who/what:** which people, services and data were affected.
- **How long:** from the first bad minute to recovery, and to detection if
  they differ.
- **Data:** lost, at risk, or none.

## Timeline (UTC)

| Time (UTC) | Event |
|---|---|
| YYYY-MM-DD HH:MM | first bad minute |
| YYYY-MM-DD HH:MM | detected (by what) |
| YYYY-MM-DD HH:MM | recovered |

## Detection

What noticed it, and what should have. Name the alert, SLO or check that
would have caught it, and say whether it exists now.

## Root cause

The chain of causes, down to the default, missing check or design choice that
let it happen. Evidence (logs, exit codes, commits) with where to find it.

## What went well

What limited the damage or sped up the fix.

## Action items

Each one is an `M-###` row in `docs/sweeps/OUTSTANDING-*.md`, a register
finding (`FND-###`), or "done: <commit>". An item with none of these gets
dropped; that is the point of the rule.

| Action | Tracked as | State |
|---|---|---|
| | | |

## Threshold check

Why this incident meets the postmortem threshold (S-WELL-SPRING Step 156,
ADR-100 question 3, default until Stevie decides): an outage of a
Stevie-facing service (Wotan, the dashboard, The Well, Zhen) longer than 1 h;
a silent failure longer than a day; or a data-loss near miss. A single page
that resolved inside the hour does not get one.
