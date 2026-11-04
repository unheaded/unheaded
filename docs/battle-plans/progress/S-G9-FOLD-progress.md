<!-- SPDX-License-Identifier: GPL-3.0-or-later -->
# S-G9-FOLD progress

## Log

- Phase 0 (Steps 1-7), 2026-11-04, branch `sprint/s-g9-fold` from develop `d8fe589d`.
  - baseline failures: 0
  - running units: 3
  - 19015 listeners: 0
  - Step 6: ADR-127 to ADR-134 free (no files match).
- Phase 1 (Steps 8-19), 2026-11-04, Haiku. Eight parked ADRs copied from payloads: ADR-127 to ADR-134 (Proposed, parked).
  - Step 17: index rows added after `[121]` anchor, `ADR-INDEX.md` +8 lines.
  - Step 18: 8 files, 8 index rows, 0 files lacking the parked status line.
  - Commits: 72074e5d (Steps 8-12), fd39d7b6 (Steps 13-19). Both signed.
  - Phase 1 exit gate: preflight then two ADR commits on `develop..HEAD`. Passed.
- Phase 2 (Steps 20-25), 2026-11-04, Sonnet. Step 21 RED as predicted (NoData undefined); Step 23 gofmt clean, vet ok, race test ok; Step 24 DefaultAddr count 0. Commit 1f988660 (signed). Exit gate passed.
- Phase 3 (Steps 26-32), 2026-11-04, Sonnet. Step 27 RED as predicted (undefined checkWireListen, serveWire, wireOptions); Step 29 two ok, gofmt clean; Step 30 all six outputs exact; Step 31 man 1 written, 1 file changed. Commit below. Exit gate passed.

## For Stevie
