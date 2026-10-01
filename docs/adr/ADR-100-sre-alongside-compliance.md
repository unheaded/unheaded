<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->

# ADR-100: SRE alongside compliance (SLOs, error budgets, actionable alerts), starting with log-volume deviation

**Status:** Proposed. The idea is recorded here; no work has started (Stevie 2026-10-01: "ADR that idea, don't work it now").
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

- **Blameless postmortems** in `docs/postmortems/YYYY-MM-DD-<slug>.md`, from a template: impact, timeline, root cause, what went well, action items.
  - Each action item becomes a register finding or a Kanban card in The Well, so it cannot be dropped.
  - The three incidents in Context are the first three postmortems.
- **Error-budget policy:** when a service's budget is exhausted, reliability work on it outranks feature work until the budget recovers. This is Stevie's call for the lab, and the default is written down so the decision is visible.
- **Toil** (manual, repetitive, automatable work, such as hand-applied remediation or re-signing commits) is noted when it happens. The remediation executor (ADR-099) and this ADR exist partly to retire it.

## Phases (none started)

| Phase | Scope |
|---|---|
| 1 | Write the three postmortems above. Add the `runbook_url` gate and a runbook for each of the 22 existing rules. Add the Watchdog alert. |
| 2 | SLO definition schema (Well table plus YAML fallback), a generator for burn-rate rules, a drift test, and the `slo` evidence kind. |
| 3 | Log-volume SLI from the log pipeline. Spam and silence SLOs for postgres, dashboard-backend, kanban and wotan first. Auth-failure SLIs. |
| 4 | Availability SLOs for The Well, Wotan, the dashboard, and CI on develop. |
| 5 | Error-budget panel on the dashboard next to Compliance; budgets as register findings. |

## Questions for Stevie (when this is picked up)

1. SLO targets for the lab: start at 99% over 28 days and tighten from data, or something else?
2. Error-budget policy: does an exhausted budget freeze feature work on that service, or only raise it on the Kanban board?
3. Postmortem threshold: every alert that reached email, or only incidents with impact (data loss, a service down more than 1 h, a silent failure longer than a day)?

## Consequences

- Operations and compliance stop being two backlogs. An SLO, an alert, a runbook and a postmortem are each an operational tool *and* a piece of audit evidence, mapped once in the crosswalk.
- Failures like a database down for a day or CI red for ten days alert someone within the SLO window instead of being found by accident.
- Cost: every service has to declare SLOs, and every alert needs a runbook and a firing test. That is deliberate friction against alerts nobody can act on.
