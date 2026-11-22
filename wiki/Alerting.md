# Alerting

Only an SLO burning its error budget notifies anyone (ADR-100). Everything else is recorded, visible in Prometheus, Alertmanager and Grafana, and never sent. The aim is a handful of mails on a bad day and none on a good one.

## The one file

`observability/alerting/policy.yaml` decides what reaches a human:

- **SLOs**: each has a name, an objective (99.5% or 99% over 28 days), whether it can page, and a `good` expression that is 1 when the service is fine this minute. A missing series counts as bad.
- **mailer**: `mode` (`dry-run` or `send`), `delivery` (`digest` or `immediate`), `immediate` (alerts mailed at once whatever `delivery` says), `max_per_day` (hard cap, 6), `max_per_hour` (3), `run_days` (raised caps on listed dates), `digest_at` (09:00) and `timezone` (America/Chicago).
- **boot**: `state_max_age` (5m), for the digest's line about a boot of the host ([A boot of the host](#a-boot-of-the-host)).
- **page** timing, and **promote**: cause alerts that should notify anyway. Keep it short; each entry needs a reason. Today: `HeartbeatMissing` (page), `TraceCollectorDown` and `SSHLoginNewSource` (ticket).

After an edit: `go run ./cmd/alert-policy` regenerates `monitoring/prometheus/rules/slo.yml` and `monitoring/alertmanager/alertmanager.yml`. CI fails if they are stale, or if a hand-written rule is not `urgency: none`. Then restart Prometheus and Alertmanager: `cd monitoring && sudo docker compose up -d --force-recreate prometheus alertmanager alert-mailer`.

**Prometheus keeps what its rules read (ADR-147 decisions 9, 10).** Every scrape job in `monitoring/prometheus/prometheus.yml` but `prometheus` and `alert-mailer` carries a `metric_relabel_configs` keep generated from the rules (`monitoring/prometheus/rule-series.txt`, 42 series on 2026-11-15); 30 days of those, nothing else. After adding a rule that reads a new series: `python3 scripts/gen-prom-keep-list.py`. `scripts/check-prometheus-rules.sh` runs `--check` first and fails until you do; a reader of Prometheus' own API that needs a series no rule reads lists it in `monitoring/prometheus/keep-extra.txt`. `up` and `scrape_*` are made by Prometheus and always kept.

## Urgency

Every alert carries `urgency`:

| urgency | what happens |
|---|---|
| `page` | Mailed within about 2 minutes. One mail per incident, plus one when everything is clear, plus a reminder after 12 h if it is still burning. |
| `ticket` | Collected, then one digest mail at 09:00, only if something is open, resolved since the last digest, or held back by the cap, or the host booted and the digest has not said so yet. |
| `none` | Nobody. All 44 hand-written rules in `monitoring/prometheus/rules/` (disk, logs, log-volume spam/silence, compliance, Well backups, KEV, trace collector, AlertPipelineHeartbeat, security detections in `detection.yml`: SSH login from a new source, SSH auth-failure bursts, outbound volume, …), except the three the policy promotes: `HeartbeatMissing` (page), `TraceCollectorDown` and `SSHLoginNewSource` (ticket). DGA, beaconing and DNS tunnelling have no rule: there is no DNS query log source yet (threat register DET-01). |

`severity` (warning/critical) still describes how bad a cause is. It no longer decides who hears about it.

### One mail a day (`delivery: digest`)

The table above is `delivery: immediate`. Since 2026-11-17 the policy says `delivery: digest` (Stevie: "cap it to 1 digest email per day ... it's a lab"), and the mailer sends the 09:00 digest and nothing else:

| What happens | Under `delivery: digest` |
|---|---|
| A page fires | Not mailed. `/v1/outbox` shows it at once with outcome `digest`; it is listed first in the next digest, under "Urgent", and the subject counts it: `[digest] 3 open (1 urgent)`. |
| A page resolves | One line under "Resolved since the last digest". |
| The alert path goes silent (`pipeline.silent_after`) | Not mailed. The next digest opens with it and goes out for it even with nothing else open; its return is one resolved line. |
| A ticket | As before. |
| The host boots (`unheaded-boot-event.service`) | Not mailed, under either delivery. The next digest carries one plain line for the boot, after the silent alert path's line if there is one, and goes out for it even with nothing else open: `west booted 2026-11-21 01:44Z; stack down; previous shutdown: not clean`. One boot is reported once. See [A boot of the host](#a-boot-of-the-host). |
| A day with nothing open, resolved or held back, and no boot to report | No mail. |

To tune it, edit the `mailer:` block and restart the mailer (no regeneration: the generated files do not read these keys):

| Want | Edit |
|---|---|
| Pages by mail again | `delivery: immediate` |
| One alert mailed at once, the rest in the digest (an intrusion or an indicator of compromise from an IDS or IPS) | Promote it with `urgency: page`, then add its name to `immediate:`. `go run ./cmd/alert-policy --check` refuses a name that is not a promoted page. |
| More or fewer mails on a day when alerts are mailed at once | `max_per_day`, `max_per_hour`; for single dates, `run_days` |
| Another hour for the digest | `digest_at` |

`curl -s localhost:19130/v1/status` shows the delivery in force.

## SLOs

| SLO | Good when | Objective | Pages |
|---|---|---|---|
| wotan | Wotan `up` on west and east | 99.5% | yes |
| dashboard | dashboard-backend `up` on west and east | 99.5% | yes |
| well | every Well database answers (`huginn_pg_up`) | 99.5% | yes |
| well-backups | backup and offsite under 26 h old, restore drill under 8 days | 99% | ticket |
| overlay | west scrapes east's node_exporter over wg0 | 99% | ticket |
| log-shipping | Vector delivered journal lines to ClickHouse in the last 30 min | 99% | ticket |
| kev-feed | the CISA KEV catalog is under 6 h old on a host | 99% | ticket |
| ci-develop | every GitHub Actions workflow's latest finished run on develop is green, data under 2 h old | 99% | ticket |

`ci-develop`'s SLI comes from `unheaded-ci-status.timer` on west
(`scripts/monitoring/ci-status-textfile.sh`, public Actions API every 10
minutes, no token) through huginn's textfile directory; a cancelled run does
not count, a stale reader counts as red. Runbook `runbooks/observe/ci-red.yaml`
(an SLO may name its own runbook in the policy; the rest share
`slo-error-budget-burn.yaml`).

The Grafana dashboard **SLO Error Budget** (`config/grafana/provisioning/dashboards/kingdom/08_slo_error_budget.json`)
shows each SLO's budget left (red when spent, amber under a quarter), the
burn rate over 1 h, 6 h and 3 d against the tier thresholds, and what is
burning now.

The burn-rate tiers come from the Google SRE Workbook, ch. 5. A tier fires when both its windows burn faster than its rate:

| Tier | Windows | Burn rate | Urgency |
|---|---|---|---|
| fast | 1 h and 5 m | 14.4× | page |
| medium | 6 h and 30 m | 6× | page |
| slow | 1 d and 2 h | 3× | ticket |
| steady | 3 d and 6 h | 1× | ticket |

An SLO with `page: false` sends every tier as a ticket. The rule tests in `monitoring/prometheus/rules/tests/slo_test.yml` pin the behaviour:
- a 3-minute blip with half the fleet down does not fire;
- 15 minutes of it pages;
- a vanished exporter counts as down.

### The same SLOs in The Well

`policy.yaml` is the source you edit and review. The Well holds a copy, one
row per SLO in `unheaded_ops.slo_definitions` (migration 040), with every
change kept in `slo_definitions_history`:

```bash
# after editing policy.yaml and regenerating:
WELL_DB=unheaded_ops WELL_PASSWORD=<ops_writer> go run ./cmd/alert-policy -sync-well
# any time; fails on drift, and fails if the Well cannot be read:
WELL_DB=unheaded_ops WELL_PASSWORD=<ops_writer> go run ./cmd/alert-policy -check
```

Without `WELL_DB`, `-check` only checks the YAML and generated files (what
CI runs). An objective may have at most two decimals in the policy.
Compliance control `UH-SLO-01` reads each SLO's budget from VictoriaMetrics
(`VICTORIA_URL`, default `http://localhost:8428`): PASS while budget is
left, FAIL when spent, NOT_ASSESSED when the series is missing.

## The mailer

`alert-mailer` (compose service in `monitoring/`, uid 16710, `127.0.0.1:19130`) is the only thing that sends alert mail. Alertmanager hands it pages and tickets by webhook and holds no credentials.

```bash
curl -s 127.0.0.1:19130/v1/status    # mode, sent today / cap, held back, open tickets
curl -s 127.0.0.1:19130/v1/outbox    # last 50 mails: sent, dry-run, capped, failed, unchanged
```

Its state (cap count, open tickets, outbox) lives in the `alert_mailer_data` volume and survives restarts.

**Mode (2026-11-15):** `dry-run` on west since 2026-11-10 (Stevie: while the kingdom ran as a test cut); `send` again in `policy.yaml` on Stevie's ADR-147 decision 2, live once S-SEE-AND-SURVIVE Phase 15 restarts the mailer with a test mail. On a date listed in `mailer.run_days.dates` the caps are 24 a day and 6 an hour (a load run's stop marks late in the day still mail); add the date before the run, delete it after.

**Mode was `send` from 2026-10-27 (265badbb).** Short term it sends from personal Gmail to stevie@bellis.tech; the Workspace account below is the longer-term sender. One synthetic page went through `smtp.gmail.com:587` with outbox outcome `sent` before the flip. `dry-run` (2026-10-20 → 10-27) decides everything as if sending, including the cap, and sends nothing; switch back to it in the policy if the outbox shows noise, and fix the SLO, not the mailer.

### Switching to send (Gmail)

Use the Google Workspace account (bellis.tech): it allows 2,000 sends a day against personal Gmail's 500, and keeps alert mail out of a personal inbox. Sending to your own address from your own account through `smtp.gmail.com` is ordinary authenticated mail with Google's own DKIM signature. At 6 a day it is nowhere near any reputation limit.

1. On the account: turn on 2-Step Verification, then create an app password at myaccount.google.com/apppasswords.
2. Store the password outside the repo, so it never passes through a shell history or chat:

   ```bash
   sudo install -d -m 0700 -o 16710 -g 16710 /etc/unheaded/alert-mailer
   sudo sh -c 'read -rs p && printf %s "$p" > /etc/unheaded/alert-mailer/smtp-password'
   sudo chown 16710:16710 /etc/unheaded/alert-mailer/smtp-password && sudo chmod 0400 /etc/unheaded/alert-mailer/smtp-password
   ```

3. Add to `monitoring/.env` (none of these is a secret):

   ```
   SMTP_SMARTHOST=smtp.gmail.com:587
   SMTP_USERNAME=you@bellis.tech
   ALERT_MAIL_FROM=you@bellis.tech
   ALERT_MAIL_TO=you@bellis.tech
   ALERT_MAILER_SECRETS_DIR=/etc/unheaded/alert-mailer
   ```

4. In the policy, set `mailer.mode: send`, then commit and `docker compose up -d alert-mailer`.

STARTTLS is required, and a login over plaintext is refused. A relay error is not counted against the cap: Alertmanager retries.

### Critical alerts on Wotan (`alerts.critical`)

After the mail decision, mailed or not, the mailer publishes every critical notification (firing and resolved) and a snapshot of what fires every 5 minutes on the Wotan topic `alerts.critical`, signed with ML-DSA-65 or not at all; every service on the bus holds changes while one fires (`pkg/critalert`, ADR-147 Phase 8). The message is metadata only: fingerprint, alert name, severity, status, host, start time. It is off until a host enrols, and a failure there never touches the mail. The enrolment steps, the signals that it works and what each failure looks like are in [[Wotan Topic Signing|Wotan-Topic-Signing]].

The mailer's settings for it, in `monitoring/.env` (the monitoring stack is started with `-f monitoring/docker-compose.yml`, so no override file is read):

| Variable | Default | Meaning |
|---|---|---|
| `ALERT_MAILER_WOTAN_ADDR` | unset: off | Wotan's HTTP address as the monitoring network reaches it: the host address its port is published on, `https://[fd00:dead:beef::1]:18000` on west |
| `ALERT_MAILER_WOTAN_GRPC_ADDR` | the same host, port 18001 | Wotan's gRPC address, `host:port` |
| `ALERT_MAILER_SECRETS_DIR` | an empty directory of the repository | the host directory with `smtp-password`, `heartbeat-token` and, for this, `alert-mailer.key` (Ed25519 service key) and `alerts-critical.key` (ML-DSA-65 seed), each 0400 and owned by uid 16710 |
| `ALERT_MAILER_WOTAN_TLS` | `true` | TLS 1.3 to Wotan with the CA below. Anything else dials plaintext, where the service token is withheld off loopback: for a Wotan without TLS and without auth only |
| `ALERT_MAILER_WOTAN_CA_FILE` | `../configs/certs/dev/ca.pem` (relative to `monitoring/`) | the CA that signed Wotan's certificate; the default is the file Prometheus mounts. One file mount: a replaced CA is seen after the container is recreated |
| `ALERT_MAILER_WOTAN_TLS_SERVER_NAME` | `wotan` | the name Wotan's certificate is verified against, since it does not carry the host address dialled (`WOTAN_TLS_SERVER_NAME`, [[Transport Cascade|Transport-Cascade]]). Verification is never off |

These settings are the Wotan client's. The mailer's own listeners (19130 for Alertmanager and the healthcheck, 19131 for heartbeats) stay plain HTTP inside the monitoring network whatever `ALERT_MAILER_WOTAN_TLS` says; a test in `cmd/alert-mailer` fails if that changes without the callers.

Metrics on `127.0.0.1:19130/metrics`: `alert_mailer_critical_published_total` (accepted by Wotan; one more every 5 minutes at least), `alert_mailer_critical_publish_refused_total` (no usable signing key), `alert_mailer_critical_publish_errors_total` (Wotan down or refusing), `alert_mailer_critical_rebuild_errors_total` (Alertmanager's firing alerts not read; no snapshot goes out until one read succeeds).

## Planned work

A reboot or a stack restart burns budget like an outage does. Silence first:

```bash
scripts/monitoring/maintenance.sh 30m "reboot west"   # 30m of work, at most 24h
scripts/monitoring/maintenance.sh --list
scripts/monitoring/maintenance.sh --end
```

**Planned stops (ADR-147 decision 4).** A silence stops mail; the SLO budgets still burn. A planned-stop mark stops that too: `scripts/monitoring/planned-stop.sh all 30m` (or an SLO name, or one container's name) writes `unheaded_planned_stop_until_timestamp_seconds{scope}` into huginn's textfile directory (`planned_stop.prom`, root's: it uses `sudo -n` as a normal user). Every SLO's `slo:good` counts a minute under a mark for its name or `all` as good, and ContainerStopped stays quiet for the marked container. `planned-stop.sh --list` shows the marks; `--end [scope]` ends them. `maintenance.sh --planned 30m "why"` (or `--planned=<scope>`) silences and marks in one go; `maintenance.sh --end` ends both. `kingdom-shutdown.sh` marks `all` for 2 hours (`--planned 8h` for a longer planned stop, at most 30d) and `kingdom-startup.sh` replaces it with 15 minutes of warm-up. It runs on every reboot (`kingdom-shutdown.service`), so its mark ends on its own: until 2026-11-15 it lasted 30 days, and a reboot with no startup muted ContainerStopped and read every SLO good for a month. A live mark still does that while it lasts: `planned-stop.sh --list` after a restart. The `alert-route` compliance probe (each link of the alert path asked directly) passes a link that is down under a mark for its container (`unheaded-prometheus`, `unheaded-alertmanager`, `unheaded-alert-mailer`, `unheaded-postgres`) or `all` only when the mark ends within `planned_stop.probe_max` in `observability/alerting/policy.yaml` (2h; `<n>m` or `<n>h`, 1m to 24h; absent: no mark counts), so a 2-hour shutdown mark reads as planned and a 30-day one still fails. `compliance-evidence` alone reads it.

**ci-develop** is recorded and notifies no one (`notify: false`, Stevie 2026-11-15, ADR-147 decision 4: out of the alert set until CI is fixed): its `slo:good` and budget are still there to read.

Alerts still fire and show; only mail stops. The duration is the work; each silence runs past it until the tiers can have cleared: pages +30m (the medium tier's short window), tickets +6h (the steady tier's). A silence that ended with the work mailed about the planned outage after it was over. `cmd/alert-policy`'s tests keep the tails in step with the tiers. `scripts/kingdom-shutdown.sh` silences 30m of work before it stops anything, and `kingdom-startup.sh` 15m of warm-up (both best effort).

## Rules that cannot fire are deleted

A rule on a metric nothing emits is a false sense of coverage. On 2026-10-27
(S-WELL-SPRING Step 163) `monad.yml` (`MonadHbHDropRateHigh`, `BGPSessionDown`,
`BFDFailoverTriggered`, `RoutingHealthDown`) and `loadbalancers.yml` (seven
`HAProxy*`/`Nginx*` rules) were deleted: no binary emits the `unheaded_*`
series and no job scrapes `haproxy_*`/`nginx_*` (checked live: zero series).
`NodeExporterDown`, the one live rule among them, moved to `host.yml`. A
deleted rule comes back with its producer and a promtool test; the load
balancer rules return with an HAProxy/Nginx exporter.

On 2026-11-15 (S-WATCH-TRUE) the `heimdall` rule group and its tests went the
same way, with the `heimdall` scrape jobs in Prometheus and VictoriaMetrics and
the `sword` job in VictoriaMetrics: no host runs heimdall-daemon, and sword is
up only during a deploy. Their code and runbooks stay.

The same sprint added three things that can fire: `WotanTopicPermissionRefused`
(the refusal counter has a bounded `caller` label, and every listed caller's
series starts at zero so a first refusal counts); `ZhenSchedulerStalled` as two
rules under one name, `reason: absent` and `reason: stale`; and
`unheaded_host_events_total` from Vector, which the IOC audit runbook reads
(account changes, crontab edits, untrusted module loads, sudo commands, OOM
kills and segfaults).

## Dead-man's switch

Two alerts watch the watchers (ADR-100 section 4):

- **`AlertPipelineHeartbeat`** (named `Watchdog` until 2026-11-11) is always firing, urgency `none`, and firing is the healthy state. It mails no one; it should
  always be on Alertmanager's page. If it is missing, rule evaluation or
  delivery is broken and every quiet alert is suspect.
- **`HeartbeatMissing{host}`** pages when a watched host has not sent a
  heartbeat for 10 minutes. East POSTs `http://192.168.13.2:19131/heartbeat/east`
  every 2 minutes (`unheaded-heartbeat.timer`, token through `LoadCredential`);
  alert-mailer records it on a heartbeat-only listener and exports
  `unheaded_heartbeat_last_seen_timestamp_seconds`. West dying is not covered
  until a receiver outside both hosts exists.

Off until configured. To turn it on (the E-D host window):

1. Generate a token once and give both hosts the same bytes:
   west `ALERT_MAILER_SECRETS_DIR/heartbeat-token` (0400, uid 16710), east
   `/etc/unheaded/heartbeat.token` (0400 root).
2. `monitoring/.env` on west: `HEARTBEAT_HOSTS=east`,
   `HEARTBEAT_LISTEN=192.168.13.2` (only while the P2P address exists at
   boot: docker will not start a container whose published address is
   missing). `docker compose up -d alert-mailer prometheus alertmanager`.
3. East: install `scripts/monitoring/heartbeat.sh`, the sysusers line and
   `deploy/systemd/unheaded-heartbeat.{service,timer}`; enable the timer.

Runbook: `runbooks/observe/heartbeat-missing.yaml`.

**The pipeline's own receipt (ADR-147, ADR-116 R4).** A rule cannot report that its own evaluator is down, so the mailer does: Alertmanager hands it `AlertPipelineHeartbeat` every 5 minutes (`POST /v1/alerts/pipeline`, the first route in `alertmanager.yml`, from `pipeline: silent_after: 20m` in the policy). With no receipt for 20 minutes it mails "alert pipeline silent" once (Prometheus, Alertmanager or the route is down), and "alert pipeline back" once when receipts return. Both count against the caps, and both go out even when a cap is spent (nothing else would retry them; review 2026-11-15), at most four such mails a day. `unheaded_alert_mailer_pipeline_last_seen_timestamp_seconds` is the last receipt. Runbook: `runbooks/observe/alert-pipeline-silent.yaml`.

## A boot of the host

West's stack does not start at boot (Stevie, decision pack 2026-11-18 row 22, ADR-150 choice 15: "2 with plan to change later"): it stays down until a person starts it, and says so. When the host comes up, Prometheus, Alertmanager and the mailer are down with the stack, so no rule can report the boot. The host writes it down itself, and the mailer reads the record when it next runs:

1. **`unheaded-boot-event.service`** (host unit, [[Systemd-Units]]) runs `scripts/monitoring/boot-event.sh record` once, late in boot. It appends one JSON line to `/var/lib/unheaded/boot-event/boots.jsonl` (boot id, boot time, host name, how the boot before ended, the stack's state then; the newest 32 are kept) and writes `host_boot.prom` in huginn's textfile directory: `unheaded_host_boot_timestamp_seconds`, `unheaded_host_boot_previous_shutdown{state}` (1 on `planned`, `clean`, `unclean` or `unknown`) and `unheaded_host_boot_stack_up`. No rule reads those series, so Prometheus does not keep them; VictoriaMetrics does.
2. **alert-mailer** reads `boots.jsonl` when the digest is due (`BOOT_EVENTS_FILE`, a read-only mount) and gives each boot it has not reported one plain line, after the silent alert path's line if there is one and before the alerts. The subject gains `, west booted` (or `, west booted 3 times`).

The line, with the host name from the record (`BOOT_EVENT_HOST`, default the host's short name) and times in UTC:

```
<host> booted <YYYY-MM-DD HH:MM>Z; <stack>; previous shutdown: <planned|clean|not clean|unknown>
west booted 2026-11-21 01:44Z; stack down; previous shutdown: not clean
```

| `<stack>` | When |
|---|---|
| `stack down` | the newest boot; no container of the stack's compose projects runs now |
| `stack up since 2026-11-21 14:02Z` | the newest boot; every one runs now, the last of them started then |
| `stack partly up (12 of 23 containers running)` | the newest boot; some run |
| `stack down at boot (no current host state)` | the newest boot, but `host_state.prom` is missing, older than `boot.state_max_age` (5m), from before the boot, or Docker did not answer: the line says what the boot recorded (`down`, `up`, `partly up`, `state unknown`) |
| `stack down at boot` | an earlier boot (two boots since the last digest give two lines, oldest first) |

"Now" comes from `host_state.prom` (`HOST_STATE_FILE`), which `unheaded-host-state.timer` writes every 30 s whether the stack runs or not. "The stack" is the compose projects in `BOOT_EVENT_STACK_PROJECTS` (default `unheaded-dev`, [[Host-Settings]]); the monitoring project is left out, so with only the mailer started to get the digest out the line still reads `stack down`. The services in `BOOT_EVENT_ONESHOT` never count: one-shots (`volume-init`, `clickhouse-schema`, the default) and, once listed, services that run only on demand; a service stopped by design and not listed there makes the line read `stack partly up`.

| `previous shutdown` | Means |
|---|---|
| `planned` | an orderly shutdown with a planned-stop mark for `all` open at that time (`kingdom-shutdown.sh` and `maintenance.sh --planned` write one) |
| `clean` | an orderly shutdown (`reboot`, `poweroff`) with no such mark |
| `not clean` | no orderly shutdown was recorded: a power loss, a hang, a long press of the power key. The unit's `ExecStop` stamps `clean-shutdown` while systemd stops the system; a boot that finds no stamp for the boot before it says this |
| `unknown` | no earlier boot on record (the first boot after the unit was installed) |

**Reported once.** The digest that first carries a boot's line, sent or (in `mode: dry-run`) recorded in the outbox, puts its boot id in the mailer's state (`boots_reported` in `state.json`); no later digest repeats it. A digest a cap held back, or the relay refused, reported nothing: the next one carries the line. A boot never mails on its own; it is not on `mailer.immediate` and cannot be put there (it is not an alert).

**When the stack is started at boot** (the intended later change) nothing here changes: the unit still records each boot, and the same line then reads `stack up since ...`. What the boot recorded may be `partly up` (the unit can run while containers are still starting); the digest reads the state when it is written.

To turn it on for a host: install the unit (its header has the commands), then give the mailer the two directories in `monitoring/.env` and recreate it:

```
ALERT_MAILER_BOOT_DIR=/var/lib/unheaded/boot-event
ALERT_MAILER_TEXTFILE_DIR=/var/lib/unheaded/textfile
```

Unset, both mounts are an empty directory of the repository and the digest carries no boot line. Check: `cat /var/lib/unheaded/boot-event/boots.jsonl` (the record), `journalctl -u unheaded-boot-event -b` (one line per boot), `curl -s 127.0.0.1:19130/v1/outbox` (the digest as sent or as it would have been). The mailer's start line logs `boot_line: true` when `BOOT_EVENTS_FILE` is set.

## Containers, resources and the watchers (ADR-147)

Before ADR-147 no alert read a resource, a stopped container or the monitoring path. All of these are `urgency: none` in their rule files; the mail comes from `promote:` in the policy (SS1 A, 2026-11-15, taken on the plan's recommendation: Stevie to confirm). The container and unit series come from `unheaded-host-state.timer` through huginn, the cgroup series from huginn itself ([[Huginn|Service-Huginn]]), the rest from node_exporter and Prometheus.

| Alert | File | Fires when | Urgency | Runbook |
|---|---|---|---|---|
| ContainerStopped | `containers.yml` | a compose container that ran in the last 7 days is stopped for 2m, with no planned-stop mark | page | `container-stopped.yaml` |
| MemoryKill | `containers.yml` | `oom_kill` rises in `system.slice` or `unheaded.slice` (kept 30m) | page | `memory-kill.yaml` |
| HostOOMKill | `containers.yml` | `node_vmstat_oom_kill` rises anywhere (login sessions too) | none | `memory-kill.yaml` |
| ContainerUnhealthy | `containers.yml` | running and failing its healthcheck for 5m | none | `container-stopped.yaml` |
| ContainerCPUThrottled | `containers.yml` | over 25% of CPU periods throttled for 30m | none | `host-pressure.yaml` |
| ContainerMemoryNearLimit | `containers.yml` | over 95% of its memory limit for 30m | none | `memory-kill.yaml` |
| HostMemoryPressure | `resources.yml` | PSI memory full over 10% (5m rate) for 10m | page | `host-pressure.yaml` |
| HostSwapping | `resources.yml` | swap-in over 1,000 pages/s for 15m | ticket | `host-pressure.yaml` |
| HostSwapNearFull | `resources.yml` | swap over 80% used for 30m | ticket | `host-pressure.yaml` |
| HostIOPressure | `resources.yml` | PSI io full over 30% (15m rate) for 30m | ticket | `host-pressure.yaml` |
| HostCPUPressure | `resources.yml` | PSI cpu some over 50% (15m rate) for 30m | none | `host-pressure.yaml` |
| HostTemperatureNearCritical | `resources.yml` | a sensor within 5 C of its critical mark for 10m | ticket | `host-temperature.yaml` |
| SliceTasksNearMax | `resources.yml` | a slice above 80% of its task cap (`TasksMax`) for 5m; on west `user-1000.slice` (4,096) | ticket | `host-pressure.yaml` |
| UnitFailed | `resources.yml` | a watched unit is `failed` for 5m | page | `unit-restart.yaml` |
| UnitRestartLoop | `resources.yml` | 3 restarts in 30 minutes | ticket | `unit-restart.yaml` |
| HostStateStale | `resources.yml` | `host_state.prom` over 5 minutes old, or Docker did not answer | ticket | `monitoring-path.yaml` |
| VictoriaMetricsDown, HuginnDown | `watchers.yml` | `up` 0 for 5m (Prometheus scrapes VictoriaMetrics as job `victoria`) | ticket | `monitoring-path.yaml` |
| AlertMailerDown | `watchers.yml` | `up` 0 for 5m; it cannot mail about itself | none | `monitoring-path.yaml` |
| PrometheusRemoteWriteFailing | `watchers.yml` | samples refused, or the queue over 5 minutes behind, for 15m | ticket | `monitoring-path.yaml` |
| AlertmanagerNotReached | `watchers.yml` | notification errors or no Alertmanager discovered for 10m | none | `monitoring-path.yaml` |

`HostDiskFull` is promoted to a page and `HostDiskAlmostFull` and `NodeExporterDown` to tickets at the same time. Each bar comes from five days of west's own series (`resources.yml`'s header). Two rules that could not fire were repaired: **TraceCollectorHighLatency** now reads the share of flushes to Wotan over 100 ms from `trace_publisher_flush_duration_seconds` (the old `trace_collector_batch_latency_seconds` was never observed), and **VectorSinkDroppingEvents** reads every component's discards (the ClickHouse sink it read blocks and never discards; `well_logs` is the one that can).

## Postmortems

Blameless, in `docs/incidents/` from `docs/incidents/TEMPLATE.md`. Written
for an outage of Wotan, the dashboard, The Well or Zhen over 1 h, a silent
failure over a day, or a data-loss near miss; not for a page that cleared
inside the hour (the default until Stevie decides, ADR-100 question 3). Every
action item is an `M-###`, a finding, or "done: <commit>".

## Every alert has a runbook

Every alert, generated or hand-written, carries `annotations.runbook_url`
naming a file in this repo: a repo path (`runbooks/observe/log-pipeline.yaml`)
or a GitHub blob/wiki URL to one. `scripts/check-prometheus-rules.sh` fails
otherwise (`go run ./cmd/alert-policy -runbooks`), with no baseline. A new
rule points at an existing runbook or comes with one (runbook/v1,
`status: draft`); `runbooks/observe/log-pipeline.yaml`,
`runbooks/security/kev-feed-stale.yaml`, `trace-collector-down.yaml` and
`high-latency.yaml` were written for the rules that had none (2026-10-27).

## When one arrives

`runbooks/observe/slo-error-budget-burn.yaml`. The mail's `Check:` line holds that SLO's first steps. If it fired and nothing was wrong, that is a false positive: fix the SLI or the objective in the policy.

## Proven by

- `go test ./cmd/alert-policy ./cmd/alert-mailer`:
  - the cap, dry-run, the digest, two mails per incident;
  - a boot of the host: one line, reported once, two boots two lines, nothing in dry-run but the outbox (`boot_test.go`);
  - state across restarts;
  - header injection;
  - STARTTLS refusal;
  - the policy's refusals.
- `scripts/check-prometheus-rules.sh`: the SLO rule tests on two Prometheus versions.
- `scripts/monitoring/test-alert-path.sh`: Prometheus → Alertmanager → alert-mailer → an SMTP sink. It checks that:
  - urgency none mails no one;
  - a page is mailed once;
  - a ticket waits for the digest;
  - the cap holds.

  The meta-gate (`check-gates-can-fail.sh`) proves it fails when the default route pages.
- `scripts/monitoring/test-boot-event.sh`: the boot record against a fake `/proc`, Docker and systemd: planned, clean, unclean and unknown; the stamp only at a shutdown; one record per boot; the bounded history.
