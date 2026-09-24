<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-094 — Own the metrics stack: retire `prometheus/client_golang`

**Status:** Proposed (Tier 1 done; Tier 2 migration planned, not started)
**Date:** 2026-09-24
**Supersedes:** nothing. **Related:** ADR-092 (log discipline), ADR-093 (shape
and the reachability rules), `pkg/metrics`, `pkg/logagg`.

## Context

Three different mechanisms produce `/metrics` in this tree. Stevie's call, and
the reason this ADR exists: the goal is not tidiness, it is **owning the full
stack and carrying no third-party library we do not need**.

| tier | how it produces the text | services |
|---|---|---|
| 1. hand-written | `fmt.Sprintf` / `Fprintf` per line | monad, sophia, kanban-app, cuirass (+ doom-bridge, routing-health) |
| 2. `prometheus/client_golang` | `promauto` + `promhttp.Handler()` | wotan, micromanager, architect, trace-collector-go, zhen-agentd |
| 3. `pkg/metrics` (ours) | `Registry.Gather` | dashboard-backend, gateway |

### Tier 1 was not just untidy — it was wrong

Hand-assembling the exposition format produced a real defect. **Four of
sophia's eight series carried no `# TYPE` line at all**, because the `Sprintf`
declared HELP/TYPE for the first metric in a group and appended the rest as
bare sample lines. Measured on the live endpoint: 7 types declared, 4 samples
undeclared, so a scraper treated four series as untyped. `pkg/logagg`'s first
collector had the mirror-image bug, emitting `# HELP` *twice*.

That is the class argument: a format typed out by hand will eventually be
typed out wrong, and nothing notices, because a malformed series still scrapes.

## Decision

### Part 1 — retire Tier 1 (DONE for the running services)

`pkg/metrics` gained `FuncMetric` (`NewFuncCounter` / `NewFuncGauge`): a metric
whose value is read from a function at scrape time. Services here already hold
counters as plain fields, so exposing them should not mean rewriting every
increment site.

monad, sophia, kanban-app and cuirass now register func-backed metrics and
serve `Registry.Gather`. Verified live per service: series names identical,
sophia's 4 undeclared samples gone, zero duplicate HELP/TYPE.

**`cmd/doom-bridge` and `cmd/routing-health` remain on Tier 1.** Both are
well-formed (HELP count equals TYPE count) and neither runs — TOOL and ORPHAN
in `docs/LIVE-PATHS.md` — so a conversion could not be verified live. Left
deliberately rather than changed blind (ADR-093 rule 2).

### Part 2 — migrate Tier 2 into `pkg/metrics` (planned)

This is the supply-chain half. What the dependency actually costs:

**Modules carried solely for the metrics client** (`go list -deps`):

```
github.com/prometheus/client_golang     (direct)
github.com/prometheus/client_model      (indirect)
github.com/prometheus/common            (indirect)
github.com/prometheus/procfs            (indirect)
github.com/beorn7/perks                 (indirect)
github.com/cespare/xxhash               (indirect)
github.com/matttproud/golang_protobuf_extensions/v2
google.golang.org/protobuf
golang.org/x/sys
```

Nine modules, of which `protobuf` is pulled in only to serve an exposition
format we do not use. **31 files import the client; 5 of those are tests.**

**What `pkg/metrics` already has:** Counter, Gauge, Histogram, Summary, their
`Vec` forms, `Registry` with `Gather`/`Handler`/`Push`, `ProcessCollector`
(cpu_seconds, open_fds, max_fds, virtual/resident memory, start_time),
`FuncMetric`, hooks and samplers.

**The one real gap: `go_*` runtime series.** `promhttp` on the default
registry publishes 33 of them on wotan today — `go_goroutines`,
`go_memstats_*`, `go_gc_duration_seconds`, `go_info`, `go_threads`. `pkg/metrics`
has no runtime collector. These are not decoration: `go_goroutines` is how the
B8 dashboard goroutine leak was diagnosed, and `go_memstats_heap_alloc_bytes`
is how the scraper OOM was traced. **Migrating without replacing them would
remove the instrument that found this repository's two worst production bugs.**

So the ordering is forced:

1. **`metrics.NewGoCollector()`** — `runtime.ReadMemStats` + `runtime/metrics`
   into the `go_*` names Prometheus publishes, same names and units, so
   existing dashboards and alerts keep working. This is the prerequisite and
   the only new code of substance.
2. **Verify it against the real thing.** Run promhttp and ours side by side in
   one process and diff the `go_*` series names and values. A name or unit
   that differs silently breaks a dashboard, and a dashboard nobody checks is
   an alert that does not fire.
3. **Provide the missing convenience:** a `promauto`-shaped constructor set so
   call sites change by import path, not by restructuring.
4. **Migrate leaf packages first** — `pkg/tracing`, `pkg/logagg`,
   `pkg/wotan-client` (8 files) — each verified by scraping a service that
   uses it before and after.
5. **Then the services**, heaviest last: zhen-agentd (5 files),
   trace-collector-go (6), micromanager, architect, and wotan last because it
   publishes the most.
6. **Drop the modules from `go.mod`** and add a gate: no tracked `.go` file may
   import `prometheus/client_golang`. Ratchet by file set, not count — the
   lesson `check-secrets-baseline.sh` already records.

### What this does NOT propose

Replacing the *protocol*. We keep emitting Prometheus text format v0.0.4 and
stay scrapeable by Prometheus, VictoriaMetrics and Grafana Agent. Owning the
stack means owning the *implementation*, not inventing a wire format — the
same distinction ADR-088 draws for deployment substrates.

## Consequences

**Good.** Nine modules leave the dependency graph, including protobuf carried
for a format we never emit. One metrics API across every service. The
malformed-exposition class is gone by construction. `pkg/logagg` drops from
three adapters to one.

**Costs.** A Go runtime collector is real work and it is the kind that is easy
to get subtly wrong — a unit error in `go_memstats_*` is invisible until
someone reads a graph during an incident. 26 production files change. And for
a while the tree carries both, which is worse than either.

**The honest risk.** `client_golang` is maintained by the Prometheus project
and is about as low-risk as a third-party dependency gets. This migration is
justified by the ownership goal, not by a security finding — worth stating so
nobody later reads it as a response to a CVE that never existed.

**Not started.** Part 1 is done and verified. Part 2 is this plan. Step 1 (the
Go collector) is the whole prerequisite; nothing else should begin until it is
diffed against promhttp and matches.
