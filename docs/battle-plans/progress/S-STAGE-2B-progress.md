<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# S-STAGE-2B progress

Plan: `docs/battle-plans/S-STAGE-2B-battle-plan.md`. Evidence: `references/s-stage-2b-log.md`.

Skip Protocol acknowledged: skip after 3x the estimate or 2 failed debug attempts, mark [STUCK], commit known-good state, continue with the next independent step.

## For Stevie
- Step 6 departs from the plan text. As written, root writes the pid to a user-owned mktemp file, and fs.protected_regular=2 refuses that with EACCES in sticky /tmp (tested on west), so doom_stop would silently do nothing and doom-runner would leak. A private mktemp -d directory was tried next, but a Claude Code safety check refused the rm -rf in that command and nothing ran; that removal was not retried. Landed instead: the launcher prints "doom-runner-pid N" into the user-owned $LOG, doom_stop reads it, kills by that pid and truncates $LOG. No new temp path, so nothing new to delete. The plan text still shows the PIDF form.
- Step 6 proven on the first real gate (gate p0, 2026-11-04): Doom object loaded, 'pipeline complete', ALL GREEN, pgrep -c doom-runner 0 after the harness. doom-runner run stays up until signalled, so the by-pid kill did the stop.
- Phase 0 resumed after the first agent was cut off during Step 7: DEADLINE reset to 13 h from the resume, Step 7 re-run from the top. Its eBPF build was a no-op (35 MB RSS), so main.rs was touched and rebuilt for the real figure, 180 MB in 0.75 s, against the plan's ~12 min estimate for the whole step. Run cost sits above the Ledger in the log because ledger appends to end of file.

