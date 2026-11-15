<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->
# S-WATCH-TRUE progress

Plan: `docs/battle-plans/S-WATCH-TRUE-battle-plan.md` (12 phases, 226 steps).

## Decisions

Answered by Stevie 2026-11-12 (copied from the plan's table).

| # | Question | Ruling | Status |
|---|---|---|---|
| A-22 | The trace collector's cost (ADR-144 item 1) | **Fix it** (freeze lifted for that item) and record it in the findings register | Answered 2026-11-12 |
| A-18 | Wotan topic refusals | **Add the alert and its runbook** | Answered 2026-11-12 |
| A-17 | Host events for the IOC audit | **(A) Vector counters by kind** | Answered 2026-11-12 |
| W1 | `sword` is scraped every 10 s and is down except during a deploy. No rule, SLO or panel reads `sword_*`. | **Remove the job** from `config/victoria/scrape.yml`. Sword's code and runbooks stay. | Answered 2026-11-12 |
| W2 | heimdall: two alert rules and two scrape jobs for a daemon no host runs | **Remove** the `heimdall` rule group, its tests and both scrape jobs (Prometheus and VictoriaMetrics). Keep heimdall's code and runbooks. A rule returns with its producer and a test (decision 25). | Answered 2026-11-12 |
| W3 | `docker-compose.override.yml` on west (gitignored, merged into every compose run) | **Record only**: list what it overrides, key names only, in the progress file and "For Stevie". Nothing is changed. | Answered 2026-11-12 |
| W4 | `bootstrap-llm-lab.sh` `:latest` x3 and `lxd/telemetry` (Grafana 10.2.2 tarball, no checksum, pre-ADR-117) | **Script**: pin the two the stack still uses to compose's exact tags; drop the Loki pull. **`lxd/telemetry/`: delete outright** (git history keeps it), with `lxd/Makefile`'s `launch-telemetry` target, its `.PHONY` entry and help lines, and every live reference in docs and wiki. Dated records are not edited. | Answered 2026-11-12 |
| W5 | ioc-audit judgment for the new kinds | **Indicators**: an account change, a crontab edit, an unsigned or out-of-tree module load. **Notes**: sudo command counts per host and user, kernel OOM kills and segfaults. Tunable by parameter. | W5 default, Stevie away (2026-11-15) |

## Before (2026-11-15 01:22Z, west up 2 d 21 h; trace collector started 2026-11-12 03:43:54Z)

| Reading | West | East |
|---|---|---|
| CPU, 30 min rate (VictoriaMetrics) | 0.93 core | 1.99 cores |
| CPU, 7-day mean (two series per host) | 0.70 / 0.83 | 1.88 / 1.95 |
| `bpf()` calls, 10 s under strace | 1,079,646 (about 108,000/s; 10 errors) | not read (attended, Phase 4B) |
| `LATENCY_MAP` | key 40 B, value 40 B, **65,536 of 65,536 entries (full)** | not read |
| Latency events published | 0 (`latency_reader_*` all 0; `trace_collector_flows_correlated_total` 0) | not read |

Tool for the `bpf()` reading: `strace -c -f -e trace=bpf` (present, attach allowed).
The reference reading of 2026-11-12 22:50Z was 0.81 core on west at 19 h up and about 71,000 `bpf()`/s. Three days up, the cost has grown and the map is full.

## Log

| Date | Steps | Note |
|---|---|---|
| 2026-11-15 | 1-16 | Phase 0. Worktree `sprint/s-watch-true` off develop `b5959085`. Tools all present. "Before" reading taken (table above). |

## For Stevie

Nothing yet.
