<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-093 — What shape is this thing we build?

**Status:** Proposed
**Date:** 2026-09-23
**Supersedes:** nothing. **Related:** ADR-080 (UPC as a general MBC substrate),
ADR-088 (deployment substrate ladder), ADR-089 (promotion workflow),
ADR-091 (The Well initdb ordering), ADR-062 (fuzz/red-team framework),
`CLAUDE.md` (mission statement), `docs/battle-plans/STAGING-PROMOTION-LOG.md`.

## Context

Stevie asked the question directly, after a month of promotion churn: *what
shape is this thing we build?*

It is worth answering in an ADR rather than in a chat reply because the shape
turns out to **explain a defect class** that has now been hit eight separate
times, and because the measured shape does not match the shape the repository
describes of itself.

Every number below is counted from the tree, not recalled.

### What is actually here

| | count | how counted |
|---|---|---|
| Go files | 1,248 | `git ls-files "*.go"` |
| Rust files | 197 | `git ls-files "*.rs"` |
| Shell / Nix / Python / JS | 164 / 92 / 70 / 41 | same |
| binaries (`cmd/` + `services/*/cmd/`) | 49 | `docs/LIVE-PATHS.md` |
| …built into the container image | 10 | same |
| …referenced by nothing outside their own directory | 14 | same |
| `services/` directories | 34 | `ls services` |
| Rust crates | 9 | `ls crates` |
| ADRs | 91 (this one included) | `ls docs/adr/*.md` |
| Protocol drafts | 46 | `ls docs/protocol/*.md` |
| Services in the default stack | 17 | `docker compose config --services` |
| …of which Unheaded-authored | 10 | the rest are postgres, grafana, clickhouse, vector, victoria, traefik, coredns |

Commits by top-level directory, last six months:

| directory | commits |
|---|---|
| `crates/` | 1,023 |
| `pkg/` | 931 |
| `docs/` | 664 |
| `cmd/` | 630 |
| `services/` | 320 |

### Three observations the numbers force

**1. The centre of gravity is not where the mission statement points.**
`CLAUDE.md` opens with "configuration management automation platform —
customer brings their app, we provide everything else." The busiest directory
in the repository is `crates/` — the UPC, MBC, xv6, Doom, the forge. Not the
platform. The platform services (`services/`) take a third of the commits the
compute work does.

This is not a criticism. It is a statement that the artefact and the elevator
pitch have diverged, and that anyone reasoning from the pitch alone will
mispredict where the work is.

**2. Documentation is a first-class artefact, not overhead.** 794 tracked
files under `docs/`, 91 ADRs, 46 protocol drafts, against 1,248 Go files. The
specs are not notes about the system; for Monad, Sophia and Wotan they are
the normative definition and the code is one implementation of them.

**3. Most of what is built is not running.** 49 binaries exist across `cmd/`
and `services/*/cmd/`. 10 ship in the container image, 10 are referenced by a
systemd unit / Nix module / K8s manifest, 15 are invoked by a script or
runbook, and 14 are referenced by nothing outside their own directory. The
default stack runs 10 Unheaded services beside 7 third-party ones.

> These counts were hand-grepped when this ADR was first written, and two of
> them were wrong: "44 binaries" missed the five that live under
> `services/*/cmd/` and ship in the image, and "18 unreferenced" came from a
> cruder sweep than the one `docs/LIVE-PATHS.md` now performs. Corrected
> against the generated inventory, which is the point of having one.

## Decision

Name the shape explicitly, in three parts, and accept what follows from it.

### The shape: a protocol, three faucets, and a workshop

**The protocol is the centre.** Monad (a 20-byte register in an IPv6
hop-by-hop header), Sophia (BPF-map dictionaries giving those bytes meaning),
Wotan (the memory model and the message bus). 46 drafts. Everything else is
downstream of a wire format. This is the part that is genuinely novel and the
part that is specified rather than merely implemented.

**Three faucets run off it**, in Stevie's own plumbing framing:

- **A computer.** The UPC: MBC bytecode, `rv2mbc`, the eBPF interpreter, xv6
  booting to an interactive shell, Doom. `crates/`, and the busiest area in
  the repo. ADR-080 already calls this a general multi-workload substrate.
- **A platform.** The Go services that do configuration management and
  observability — wotan, timeguru, captain, architect, micromanager, monad,
  sophia, dashboard-backend, kanban-app, cuirass. This is what the mission
  statement describes and what the compose stack runs.
- **A substrate ladder.** Compose, systemd, NixOS, Ansible, Terraform, K8s,
  `.deb` — ADR-088's additive deployment targets. Not a product; a set of
  ways to land the other two.

**Around all of it, a workshop.** `tomb/` (fuzzing and red-team), `eval/`,
`references/`, the 18 unreferenced binaries, the doom harnesses, the forge.
These are instruments, not deliverables. A workshop is the honest word for a
place where most of the tools on the bench are not in the thing being built.

This is also a **solo passion project** (one person and an AI assistant), not
an organisation, and the shape reflects that: breadth over headcount,
specification over coordination, many experiments kept because keeping them
costs nothing.

### The load-bearing consequence: liveness is the scarce resource

If 10 of 49 binaries are containerised and 14 are referenced by nothing at
all, then **most code in this repository is never exercised end to end.**
That is the defining structural property, and it has a direct and now
well-evidenced consequence:

> In this shape, the default failure is not a wrong implementation.
> It is a correct implementation that nothing reaches.

Eight instances found so far, all of them mechanisms that *were* correct and
*were* unreachable:

| # | mechanism | why it never fired |
|---|---|---|
| 1 | `003_app_schema.sql`'s `kanban_tasks` | initdb routing meant nothing ever executed it; two tables shared one name |
| 2 | `check-timeline-freshness.sh` | bare invocation defaulted to `--report`, which always exits 0 — green for three batches while CI's guard was red |
| 3 | B2–B6 gate loops | passed because nothing reached their assertion |
| 4 | `logagg.Publisher` | cancelled its context before the publish goroutine ran, and had never joined its topics — no service had *ever* forwarded a log line |
| 5 | Wotan's `config.*` signature check | lived behind `if verifier != nil`, and the constructor `cmd/wotan` uses never set it |
| 6 | logagg Prometheus counters | registered in a registry that nine of ten services do not serve |
| 7 | LICH-008 / LICH-010 | fuzzed reimplementations of their targets; Go's `internal` rule made the real ones unreachable from `tomb/` |
| 8 | trace-collector's config precedence fix | landed in `runUnifiedMode`, while the service starts in anamnesis mode by default |

Every one compiled. Most had tests. Several had been "working" for months.

### What follows, as rules

1. **A gate is not a gate until it has been observed failing.** Provoke it
   red, then restore. `check-timeline-freshness.sh` is the cautionary case:
   three batches reported PASS from a mode that could not fail.
2. **Verify against the live path, not the diff.** Reading the change is not
   evidence it runs. The trace-collector fix compiled, tested green, and did
   nothing because the service has two entry points.
3. **"Has a function" is not "is reached."** LICH-008 had `Fuzz*` functions
   for months and could not have found anything. Before trusting any
   mechanism, ask what executes it.
4. **Prefer the check that reads the repository over the one that reads the
   host.** `check-compose-log-caps.sh` inspects the compose file on purpose:
   inspecting running containers would have passed on this machine, by
   reading an untracked `/etc/docker/daemon.json` no contributor has.
5. **Record reachability in the inventory.** ADR-062's campaign table now
   carries a placement rule for exactly this reason. Component lists should
   say what runs, not only what exists. **Done:** `docs/LIVE-PATHS.md`,
   generated by `scripts/live-path-inventory.sh` and gated in CI, classifies
   every binary CONTAINER / SUPERVISED / TOOL / ORPHAN. A new binary cannot
   land without being classified.

## Consequences

**Good.** The shape is now something that can be reasoned about instead of
inferred. The recurring defect class has a name and a stated cause rather
than being rediscovered each time with fresh surprise. Newcomers — including
future sessions of this assistant — can be told "most of this does not run,
check what reaches your change" as a first-class instruction.

**Costs.** Naming the workshop as a workshop invites the question of whether
the 18 unreferenced binaries should be deleted. This ADR does not propose
that: in a solo learning project, kept experiments are the point, and ADR-090
(the Total Source Sweep) already owns that question. But the count should be
tracked, so that "we keep experiments" stays a decision and does not decay
into "nobody knows what these are."

**Unresolved, and deliberately left open:**

- **The pitch and the artefact have diverged.** Either `CLAUDE.md`'s mission
  statement should acknowledge the compute work as a first-class faucet
  rather than a curiosity, or the compute work should be described as the
  research track that feeds the platform. Stevie's call; this ADR only
  records that the two currently disagree.
- **Three metrics conventions** coexist (`pkg/metrics` registry, promhttp
  default registry, hand-rolled exposition), which is why `pkg/logagg` ships
  three adapters. Converging them would delete two and remove a class of
  "registered where nothing serves" bug.

## Notes

The question was asked in four words and deserves a four-word answer, so:
**a protocol with faucets.** Everything above is the evidence for that
sentence and the consequences of taking it seriously.
