<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-094 — Own the metrics stack: retire `prometheus/client_golang`

**Status:** Proposed (Tier 1 done; Tier 2 steps 1-3 done; step 4 next)
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
`Vec` forms, `Registry` with `Gather`/`Handler`/`Push`, `FuncMetric`.

**Correction (2026-09-24):** this paragraph originally listed
`ProcessCollector` as present. It is not. The type exists and its `Write` is a
placeholder that returns nil — it builds six `process_*` series and emits
none, and nothing in the tree calls `NewProcessCollector`. `promhttp` publishes
7 `process_*` series by default (`cpu_seconds_total`, `open_fds`, `max_fds`,
`virtual_memory_bytes`, `virtual_memory_max_bytes`, `resident_memory_bytes`,
`start_time_seconds`). The list also named "hooks and samplers"; there are
none in the package. So there are two gaps, not one; see step 1b.

**The first gap: `go_*` runtime series.** `promhttp` on the default
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

### Step 1 — DONE (2026-09-24)

`pkg/metrics/gocollector.go`: `NewGoCollector()` publishes the 27 `go_*`
families client_golang v1.18 registers by default, with identical names,
types and help text. Memory figures use client_golang's own derivation from
`runtime/metrics` (no stop-the-world `ReadMemStats`), extracted as the pure
function `deriveMemstats`. One runtime reading is shared by every family in a
scrape, so `heap_sys == heap_inuse + heap_idle` holds within a scrape.

**How it was verified against the real thing.** Two independent reads of a
live runtime never agree, and a tolerance wide enough to absorb that absorbs
real errors — a 10% band on `last_gc_time_seconds` accepts a timestamp five
years off. So the parity test pauses the GC and reads ours, then
client_golang, then ours again; client_golang's value must fall inside that
bracket. Measured over 300 runs, every series stayed inside with zero slack
except four with a stated, measured reason (goroutines, threads, the
`gc_sys`/`other_sys` class trade, and `next_gc`, which counts stack space).
500/500 green.

**Watched it fail.** Eleven planted bugs, each caught:

| planted bug | caught by |
|---|---|
| `stack_sys` drops os-stacks | `DeriveMemstats_SumsTheRightClasses` only |
| `mspan_sys` / `mcache_sys` drop the free class | both |
| `last_gc` in nanoseconds | parity |
| GC pause sum in nanoseconds | parity |
| `frees` drops tiny allocs | both |
| `heap_idle` drops released | both |
| `gc_sys` reads the wrong class | both |
| pause quantiles shifted by one | parity |
| a runtime/metrics key renamed by a Go upgrade | `EveryRuntimeKeyExists` |
| snapshot cache disabled (mixed readings in one scrape) | `OneReadingPerScrape` |

The first row is why `deriveMemstats` is a pure function. os-stacks reads 0
on a cgo-free Linux binary, so dropping it is invisible at runtime and the
parity test passed it. `pkg/afxdp` uses cgo, where it need not be 0. The
class-bit test gives each class its own bit, so a dropped or wrong class is
visible no matter what this host holds.

`TestGoCollector_PublishesTheClientGolangNames` pins the 27 names without
importing client_golang, so it outlives the parity test at step 6.

### Step 1b — DONE (2026-09-24)

`pkg/metrics/processcollector*.go`: `NewProcessCollector("")` publishes the 7
`process_*` families promhttp does, same names/types/help, Linux only
(elsewhere it publishes nothing rather than something invented). The
placeholder is gone; nothing had called it. A failed read omits the sample
rather than publishing 0 — a CPU counter reading 0 is a reset to `rate()`.

Verified with the same bracket (300/300, zero slack except float rounding on
start time) and ten planted bugs, all caught. The bracket alone missed four:
a test process has used ~0 CPU ticks, so `utime` vs `utime+stime` and ticks
vs seconds read identically, and soft == hard rlimits on this host hide a
`Cur`/`Max` swap. Hence `deriveProcess` is pure and tested with values that
tell the fields apart. `comm` is parsed from the last `)`, tested with a
process named `evil) (x y`. The pre-6.2 fd-count fallback never runs on this
kernel, so it is tested directly against the fast path.

Step 2 ("verify against the real thing") is satisfied for both collectors
by the parity tests.

### Found while surveying step 3 — two core defects (FIXED 2026-09-24)

Step 3 would move ~30 files onto `pkg/metrics`' Vec and Histogram types. Both
have defects that client_golang rules out, reproduced 2026-09-24:

1. **No label validation.** `CounterVec.WithLabels(Labels{"cdoe": "200"})`
   on a vec declared with `code` publishes `c_total{cdoe="200"}`, and
   `WithLabels(Labels{})` publishes a bare `c_total`. client_golang panics on
   a mismatched label set. Same for `GaugeVec` and `HistogramVec`. The one
   external caller today is `cmd/dashboard-backend` (host gauges).
2. **Duplicate histogram buckets.** `Buckets: {1, 1, +Inf}` emits
   `le="1"` twice and `le="+Inf"` twice. Duplicate series are invalid
   exposition and Prometheus rejects the whole scrape. client_golang panics
   on non-increasing buckets and strips a trailing `+Inf`. No caller in the
   tree passes `+Inf` today.

Fix both, each with a planted-bug check, before step 3 adds a
`WithLabelValues(...)` shim. The survey found 109 `.WithLabelValues(`
calls, which are positional, so the shim has to map values onto the
declared label names, and that mapping is only as safe as the validation
underneath it.

**Fixed.** Every Vec now panics on a label set that does not exactly match
the declared names, as client_golang does. The check runs only when a series
is first created, so the hot path is unchanged. Histogram buckets are
sorted, a trailing `+Inf` is dropped (Write always emits it), and duplicate
or NaN bounds panic. `le` and `quantile` are refused as user labels. Each Vec
gained `WithLabelValues(...)`, positional in declaration order, which is the
first half of step 3. Fifteen planted bugs, all caught; the one mutant that
did not compile was rewritten until it did, and then it was caught too.

Before tightening, the only external caller was checked:
`cmd/dashboard-backend` passes exactly `{"host"}` and `{"host","mount"}`,
matching its declarations. **Side finding there:** `dashboard_http_requests_total`
and `dashboard_http_request_duration_seconds` are declared and registered,
but nothing increments or observes them, so `/metrics` carries two empty
families. That is ADR-093's defect class. Recorded, not fixed here.

**The parity test has a rare flake.** It failed once in about 720
full-package runs on 2026-09-24. It did not reproduce in 20 reruns of the
same conditions, or in 200 further full-package runs. The bracket bounds a
paused-GC read of a live runtime, and something occasionally escapes it. It
was not chased; if it recurs, capture which series escaped before widening
anything.

### Step 3 — DONE (2026-09-24)

`pkg/metrics/prom` presents pkg/metrics under client_golang's names
(`CounterOpts`, `NewCounterVec`, `DefBuckets`, `BuildFQName`, `Registerer`,
`MustRegister`, `Handler`, …), and `pkg/metrics/auto` is promauto
(`auto.NewCounter`, `auto.With(reg)`). A file migrates by import path. The
only visible difference: client_golang's `Counter`/`Gauge`/`Histogram` are
interfaces, and here they are pointer aliases. Nothing in the tree
type-asserts on them.

It covers exactly what the survey found in use. The survey's four
`.Delete(` hits turned out to be BPF maps and caches, not metrics, so no
`Delete` was added.

Two core changes came with it, both for parity with what promhttp serves:
- `DefaultRegistry` now carries the `go_*` and `process_*` collectors, as
  client_golang's default does. That changes no output today, because
  nothing serves it (see below), and it means a service moving off
  `promhttp.Handler()` keeps `go_goroutines`.
- `Registry.Gather` omits families with no samples, as client_golang does.
  Before, an untouched Vec published a bare HELP/TYPE.

Verified by driving the same declarations and operations through
client_golang and the shim and comparing served output exactly, with label
order canonicalised (client_golang writes `le` last). 30+ series, nothing
missing, nothing extra. Thirteen planted bugs, all caught.

**Findings from the survey, recorded not fixed:**
- **`pkg/secrets` registers 12 metrics into `metrics.DefaultRegistry`, which
  nothing serves.** No `metrics.Handler()` call exists outside the package,
  so rotation and secret-operation counters have never reached a scrape. The
  registration errors are also discarded (`_ = metrics.Register(...)`).
  Serving the default registry from the services that use `pkg/secrets`
  fixes it.
- **`cmd/ebpf-exporter` uses `err.Error()` as a label value**
  (`main.go:241`, `:270`). Every distinct error message creates a new series
  that never goes away, so memory grows without bound. Fix when that file
  migrates in step 5.
