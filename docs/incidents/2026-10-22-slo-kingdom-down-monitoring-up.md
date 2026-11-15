<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# 2026-10-22: five SLOs read an outage for three days while west's kingdom stack was down and monitoring stayed up

Blameless postmortem (ADR-100 section 5), written for S-EVIDENCE-RUNS Step 41 (sweep row E-05).

**Status:** draft (written 2026-11-15 from Prometheus history and the commit log; not yet reviewed by Stevie)
**Services:** on west: Wotan, The Well, dashboard-backend, Vector (journal shipping), the Well backup chain

## Summary

From 2026-10-22 about 14:00 UTC the monitoring stack on west ran while the services it watches did not. Wotan and The
Well read down until about 2026-10-24 03:30, dashboard-backend and Vector until Prometheus itself stopped at about
2026-10-25 20:00. Those samples are what spent five error budgets: they sit inside the 28-day window until
2026-11-22, and the daily readings since 2026-11-10 are good.

## Impact

- **Who/what:** the five SLOs `dashboard`, `wotan`, `well`, `well-backups` and `log-shipping` (register rows
  `slo:*`, UH-SLO-01). No user other than Stevie; west is a lab host.
- **How long:** Wotan and The Well about 37 h (2026-10-22 14:00 to 2026-10-24 03:30). dashboard-backend on west and
  journal shipping about 78 h (to 2026-10-25 20:00, where the samples end; the next ones, 2026-10-27, are good).
- **Data:** none known. Journal lines of the window were not shipped to ClickHouse while Vector was down.

## Timeline (UTC)

Read from Prometheus on west (`avg_over_time(slo:good[6h])`, 6-hour buckets, one sample a minute).

| Time (UTC) | Event |
|---|---|
| 2026-10-22 ~03:00 to ~13:00 | no samples at all: Prometheus was down too |
| 2026-10-22 14:01 | commit `7f6b1101`: kingdom-shutdown's straggler sweep now leaves the monitoring stack running |
| 2026-10-22 14:07 | first bad bucket: `dashboard` 0.50, `wotan` 0.50, `well` 0, `log-shipping` 0 |
| 2026-10-23 02:07 | `log-shipping` reads 1.0 for one bucket, then 0 again |
| 2026-10-24 02:07 | `wotan` 0.60 and `well` 0.74: both recover inside this bucket |
| 2026-10-24 08:07 | `well-backups` 0.73: a backup succeeds again |
| 2026-10-25 20:07 | last bucket with `dashboard` 0.50 and `log-shipping` 0; samples end here |
| 2026-10-27 02:07 | samples resume; `dashboard`, `wotan`, `well`, `log-shipping` read 1.0 |
| 2026-11-15 | detected, by reading the SLO history bucket by bucket (this postmortem) |

On 2026-10-24 12:00 the targets read: `up{job="dashboard-backend"}` 0 on west and 1 on east; `up{job="wotan"}` 1 on
both; `up{job="vector"}` 0; `vector_component_sent_events_total` absent.

## Detection

Nothing noticed it at the time. The burn-rate alerts existed (ADR-100) but the mailer was in dry-run, and a stop on
purpose looks the same to them as an outage. It was found three weeks later, when the register's six `slo:*` rows
were read against their samples instead of waiting for the window to pass.

The plan's own reading (Steps 38-39) looks in VictoriaMetrics, which holds none of this window (9,598 samples of
`slo:good` in 28 days against Prometheus's 14,671). That is why the daily readings looked clean while the budgets
read -1,655 % to -3,405 %: the budget rule runs in Prometheus, on Prometheus's samples, and its arithmetic is right
(`dashboard`: 1 - (1 - 0.8248) / 0.005 = -34.05).

## Root cause

1. **The services were down while the evaluator was up.** Before `7f6b1101`, a kingdom shutdown also stopped the
   monitoring stack, so a stop left no samples and cost no budget (`avg_over_time` skips absent samples). From that
   commit on, monitoring survives a shutdown by design (S14e: something must watch the next boot), and every
   `slo:good` rule ends in `or on() vector(0)`, so a stopped service is a bad sample.
2. **Why the stack was down on 2026-10-22, and why west's dashboard-backend and Vector stayed down 41 h longer than
   Wotan and The Well, is not recoverable.** West's journal starts at the boot of 2026-10-26 17:46; nothing earlier
   is kept. The commit log of 2026-10-22 and 2026-10-23 shows a shutdown and a reboot being worked on, and no
   incident.
3. **`well-backups` has two more causes in the same week.** On 2026-10-21 the newest backup was 37.9 h old at
   12:00 (limit 26 h): a nightly run had been missed before the stop. On 2026-10-27 02:07 the three backup
   timestamps were absent after the reboot, which the rule reads as 0.

## What went well

Prometheus kept the samples (30-day retention), so the window could be read three weeks later. The budget rule
itself needed no change.

## Action items

| Action | Tracked as | State |
|---|---|---|
| Re-read the five budgets after the window has passed (2026-11-25, 28 days after the last large bad bucket of 2026-10-27) | register rows `slo:dashboard`, `slo:wotan`, `slo:well`, `slo:well-backups`, `slo:log-shipping` (dated line, 2026-11-15) | open |
| A stop on purpose spends error budget whenever monitoring is left up: decide whether a planned stop is marked (a silence the budget rule honours) or counted | for Stevie: S-EVIDENCE-RUNS progress file, "For Stevie" | open, no owner yet |
| VictoriaMetrics lacks 2026-10-22 14:00 to 2026-10-25 20:00 of `slo:good`; readings of SLO history go to Prometheus until the windows match | S-EVIDENCE-RUNS progress file, Step 38 note | noted |

## Threshold check

Measured as an outage of Wotan, the dashboard and The Well longer than 1 h, and a silent one longer than a day.
Whether it was an outage or a stop on purpose that monitoring was newly able to see, the retained evidence cannot
say; the postmortem exists so the six register rows stop saying "re-read later" without a cause.
