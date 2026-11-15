<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-144: Footprint: measure the stack, then shrink the hungriest parts

**Status:** Proposed (parked). The measurement is done; no code changed.
**Date:** 2026-11-12
**Related:** ADR-012 (eBPF instruction budget), ADR-092 (log pipeline), ADR-117 (one Grafana), ADR-100 (SLOs), ADR-116 (monitor the monitors), ADR-142 (Wotan speed), ADR-112 (plan freeze), `wiki/Profiling.md`, `scripts/check-unit-ceilings.sh`, S-E1-LIVE Evening 16 (the log store week).

## The pitch (Stevie, 2026-11-12, verbatim)

> look into highest cpu usage and highest mem usage items in our stack - we're currently at 5.9GB consumed memory and 10% cpu with trace-collector pinning a core around 90% ---- would like to assess then write up a future ADR for optimizing the hungriest of our application to shrink system footprint (unheaded is meant to be something other applications and services run on for easy monitoring and compliance out of the box)

## How it was measured

On west, 2026-11-12 about 07:10 UTC, three and a half hours after a boot, the whole stack up, Zhen and its model server down (as left), one Claude Code session running. CPU is each cgroup's `cpu.stat` over a 120 s window, as a share of one core. Memory is the cgroup's `memory.current` (which counts page cache the kernel will give back) and, beside it, the anonymous part from `memory.stat` (what the process really holds). The trace collector was then profiled for 10 s with `strace -c` and `perf`. West has 12 cores and 14.3 GiB.

## What it costs today

**The host:** 6.8% busy across 12 cores. `free` says 5.3 GiB used, but anonymous memory is 2.0 GiB for the whole host (0.4 GiB of that was the measuring session itself) and 7.8 GiB is page cache. The 5.9 GiB that was seen is mostly cache: under pressure the kernel returns it. The stack's own anonymous memory is about 1.5 GiB.

**CPU, top of the list (share of one core):**

| Component | CPU | Ours? |
|---|---:|---|
| `unheaded-trace-collector` | 49.9% | ours |
| ClickHouse | 3.8% | third party |
| Wotan | 3.0% | ours |
| dashboard-backend | 2.8% | ours |
| containerd | 1.8% | third party |
| Akira | 1.4% | ours |
| node-exporter | 1.4% | third party |
| everything else, each | under 1% | |

The trace collector is 70% of all the CPU the stack uses.

**Memory, top of the list (MiB):**

| Component | cgroup total | anonymous | Ours? | Note |
|---|---:|---:|---|---|
| docker daemon | 1306 | 145 | third party | the rest is cache from that night's image builds |
| ClickHouse | 1120 | 204 | third party | limit 2 GiB; mostly its own data files in cache |
| Prometheus | 272 | 113 | third party | rule evaluator only since ADR-117 |
| Grafana | 253 | 80 | third party | |
| vor | 238 | 229 | ours | 2271 topics held in memory |
| Pi-hole (LXD) | 169 | 12 | third party | not part of Unheaded |
| containerd | 160 | 111 | third party | |
| dashboard-backend | 157 | 153 | ours | the largest of our Go services |
| Traefik | 111 | 17 | third party | |
| VictoriaMetrics | 100 | 61 | third party | |
| Wotan | 95 | 77 | ours | |
| trace collector | 70 | 14 | ours | |

Not running and so not counted: Zhen's web app and llama-server (the model is a 4.7 GB file loaded onto the GPU; test only).

## The trace collector: found, and it is a bug, not a tuning question

Ten seconds of `strace -c`: **713,032 `bpf()` system calls, about 71,000 a second**, plus 70,000 `nanosleep`. `perf` puts the kernel time in `map_get_next_key`. The collector is walking a BPF hash map one key at a time, without pause.

Which map and why:

- `LATENCY_MAP` (`ebpf/latency-probe`, a plain hash, 65,536 entries at most) held **43,863 entries** on west at the time; `FLOWS` held 6,227.
- **Two readers walk it, each every 200 ms** (both log their start on west): the direct reader in `cmd/trace-collector-go/main.go` and `LatencyReader` in `latency_reader.go`. Each uses `Iterate()`, which is two system calls per entry. 43,863 entries, twice, five times a second is over 800,000 calls a second asked for; the loops cannot keep up, so the process never rests.
- **The map only grows.** The probe only inserts, and the direct reader only reads. `LatencyReader` can delete what it has read, but `DeleteAfterRead` is off by default and the unit does not turn it on; the map grew by about 12,000 entries an hour on west. The CPU grows with the map. VictoriaMetrics shows it: west's collector used 0.08, 0.16, 0.25, 0.33, 0.41, 0.48 of a core at half-hour steps after the 03:40 boot, a straight line; its seven-day peak is 0.94.
- **East is worse:** its collector has averaged **1.56 cores for 20 days** on a 4-core host (25,531 entries at the time, so more than the map walk is in play there, or the map has been through a cycle; not examined further).
- **A second fault rides on it.** The probe's insert ignores failure (`let _ = LATENCY_MAP.insert(...)`). Once the map is full, a new flow gets no entry: latency is then measured only for flows that existed before it filled.

## To settle in this ADR before any plan

1. **Fix the latency path** (the only item here that is a defect). Options, cheapest first, each to be measured before and after:
   - batch reads (`BatchLookup`, one system call per thousands of entries) and a longer sweep (1 s or more): expected to take the collector from about half a core to a few percent;
   - bound the map: an LRU hash, or delete entries that are stale, so it cannot fill and cannot grow without limit;
   - the better shape: the probe pushes one event per completed measurement through a ring buffer and the collector reads only what happened, instead of sweeping everything that exists.
   Whichever is chosen, the probe must count a failed insert instead of dropping it silently.
2. **Look at east before anything else there:** 1.5 cores of 4 for three weeks is the largest single cost in the kingdom.
3. **A target footprint for a small adopter host.** A number to design against, for example: the whole stack idle under 10% of one core and under 1 GiB of anonymous memory on a 2-core, 4 GiB machine. Stevie sets it.
4. **The third-party half of the memory.** ClickHouse, Prometheus, Grafana, VictoriaMetrics, Traefik and the two container daemons hold about 730 MiB anonymous between them and most of the cache. What is optional for a small host, and what the log store week (Step 284) decides about ClickHouse, belongs here; ADR-117 already removed a second Grafana, Loki and a second VictoriaMetrics.
5. **Our own services, in order of size:** vor (229 MiB: does it need every sheet in memory?), dashboard-backend (153 MiB: what the scraper keeps), Wotan (77 MiB; ADR-142 measures it).
6. **A gate, so it stays shrunk.** A per-component CPU and memory budget checked on west after a soak (`check-unit-ceilings.sh` only checks that a ceiling is set, not that it is met), and an alert on a unit that holds more than a set share of a core for an hour: this fault ran for weeks and no alert named it.
7. **How to measure.** The script used here reads cgroup counters and is not in the repository; whether it becomes a tool (`scripts/`), a dashboard panel from the metrics already collected, or both.

## Decision

None. Parked under ADR-112. Item 1 is a defect in running code and can be taken on its own, ahead of the rest, on Stevie's word.

## Update 2026-11-12 (S-WATCH-TRUE)

Stevie, 2026-11-12: fix item 1 (the freeze is lifted for that item only) and record the defect in the findings register. It is **FND-044**. The text above is the record of that morning and is left as written; what follows corrects it and records the decision. Plan: `docs/battle-plans/S-WATCH-TRUE-battle-plan.md`, numbers in `docs/battle-plans/progress/S-WATCH-TRUE-progress.md`.

**Corrections, found while grounding the fix against the code and the live west:**

- **One reader, not two.** `cmd/trace-collector-go/main.go` ran the direct cilium/ebpf sweep **or** the `LatencyReader` fallback, never both. The two start-up lines read above as two readers were both logged by the direct branch. `latency_reader_polls_total` is 0 on west. The cost was one `Iterate()` walk every 200 ms, two `bpf()` calls per entry, with no delete and no metrics of its own.
- **West published no latency events at all.** The installed object `/usr/local/lib/unheaded/ebpf/latency-probe` (2026-10-18) has a 40-byte `LATENCY_MAP` key, the old 5-tuple. The binary (2026-11-10) decodes the 24-byte `LatencyTraceKey` of S-WIRE-HONEST. Every entry failed to decode and was skipped by a bare `continue`; nothing counted it. The S-WIRE-HONEST rule "probe and collector ship together" was broken at deploy. So the latency panels on west were not stale some hours after boot: they were empty.
- **`LATENCY_EVENTS` is written and never read.** The probe reserves a ring buffer record for every sample, and no userspace code opens that ring. It is full, so every reserve fails and `STAT_EVENTS_DROPPED` rises. `LATENCY_STATS` was not exported either, so that was invisible too.
- **`INFLIGHT` and `SOCK_TRACE` also drop on full.** Both are plain hashes whose inserts ignore failure, the same shape as the fault above. Noted, not fixed: parked with the ring buffer reader below.
- **East and west, three days on (2026-11-15 01:22 UTC):** west 0.93 core at 2 d 21 h up, about 108,000 `bpf()` calls a second, the map full at 65,536 entries; east 1.99 cores. East runs a 2026-10-22 binary with a 2026-10-20 object, both from before S-WIRE-HONEST.

**Decision for item 1** (the first two options together, plus the counters; the third stays open):

- `LATENCY_MAP` is an **LRU hash**: a full map evicts the least recently used entry instead of refusing new threads. A refused insert is **counted** (`LATENCY_STATS` key 9, `map_insert_failed`).
- The collector reads the map with **batch lookups** (4,096 entries a call) on `--latency-sweep-interval`, default **2 s**, refused under 500 ms. A kernel without batch lookups is walked as before and says so once.
- The collector **refuses to load** an object whose `LATENCY_MAP` is not the LRU 24-byte-key, 40-byte-value layout: one error at start instead of silence.
- On `/metrics`: `trace_collector_latency_sweep_seconds`, `trace_collector_latency_map_entries`, `trace_collector_latency_decode_errors_total`, and the probe's own counters as `trace_collector_latency_probe_events_total{stat}`.
- The pass bar is live, on west and then east (attended): CPU, `bpf()` rate, decode errors and latency events before and after, in the progress file. FND-044 closes when both hosts meet its `verify` line.

**Parked, with this ADR:** the ring buffer reader (the "better shape": the collector reads `LATENCY_EVENTS` and stops sweeping), which also ends the `events_dropped` count, and bounding `INFLIGHT` and `SOCK_TRACE`. They are the next change to the trace collector, not part of this fix.

**Status:** unchanged, Proposed (parked), for items 2 to 7. Only item 1 moved.
