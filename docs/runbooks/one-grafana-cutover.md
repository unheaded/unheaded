<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Runbook: one Grafana cut-over (ADR-117)

Moves a host that still runs the monitoring stack's Grafana, Loki and second VictoriaMetrics onto the files as they are after branch `sprint/s-one-grafana`. Written for west. Prose for a person at the keyboard, not a Champion runbook: nothing here is in `runbooks/MANIFEST.sha256`.

What was found and why nothing is lost: `docs/sweeps/ONE-GRAFANA-INVENTORY-2026-11-11.md`.

**Who runs it.** Steps 1 to 6 recreate or reload containers of the two compose stacks. Steps 7 to 9 delete containers and volumes: those wait for Stevie's word. No firewall rule, sshd setting, host unit or Alertmanager silence is touched by hand.

**How long.** About ten minutes. Each step names what restarts.

## The trap in the merge

The compose stacks mount files from the `develop` checkout (`~/tmp/unheaded`), so the merge itself changes what running containers see:

- `monitoring/grafana/dashboards/` is deleted. Both Grafanas mount that directory: from the moment of the merge the Grafana on 3001 shows no dashboards, until step 2 recreates it.
- `monitoring/prometheus/` is a directory mount. Prometheus sees the new `prometheus.yml` at once but acts on it only at a reload (step 5) **or at its next restart**. After that it stops forwarding scraped series to the store. If Prometheus restarts before step 3, the dashboards lose their host series until step 3 is done. So: merge, then go straight through step 5.
- `config/victoria/scrape.yml` is a single-file mount. The running VictoriaMetrics keeps the file it started with until it is recreated (step 3).

To split the work, merge `99e18bbb` first (dashboards and scrape hand-over, nothing deleted), run steps 1 to 6, and merge the rest of the branch before step 7.

## 0. Before

On west, in `~/tmp/unheaded`, on `develop` with the branch merged.

```
git log -1 --format='%h %s'
sudo docker ps --format '{{.Names}}\t{{.Status}}' | grep -E 'grafana|victoria|prometheus|alertmanager|alert-mailer|loki'
curl -fsS http://127.0.0.1:8428/-/healthy
curl -fsS http://127.0.0.1:9090/-/healthy
curl -fsS http://127.0.0.1:9093/api/v2/silences | python3 -c 'import json,sys; print([ (s["status"]["state"], s["endsAt"]) for s in json.load(sys.stdin) if s["status"]["state"]=="active"])'
test -s .env && echo ".env present"
```

Expect eight containers up (two Grafanas, two VictoriaMetrics, Prometheus, Alertmanager, alert-mailer, Loki), both health answers, and the standing silence active with an end time after this sitting. If the silence ends within the hour, renew it first (`scripts/monitoring/maintenance.sh`); the cut-over does not need it, the test cut does.

Note the count of series now, to compare in step 4:

```
curl -fsS 'http://127.0.0.1:8428/api/v1/query?query=count(up)' | python3 -c 'import json,sys; print(json.load(sys.stdin)["data"]["result"][0]["value"][1])'
```

## 1. Lint what is about to run

```
sudo docker compose config --quiet && echo main-ok
sudo docker compose -f monitoring/docker-compose.yml config --quiet && echo monitoring-ok
./scripts/check-image-drift.sh
```

Both `-ok` lines must print. `check-image-drift.sh` exits 1 here and lists exactly three containers with no service (`unheaded-monitoring-grafana`, `unheaded-loki`, `unheaded-victoriametrics`). Anything else in its list is drift that predates this work: stop and read it.

## 2. Recreate the Grafana on 3001

```
sudo docker compose up -d grafana
```

**Restarts:** `unheaded-grafana` only, about ten seconds. Its state is on tmpfs, so every browser session on 3001 is logged out, as at any restart.

Check:

```
curl -fsS http://127.0.0.1:3001/api/health
sudo docker exec unheaded-grafana ls /etc/grafana/provisioning/dashboards/kingdom
sudo docker exec unheaded-grafana sh -c 'ls /tmp/plugins 2>/dev/null | wc -l'
```

Expect `"database": "ok"`, eight JSON files, and `0` plugins. In the browser: eight dashboards in the General folder.

**Rollback:** `git revert` the merge (or check out the previous `docker-compose.yml` and `monitoring/grafana/`), then `sudo docker compose up -d grafana`.

## 3. Recreate VictoriaMetrics with the full scrape file

```
sudo docker compose up -d victoria
```

**Restarts:** `unheaded-victoria` only, ten to twenty seconds. Docker creates one new bridge, `br-unhe-scrape` (network `unheaded-dev_scrape`, 172.28.6.0/24 and fd00:d0c0:e706::/64), and its own firewall rules for it. The data volume is kept. While it is down: Grafana panels error, huginn's pushes and Prometheus's remote-write retry, and `pkg/sloguard` fails closed (a canary or chaos run started in those seconds is refused).

Check:

```
curl -fsS http://127.0.0.1:8428/-/healthy
curl -fsS -g 'http://[fd00:dead:beef::1]:8428/-/healthy'
ssh east "curl -fsS -g 'http://[fd00:dead:beef::1]:8428/-/healthy'"
```

All three must answer. The second and third are the publish on the overlay address, which Prometheus's remote-write uses. East's huginn pushes to the `UNHEADED_P2P_BIND` address: `ssh east 'journalctl -u huginn --since -5min | tail -5'` should show no push errors after the first minute. It has not been exercised with an IPv6-enabled container before: **if either fails, roll this step back.**

**Rollback:** in `docker-compose.yml` remove `- scrape` from `victoria`'s `networks:` and the `-enableTCP6` line, `sudo docker compose up -d victoria`, and do not run step 5. The store is then as before and Prometheus still forwards everything.

## 4. Targets

Wait a minute, then:

```
curl -fsS http://127.0.0.1:8428/api/v1/targets | python3 -c '
import json, sys
t = json.load(sys.stdin)["data"]["activeTargets"]
up = [x for x in t if x["health"] == "up"]
print(len(up), "of", len(t), "up")
for x in t:
    if x["health"] != "up":
        print("  down", x["labels"]["job"], x["scrapeUrl"])'
```

Expect 27 targets. In the proof container 21 were up. Known down, each for a reason outside this work:

| Target | Why |
|---|---|
| `heimdall` on both hosts | nothing listens on 19101 |
| `syslog-relay` on west | not installed (next.md question 19) |
| `zhen-agentd` on west | runs only as a transient test unit |
| `sword` | down before this work too |
| `logstore-writer` | unreachable from the proof container's network; expected up here |

A host unit on west or east that is up in Prometheus (`http://127.0.0.1:9090/targets`) and down here is a miss: do not run step 5 until it is understood. Every job in Prometheus except `alert-mailer` and `prometheus` must appear in this list.

## 5. Reload Prometheus

```
sudo docker kill -s HUP unheaded-prometheus
sudo docker logs --since 1m unheaded-prometheus 2>&1 | grep -i 'loading configuration\|completed loading\|err'
```

**Restarts:** nothing. A reload keeps rule state. Prometheus drops the four dead jobs (27 targets) and forwards only rule results, `ALERTS` and its own two jobs.

Check:

```
curl -fsS http://127.0.0.1:9090/api/v1/targets | python3 -c 'import json,sys; t=json.load(sys.stdin)["data"]["activeTargets"]; print(len(t), "targets")'
curl -fsS 'http://127.0.0.1:8428/api/v1/query?query=count(slo:error_ratio:5m)'
curl -fsS 'http://127.0.0.1:8428/api/v1/query?query=count(ALERTS)'
curl -fsS -g 'http://127.0.0.1:9090/api/v1/query?query=rate(prometheus_remote_storage_samples_failed_total[5m])'
```

Expect 19 targets (was 46), both counts non-empty, and a failure rate of 0.

**Rollback:** restore the previous `monitoring/prometheus/prometheus.yml` and send HUP again.

## 6. Look

Open the eight dashboards on `http://<west>:3001`. Host panels come from the store's own scrapes now. Series that used to arrive through Prometheus carried `cluster` and `env` labels; the scraped ones do not, so a graph may show a seam at the minute of step 5 and the old series end there.

`09_compliance` stays empty until `cmd/unheaded-baseline` runs.

## 7. Copy the old Grafana's database (Stevie's word from here)

It holds one user row with a password hash and nothing the repo lacks. Copy it anyway, root-only, on the host and not in the repo:

```
sudo docker stop unheaded-monitoring-grafana
sudo install -d -m 0700 /srv/backups/unheaded-grafana-10.4
sudo docker cp unheaded-monitoring-grafana:/var/lib/grafana/grafana.db /srv/backups/unheaded-grafana-10.4/grafana-2026-11-11.db
sudo chmod 0600 /srv/backups/unheaded-grafana-10.4/grafana-2026-11-11.db
sudo sh -c 'ls -l /srv/backups/unheaded-grafana-10.4/'
```

**Stops:** the Grafana on 3000. The copy is taken from the stopped container so the file is whole.

## 8. Remove the three retired containers

`docker compose up` only warns about containers whose service is gone, and `scripts/kingdom-startup.sh` runs exactly that, so they are removed by name:

```
sudo docker stop unheaded-loki unheaded-victoriametrics
sudo docker rm unheaded-monitoring-grafana unheaded-loki unheaded-victoriametrics
sudo docker compose -f monitoring/docker-compose.yml up -d
./scripts/check-image-drift.sh
```

**Restarts:** the `volume-init` one-shot runs again (its volume list changed). Prometheus, Alertmanager and alert-mailer are unchanged in the file and should stay up; if compose recreates one, its data is on a volume and only pending `for:` timers start over. The silence lives in Alertmanager's volume.

`check-image-drift.sh` must now pass.

**Rollback:** `git revert 2613b0c2`, `sudo docker compose -f monitoring/docker-compose.yml up -d`. The volumes are still there until step 9.

## 9. Remove their volumes

Only after step 8 has run clean for a day, or at once if Stevie says so. This is the one step with no way back.

```
sudo docker volume ls --format '{{.Name}}' | grep '^monitoring_'
sudo docker volume rm monitoring_grafana_data monitoring_loki_data monitoring_victoriametrics_data
```

Keep `monitoring_prometheus_data`, `monitoring_alertmanager_data`, `monitoring_alert_mailer_data`.

## 10. After

- **LAN forwards.** The temporary forward on 3000 now points at nothing. Take 3000 out of `PORTS` in `~/.local/state/unheaded/lanfwd.sh` at the next `down`/`up`, or leave it until the forwards come down. The forwards on 9090, 9093 and 8428 are unaffected; 8428 blinks during step 3. Grafana on the wireless address is `http://192.168.69.184:3001`, as before.
- **next.md.** The "Grafana (monitoring, forward)" row of the test-sitting table goes. Todo 11 (every scrape job matches something deployed): the four dead jobs are gone; `heimdall`, `syslog-relay`, `zhen-agentd` and `sword` remain down and listed. Todo 9 and question 19 (`syslog-relay`) are unchanged. The stop line (`cd monitoring && sudo docker compose down`) still holds.
- **Wiki.** `wiki/Port-Registry.md`, `wiki/Alerting.md` and `wiki/Architecture.md` already describe the result.
- **East.** East has no docker and no Grafana, Loki, Prometheus or VictoriaMetrics unit (checked 2026-11-11). Its host units are scraped from west over the overlay; nothing to do there.

## What does not change

Prometheus, Alertmanager and alert-mailer stay as the path that evaluates rules and mails. `mailer.mode` stays as `observability/alerting/policy.yaml` sets it. Prometheus retires when its rules have another evaluator: ADR-117, note of 2026-11-11.
