<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-147: Every service earns its place: use, cost, reliability and scale, measured

**Status:** Proposed. The analysis started 2026-11-15 on Stevie's word (last update below); no plan yet, no load run, nothing stopped.
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

## Update 2026-11-15 (the stress run: pitched, not started)

Stevie, later the same day, verbatim:

> id like to stress test the severs running as a cluster at 80-90% load for a prolonged period with real applications, work load, iops, memmory utilization - i'd like to fine tune not only the unheaded and bellistech applications but also the performance of east and west as well

> that was meant to be appended to adr not done right now

No load was run. What was read to ground it, on both hosts, changing nothing:

- **West:** Ryzen 5 7600X, 12 threads, 14 GiB. Root is on the 1 TB NVMe. **`/var` is the 2 TB spinning disk** (`sdb`, ST2000DM001), and Docker's data lives there: ClickHouse, Postgres, VictoriaMetrics, Vector's buffer. A 512 GB SATA SSD and a 500 GB spinning disk hold nothing mounted.
- **West already stalls on I/O at idle.** Pressure (`/proc/pressure/io`): some task was waiting on I/O 7.0% of the last five minutes, all tasks 6.6%. Over 10 s `sdb` took 9 writes a second, 0.3 MB/s, 12 ms a write, 10% busy. The cgroups that wait are Postgres, Vector, VictoriaMetrics and ClickHouse, in that order. CPU pressure was 1.3%, memory 0.2%.
- **East:** AMD A8-5500, 4 cores, 7 GiB, root on a SATA SSD, swappiness 60 (west: 10). CPU pressure 2.4%, which is the trace collector.
- **Between them:** 1 Gb/s full duplex on `enp16s0`, and WireGuard.
- **Load tools present:** west has `fio`, `hey`, `perf` and `bpftrace`; east has `perf`. In the repository: `scripts/stress-cannon.py` (Monad packets through Shield), `cmd/perf-benchmark` (HTTP, HTTP/3, containers, end to end), `cmd/chaos-controller`. `stress-ng`, `sysbench`, `pgbench` and `iperf3` are not installed; each is a third-party addition and needs Stevie's yes.

**To settle, continuing the list above:**

12. **Which real applications are the load.** Which of Stevie's programs run on the cluster for the test (netbox, the blog and site, vor, orb, the rss and weather daemons, raven, Zhen with its model server, others), and what drives them: replayed requests, a recorded day, or written clients.
13. **What 80 to 90% means:** of CPU, of memory, of disk throughput, or all three at once. Pressure is read beside utilisation: west stalls on I/O with its disk 10% busy.
14. **How long,** and who attends: hours, a day, a week.
15. **Rails.** The load runs in its own cgroup slice with CPU, memory and I/O limits, so the stack and sshd keep a reserve. Swap is not counted as memory. Stop marks are set before the start: all tasks stalled on memory, an SLO burning, a unit killed for memory, a disk too hot. Not during an evidence window (the log store week ends about 2026-11-17). East is attended.
16. **What passes:** the SLOs (ADR-100) hold under load; every alert that should fire does; every service restarts clean under load; nothing grows without bound; the 99th percentile latency of each service is on record.
17. **Tuning order after the first run,** one change at a time, each with a before and an after: where the data lives (the databases on the NVMe or the SSD instead of the spinning disk: expected to be the largest gain, not measured), then the I/O scheduler, swappiness and dirty ratios, cgroup weights, the Go runtime (GOMAXPROCS, GOGC, the memory limit), Postgres, ClickHouse and VictoriaMetrics settings, the NIC and WireGuard.
18. **What "cluster" means for the test.** Today: two hosts joined by BGP and WireGuard, Wotan in memory, one Well with no replica. What the run should prove about losing a host.
19. **Which generators:** `fio` and `hey` are present; the others need a yes.
20. **Where:** on east and west as they are, or on the larger lab first.

**Status:** unchanged, Proposed (parked).

## Update 2026-11-15 (how the stress run is to be worked)

Stevie, verbatim:

> as a large well thought out detail oriented multi /unheaded-skill(a few) effort

So items 12 to 20 are not settled in one pass by one reader. Each has an owner among the skills. The owners answer their items in writing, in this ADR, with numbers from the hosts; the plan is forged only after that, and only when Stevie lifts the freeze for it.

| Skill | Owns | What its answer contains |
|---|---|---|
| Scientist | 13, 14, 16 | The experiment: what is held fixed and what is varied, how the load is stepped up to 80 to 90%, how long a run must be to tell a change from noise, the pass marks and how each is read |
| Architect | 15, 17 (hosts), 18, 20 | The rails and stop marks; the storage layout (what moves off the spinning disk, to where); kernel, cgroup and network settings in the order they are tried; what "cluster" must prove; which lab |
| Erda | 17 (data) | Postgres, ClickHouse and VictoriaMetrics under load: their settings, their growth, where their files live, what a restart under load costs |
| Developer | 12, 19, 17 (Go runtime) | The clients that drive each real application and what a recorded day of each looks like; the generators; GOMAXPROCS, GOGC and memory limits per service, measured |
| Micromanager | 8, and the order of work | One reliability row per service before the run; the list of what must be true to start, to continue and to call it done |
| Warmonger | the battle plan | Written last, from the answers above: every command, gate, stop mark and rollback |

**Status:** unchanged, Proposed (parked).

## Update 2026-11-15 (a project, worked by a round table)

Stevie, verbatim:

> all 3, deep dive analaysisd for ADR 147 will require a full blown multi skilled  /unheaded-scientist /unheaded-architect /unheaded-archimedes /unheaded-librarian /anthropic-skills:unheaded-developer /anthropic-skills:unheaded-micromanager /anthropic-skills:unheaded-busboy round table to then forge a /anthropic-skills:unheaded-warmonger battle plan. This will be a project not a quick task - detailed planned and coordinated steps, documentation, recommendations, notations, observations, and fixes. we don't need anything flash or wordy - we are going for a fine tuned machine(application stack) at the end of the day not a pretty gui - we have spent a lot of work on getting the UI going but it's not my passion - system engineering and network enginering are - unheaded and bellistech should showcase that and be a reflection of that.

> probaly include /anthropic-skills:unheaded-blackmage too

**Item 13 is answered: all three.** The load is 80 to 90% of CPU, of memory and of disk throughput at once. ("all 3" is read as the answer to item 13; if it meant something else, this line is corrected by a dated note.)

**What the project is.** Not a task and not one sprint. Its outputs are coordinated steps, a record of observations and notations as they are made, recommendations with the number behind each, and fixes. Its end is a tuned machine: the application stack and the two hosts under it. No new interface is built for it; results are read from the metrics that exist and from a terminal. Systems engineering and network engineering are what Unheaded and the bellistech programs are meant to show, so the hosts, the kernel, storage and the network between east and west are in scope as much as the services. The writing is plain: tables and numbers, no more words than the reading needs.

**The table.** This replaces the owner table in the update above.

| Skill | Its part |
|---|---|
| Scientist | The experiment (items 13, 14, 16): what is held fixed, what is varied, how the load is stepped to 80 to 90% of all three, how long a run must be to tell a change from noise, the pass marks |
| Archimedes | The numbers: what 80 to 90% is in units on each host (cores, bytes, IOPS, MB/s), a capacity model per service, run length and variance, growth forecasts; every figure with its bounds and the command that made it |
| Architect | The hosts and the network (items 15, 17, 18, 20): storage layout, kernel and cgroup settings, the link, BGP and WireGuard between east and west, what "cluster" must prove, the rails, which lab |
| Developer | The load and the code (items 12, 19, and the Go runtime in 17): clients that drive each real application, the generators, runtime settings measured per service, the fixes the run turns up |
| BlackMage | The run as an attack: what breaks first, what a load can exhaust or amplify, whether limits, alerts and security controls still hold at 90%, whether the load can leave its rails |
| Micromanager | Items 1 to 11 in order, one reliability row per service (item 8), and the lists of what must be true to start, to continue and to call it done |
| Librarian | The record: where observations, notations, recommendations and fixes are written as they happen (this ADR, a progress file, the wiki for facts about our hosts, vor sheets for general system knowledge), and that every document says the same thing |
| Busboy | The handoffs: who waits on whom, and what else on the hosts the project must not disturb (the log store week, the attended east steps, the larger lab) |
| Warmonger | The battle plan, forged last from the table's answers, on Stevie's word |

Not named by Stevie and so not at the table: Erda. The database part of item 17 sits with Architect unless he adds her.

**Status:** unchanged, Proposed (parked). No load run, nothing changed on a host.

## Update 2026-11-15 (answers; the analysis starts)

Stevie answered four questions. His words, verbatim:

- When the round table starts: **now.** It reads the hosts and writes; it runs no load and changes no host.
- What "all 3" meant:
  > BOTH! also included /unheaded-erda at the table.  start item 2 - with cover all our bases - let's fan out on what-ifs, how does the system function under "all imagineable" (of course not all but a varied array of conditions maybe all three at once maybe just too much disk I/O) - included as line items for project completion is implmenting all findings, and performance  optimizations. DESIGN FLAWS - there is some weird wacky stuff conjoured up at 3am on little sleep in our stack!
- Which programs are the real load:
  > Blog and site, orb, vor, " i dont want to run the rss-daemon or weather-daemon here - weather was designed to keep google api key usage in free catagory, no monthly bill - and rss polls real human's websites - I dont want to generate any unessary traffic or load for them"
- Erda: **at the table.** The database part of item 17 is hers.

What follows from them:

- **Item 12 is answered:** the blog and site, orb and vor. Not the rss daemon and not the weather daemon: one would spend a metered key, the other would load other people's sites. No test in this project sends traffic to a host Stevie does not own.
- **Item 13 stands and widens.** All three resources at 80 to 90% at once is one condition among several. The run is a matrix of conditions: each resource alone, pairs, all three, and faults on top (a slow disk, a full disk, a lost link, a lost host). The table writes the matrix.
- **The three first steps are part of the project:** the four role services (item 2), the caller behind Wotan's 68 reads a second (item 4), and the trace collector on east (item 7, attended). "start item 2" is read as: begin with the services and the Wotan caller while the matrix is designed.
- **Done means fixed.** The project is complete when every finding is implemented or has a written reason why not, and the optimizations are in and measured. A list of recommendations is not the end.
- **Design flaws are in scope by name.** The table looks for what was built wrong, not only for what is slow: loops on timers, queues with no bound, state with no owner, a second copy of something, a service with no caller.
- **The freeze (ADR-112) is lifted for this ADR's analysis.** The battle plan still waits on Stevie's word after he reads the table's answers.

The table's answers are written below this line as they come in, each under its skill's name, dated.

