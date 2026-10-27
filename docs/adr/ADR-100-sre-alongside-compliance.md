<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-100: SRE alongside compliance (SLOs, error budgets, actionable alerts), starting with log-volume deviation

**Status:** Accepted 2026-10-20; Phases 1 (partly) and 2 built, see Implementation. Proposed 2026-10-01 (Stevie: "ADR that idea, don't work it now").
**Date:** 2026-10-01
**Related:**

- ADR-097 (crosswalk: controls and evidence)
- ADR-098 (findings register; audit and enforcing modes)
- ADR-099 (remediation as code; The Well is the source of truth, YAML is the fallback)
- ADR-086 (Muninn fan-out)
- ADR-092 (log discipline and rotation budgets)
- ADR-094 (own the metrics stack)
- ADR-029 (Akira consensus health)
- `docs/battle-plans/COMPLIANCE-WELL-BATTLE-PLAN.md`

## Context

Stevie, 2026-10-01:

- "We should alert on log spam deviations SLO."
- "I want SRE best practices and mentality baked in alongside the compliance aspect."

Three failures from the last ten days. Each was silent, and each is exactly what SRE practice is designed to catch:

| What happened | How long nobody knew | What would have caught it |
|---|---|---|
| A burst of `password authentication failed for user "unheaded"` in The Well's log (2026-09-30 02:21Z). The likely cause is `pkg/database` defaulting to the superuser with a wrong password. | Found by accident on 2026-10-01 | A log-volume deviation alert on postgres error lines, and an auth-failure rate SLI |
| The Well (`unheaded-postgres`) exited with code 255 after the 2026-09-30 reboot. Kanban fell back to SQLite. | About 21 h | An availability SLO on The Well, and an alert on the fallback (COMPLIANCE-WELL Phase 3) |
| Static Analysis (clippy) was red on develop because the runners lack protoc. | Since at least 2026-09-21 | A "CI red on the default branch for more than 24 h" alert |

What exists today:

- 22 Prometheus alert rules, none of which carry a `runbook_url`.
- Alertmanager, Loki and promtail in `monitoring/`.
- ADR-092 per-service rotation budgets. These *cap* log damage, but nothing *notices* a deviation.
- No SLOs, no error budgets, no postmortem record.

Compliance frameworks ask for the same things SRE practice produces:

| SRE practice | Framework requirements it serves |
|---|---|
| Monitoring and anomaly alerts | NIST 800-53 SI-4, AU-6; SOC 2 CC7.2; ISO 27001 A.8.16 |
| Availability objectives | SOC 2 A1.1/A1.2; CP family |
| Incident response and postmortems | IR-4, IR-5; CC7.3–CC7.5; A.5.24–A.5.27 |
| Capacity | CC7.2, SC-5, AU-4 (log storage capacity) |

Built side by side, each SRE artifact is also evidence. That is the overlapping-controls idea from ADR-097, applied to operations.

## Decision

### 1. SLOs are data, declared once, and evidence for compliance

- Every service declares its SLIs and SLOs: what is measured, the objective, the window, the error-budget policy.
- **Storage** follows the ADR-099 rule. The Well (`unheaded_config.slo_definitions`) is the source of truth. `observability/slo/<service>.yaml` is the seed and the fallback.
- **Alert rules** are generated from the definitions (as `baseline.yaml` is), with a drift test. They are not hand-written.
- **Each SLO is an evidence source in the crosswalk** (new kind `slo`). It passes when the budget has not run out over the window. A blown budget is a finding in the ADR-098 register, triaged like any other.

### 2. Alert on burn rate, not on static thresholds

- Use multi-window, multi-burn-rate alerts (Google SRE Workbook, ch. 5). Each alert has two tiers, and each tier needs a long and a short window to agree:
  - **Fast:** 1 h / 5 m at 14.4× the budget rate.
  - **Slow:** 6 h / 30 m at 6×.
- A spike that clears on its own does not page. A slow leak still does.
- **Routing:** email only, for now (Stevie, ADR-098 answer 1). Page-versus-ticket severity is defined now, so that PagerDuty later is a routing change, not a redesign.

### 3. Log-volume deviation SLO (the first instance)

- **SLI:** per service and level, lines per minute and bytes per minute, measured by the log pipeline (Muninn/promtail into VictoriaMetrics).
  - Lines are counted, never stored for this purpose.
  - This preserves zero user data access: volume is metadata, not content.
- **Deviation** means two things:
  1. **Spam:** the current rate exceeds the service's own baseline (the same hour over the last 7 days, median) by a ratio set per service. The ratio starts at 5× for `error`/`warn` and 10× overall, and is tuned from data.
  2. **Silence:** a service that normally logs goes quiet. Silence is how a dead process looks. Absence is a deviation too.
- **The SLO:** each service is inside its volume envelope 99% of minutes over 28 days. Burn-rate alerts come from section 2.
- **Why an SLO and not a plain threshold:** log volume differs by orders of magnitude between services, and a fixed number would be wrong for nearly all of them. A per-service baseline plus a budget catches the auth-failure burst above without crying wolf on Zhen's normal chatter.
- **Security-relevant patterns get their own SLIs on top of volume.** Examples: authentication failures per minute per service, sudo failures, sshd invalid users. These also serve AU-6 (audit review) and feed Sentinel.
- **ADR-092's rotation budgets stay as the backstop.** The SLO notices; rotation contains the damage.

### 4. Every alert is actionable, and proven able to fire

- Each rule carries a `runbook_url` that points at a `runbooks/` entry: what it means, the first three checks, how to silence it safely. A gate fails the build on a rule without one.
- Each rule has a promtool test that makes it fire. This is the ADR-093 / `check-gates-can-fail.sh` principle applied to alerts: an alert never seen firing is a wish.
- A **dead-man's switch**: an always-firing `Watchdog` alert. When it stops arriving, the alerting path itself is broken.
- Alert on symptoms users and operators feel, such as "The Well unavailable" or "compliance served from YAML fallback". Causes go on dashboards.

### 5. Postmortems, toil and budget policy

- **Blameless postmortems** in `docs/incidents/YYYY-MM-DD-<slug>.md`, from `docs/incidents/TEMPLATE.md`: summary, impact, timeline, detection, root cause, what went well, action items, threshold check.
  - Each action item becomes a register finding, an `M-###` row or a Kanban card in The Well, so it cannot be dropped.
  - The three incidents in Context are the first three postmortems, plus Wotan's silent OOM outage (2026-10-18): `2026-09-21-ci-clippy-red-on-develop.md`, `2026-09-30-well-auth-failure-burst.md`, `2026-09-30-well-exit-silent-outage.md`, `2026-10-18-wotan-oom-silent-outage.md` (written 2026-10-27, drafts).
- **Error-budget policy:** when a service's budget is exhausted, reliability work on it outranks feature work until the budget recovers. This is Stevie's call for the lab, and the default is written down so the decision is visible.
- **Toil** (manual, repetitive, automatable work, such as hand-applied remediation or re-signing commits) is noted when it happens. The remediation executor (ADR-099) and this ADR exist partly to retire it.

## Phases (status in the dated sections below)

| Phase | Scope |
|---|---|
| 1 | Write the three postmortems above. Add the `runbook_url` gate and a runbook for each of the 22 existing rules. Add the Watchdog alert. |
| 2 | SLO definition schema (Well table plus YAML fallback), a generator for burn-rate rules, a drift test, and the `slo` evidence kind. Built 2026-10-27 (S-WELL-SPRING Phase 3, below). |
| 3 | Log-volume SLI from the log pipeline. Spam and silence SLOs for postgres, dashboard-backend, kanban and wotan first. Auth-failure SLIs. |
| 4 | Availability SLOs for The Well, Wotan, the dashboard, and CI on develop. |
| 5 | Error-budget panel on the dashboard next to Compliance; budgets as register findings. |

## Questions for Stevie (when this is picked up)

1. SLO targets for the lab: start at 99% over 28 days and tighten from data, or something else?
2. Error-budget policy: does an exhausted budget freeze feature work on that service, or only raise it on the Kanban board?
3. Postmortem threshold: every alert that reached email, or only incidents with impact (data loss, a service down more than 1 h, a silent failure longer than a day)?

## Decisions (Stevie, 2026-10-20)

- "Think about Google's SRE mentality with alerting and monitoring: deviation in SLO and SLA should be the only alerts, no noise. We should have a way to fine tune and control all this and attach labels regarding urgency."
- "I'm not going to read 500 alerts, I might read 5 if something is worth checking out."
- "I also don't want to blow up my inbox with self-created spam or false positives, or get on Gmail's naughty list and have my domain blacklisted."

These answer the questions above:

1. **Targets:** 99.5% over 28 days for what Stevie uses directly (Wotan, the dashboard, The Well); 99% for the rest. Tighten from data.
2. **Budget policy:** not decided. Exhausted budgets show on Alertmanager/Grafana; nothing is frozen.
3. **Postmortems:** not decided. Default until Stevie decides (S-WELL-SPRING Step 156, decision 23): a postmortem for any outage of a Stevie-facing service (Wotan, the dashboard, The Well, Zhen) longer than 1 h, any silent failure longer than a day, and any data-loss near miss; never for a single page that resolved inside the hour.

## Implementation (2026-10-20)

| Piece | Where |
|---|---|
| Policy, the one file to tune: SLOs, mailer cap/digest/mode, promotions | `observability/alerting/policy.yaml` |
| Generator and checks (stale files, hand rules not `urgency: none`, policy refusals) | `cmd/alert-policy`, CI step in security.yml |
| SLO rules: SLIs, 7 error-ratio windows, budget remaining, 4 burn-rate tiers per SLO | `monitoring/prometheus/rules/slo.yml` (generated), `tests/slo_test.yml` |
| Routing by `urgency` (page / ticket / none) | `monitoring/alertmanager/alertmanager.yml` (generated, no secrets) |
| The only sender: daily cap, ticket digest, two mails per incident, dry-run, outbox | `cmd/alert-mailer`, `docker/alert-mailer/Dockerfile`, compose service `alert-mailer` |
| Planned-work silences | `scripts/monitoring/maintenance.sh` |
| Runbook for every notifying alert | `runbooks/observe/slo-error-budget-burn.yaml`; since 2026-10-27 every alert, gated by `go run ./cmd/alert-policy -runbooks` in `scripts/check-prometheus-rules.sh` (S-WELL-SPRING Steps 172-178) |
| End-to-end proof | `scripts/monitoring/test-alert-path.sh` (meta-gate provoked) |
| Operator page | `wiki/Alerting.md` |

Changes from the text above:

- **SLIs are time-based** ("good this minute"), not request ratios. Lab traffic is too thin for request ratios, and a dead service sends no requests, so its ratio would look fine.
- **Every hand-written rule is `urgency: none`.** The rules still evaluate and show in Alertmanager and Grafana; they never notify. A cause alert can be promoted in the policy, with a reason.
- **Alertmanager sends no mail itself.** alert-mailer does, so the cap and the digest hold no matter how Alertmanager groups.
- **No Watchdog yet.** A dead-man's switch needs a receiver outside this host (a heartbeat service). Mailing one daily would be exactly the noise this ADR removes. Built 2026-10-27 (S-WELL-SPRING Phase 6), see below.
- **Log-volume deviation (Phase 3):** built 2026-10-25 for the journal, see below. `log-shipping` covers a host delivering nothing.

## Phase 3, 2026-10-25: log-volume deviation and auth-failure SLIs (journal)

| Piece | Where |
|---|---|
| SLIs: `unheaded_log_lines_total` and `unheaded_log_bytes_total` {host, unit, level}; `unheaded_auth_failures_total` {host, unit, kind=ssh/sudo/pam} | `config/vector/vector.yaml` (`journal_volume*`, `journal_auth*`), exported on Vector's :9598 |
| SLI tests: no text in the metric, transient units collapse, each failure pattern matches, an accepted login does not | `config/vector/tests/volume.yaml`, `scripts/test-vector-config.sh` (security.yml) |
| Baseline: median of the unit's own 10m rate at this time of day on each of the last 7 days, needing 4 of them | `monitoring/prometheus/rules/logs.yml` group `log-volume` |
| `LogSpam` (over 10x and at least 30 lines/min), `LogErrorSpam` (warning or worse, over 5x and at least 10/min), `LogSilence` (usually at least 1 line/min, nothing for 10m while the host still ships), `AuthFailureBurst` (20 in 10m) | same file; promtool tests in `tests/logs_test.yml`, each provoked red by a mutation (ratio, 4-day minimum, floor, host guard) |

Lines are counted, never stored for this; the metric holds no message text.
All four alerts are `urgency: none`, like every hand-written rule: they show
in Prometheus, Alertmanager and Grafana and mail no one. Not done:

- **The SLO** ("inside the envelope 99% of minutes over 28 days") in
  `policy.yaml`. That makes it mail, so it waits for a week of data to tune
  the ratios and floors against, as this ADR says.
- **postgres, dashboard-backend, kanban and wotan**, the first services this
  ADR named: they log to container stdout, which does not reach the journal
  (ADR-092 step 8, a host decision). Their volume appears here once it does.
- The Well's own auth failures (the 2026-09-30 incident) are in postgres's
  container log, so the same.

Deploying is a Vector restart (`docker compose up -d vector`); nothing
here restarts it.

Live on west in `mode: dry-run` from 2026-10-20. All 7 SLIs good, nothing firing. Next: a week of outbox review, then the Gmail app password and `mode: send` (wiki/Alerting.md).

## Watchdog and heartbeat, 2026-10-27 (S-WELL-SPRING Phase 6)

Placement (decision 22 default, ADR-098 Q2): west and east watch each other;
a receiver outside both hosts later. West dying is noticed by Stevie until
then; that is the recorded gap.

| Piece | Where |
|---|---|
| `Watchdog`: `vector(1)`, urgency none, always firing; missing from Alertmanager means the alerting path is broken | `monitoring/prometheus/rules/watchdog.yml`, `tests/watchdog_test.yml` |
| Heartbeat receiver: `POST /heartbeat/{host}` with `X-Heartbeat-Token`, on its own listener (19131) so the other host cannot reach the webhooks or the outbox; gauge `unheaded_heartbeat_last_seen_timestamp_seconds{host}`; off unless `HEARTBEAT_HOSTS` is set | `cmd/alert-mailer/heartbeat.go`, compose `HEARTBEAT_*` |
| `HeartbeatMissing` (10 min quiet): promoted to page in the policy, the only alert that pages on an absence | `monitoring/prometheus/rules/heartbeat.yml`, `policy.yaml` `promote:` |
| East sender every 2 min as `unheaded-heartbeat` (uid 16711), token through `LoadCredential` | `deploy/systemd/unheaded-heartbeat.{service,timer}`, `scripts/monitoring/heartbeat.sh` |
| Runbook | `runbooks/observe/heartbeat-missing.yaml` (draft) |

Live after the E-D host window: token on both hosts, `HEARTBEAT_HOSTS=east`
and `HEARTBEAT_LISTEN=192.168.13.2` in `monitoring/.env`, the east timer.

## CI SLO and error-budget panel, 2026-10-27 (S-WELL-SPRING Step 179)

| Piece | Where |
|---|---|
| `ci-develop` SLO (99 %, tickets): every workflow's latest finished run on develop green, data under 2 h old | `observability/alerting/policy.yaml`; tests in `tests/slo_test.yml` |
| SLI writer: public Actions API every 10 min into huginn's textfile dir, as `unheaded-ci-status` (uid 16712) | `scripts/monitoring/ci-status-textfile.sh`, `deploy/systemd/unheaded-ci-status.{service,timer}` |
| Per-SLO runbook (`runbook:` in the policy) | `cmd/alert-policy`; `runbooks/observe/ci-red.yaml` |
| Error-budget dashboard (Phase 5's panel, in Grafana rather than the dashboard UI) | `monitoring/grafana/dashboards/08_slo_error_budget.json` |

Live after the timer is installed on west (E-D window), before Prometheus
loads the new rules: until the first file lands the SLI has no data, which
counts as bad.

## Consequences

- Operations and compliance stop being two backlogs. An SLO, an alert, a runbook and a postmortem are each an operational tool *and* a piece of audit evidence, mapped once in the crosswalk.
- Failures like a database down for a day or CI red for ten days alert someone within the SLO window instead of being found by accident.
- Cost: every service has to declare SLOs, and every alert needs a runbook and a firing test. That is deliberate friction against alerts nobody can act on.

## SLOs as data, 2026-10-27 (S-WELL-SPRING Phase 3)

`policy.yaml` stays the reviewable source: an objective changes by a reviewed
commit. The Well holds the same definitions as rows, which is what the
dashboard and the compliance evidence read.

| Piece | Where |
|---|---|
| `slo_definitions` (objective as a fraction, `slo_window`, time-based SLI, urgency, runbook URL, the policy file's sha256) and an append-only `slo_definitions_history` written by trigger (old/new row, session user, `well.change_reason`) | `db/migrations/040_ops_slo_definitions.sql` (unheaded_ops; ops_writer writes, ops_reader reads) |
| Store: `All`, `Sync` (one transaction: upsert, delete SLOs that left the YAML, refuse an empty set; a row changes only when its definition does) | `pkg/database/slo_store.go` |
| `-sync-well` writes the policy into the Well; `-check` with `WELL_DB` set also fails on drift (`drift: slo wotan objective well=0.99 yaml=0.995`, rows missing either side) and fails closed when the Well cannot be read; without `WELL_DB` it is the YAML-only check CI runs | `cmd/alert-policy/well.go` |
| Proof on a throwaway server (CI `well-schema` job; `scripts/ci-local.sh` adds the live drift leg when `WELL_DB` is set) | `scripts/well/test-slo-definitions.sh` |
| Evidence kind `slo` (ref = SLO name): PASS while `slo:error_budget_remaining` > 0 in VictoriaMetrics (`VICTORIA_URL`, default `http://localhost:8428`), FAIL at or below 0, no record (NOT_ASSESSED) when the series is missing | `pkg/compliance/crosswalk/collect/slo.go`, `cmd/compliance-evidence` |
| Control `UH-SLO-01`, one source per SLO (a test fails when the catalog and the policy name different SLOs) | `compliance/catalog/controls.yaml` |

The policy now refuses an objective with more than two decimals (the Well
stores numeric(6,4)). Live state: 040 waits for the E-A migration window;
then `-sync-well` once and `-check` with `WELL_DB` set. Read on
2026-10-27, UH-SLO-01 is FAIL: five of the seven measured budgets are spent
(wotan, dashboard, well, well-backups, log-shipping); overlay and kev-feed
pass; ci-develop has no series until its timer runs. Editing objectives from
the dashboard admin with a signed change record (ADR-099 section 5) waits
for S-IRON-GATE's board login; not built.
