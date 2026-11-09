<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# S-NIGHT-SHIFT progress

Plan: `docs/battle-plans/S-NIGHT-SHIFT-battle-plan.md`. Resume at the first step not listed as done below.

## Log

- Steps 1-8 done 2026-11-09: develop clean at `5fddaa5b`, signing OK, /var 1.1T free, user slice 4096 tasks / 12 GiB, watchdog running.
- Steps 9-20 resolved without a migration: the `wotan_*` tables lived only in the legacy database `unheaded`, which was dumped (`~/backups/well/2026-10-15/unheaded-legacy-final.dump`) and dropped on 2026-10-15 (`docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md` item 3.6, `wiki/The-Well.md`). No database the MANIFEST routes holds them (2026-10-15 snapshots). Migration 050 not written; S-E1-LIVE unchanged. Plan amendment: Appendix C's "Apply migrations 047, 049, 050" reads 047 and 049.
- Steps 21-32 done: `da9a8725` removes the 68-byte TRACE_MAP and STATS decoders and their tests; `FuzzDecodeKernelPacketEvent` replaces the dead fuzz target (20 s, no crash); race tests, build, man, script tests OK. Found on the way: check-nosec-ratchet red on develop from the syslog merge (`865e53ff`): `ab8dd7e0` (test file) and `45ea7023` (port range check instead of two G115 annotations; pinned gosec clean); ratchet PASS at 863.

## STUCK

(none)

## For Stevie

- Answer 26 (drop Wotan's old tables) needs nothing: they were dropped with the legacy `unheaded` database on 2026-10-15.

## Unsigned

(none)

## Marshal

- Phase 0 gate: PASS. Mission S-NIGHT-SHIFT, unattended only; out of scope = Appendix C. Clearance: PROCEED to Phase 1.
- Phase 1 gate: PASS (moot, evidence above). Plan deviation S1, justified: the premise (tables still exist) was false; no work invented to replace it. PROCEED to Phase 2.
- Phase 2 gate: PASS. Citation S2 (coordinator): today's merges `865e53ff` and `9da03ff4` were committed without the nosec ratchet, and a chained `gate | tail && commit` hid a red gate once. Corrected: gates are read on their own line before any commit. Phase 6 runs every row. PROCEED to Phase 3.
