<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-147: Every service earns its place: use, cost, reliability and scale, measured

**Status:** Proposed (parked). Measured 2026-11-15; no code changed, nothing stopped.
**Date:** 2026-11-15
**Related:** ADR-144 (footprint: what each part costs; this ADR continues it), ADR-142 (Wotan speed), ADR-117 (one Grafana), ADR-100 (SLOs), ADR-116 (monitor the monitors), ADR-112 (plan freeze), FND-044, `docs/battle-plans/progress/S-WATCH-TRUE-progress.md`.

## The pitch (Stevie, 2026-11-15, verbatim)

> we need to identify poor performance and bloaty AI slop applications... the lighter it is on resources leaves more room for other real appliations - unheaded doesn't really do anything other than function as a monitorijg stack and compliance tracker - not bad things to have but it would be nice to have other services on west and east if i wanted to add anything

> our code and stack should be reliable and scalable

> i feel like we approaching alpha but it's a flimsy scaffolded mock up right now - i will set a up a more lucious lab that brings east and west to the big boy era

## What this adds to ADR-144

ADR-144 asked what each component costs. This asks whether each one earns it. A service stays if it passes four tests, each measured on a running host and not taken from its README.

1. **Use.** Something calls it, or a person reads what it makes.
2. **Cost.** Its idle CPU and memory are known, and so is what they grow with. A cost that grows with uptime is a defect (FND-044 was one).
3. **Reliability.** It restarts clean. What it loses on a restart is written down. Its failure raises an alert that has been seen to fire. A test fails when it breaks.
4. **Scale.** Its cost per host, per event or per entry is known, and so is what happens at the limit: a full map, a full channel, a full disk.

A service that fails test 1 is removed or folded into another. A service that passes test 1 and fails another is fixed.

## How it was measured

West, 2026-11-15 16:13 UTC, 3 days 12 hours after a boot, the whole stack up, Zhen's model server down. CPU and memory per container are one `docker stats` sample; host units are systemd's `MemoryCurrent` (it counts page cache). Use is `unheaded_http_requests_total` in VictoriaMetrics over 7 days, leaving out `/metrics`, `/health`, `/ready` and `/live`. Memory over time is `process_resident_memory_bytes` at 08:00 UTC on each of the last four days. East was read with `ps` over ssh; nothing was changed there. West has 12 cores and 14 GiB; east has 4 cores and 7 GiB.

## Cost

**West.** 23 containers and 8 host units. All the containers together use about 13% of one core and 1.7 GiB; the host units hold 0.4 GiB more. The memory limits set on the containers sum to 8,064 MiB: more than half the host is promised and about a fifth of the promise is used.

| Component | CPU (share of one core) | Memory (MiB) | Ours? |
|---|---:|---:|---|
| ClickHouse | 5.1% | 508 | third party |
| VictoriaMetrics | 2.5% | 86 | third party |
| Wotan | 1.8% | 159 | ours |
| trace collector | 1.6% (93% before S-WATCH-TRUE) | 60 | ours |
| Prometheus | 0.9% | 137 | third party |
| dashboard-backend | 0.8% | 298 | ours |
| Grafana | 0.4% | 127 | third party |
| vor | 0.0% | 232 | ours |
| timeguru, sophia, monad, micromanager, captain, architect, kanban, cuirass, akira | 1.0% together | 184 together | ours |
| the other eight containers | 0.3% together | 204 together | mixed |

**Nothing of ours grew.** At 08:00 UTC on 11-12, 11-13, 11-14 and 11-15, dashboard-backend held 330, 391, 360 and 329 MiB, and its goroutine count stayed between 164 and 168. Wotan held about 160 MiB from its second day. No leak was seen in three and a half days. dashboard-backend is the largest process we wrote.

**East.** No containers. The trace collector uses 1.6 to 2.0 of its 4 cores (24 h mean 1.996) and the load is 2.8: it is still the largest single cost in the kingdom. The fix is built and measured on west; the deploy on east is attended (S-WATCH-TRUE Phase 4B). Everything else of ours on east is under 1% of a core each. The largest memory there is journald, 339 MiB.

## Use

Requests in 7 days that were not a health check or a scrape:

| Service | Requests | What they were |
|---|---:|---|
| wotan | 37,043,668 | 35.3 million `GET /api/v1/topics/`, 1.7 million `POST` |
| dashboard-backend | 180,026 | eight API paths, read on a timer |
| logstore-writer | 56,818 | |
| kanban-app | 351 | the board, its stream, a few task writes |
| timeguru | 29 | `/timeline` 18, `/api/v1/timeline` 9 |
| wiki-server | 25 | |
| unheaded-daemon | 14 | |
| monad | 13 | `/api/v1/stats` 9 |
| sophia | 12 | `/api/v1/knowledge` 9 |
| architect | 11 | `/design` 9 |
| captain | 11 | `/api/v1/strategy` 9 |
| micromanager | 11 | `/api/v1/backlog` 9 |
| alert-mailer | 8 | |
| kev-poller | 4 | |

**architect, captain, micromanager and timeguru** are four containers. Their READMEs call them "architecture decision tracking", "leadership and strategy", "task execution and tracking" and "timeline tracking". In a week each was asked for its one list nine times; the same count on every one points to one sweep, not to a reader. Each also holds two or three Wotan streams and reports its health.

**What this does not show.** HTTP is one signal. alert-mailer, kev-poller and logstore-writer work on a timer or on events and are not judged by it. What each service publishes to Wotan or reads from it is not counted per client, so monad, sophia, akira and cuirass are not judged here either.

## Found on the way

- **Wotan answers 68 `GET /api/v1/topics/` a second** (35.3 million in 7 days), all with status 200, while every client reports its stream up and counts no fallback. Who asks is not known. A read loop on a timer is the shape the trace collector had.
- **`TraceCollectorHighLatency` cannot fire:** nothing has observed its histogram in the 14 days VictoriaMetrics holds (S-WATCH-TRUE).
- **Akira holds 44 of its 64 MiB limit.**
- **Disk on west that is not the stack:** 428 Docker images, 30 in use, 57 GB (52 GB reclaimable; `rocm/pytorch` is 31 GB, `open-webui` 4.5 GB, then netbox, jenkins, three suricata tags, alloy); build cache 261 GB; 137 volumes, 22 in use. An empty `kind` network from March.

## To settle

1. **The four tests.** Are they the right ones, and what are the pass marks? ADR-144 item 3 (a target footprint for a small host) is still unset and belongs here.
2. **architect, captain, micromanager, timeguru:** remove, fold into one service, or keep? Before the answer: read who calls each (the kanban board, the dashboard, Zhen), count their Wotan traffic for a week, then stop one on west for a day and see what notices.
3. **monad, sophia, akira, cuirass:** the same question, once Wotan counts messages per client.
4. **Wotan's 68 reads a second:** find the caller. If it is a poll, replace it with the stream the client already holds.
5. **dashboard-backend:** what it keeps in 300 MiB, and whether eight polled endpoints should be one stream.
6. **The third-party half** (ClickHouse, Prometheus, Grafana, VictoriaMetrics, Traefik): ADR-144 item 4 stands; the log store week decides ClickHouse.
7. **East:** the attended deploy gives back about 1.9 cores of 4.
8. **Reliability, per service that stays:** one row each: what a restart loses, which alert names its failure and when that alert last fired in a test, which test fails when it breaks. Where a row cannot be filled, that is the work.
9. **Scale, per service that stays:** the number its cost grows with, the limit, and what it does at the limit. A run at ten times today's event rate, on west or on the larger lab Stevie plans, before a third host is added.
10. **A gate so it stays that way:** ADR-144 item 6 (a CPU and memory budget per component checked after a soak, and an alert on a unit that holds a set share of a core for an hour).
11. **Housekeeping outside the stack:** the Docker images, build cache and volumes; the `kind` network. Stevie's hands.

## Decision

None. Parked under ADR-112. Nothing is removed or stopped on this ADR's word.
