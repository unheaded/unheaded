<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# 2026-09-30: thousands of failed superuser logins on The Well, unseen for a week

**Status:** draft (written 2026-10-27 from the session record and the COMPLIANCE-WELL plan; not yet reviewed by Stevie)
**Services:** The Well (`unheaded-postgres`), dashboard-backend and any service relying on `pkg/database` defaults

## Summary

From 2026-09-24 to 2026-09-30 The Well logged 4,674 failed logins as the
superuser `unheaded` (3,213 of them on 09-29), ending in a burst at 02:21Z on
09-30. No attacker: `pkg/database` defaulted to the superuser with the
password `unheaded`, while the volume had been initialised with
`unheaded_dev`, so any service started without `WELL_USER`/`WELL_PASSWORD`
tried to log in as the superuser with the wrong password. It was found by
accident on 2026-10-01 while reading the log for the Well's exit.

## Impact

- **Who/what:** services that relied on the default (dashboard-backend calls
  `database.Connect(OpsWriterConfig())`) never reached The Well, so their writes
  were lost or fell back. The log itself was noise that would have hidden a real
  brute-force attempt.
- **How long:** at least 7 days (09-24 to 09-30) before anyone looked.
- **Data:** none lost from The Well; the failed connections wrote nothing. The
  real exposure was the default itself: a superuser login with a password
  shipped in the source.

## Timeline (UTC)

| Time (UTC) | Event |
|---|---|
| 2026-09-24 | First failed superuser logins in the surviving log |
| 2026-09-29 | 3,213 failures that day |
| 2026-09-30 02:21 | Last burst before the reboot (and before The Well's exit 255 at 05:06) |
| 2026-10-01 | Found while reading The Well's log for the exit; traced to the default (FND-007) |
| 2026-10-01 | `pkg/database` has no built-in credential, scoped default users, quoted DSN: `296e45f9` |
| 2026-10-15 | Kanban connects as `app_kanban`, not the superuser (FND-008): `9f47fc2f` |

## Detection

Nothing detected it; it was found while looking at something else.

What should have, and whether it exists now:

- **A log-volume deviation SLI on postgres error lines** and an
  **auth-failure rate SLI** (ADR-100 section 3). Built for the journal on
  2026-10-25 (`LogErrorSpam`, `AuthFailureBurst` in `rules/logs.yml`), but
  postgres logs to container stdout, which does not reach the journal yet, so
  this exact incident would still go unseen (M-082).
- **Fail-loud configuration**: a service with no Well credential now refuses
  to start instead of guessing (`296e45f9`). That alone stops this class.

## Root cause

1. A convenience default in `pkg/database/config.go`: `WELL_USER=unheaded`,
   `WELL_PASSWORD=unheaded`, the superuser, when nothing was set.
2. A second, different default in compose: the volume was initialised with
   `POSTGRES_PASSWORD:-unheaded_dev`, so the two defaults disagreed and every
   defaulted connection failed.
3. Services retried on a loop, so a configuration gap became a log flood.
4. Nothing read The Well's log: it went to container stdout with no
   volume or error-rate check on it.

## What went well

- Because the two defaults disagreed, the superuser login never succeeded.
- The fix removed the default rather than correcting the password, so the
  same mistake cannot recur by a typo; missing credentials now name the
  variables that apply (`6926bab5`).

## Action items

| Action | Tracked as | State |
|---|---|---|
| No built-in credential in `pkg/database`; fail loud when unset | FND-007, done: `296e45f9` | done |
| Kanban off the superuser | FND-008, done: `9f47fc2f` | done |
| Container stdout (postgres) into the log pipeline so `LogErrorSpam` and `AuthFailureBurst` see it | M-082 | open |
| The Well's own auth failures as an SLI | M-082 (ADR-100 Phase 3 "not done" list) | open |

## Threshold check

Meets it as a silent failure longer than a day (at least seven days of
refused connections nobody saw). ADR-100 also names it as one of the three
incidents its postmortem practice starts with.
