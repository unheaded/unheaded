<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-151: The log store is a setting, not a dependency

**Status:** Accepted (Stevie Bellis, 2026-11-18; `docs/battle-plans/DECISION-PACK-2026-11-18.md` round 6, row 23): accepted; its own plan after S-LOG-WEEK-2's verdict. L1 to L6 are answered as recommended, see "Stevie's answers, 2026-11-18". Not built: "No code now". Previously: Proposed (parked; decides nothing). No plan, no branch, no code, no host changed. Every choice that is Stevie's is under "To settle" with its options and one recommendation.
**Date:** 2026-11-18
**Related:** ADR-117 (one log store), S-E1-LIVE Evening 16 and Step 284 (the measured week), ADR-147 decision 5 and plan 5 (S-LOG-WEEK-2, the fair second week), ADR-115 (the smallest install), ADR-113 (the SBOM trends down), S-LOG-HARVEST (the log path: Vector, redaction, host identity), `wiki/Host-Settings.md` (values a deployment sets).

## Why this ADR

Stevie closed the log store week on 2026-11-18 (S-E1-LIVE Step 284):

> keep clickhouse for now but create ADR to remove reliance from it --- maybe we have a couple configure-able packages to support the alternatives rather than being fully dependant on it so user could chose whatever solution they want. should be configurable by a configuration file

So ClickHouse stays as west's record, and the question changes from "which store" to "how does a deployment choose one". This ADR writes down what depends on ClickHouse today, a shape for the choice, and what is Stevie's to settle. It builds nothing.

## What the week measured

Seven days, twelve readings (`docs/battle-plans/progress/S-E1-LIVE-progress.md`, "Step 284, prepared for Stevie" and "The closing reading"). West's own journal rows went to both stores.

| Test | ClickHouse | Postgres copy (the Well) |
|---|---|---|
| Disk for the same rows | 72 bytes a row | 1,462 bytes a row (20x) |
| Last hour by host, p95 | 6 to 30 ms | 0.7 to 0.9 ms (faster) |
| Text match over 24 h, p95 | 20 to 42 ms | 4.3 to 16.1 s |
| 7-day count by unit, p95 | 13 to 41 ms | 5.1 to 31.0 s |

The week was not fair to either: ClickHouse took one-second inserts and Postgres ran inside 256 MiB with no index for the two slow queries. Plan 5 repeats it with both fixed, and then adds the two indexes for one day. Its numbers are the first real input to this ADR.

## What depends on ClickHouse today

| Part | Where | What it assumes |
|---|---|---|
| The write path | `config/vector/vector.yaml`, sink `clickhouse` (and since Evening 16 a second sink, `well_logs`, to `cmd/logstore-writer`) | ClickHouse's HTTP insert and its table |
| The schema and retention | `config/clickhouse/schema/` | MergeTree; TTL 90 days, 455 for audit rows |
| The container and its limits | `docker-compose.yml` (`clickhouse`, `clickhouse-schema`) | the image, 2 GiB |
| Queries people and jobs run | `scripts/logstore-measure.sh`, `scripts/pihole-history/`, the `runbooks/security/` and `runbooks/observe/` steps that read logs | ClickHouse SQL |
| Dashboards | Grafana's ClickHouse data source | the plugin and ClickHouse SQL |
| Backup and drill | `scripts/backup/clickhouse-*.sh`, their units | `BACKUP ... TO Disk` |
| Alerts about the store | `monitoring/prometheus/rules/` (three files) | ClickHouse's own metrics |
| Identity | `pkg/uids` (the service account) | one store, one uid |

`cmd/logstore-writer` and migration 048 are the only code that writes log rows anywhere else, and they exist for the measurement.

## A shape for the choice

Three seams, each small, so a deployment picks a store in one file and nothing else names it.

| Seam | Today | Proposed |
|---|---|---|
| **Write**: where Vector sends rows | one sink written by hand | the sink block is generated from the chosen store's driver, like `alertmanager.yml` is generated from `policy.yaml`; a store with no native Vector sink is reached through one small writer (what `logstore-writer` already is for Postgres) |
| **Read**: how a job or a page asks for rows | SQL for one store, written at each call site | a package (`pkg/logstore`) with a few named queries (the last hour by host, a text match over a window, a count by unit over days, rows for one unit) and one driver per store; scripts and runbooks call a small CLI over it instead of a store's own client |
| **Keep**: retention, backup, restore drill, health | per-store scripts | each driver names its own retention setting, backup command and health metric; the units call the driver |

The setting itself: one key in one file, read by the generator and by `pkg/logstore`, for example

```yaml
# /etc/unheaded/logstore.yaml (or a block in an existing deployment file; see "To settle")
store: clickhouse        # clickhouse | postgres | none
retention_days: 90
```

`none` is a real choice for the smallest install (ADR-115): the journal on each host and nothing central.

## To settle (Stevie's)

| # | Question | Options | Recommended |
|---|---|---|---|
| L1 | Which stores get a driver first | (A) ClickHouse and Postgres (the Well), both already measured here, plus `none`; (B) A plus one more log-native store; (C) ClickHouse only, behind the seams, others later | **A.** No new third-party software; a small shop can run with the Well alone if plan 5's numbers allow |
| L2 | A third store, if any | VictoriaLogs (same vendor as the metrics store already here), Loki, OpenSearch. Each is new third-party software and needs Stevie's yes | None until L1 is built and plan 5 is read |
| L3 | Where the setting lives | (A) its own file, `/etc/unheaded/logstore.yaml`, with a default in the repository; (B) a block in an existing deployment file | **A.** One file per choice is how `policy.yaml` works |
| L4 | Do dashboards follow the setting | (A) yes: the log panels are generated per driver; (B) no: Grafana keeps its ClickHouse panels and other stores get the CLI and the dashboard backend only | **B** first; A when a second driver is in use |
| L5 | When | (A) after plan 5's verdict, as its own plan in the series; (B) before plan 5, so the second week is measured through the seams | **A.** The freeze holds, and the second week should measure the stores, not new code |
| L6 | What happens to the measurement path (`logstore-writer`, migration 048) | (A) it becomes the Postgres driver's writer; (B) it is removed with the week and the driver is written fresh | **A** |

## What this does not decide

Which store west runs (ClickHouse, by Step 284, until plan 5 says otherwise), the log path's redaction and host identity rules (S-LOG-HARVEST's hold for every driver), and the metrics store (VictoriaMetrics, ADR-117).

## Stevie's answers, 2026-11-18

From `docs/battle-plans/DECISION-PACK-2026-11-18.md`, round 6, row 23: L1 to L6 as recommended, "Accept all six. No code now." The text above is the record as proposed and is not edited.

| # | Question | Answer, in this ADR's own options |
|---|---|---|
| L1 | Which stores get a driver first | **A**: ClickHouse and Postgres (the Well), plus `none` |
| L2 | A third store, if any | **None yet**: none until L1 is built and plan 5 is read. A third store would be new third-party software and would need his yes |
| L3 | Where the setting lives | **A**: its own file, `/etc/unheaded/logstore.yaml`, with a default in the repository |
| L4 | Do dashboards follow the setting | **B first**: Grafana keeps its ClickHouse panels; other stores get the CLI and the dashboard backend only. A when a second driver is in use |
| L5 | When | **A**: after plan 5's verdict, as its own plan in the series |
| L6 | What happens to the measurement path (`logstore-writer`, migration 048) | **A**: it becomes the Postgres driver's writer |

State after this answer: accepted; its own plan after S-LOG-WEEK-2's verdict. Nothing is built and no plan is written. "What this does not decide" still holds: ClickHouse stays west's log store until plan 5 (S-LOG-WEEK-2) says otherwise.

## Notes 2026-11-21 (decision pack 2026-11-18, row 41)

Settled by Claude under row 41, reviewable there. The text above is not edited.

- **The `cmd/logstore-writer/main.go` row in `docs/policy/http-auth-baseline.txt` (M-207.2).** Accepted as it stands until this ADR's driver work: the writer checks its own static bearer token (`LOGSTORE_INGEST_TOKEN`, 32 bytes or more, compared in constant time) and not `pkg/auth`, on a link that refuses plaintext. A condition for the plan that builds L6 (the writer becomes the Postgres driver's writer): that work moves its ingest to service tokens (`pkg/auth`) and deletes the baseline row in the same change. Until then the row stays, and `scripts/check-http-auth.sh` keeps it honest. Undo: strike this note; the row is not touched either way.
