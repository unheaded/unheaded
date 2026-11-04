<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-116: Monitor the monitors: an expected signal that goes missing is an alert

**Status:** Proposed (pitch, parked under ADR-112). Emphasis: **high**. Brainstorm before acceptance; nothing is built.
**Date:** 2026-10-31
**Grounded on:** `churn/2026-10-30` at `a21593f5`, plus live checks on west and east 2026-10-31
**Related:**
- ADR-100 (SRE alongside compliance: Watchdog, heartbeat)
- ADR-092 (log pipeline)
- ADR-098 (findings register)
- ADR-113 (daily SBOM checks)
- ADR-115 (shop floor F9)
- S-LOG-HARVEST (logcov, `LogSourceSilent`)
- `scripts/check-gates-can-fail.sh` (the same idea for CI gates)

## The pitch (Stevie, 2026-10-31, verbatim)

> adr update/creatation/emphasis - monitors and monitoring must be monitored! we need to know when we don't see something we expect to see!

## Context: what went unseen this week

Each item below was found by a person (or by Claude reading state), never by an alert.

| What stopped | How long nobody was told | Found by |
|---|---|---|
| **The whole monitoring stack** (Prometheus, Alertmanager, alert-mailer, Grafana): 0 containers | since the 2026-10-28 reboot | reading `docker ps` (FND-001) |
| **Well backups** (`unheaded-well-backup.service` failed on west; `unheaded-well-offsite-check.service` failed on east) | since the stack went down | `systemctl --failed` |
| **Lich nightly `fuzz_mbc_execute`** | red every night since 2026-10-28 | reading the GitHub Actions API |
| **All LAN DNS through Pi-hole**: about 50,000 queries a day went to 0 | **since 2026-08-22, over two months** | IOC sweep, 2026-10-30 |
| **East's dead-man's-switch sender**: `unheaded-heartbeat.timer` on east is `inactive` | unknown | live check, 2026-10-31 |
| **Heimdall hook events dropped silently** when the ring buffer filled | until LICH-012 L12d (fixed 2026-10-30) | red-team campaign |
| **`sbom/SBOM.md`** last generated 2026-03-15; `references/timeline.md` 2026-10-28; wiki mirror unsynced 10-23 → 10-30 | weeks to months | reading files |

**The pattern:** each failure was *silence*, and the system treats silence as "fine".

## What exists (ADR-100 and since)

- **`Watchdog` (`rules/watchdog.yml`):** always firing, urgency none. Its own comment says it is the proof *"on Alertmanager's page … If Watchdog is missing there, the alerting path is broken"*. That proof only works **if a human looks at the page**. Nothing notices its absence.
- **Dead-man's switch (ADR-100 §4):** east POSTs `/heartbeat/east` to alert-mailer's 19131 listener on west every 2 min. `HeartbeatMissing` pages after 10 min. But:
  - alert-mailer runs **inside the monitoring compose stack on west**, so when that stack is down, the receiver is down too;
  - only east is watched, and **nothing watches west**;
  - east's sender is not active.
- **Absence rules:** some exist (`absent(…)` / `time() - …` in compliance, heartbeat, slo, threat-feeds, well, zhen) and `logs.yml` has `JournalNotShipping`, `LogSilence`, `VectorDown`. All of them are evaluated by the Prometheus that went down.
- **Rule tests:** 50 alert names, 44 of them covered by `promtool` tests. Tests prove a rule fires on synthetic data, not that a page reaches a person.
- **CI gates:** `check-gates-can-fail.sh` already enforces "a gate that cannot fail is worse than none". Monitoring has no equivalent.

## Decision direction (Claude's recommendations, springboard)

### R1. One principle, stated everywhere

**Every expected signal has a stated expectation, and missing it is an alert.** A signal is a heartbeat, a scrape, a log line, a timer run, a backup, a CI run, a report, a DNS query volume or a document refresh. "No data" is never "OK". Applies to Kingdom and Shop (ADR-115 F9) alike.

### R2. An expectations registry, gated in CI

`config/monitoring/expectations.yaml` (or a schema in the Well) lists each expected signal:
- source;
- cadence;
- maximum silence;
- the minimum plausible volume where volume matters;
- what its absence means, in plain words;
- owner;
- runbook.

**CI gate (same spirit as S-LOG-HARVEST's `check-log-sources.sh`):** fails when any of these has no expectation:
- a systemd timer or unit;
- a scheduled workflow;
- a Prometheus scrape job;
- a log source;
- a backup job;
- a generated document.

### R3. Absence is judged by something that does not depend on what it watches

Layered, so no single failure silences everything:

1. **In-stack:** Prometheus absence and staleness rules (`absent_over_time`, `time() - last_seen`, `up == 0`) for everything the stack scrapes.
2. **Out of stack, on each host:** a small first-party **warden**, a systemd unit with its own uid, outside compose, stdlib Go. Every minute it reads:
   - `systemctl show` (`Result`, `LastTriggerUSec`, `NRestarts`) for every expected unit and timer;
   - journald's last-entry time per expected source;
   - the mtimes of expected files (backups, reports);
   - local `/health` endpoints.

   Its findings go to the local log and to the *other* host.
3. **Mutual heartbeats:** west watches east **and** east watches west, warden to warden. Neither depends on the compose stack. A heartbeat carries a digest of the sender's findings ("47 expected signals, 3 missing"), not only "alive".
4. **Out-of-band last resort:** if both hosts' monitoring is down, someone must still hear. Options:
   - a native path: east mails Stevie directly over a different SMTP route when west is silent;
   - an outside check-in service (third party, for the sitting);
   - a scheduled CI job that expects a daily signed report and fails publicly when it is missing.

   That is open question 1.

### R4. Prove that alerts arrive, not only that rules fire

- **End-to-end canary, daily:** Watchdog (or a dedicated `CanaryDaily`) must not just fire. Its *delivery* must be recorded: alert-mailer writes a receipt and the warden checks a receipt exists in the last 24 h. No receipt is itself an alert, raised through the out-of-band path.
- **`check-monitors-can-fail` (test, CI):**
  - every alert rule needs a `promtool` test that fires it (6 of 50 are missing today);
  - every expectation in R2 needs a provocation test, like `check-gates-can-fail.sh`;
  - a monitor that cannot fail fails the build.
- **Game day:** periodically stop a monitored thing on purpose (Yaldabaoth, inside the chaos allowlist) and measure time to alert.

### R5. Volume drops count, not just zero

Pi-hole went from about 50k LAN queries a day to 0 and nothing noticed. Zero is the easy case. Rules should also catch "fell more than 90 % below its 7-day median" for sources whose volume matters:
- DNS queries per client class;
- log lines per source;
- requests per service;
- backup size.

They should also catch the reverse (sudden spikes), but absence is the priority here.

### R6. Freshness of artifacts is monitored too

Generated or maintained artifacts get a freshness expectation:
- the SBOM and inventory: daily (ADR-113);
- the timeline;
- the wiki mirror sync;
- the CBOM;
- the compliance evidence;
- the findings register.

`check-timeline-freshness.sh` already does this for one file. Generalize it.

### R7. The monitoring path monitors itself

Each of these is an expectation in R2:
- Prometheus rule-evaluation failures and scrape `up == 0`;
- Alertmanager `notifications_failed_total`;
- alert-mailer outbox depth and SMTP failures;
- Vector buffer and sink errors (already alerted, `logs.yml`);
- Heimdall `LOST` counters (L12d);
- the warden's own heartbeat.

### R8. A positive daily signal for the human

One plain-language daily line: *"Checked 47 expected signals. All present."* or the list of what is missing.
- The human learns the line arrives every day, so **its absence is itself noticed**: the human-level dead-man's switch.
- It is the same daily line ADR-113 (daily SBOM) and ADR-115 (shop report) want, so one daily report, not three.

### R9. Audit and logging

Under the ADR-113 R8 rules:
- every missing-signal finding, delivery receipt and heartbeat gap is recorded, with first-seen and cleared times;
- each finding goes to the findings register (ADR-098) when it lasts past its window;
- retention is the audit class (455 days).

### R10. No new third party

The warden, the registry, the gates and the canary are first-party (stdlib Go, systemd, journald, Prometheus already present). Only R3's out-of-band channel might be outside. That is for the sitting.

## Open questions for the brainstorm

1. **Out-of-band last resort (R3.4):** native (east mails directly), an outside check-in service (third party), a CI job expecting a daily signed report, or several?
2. Move alert-mailer and the heartbeat receiver **out of the compose stack** into host units, so the receiver outlives the stack? Or let the warden take over the heartbeat role?
3. Who is told, and how: email only, or a phone push for the few absence alerts that page?
4. Default windows (e.g. heartbeat 10 min, timers 2× their period, daily jobs 26 h, volume drop 90 % vs 7-day median): accept as defaults and tune per signal?
5. Should the warden absorb S-LOG-HARVEST's logcov prober and ADR-113's daily collectors (one host agent), or stay separate small units?
6. Install east's heartbeat sender now (it is committed but inactive)? A small change, but under ADR-112 it waits for Stevie's word.

## Decisions 2026-11-03 (Stevie, rapid-fire sitting)

Status stays Proposed and parked under ADR-112; these answers close the open questions so the work can be sequenced when the freeze lifts.

1. **Out-of-band last resort: all three.** Native (east mails directly) + a CI job that expects a daily signed report + an outside check-in service. The outside service is a third party and goes through the third-party sitting first.
2. **Host units** for alert-mailer and the heartbeat receiver, out of the compose stack, so the receiver outlives the stack.
3. **Email + phone push** for the absence alerts that page. Self-hosted push first; a third-party push service only via the sitting.
4. **Accept the default windows**, tune per signal.
5. **Separate small units.** The warden watches; logcov and the collectors keep their own timers and uids. Developer readout given at the sitting: none of warden, logcov or the collectors exist as code yet; what exists is alert-mailer, the `unheaded-heartbeat` timer (inactive on east), huginn, healthprobe, unheaded-baseline and unheaded-remediate.
6. **Install east's heartbeat sender: yes** (`deploy/systemd/unheaded-heartbeat.{service,timer}`).

## Decision

None yet. Parked under ADR-112, marked high emphasis.

## Decisions 2026-11-04 (decision pack, round 3)

- DP-710: outbound alert mail gets its own expectation: send-only, no listening relay, a per-hour cap, every send logged, a volume alert.
