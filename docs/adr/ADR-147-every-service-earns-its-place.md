<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-147: Every service earns its place: use, cost, reliability and scale, measured

**Status:** Proposed. The round table reported 2026-11-15 ("The table's answers" below). No plan yet: it waits on Stevie's decisions. No load run, nothing stopped, no host changed.
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

Stevie, a few minutes later, verbatim:

> we can add other services I might like to have local in the house - music/video streaming something or whatever - something game related - zhenai should be treated like a production application in itself not a default part of unheaded

- **Tenants.** The hosts are to carry programs that are not Unheaded: a media server, something for games, others not yet named. None is chosen here and none is installed without Stevie's yes (each is third-party). The table designs for a tenant it has not met: what a tenant is given (CPU, memory, disk, network, a name, a login), what it is refused, and how the stack keeps its own share.
- **Zhen is a tenant.** Zhenai is treated as a production application that runs on the platform, with its own budget, its own alerts and its own place in the load, not as a default part of Unheaded. Where the stack assumes Zhen is there, that is a finding.

Stevie, again, verbatim:

> unheaded services might be on every host in an environment but you would only need a cluster running zhenai

- **Three sets, three budgets.** What runs on every host (collectors, the baseline, the agent), what runs once per environment (Wotan, VictoriaMetrics, the Well, the dashboards), and what is a tenant on a few hosts (Zhen). The first set's cost is multiplied by the number of hosts, so it carries the tightest budget and is the first thing test 4 (scale) is applied to. The table sorts every service into one of the three; a service that fits none is a finding.

The table's answers are written below this line as they come in, each under its skill's name, dated.

## The table's answers (2026-11-15)

Seven seats (Scientist, Archimedes, Architect, Erda, Developer, BlackMage, Micromanager) read both hosts and the code, each for 12 to 22 minutes, read only. The Librarian's and Busboy's parts are this merge. Each seat's full report (29 to 48 kB: every figure with its command, design flaws with `file:line`, what-ifs, recommendations, questions) is on west in `~/tmp/adr147-table/`. They are not in the repository: together they map the live lab's listeners and weak points, and committing that is Stevie's call. "(checked)" below means the coordinator measured it again; everything else stands on one seat's measurement.

### What the analysis disturbed

- **The log store week, 17:04 to 17:10Z.** The Architect seat walked west's `/var` with `du` twice; I/O pressure went from about 6% to 39% (checked). Recorded in `docs/battle-plans/progress/S-E1-LIVE-progress.md`. The brief forbade load and did not forbid a walk of that disk.
- Two seats wrote and removed a temporary file in `/dev/shm`.

### Corrections to the text above

1. **The use table overstated dashboard-backend.** Of its requests in 7 days, 98% were status 401 from a logged-out page that polls without end (183,809 of 187,651, checked). Real use was about 3,800.
2. **"Nothing of ours grew"** holds for the two processes named and no further. West's available memory fell 138 MiB a day, 121 of it with no owner, because nothing records memory per cgroup (Scientist; not checked).
3. **VictoriaMetrics holds 5.2 continuous days,** not 14: a 12-day hole and a label change. Only 62 hours on west are steady enough to be a baseline.
4. **West has about 11 GiB for programs,** not 14: 2 GiB is reserved as huge pages and 1.93 GiB of it is idle (checked).
5. The container limits sum to 8,064 MiB over the 23 running containers (above); Archimedes counts 10,880 MiB promised. Not tied out.

### Findings, merged

The seats wrote 114 design-flaw rows; many are one thing seen from two sides. Agreement between seats that worked apart is noted, since it is evidence.

**Disk**

| Finding | Seats |
|---|---|
| The idle stall is flush count on a 7,200 rpm disk: 17.9 ms a flush against 1.7 ms on the NVMe; `sdb` is inside a flush 14% of uptime; all 41 sync calls in a 10 s sample came from VictoriaMetrics, Postgres and Vector | Architect, Erda, Archimedes |
| **Vector inserts into ClickHouse every second** (the sink has no `batch` block, checked): 47,000 parts a day of about 5 rows, 42 GiB a day merged to keep 127 MiB, 45 to 71 GB a day written, about 68% of all writes to `sdb`; 337 bytes written per byte kept | Erda, Archimedes |
| **Postgres's 256 MiB limit caps its page cache:** the limit was hit 52,740 times (checked) and 35 to 39 GB was read again from `sdb`; its log queries run 80 to 180 times slower than ClickHouse's. With the row above, this colours the log store week's verdict | Architect, Erda, Archimedes |
| Every Akira health report is a durable Postgres transaction: 0.68 commits a second on a 22-row table, 73% of all WAL flushes | Erda, Developer |
| Every log line is written three times (journald: 19.8 GiB in 3.5 days; rsyslog, uncapped; ClickHouse), and health checks make the lines (sophia: 60 a minute) | Architect, BlackMage, Developer |
| Prometheus and VictoriaMetrics store the same samples for 30 days | Erda |
| `sdb` has 38,246 power-on hours and carries every database | Scientist, Archimedes |

**Memory**

| Finding | Seats |
|---|---|
| **A shortage becomes a stall on the database disk:** the 4 GiB of swap on the NVMe is 78% used at idle (checked), the next 30 GiB is a file on `sdb`, `vm.overcommit_memory` is 1 (checked), and a container may swap as much as its limit | Architect, BlackMage, Scientist, Archimedes |
| 2 GiB of huge pages reserved by a bootstrap script for the model server; 1.93 GiB idle (checked) | Architect, Archimedes |
| Ten Go containers have a memory limit the Go runtime is not told about | Developer |
| No weights and no floor: sshd, journald, Docker and every container are peers; the user slice may take 12 of 12.3 GiB; no container has a pid limit | Architect, BlackMage |

**Recovery and alarm**

| Finding | Seats |
|---|---|
| **18 of 23 containers do not come back after a crash** (checked): the host's override sets restart `no` to keep the stack from starting at boot. Wotan, the Well, VictoriaMetrics and ClickHouse are among them | Architect, Erda, BlackMage, Micromanager |
| **No alert reaches a person:** the mailer has been in dry-run since 11-10 (checked); 47 of 87 rules carry `urgency: none`, `HostDiskFull` among them | BlackMage, Micromanager |
| **No rule reads a resource:** nothing on pressure, CPU, memory, swap, an out-of-memory kill, a restart count or temperature (checked). Vector counts memory kills and nothing reads the counter | Scientist, BlackMage, Micromanager |
| Rules that cannot fire: `TraceCollectorHighLatency`, both AIDE rules (their series does not exist), `VectorSinkDroppingEvents` (it watches a buffer that blocks and never drops). 36 of 55 alerts have never fired in 26 days | BlackMage, Micromanager |
| **The SLOs cannot fail under load:** all eight measure "it answers" or "it is fresh"; six budgets are negative at idle (checked); `/health` answers 200 when degraded; the Wotan SLO measures one host of two | Scientist, BlackMage, Micromanager |
| **The latency histogram cannot see our latencies:** its first bucket ends at 5 ms (checked) and services answer in 0.1 to 0.4 ms, so every p50 reads 2.5 ms and every p99 4.95 ms | Scientist, Archimedes |
| The watchers are unwatched: nothing can report west down; no rule for VictoriaMetrics, Alertmanager or the mailer down | Architect, BlackMage, Micromanager |
| Backups: Postgres only (its restore drill passed 2026-11-15). ClickHouse holds audit logs kept 455 days with no backup; VictoriaMetrics has none | Erda, Micromanager |
| `scripts/qa-smoke.sh` probes plain http against TLS listeners and is always red; systemd start limits are set so they cannot trip (wiki-server: 15 restarts, active) | Micromanager, BlackMage |

**Wotan**

| Finding | Seats |
|---|---|
| **The 68 reads a second are 34 poll loops** (item 4, answered). A "stream" on the client is a 500 ms poll per topic unless gRPC is used; a gRPC stream that fails once polls for ever; the health output still says stream. 79% fetch nothing anyone reads: 12 fill a discovery registry with no reader, 6 poll `alerts.critical` (no publisher), 9 are dashboard topics with no publisher | Developer; Micromanager and BlackMage reached the same code apart |
| Up to four Wotan clients per process: 63 members for 14 processes | Developer |
| Every trace event is published twice (`traces.*` and `ebpf.*.events`): 37% of Wotan's payload is a copy | Developer, Archimedes |
| Room count times room size allows 78 GiB in a 256 MiB container; the rate limit is keyed on an address that every proxied client shares | Archimedes, BlackMage |

**Services with no caller** (items 2 and 3)

| Finding | Seats |
|---|---|
| architect, captain, micromanager: empty data directories; their only callers are a compliance probe and health checks | Developer, Micromanager |
| monad runs demo handlers; sophia's inference loop has no body; Cuirass on west reconciles a mock LXD and nobody reads its drift topic | Developer |
| timeguru keeps its database on tmpfs while the Well holds an empty table for the same data; kanban can fall back to a SQLite file of 10-28 and split the board | Erda |
| East: seven units on the shared account west retired, with no memory ceiling; five have had no bus since east's Wotan was switched off on 11-10 (checked) and log refused connections (240 lines in a minute, checked) | Architect, Micromanager, Archimedes |
| FRR on west runs three routing daemons with no configuration | Architect |

**Akira, eBPF**

| Finding | Seats |
|---|---|
| Akira (0.10 core, 64 MiB) is throttled in 44% of its periods at idle (checked), starts an interpreter every 30 s to check a certificate, has left 18 zombies (checked), and its verdicts are refused for lack of a signing key. Probing a role service costs more CPU than the service | Architect, Developer, BlackMage, Archimedes |
| packet-marker's `FLOW_STATE` map is a plain hash, never deleted from, insert failures ignored, no reader found: full on east, full on west in about 30 h. flow-tracker's event ring is not drained: 94% dropped | BlackMage |
| East's collector spends 2.0 cores and its events do not arrive: its fallback queue has read exactly its cap, 1,000, for five days | Archimedes, Scientist |

**Network and cluster** (item 18)

| Finding | Seats |
|---|---|
| One wire, and west's uplink is wifi. East reaches DNS, time and the internet only through west. Every alert sender is on west | Architect |
| BGP and BFD run inside the only tunnel and announce a blackhole each way: if the session falls, no flow changes | Architect |
| The WireGuard and bird files in the repository differ from what is deployed; west's sysctls were written once by a bootstrap script for a model server, east runs defaults, and no file in the repository is the source of either | Architect |
| West has no input filter: node_exporter, huginn, Docker's metrics, kev-poller, the kanban task list and the dashboard's `/metrics` answer the wifi LAN with no credential. Alertmanager and Prometheus on loopback take no credential, so any local process can silence every alert | BlackMage, Architect |
| East's CPU: 61 °C at the check, 68 °C in the seat's window, critical at 70 °C | Scientist |

**Zhen assumed** (the tenant rule)

Akira's default targets name `zhenai`, `inference` and `gateway`; 10 of 13 tables in `unheaded_app` are Zhen's and its role holds three connections with the model server down; the huge pages; the IOC audit and the health sweep are scheduled inside Zhen; Zhen's rules sit in the platform's rule set (`ZhenSchedulerStalled` fired for 3,617 minutes while Zhen was paused on purpose). Seats: Architect, Erda, Developer, BlackMage, Micromanager.

**Measuring**

No series exists for the blog, the site, orb or vor; nginx does not log request time; CPU and memory are recorded only for Go processes that export them; the recorders sit on the disk a disk run would load. Seats: Scientist, Archimedes.

### The experiment (Scientist, Archimedes)

- **38 runs, not the 1,792 of the full matrix.** Load is made at fixed rates inside a capped slice. Pressure is a result and a stop rule, never the setpoint.
- **What can be told from noise:** a 20% change on a disk series needs six one-hour blocks each side, about 12 hours per tuning change. A 5% change needs about 7 days per change (whole-host CPU on west: about 130 days). 5% is not a usable bar.
- **80 to 90% in units, west:** memory, of 10,976 MiB usable; CPU, 9.6 to 10.8 cpu-seconds a second of logical time, or 5.5 to 9.4 of real work because of SMT; `sdb`, about 55 flushes a second at most. Device ceilings stay estimates until `fio` may read the raw devices.
- **Predicted at 85% disk busy:** I/O pressure about 41%, log inserts 9.5 times slower.
- **Pass marks proposed:** the 99th percentile at 85% load at most three times idle; no drops and no page at 85%; growth under 1% a day.
- **Instruments come before load:** latency buckets, resource alerts, per-cgroup series, series for the four applications.

### Order of work proposed (Micromanager, Busboy)

| Stage | What | Waits on |
|---|---|---|
| 0 | Stevie's decisions (below); one deduplicated finding list with the template: id, seat, evidence, fix, owner, before, after, status | Stevie |
| 1 | **See and survive:** crashed containers return; mail or a watched terminal; resource alerts; latency buckets; per-cgroup and application series; the rules that cannot fire; backups for ClickHouse and VictoriaMetrics; a smoke check that can pass | decisions 1 to 4 |
| 2 | **Waste out,** each measured before and after: Vector's batch; Postgres's limit; the heartbeat transaction; the poll loops, the unread registry, `alerts.critical`; the doubled trace topics; the tripled logs; Akira; `FLOW_STATE`; the services with no caller; Zhen's separation | the log store week (ends about 11-17) for anything touching Vector, ClickHouse or Postgres |
| 3 | **Layout:** the databases off the spinning disk (three layouts in the Architect's report; the smallest is a 32 GiB volume on the NVMe, 5 to 10 minutes of stack downtime, undone by an unmount; flush time expected 17.9 → 1.7 ms); swap; huge pages; floors, weights and pid limits; an input filter | attended |
| 4 | **East:** the trace collector deploy; its Wotan and its seven units | attended; S-E1-LIVE Evening 10B |
| 5 | **Baseline:** five to seven quiet days on the new layout | stages 1 to 4 |
| 6 | **The run:** the 38 conditions, inside the rails | Stevie present for east |
| 7 | **Tuning,** one change at a time, about 12 hours each | |
| 8 | **Soak** of seven days; done when every finding is fixed or has a written reason | |

### Decisions for Stevie

The seats asked 58 questions. Merged:

1. **Restart.** Keep "nothing starts at boot" and let a crashed container return (`on-failure`)?
2. **Alarm.** Mail on, or a watched terminal for the run? The cap of six a day? Rules with `urgency: none` such as `HostDiskFull`?
3. **Who watches west.** East, or something outside both?
4. **Does a planned stop spend error budget?** Six budgets are negative from stops that were ordered. And `ci-develop`, firing for 23.8 days: fix CI or take it out of the set?
5. **The log store week.** Read its verdict knowing ClickHouse ran with one-second inserts and Postgres inside 256 MiB, or measure again after both are fixed? The log table in Postgres reaches its 4 GiB cap about 2026-11-24: which store keeps the logs?
6. **Backups** for ClickHouse (audit logs, 455 days) and VictoriaMetrics, or "not backed up" in writing?
7. **architect, captain, micromanager:** remove or fold into one. **monad, sophia:** the same question. **Cuirass on west:** mock, real LXD, or not run. **timeguru's** two homes.
8. **`alerts.critical`:** build the publisher or drop the rule that every service subscribes. **Discovery:** build a reader or go back to static endpoints.
9. **The dashboard's private metric store:** may every graph depend on VictoriaMetrics?
10. **Prometheus beside VictoriaMetrics:** keep both, or evaluate rules against VictoriaMetrics (`vmalert`, third-party)?
11. **Disks.** May `sdc` (512 GB SSD) and `sda` (500 GB), each holding an NTFS filesystem, be wiped? Which layout: the database volumes first, or all of `/var` in one attended sitting? Is the journal part of the move?
12. **Swap:** none, or a small one on the NVMe only. **Huge pages:** release the 2 GiB?
13. **Shares.** A floor for the stack (3 GiB and 2 cores proposed), a ceiling for all tenants together, a ceiling for login sessions. The GPU: Zhen's alone, or shared?
14. **One way in for a tenant:** a systemd unit in a slice, a compose project, or an LXD instance. Will a tenant get Docker access? (If yes it is root.)
15. **Zhen as a tenant:** its tables in their own database; the IOC audit and the health sweep on timers the platform owns.
16. **West's open ports on the wifi LAN** (9100, 9110, 9323, 19120, the kanban task list): intended, or closed?
17. **East.** What is it for: the every-host set only, or a second home for the once-per-environment services? Its own uplink? Its seven services: stop until Evening 10B, or do 10B first? Run it at 85% at all, given 68 °C against 70?
18. **The network.** A second NIC or a switch for west? BGP carrying real routes over two paths, or static routes?
19. **`FLOW_STATE`:** is anything meant to read it?
20. **The experiment.** The pass marks; 20% as the smallest change worth keeping; the governor as shipped or pinned; a deploy freeze during each stage; the seven-day soak and who watches it; clock skew and a truly full disk in a lab only.
21. **CPU at 80 to 90%:** of logical time or of real work? **The reserve:** 7% of memory held back?
22. **Tools:** `fio` reading the raw devices; `iperf3` added.
23. **Load.** May orion's nginx log, with client addresses removed, be the recorded day for the site? May vor get an access log? What counts as load on orb (viewers, refresh)?
24. **Orphans:** the old `unheaded-well-data` volume, three ClickHouse tables with no retention, the SQLite fallback.
25. **The seat reports:** into the repository, or kept on the host.

### Stevie's answers to the decisions (2026-11-15)

By decision number. He chose to answer all of them before the plan is forged.

- **1. Restart: keep `no`.** A container that dies stays down until a person starts it. In the run, each death is then a finding that can be seen. It follows that stage 1 must make a stopped container raise an alert, since nothing brings it back.
- **7 (first part). architect, captain, micromanager: remove all three,** with their volumes, scrape jobs, health targets and the rule that every service subscribes on their behalf. monad, sophia, Cuirass on west and timeguru's two homes are still open.
- **25. The seat reports stay on the host** (`~/tmp/adr147-table/` on west), outside the repository.
- **2. Alarm: mail on.** The mailer leaves dry-run. A stopped container and a memory kill each get a rule that mails. The cap of six a day is raised for run days. (The `urgency: none` rules, `HostDiskFull` among them, are read against this in the plan: a rule that should mail gets an urgency.)
- **5. The log store week: measure again after the fixes.** This week ends as it stands (about 2026-11-17) and is the record of the two defects. Then Vector's batching and Postgres's limit are fixed, and a second, fair week is run before the log store is chosen. Nothing touching Vector, ClickHouse or Postgres changes before this week ends.
- **7 (rest). Remove the monad service and the sophia service; stop Cuirass on west.** These are the HTTP containers; the Monad wire format, the Sophia dictionaries and the eBPF maps are not touched. Cuirass runs only where there is a real LXD to reconcile. timeguru's two homes for one table is still open.
- **11. Disks: the database volumes go to the NVMe** (Architect's layout 1): one 32 GiB volume for Postgres, ClickHouse, VictoriaMetrics, Prometheus and Vector; 5 to 10 minutes of stack downtime; undone by an unmount. `sdc` and `sda` are not touched. Whether the journal moves too is still open.
- **3. East watches west.** With it, a rule for every decision that follows, verbatim: "Highly available and scalable design is the end goal with everything - build it as if we have a real production target". So east needs its own way out to say that west is gone, and the choices on east's role (17) and the network (18) are read against this rule.
- **4. A planned stop does not spend error budget.** A stop that Stevie orders is excluded, so the budgets mean something and can be a pass mark. `ci-develop` leaves the alert set until CI is fixed.
- **6. Back up both** ClickHouse (the audit logs) and VictoriaMetrics, each with a restore drill, before any load is run.
- **12. Swap and huge pages:** Stevie asked for a recommendation. Given: remove the 30 GiB swap file on the database disk and the two unused swap files; keep the 4 GiB on the NVMe; release the 2 GiB of huge pages; add alerts on swap use and memory pressure. Reasons: with restart `no`, turning swap off altogether makes every brief spike a kill that stays dead, while a small swap on the NVMe absorbs a spike at NVMe speed; swap on the spinning disk is the path by which a memory shortage stalls every database; the huge pages give 1.93 GiB back, which is most of what is in swap now. His answer is recorded when given.
- **12. Yes, as recommended:** the swap file on the database disk and the two unused ones are removed, the 4 GiB on the NVMe stays, the 2 GiB of huge pages is released, and swap use and memory pressure get alerts. An attended change on west.
- **17 (role). East is a second home, as far as it can be.** It gets its own uplink, watches west, and can run the once-per-environment services (Wotan, alerting, a copy of the Well) when west is gone. It is designed as a real pair; its hardware (4 cores, 7 GiB, 61 to 68 °C against 70) sets what it can carry, and the trace collector fix goes in first. Its seven idle services and whether it is run at 85% are still open.
- **18. Two paths, and BGP carries real routes.** A second wired path (a NIC or a switch, which Stevie adds) gives east and west two links; BGP with BFD announces real service prefixes and fails over between them. The network becomes something the run can break and watch recover. The idle FRR daemons go.
- **14. One way in for a tenant: an LXD instance each,** with its own limits, network and storage, placed by Cuirass. No tenant gets Docker access to a host. Zhen moves into one. This changes the answer to 7 for Cuirass: on west it stops reconciling a mock and runs against the real LXD once there is a tenant to place.
- **8. Build both properly.** `alerts.critical` gets a publisher and discovery gets a reader, so both do real work, and both run on real streams: the client's "stream" stops being a 500 ms poll, and a stream that fell back returns when it can. The 34 poll loops end by being replaced, not by being deleted.
- **9 and 10. One store, two readers.** VictoriaMetrics is the store. Prometheus keeps only what rule evaluation needs (hours, not 30 days). The dashboard reads VictoriaMetrics and drops its private copy. No new third-party tool.
- **15. Zhen is a module with a switch.** Verbatim:
  > I think zhen should be modular - a major attractive feature, cool, fun, nice to have - but the unheaded platform, or project should be able to be used by people who do not want or need any AI. flip the switch and you gain amazing functionality, it's what I hope to run on a real public internet facing server (unheaded with zhen enabled as backend - i do not mean allowing anyone to hit the zhenai api or ui from the web but it being used to administer and manager the system some public facing website or web app exist - maybe just running bellis.tech and the other stuff currentlty on my vps someday once i really trust it,...

  Read as: (a) the platform is whole without Zhen: nothing in its health list, its database, its rule set, its schedules or its memory reservations assumes Zhen; the IOC audit and the health sweep run on timers the platform owns; (b) one switch turns Zhen on, as a tenant in its own instance (decision 14) with its own database, backup scope, rules and budget; (c) when on, Zhen administers and manages the system from behind: its API and UI are never reachable from the public side of a host that serves a public site; (d) the target this is built toward is a public, internet-facing server carrying real sites, once it is trusted.
- **16. Close them.** An input filter on west: the metric ports, the kanban task list and the dashboard's `/metrics` answer loopback, the link to east and WireGuard only. sshd and the fronts used from the house stay reachable.
- **13. Shares: no numbers now.** Verbatim: "determining this answer will be part of the quest in acheiving completion of ADR-147.... no numbers should be wrote out at this time without through group involed testing - that said /unheaded-scientist should probably be there". So the floor for the stack and the ceilings for tenants and sessions are an output of the project, found by measurement with the table present and the Scientist leading. The figures proposed above (3 GiB, 2 cores, 7% reserve) are a seat's proposal and bind nothing.
- **20. The experiment: as proposed, and then again at 5%.** First pass: the 38 runs, the proposed pass marks, 20% as the smallest change kept, no deploys during a stage, clock skew and a truly full disk in a lab only, a 7-day soak. Second pass: fine tuning, keeping changes as small as 5%, at about a week of measurement per change. Verbatim: "we are going for fine tuning. we can't hit version 1.0 and have a joke application - if i share this with anyone, I want to be able to proudly share it  with gusto".
- **21. 80 to 90% of real work,** not of logical CPU time: the maximum is found first with a ramp, and the load is held at 80 to 90% of it. And the frame for the whole project, verbatim:
  > let's mock this out as 2 AWS instances or physical servers in a datacenter --- unheaded is THE control plane tool for managing the entire infrastructure, the web the glue between every stack and configuration with observability and enforcement baked in... we will likely even want to do some test deployments of unheaded on larger 10+ node clusters in AWS,GPC, or Azure at some point to really feel things out --- that will require some cheddar from me being spent but i think that should happen before we hit 1.0

  So east and west are treated as two production servers, not a desk lab, and a stage is added after the soak: **Unheaded deployed on a cluster of ten or more nodes in a public cloud before 1.0**, at Stevie's cost and on his word (ADR-143 is the template for it).
- **22. Tools: `iperf3` is installed** on west and east, from the distribution's package. `fio` on the raw devices, `fio` writing in a scratch volume and `stress-ng` were offered and not chosen: the device ceilings stay estimates, and the load is made by the real applications and the repository's own generators.
- **17 (east's services). Rebuild east properly first.** Before any run east gets the trace collector fix, its own Wotan back, each service on its own account with limits, and the role services removed there too. This is S-E1-LIVE Evening 10B, attended.
- **17 (east's heat). After the fix, with a temperature stop.** The fix goes in, east's idle temperature is read, then east is loaded with a stop mark a few degrees under its limit. If it cannot hold 85% under the mark, that figure is recorded as east's ceiling.
- **23. Real load: orion's nginx log replayed, and vor gets an access log.** A recorded day from the public server's log, client addresses removed, is replayed against the blog and site on the cluster. vor starts logging requests so a day of its use can be recorded and replayed. What counts as load on orb was offered (many viewers, found by a ramp) and not chosen: open, a gate in the plan.
- **24, 19, 7 (timeguru). Delete the orphans; bound `FLOW_STATE`; timeguru gets one home.** The old `unheaded-well-data` volume, the three ClickHouse system tables with no retention, three empty service volumes and kanban's stale SQLite fallback go. `FLOW_STATE` becomes an LRU that counts a failed insert; whether anything reads it is settled in the plan. timeguru keeps its data in the Well's table.
- **11 (journal).** Moving the journal to the NVMe and turning off rsyslog's second copy was offered and not chosen: the journal stays where it is.

**All 25 are answered.** Open and carried as gates in the plan: what counts as load on orb; whether the GPU is shared with a tenant; every number for the shares (13), which the project measures.

## Update 2026-11-16: west powers up and down at random

Stevie, 2026-11-16: "do take note that this host west powers up and down randomly - account for that". `journalctl --list-boots` on west: 8 boots from 2026-10-28 to 2026-11-12, some hours apart; the current boot began 2026-11-12 03:40Z.

What it means for this series (each item belongs to the plan named; none is built yet):

| Case | Today | Plan |
|---|---|---|
| After an unplanned power-up the compose stack (the Well, VictoriaMetrics, Prometheus, Alertmanager, alert-mailer, every service) stays down: restart policy is `no` (decision 1) and `kingdom-startup.service` is disabled | Nothing starts it; nothing outside west notices | S-SEE-AND-SURVIVE: the alert path watching itself must be seen from east (a dead man's switch east reads), since west's own monitoring is down with the stack. Whether `kingdom-startup` runs at boot is a question for Stevie that decision 1 does not answer (decision 1 is about crashed containers, not a cold host) |
| `kingdom-shutdown` writes the planned-stop mark only on a clean shutdown | A power loss leaves no mark, so after a manual start the stopped-container rule counts the outage | S-SEE-AND-SURVIVE: the boot path records "unplanned boot" (previous boot did not end cleanly) as its own event and metric |
| Databases and the log store lose power mid-write | Postgres and ClickHouse recover by their own logs; the backup drills prove sets, not crash recovery | S-WEST-GROUND (volume move, fstab, swap) proves a cold boot, not only a clean reboot; S-PROVING-RUN adds a power-cut condition |
| Wotan is in-memory by default | Every power cycle loses its state | S-WASTE-OUT's late-joiner snapshots (critical alerts) already assume this; plan 8 measures it |
| The log store week | A power cycle during it is a disturbance to record, like a walk of `/var` | S-E1-LIVE progress file |
| Work in flight (lanes, background runs) | Lost on power-off; `/tmp` is tmpfs | Commit often on branches; `~/tmp/next.md` kept current |

