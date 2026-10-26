<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# Game day: <name> (<YYYY-MM-DD>)

Copy this file to `docs/gamedays/<YYYY-MM-DD>-<slug>.md`. Fill every section
before the window opens; fill the timeline and results during and after. A
cell with no number from a CSV, a Sword or chaos record, or VictoriaMetrics
stays empty: nothing is estimated.

## Scope

| Field | Value |
|---|---|
| Hosts | |
| Window | <start>–<end> UTC, silence id |
| Workload | |
| Deploy service | Sword `<url>` |
| Chaos controller | `<url>` (west), `<url>` (east) |
| Meter | `steel-echo load --discover ... --rps N --out <csv>` |

## Blast radius (exact)

Instances, units and interfaces chaos may touch. Anything not listed is out
of scope by the allowlist (D-17), not by intent.

| Kind | Name | Host | How it is allowlisted |
|---|---|---|---|
| | | | |

Never in scope: `pihole` (LXD project `default`), physical NICs, `ssh*`,
`systemd-*`, `dbus*`, any `*.target`.

## Abort criteria (D-18, D-20)

Abort through any of the three paths (`POST /api/v1/chaos/abort`, Wotan
`chaos.abort`, `touch /run/unheaded/chaos/ABORT`) when:

- a Kingdom `page: true` SLO fast-burns (`slo:error_ratio:5m` above its
  burn threshold) for a service the scenario did not target;
- the workload's availability over any rolling minute drops below the
  scenario's floor;
- a revert fails verification (`chaos.revert_failed`);
- anything outside the blast radius changes.

## Scenarios

For each: hypothesis (steady state + expected impact), the exact request
bodies, the success criteria with numbers, who watches what.

### Scenario A: <name>

- **Hypothesis**:
- **Inject**:
- **Success**:
- **Watch**:

## Timeline

| UTC | Event | Source |
|---|---|---|
| | | |

## Results

| Scenario | Metric | Value | Source | Pass |
|---|---|---|---|---|
| | | | | |

## Post-state

| Check | Expected | Observed |
|---|---|---|
| `lxc list --project unheaded` | | |
| pihole `Started` | unchanged | |
| `tc qdisc show \| grep -c c4a0` | 0 | |
| chaos journals (west, east) | empty | |
| east `systemctl list-units "unheaded-*"` | = baseline | |

## What went well / what did not

## Action items

Each becomes an ADR-098 register finding or a Kanban card in The Well; the id
goes here. No loose TODOs.

| Item | Where filed | Id |
|---|---|---|
| | | |
