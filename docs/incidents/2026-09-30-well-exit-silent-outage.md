<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# 2026-09-30: The Well exited after a reboot and nobody knew for 21 hours

**Status:** draft (written 2026-10-27 from the session record and the ERDA audit; not yet reviewed by Stevie)
**Services:** The Well (`unheaded-postgres`), Kanban, compliance evidence, dashboard-backend's ops writes

## Summary

`unheaded-postgres` exited with code 255 at 2026-09-30 05:06:26Z, about 2.5
minutes after west booted. It was not OOM-killed and logged no shutdown:
dockerd went down under it. Kanban fell back to SQLite and kept working, which
hid the outage; it was found by accident about 21 hours later. The Well then
stayed down on purpose until 2026-10-15, while backups that covered every
database were built first (ERDA audit, Phase 0-1). This is the "Well silent
outage" and the "Well exit 255" in M-081: one incident.

## Impact

- **Who/what:** every database in The Well. Kanban served from its SQLite
  fallback (writes there did not reach The Well); anything else that wrote to
  The Well, such as dashboard-backend's ops writes, failed.
- **How long:** about 21 h before anyone knew; down about two weeks in total,
  the remainder as a deliberate hold (start only after a full backup and a
  restore drill existed).
- **Data:** no loss found. The volume (`unheaded-dev_postgres-data`)
  crash-recovered on start. At the time, backups dumped only the legacy
  database `unheaded`, none of app/ops/config and no globals, never restored:
  a data-loss near miss had the volume been damaged.

## Timeline (UTC)

| Time (UTC) | Event |
|---|---|
| 2026-09-24 .. 09-30 | 4,674 failed superuser logins in The Well's log (see the auth-burst postmortem) |
| 2026-09-30 05:04 | West boots |
| 2026-09-30 05:06:26 | `unheaded-postgres` exits 255, no shutdown lines; dockerd restarted 05:06:42 |
| 2026-10-01 ~02:00 | Found by accident during compliance work, about 21 h later (exact minute not logged); Kanban on its SQLite fallback |
| 2026-10-01 | Starting it held for Stevie's yes; backups first |
| 2026-10-13 | ERDA audit: cause is dockerd, not Postgres; backups cover only the legacy DB |
| 2026-10-15 | Raw volume archived, full backup + drill, The Well started and migrated |

## Detection

Nothing detected it. The Kanban fallback, built to keep the board alive,
made the outage invisible from the UI.

What should have, and whether it exists now:

- **The `well` SLO** (`min(huginn_pg_up)` per database, 99.5 %, pages):
  exists since 2026-10-20.
- **A fallback alert**: `ComplianceWellFallback` in
  `monitoring/prometheus/rules/compliance.yml` (runbook
  `runbooks/observe/compliance-well-fallback.yaml`): exists for compliance;
  Kanban's SQLite fallback still has no alert of its own (the `well` SLO
  covers the cause).
- **The `well-backups` SLO** (backup, offsite and drill freshness): exists
  since 2026-10-20; a stopped Well fails the nightly backup loudly.
- The monitoring stack running whenever the Kingdom runs: M-033, done
  `d9c1980a`.

## Root cause

1. dockerd went down shortly after boot and took the container with it (exit
   255, `FinishedAt` 05:06:26Z, dockerd restart 05:06:42). Why dockerd
   restarted was not established from the logs that survived.
2. The container did not come back: the host-local override sets
   `restart: "no"` (nothing starts at boot, by design).
3. No SLO, no alert on the fallback, and no monitoring stack running, so the
   outage was silent; the SQLite fallback removed the last visible symptom.

## What went well

- The fallbacks did their job: the board and the compliance page stayed up.
- The response put data first: the volume was archived and every database
  backed up and restore-drilled before The Well was started (ERDA audit
  `docs/battle-plans/WELL-ERDA-AUDIT-2026-10-13.md`).

## Action items

| Action | Tracked as | State |
|---|---|---|
| The Well's availability SLO, paging | done: policy.yaml `well` SLO (ADR-100, 2026-10-20) | done |
| Backups of every database + globals, restore drill, offsite to east | done: ERDA Phase 1 (`unheaded-well-{backup,drill,offsite}` timers, 2026-10-15) | done |
| Backup freshness SLO | done: policy.yaml `well-backups` | done |
| Monitoring stack starts with the Kingdom | M-033, done: `d9c1980a` | done |
| Kanban and timeguru refuse to start on a missing migration instead of creating tables | M-073 | open |
| PG 16 to 18 on The Well | M-071 (E-B) | open |
| Dead-man's switch for a host that stops reporting | M-080, S-WELL-SPRING Steps 145-155 | built; live after E-D |

## Threshold check

Meets it twice over: an outage of a Stevie-facing service (The Well) far
longer than 1 h, a silent failure of about 21 h, and a data-loss near miss
(backups did not cover the databases in use).
