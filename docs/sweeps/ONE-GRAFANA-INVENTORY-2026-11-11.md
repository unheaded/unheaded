<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# One Grafana: what west ran on 2026-11-11

ADR-117 step 1. Read from the running containers on west and from `develop` at `6793ce20`, read-only, before anything was changed. A dated record: later changes get a note at the end, not an edit.

## The duplicates

| Job | Main stack (`docker-compose.yml`, project `unheaded-dev`) | Monitoring stack (`monitoring/docker-compose.yml`, project `monitoring`) |
|---|---|---|
| Dashboards | `unheaded-grafana` 11.5.1, port 3001, state on tmpfs | `unheaded-monitoring-grafana` 10.4.0, port 3000, volume `monitoring_grafana_data` |
| Metrics | `unheaded-victoria` v1.155.0, port 8428, 12 targets, 1547 metric names | `unheaded-prometheus` v3.15.1, port 9090, 46 targets, 1263 names; `unheaded-victoriametrics` v1.155.0, 0 series |
| Logs | `unheaded-clickhouse` 24.12 fed by `unheaded-vector` | `unheaded-loki` 3.0.1, 0 streams, no shipper |
| Alert path | none | `unheaded-prometheus` (rules), `unheaded-alertmanager` v0.34.1, `unheaded-alert-mailer` |

ADR-117's Context table names `docker/telemetry/docker-compose.telemetry.yml` as the monitoring stack. The stack that runs on west is `monitoring/docker-compose.yml`; `docker/telemetry/` was an older, unpinned copy that nothing started and no gate read.

## Grafana

Both Grafanas mounted the same directory, `monitoring/grafana/dashboards` (14 JSON files), and both read the main stack's VictoriaMetrics. Neither read Prometheus, Loki or the second VictoriaMetrics for a dashboard.

The 10.4 Grafana's database (`grafana.db` in `monitoring_grafana_data`): one user (the admin) and no dashboard that is not provisioned from the repo. It also had a Loki datasource provisioned; no dashboard used it. Nothing in the database is needed after retirement; the runbook still takes a copy first.

Dashboards, judged by whether their metric names exist in the main store:

| File | Names present | Verdict |
|---|---|---|
| `01_host_overview` | 13 of 13 | kept |
| `02_postgres_health` | 8 of 8 | kept |
| `03_wireguard` | 4 of 7 | kept, three dead panels dropped (`unheaded_wireguard_*`, `probe_duration_seconds` have no emitter) |
| `07_host_resources` | 12 of 12 | kept |
| `08_slo_error_budget` | 7 of 7 | kept |
| `09_compliance` | 0 of 8 | kept: the emitter (`cmd/unheaded-baseline`) exists and is not running yet |
| `infrastructure` | 11 of 11 | kept; every panel was empty because it selected hosts `forge` and `outpost`, labels no series carries. It selects the two node_exporter instances now |
| `02_ebpf_pipeline` | 1 of 8 | not ported, no emitter |
| `04_llm_inference` | 2 of 8 | not ported, no `vllm_*` series |
| `05_services` | 1 of 5 | not ported, no emitter |
| `06_loadbalancers` | 0 of 11 | not ported, no HAProxy or nginx exporter is scraped |
| `container_fleet` | 0 of 6 | not ported, no emitter |
| `firewall` | 0 of 9 | not ported, no emitter |
| `routing_bgp` | 0 of 7 | not ported, no emitter |

`config/grafana/unheaded-kingdom-dashboard.json` (the Kingdom services board, an API-envelope file that file provisioning could not load) becomes `kingdom/00_kingdom_services.json`. The seven that were not ported are in git history at `2613b0c2^`.

The 11.5.1 Grafana had fetched two app plugins from grafana.com at start (`grafana-lokiexplore-app`, `grafana-pyroscope-app`, in `/tmp/plugins`). No dashboard uses either. `GF_PLUGINS_PREINSTALL_DISABLED` stops the fetch.

## Metrics

Prometheus, 17 jobs, 46 targets, 31 down:

| Job | Up | Down | After |
|---|---:|---:|---|
| `prometheus`, `alert-mailer` | 2 | 0 | stay in Prometheus only; forwarded to the store |
| `node_exporter`, `unheaded-daemon`, `trace-collector`, `kev-poller`, `dashboard-backend` | 10 | 0 | in both files |
| `huginn`, `vector`, `wotan` | 3 | 0 | in both files |
| `heimdall` | 0 | 2 | in both files; nothing answers on 19101 on either host |
| `syslog-relay` | 0 | 1 | in both files; the relay is not installed on west (next.md question 19) |
| `zhen-agentd` | 0 | 1 | in both files; runs today only as a transient test unit |
| `frr_exporter`, `bird_exporter` | 0 | 2 | removed: nothing listens, no rule reads them |
| `routing_health` | 0 | 2 | removed, same |
| `doom_services` | 0 | 23 | removed: the old 16666-16689 port block, nothing listens |

Main VictoriaMetrics before: 12 jobs, all compose services, 11 up (`sword` down). It scraped no host unit and nothing on east; those series reached it only through Prometheus's `remote_write`, which forwarded everything Prometheus scraped.

The running container also read an older scrape file than the checkout: 12 jobs against 13 on `develop`. It was started 2026-11-10 13:27Z; `a5e01c12` added the `logstore-writer` job at 18:26Z. The file is a single-file bind mount, which keeps the file the container started with when git replaces it, so `logstore-writer` has not been scraped since the log store week began. Recreating the container (cut-over step 3) picks the current file up.

After: `config/victoria/scrape.yml` has 21 jobs and 27 targets, including the host units on both hosts over the overlay. Prometheus forwards only what the store cannot scrape itself: recording-rule series, `ALERTS`, `ALERTS_FOR_STATE`, and its own two jobs.

The second VictoriaMetrics (`unheaded-victoriametrics`) held 0 series: nothing wrote to it.

## Logs

Loki: 0 streams. No promtail container ran; `monitoring/promtail/*.yaml` were config files for a shipper the compose file did not start. No dashboard had a Loki panel. The log store question (ClickHouse or the Well) is the measured week of S-E1-LIVE Evening 16 and is not touched here.

## Alert rules

Prometheus evaluates 12 rule files: 84 alerting rules and 35 recording rules.

| File | Alerting | Recording |
|---|---:|---:|
| `slo.yml` (generated from `observability/alerting/policy.yaml`) | 32 | 31 |
| `logs.yml` | 12 | 4 |
| `compliance.yml` | 10 | 0 |
| `host.yml` | 9 | 0 |
| `well.yml` | 6 | 0 |
| `collector.yml`, `detection.yml` | 4 each | 0 |
| `threat-feeds.yml` | 3 | 0 |
| `heartbeat.yml`, `network.yml`, `watchdog.yml`, `zhen.yml` | 1 each | 0 |

By what notifies (ADR-100): 6 rules carry `urgency: page` and 26 `urgency: ticket`, all of them `ErrorBudgetBurn` tiers in `slo.yml`. The other 52 are cause alerts and notify nobody unless `policy.yaml` promotes them; it promotes four (`HeartbeatMissing` as a page, `TraceCollectorDown`, `SSHLoginNewSource`, `AlertMailVolumeHigh` as tickets).

Firing at the time of the read, all under the standing silence: `Watchdog` (by design), `SyslogRelayDown`, `ZhenSchedulerStalled`, `ComplianceFindingsOverdue`, `ErrorBudgetBurn` on four SLOs.

**What evaluates rules if Prometheus goes: nothing in the tree.** `cmd/healthprobe` is a container health check. `cmd/alert-policy` generates `slo.yml` and `alertmanager.yml` and evaluates nothing. ADR-116's warden is Proposed and not built. VictoriaMetrics single-node does not evaluate alerting rules; its evaluator is a separate image (`vmalert`), which ADR-117 step 3 rules out. So Prometheus stays as the evaluator and its retirement is a question for Stevie (ADR-117, 2026-11-11 note).

## Other copies found, not touched

| Where | What | Why left |
|---|---|---|
| `deploy/sophia-eye/docker-compose.yml` | its own `prom/prometheus` v3.15.1 on 127.0.0.1:20108, tag only | not running on west; its targets are on its own bridge. DP-207 says fold into the main store: a separate change |
| `monitoring/prometheus/prometheus-host-b.yml` | a Prometheus config for east | read by `nixos/modules/observability.nix` |
| `lxd/telemetry/` | LXD profiles for a telemetry container | not deployed |
| `tomb/dark-mirror/docker-compose.yml` | Grafana, Prometheus, Loki for the tomb lab | archived lab. It carries a literal Grafana admin password in the file, which the gitleaks rules do not match and no baseline lists: a question for Stevie (lab default, or a value used anywhere else?) |
| `scripts/bootstrap-llm-lab.sh` | Loki lines | lab bootstrap, not the kingdom |
| `pkg/observability` | Loki and Prometheus client adapters | product code, not a deployment |
| `pkg/uids` | uid 16702 reserved for Loki | reserved numbers are never reused |

`kubernetes/` no longer exists in the tree (ADR-117 question 3 answered itself).

## Drift

Every digest-pinned container on west runs the digest its file pins. What the files handle loosely:

- `postgres:16.15-alpine`: tag only, the one row of `docs/policy/image-digest-baseline.txt`.
- `haproxy:2.9.15-alpine` ×2 (`docker/docker-compose.loadbalancers.yml`) and `unheaded-sword` (`docker/compose.cuirass-lxd.yml`) run in the main project from files the digest and tag gates do not read. HAProxy has a tag and no digest.
- An untracked `docker-compose.override.yml` in the main checkout.
- After the retirement commit and until the cut-over: three containers whose service is gone. `scripts/check-image-drift.sh` reports them.
