<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# 2026-10-18: Wotan OOM-killed, down 10 hours, nobody told

**Status:** draft (written 2026-10-27 from the session record; not yet reviewed by Stevie)
**Services:** Wotan on west, and everything that publishes to or reads from it (dashboard live view, Kanban events, discovery, logs.*)

## Summary

Wotan on west was OOM-killed at 06:42 on 2026-10-18 and stayed down for about
10 hours. Its container did not restart (host-local `docker-compose.override.yml`
sets `restart: "no"`, by design: nothing starts at boot), no alert existed, and
the monitoring stack was not running, so it was found only when Stevie loaded
the stack that afternoon. Wotan died twice more the same day from two further
bugs found while fixing the first.

## Impact

- **Who/what:** the message bus on west. Services kept running but published
  into nothing; the dashboard's live view, Kanban's live updates, discovery and
  the `logs.*` aggregation were empty or stale. East's Wotan was unaffected.
- **How long:** about 10 h down before detection; detection and recovery came
  together (Stevie loading the stack).
- **Data:** Wotan's in-memory rooms (recent messages) were lost; nothing in The
  Well was affected.

## Timeline (UTC)

| Time (UTC) | Event |
|---|---|
| 2026-10-18 06:42 | Wotan OOM-killed (container limit 256 MiB). Restart policy `no`; stays down |
| 2026-10-18 ~16:40 | Found while Stevie was loading the stack (no alert, monitoring stack not running); about 10 h later, exact minute not logged |
| 2026-10-18 | Per-room byte budget lands, `0fea41f1`; Wotan back up |
| 2026-10-18 17:03 | Wotan killed again: a request-timeout middleware wrote into a recycled response writer under build load (nil bufio panic). Fixed `af6beae2` |
| 2026-10-18 23:25 | OOM-killed again, after the per-room fix: no budget across rooms (~10 services x levels of `logs.*`, 10k messages each) |
| 2026-10-19 | One memory budget for all topics together, `71a29277`; reproduced in a 256M container (old image OOMKilled, new image holds at ~139 MiB) |

## Detection

Nothing detected it; a person did, ten hours later.

What should have:

- **The `wotan` SLO** (`observability/alerting/policy.yaml`, `up{job="wotan"}`
  on both hosts, 99.5 %, pages). It did not exist until 2026-10-20. It exists
  now and pages within minutes, but only while the monitoring stack runs.
- **The monitoring stack starting with the Kingdom** (FND-024, M-033):
  `scripts/kingdom-startup.sh` starts it first since `d9c1980a` (2026-10-27).
- **A dead-man's switch** for the case where the host itself, or the alerting
  path, is what died: the always-firing `Watchdog` rule and the east-to-west
  heartbeat (`HeartbeatMissing`, M-080), S-WELL-SPRING Phase 6.

## Root cause

1. Wotan's per-topic ring buffers were capped by message count only (10k). A
   `traces.flow` batch is ~32 KB and was mirrored twice, so a few busy trace
   rooms could hold ~700 MB in a 256 MiB container.
2. After a per-room byte cap (`0fea41f1`), the number of rooms was still
   unbounded in bytes: every service's `logs.<service>.<level>` topic held 10k
   messages, which reproduced at 290 MiB heap with 50 topics. One global budget
   with oldest-first eviction from the largest rooms fixed it (`71a29277`).
3. A separate crash path: the timeout middleware kept writing after the
   handler timed out (`af6beae2`).
4. Why it stayed down silently: restart policy `no` on purpose (Stevie,
   2026-09-08: nothing starts at boot), no SLO on Wotan yet, and no monitoring
   stack running to evaluate one.

## What went well

- The cause reproduced in a container with the production limit before the fix
  was trusted, and the fix was proven the same way.
- Two other defects surfaced and were fixed the same day instead of being
  logged for later.
- Wotan's request logging was cut to failures only (`6b3d4a06`): two info lines
  per request had filled its 30 MB log in ~4 minutes and erased the pre-OOM
  evidence.

## Action items

| Action | Tracked as | State |
|---|---|---|
| Per-room byte budget | done: `0fea41f1` | done |
| One memory budget across all rooms | done: `71a29277` | done |
| Timeout middleware crash | done: `af6beae2` | done |
| Wotan availability SLO that pages | done: policy.yaml `wotan` SLO (ADR-100, 2026-10-20) | done |
| Monitoring stack starts with the Kingdom | M-033 (FND-024), done: `d9c1980a` | done |
| Watchdog + east/west heartbeat | M-080, S-WELL-SPRING Steps 145-155 | built; live after the E-D window |
| OOM recovery runbook | done: `f8cd1678` (`runbooks/wotan/oom-state-recovery.yaml`) | done (draft) |
| Restart-at-boot policy for the main stack | Stevie's 2026-09-08 decision stands; with the SLO paging, a crash is no longer silent while monitoring runs | no change |

## Threshold check

Meets it: an outage of a Stevie-facing service (Wotan) of about 10 hours,
well over the 1 h line, and silent the whole time.
