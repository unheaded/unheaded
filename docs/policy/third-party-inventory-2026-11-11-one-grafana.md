<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2026 Stevie Bellis.
-->

# Third-party inventory: images dropped by ADR-117 (2026-11-11)

A dated note beside `third-party-inventory-2026-10-29.md` (ADR-117 step 6). That file is a dated record and is not edited; this one records what changed in its **Container images** section on branch `sprint/s-one-grafana`. Nothing was added.

## Dropped from the tree

| Image | Was in | Commit | Still in the tree |
|---|---|---|---|
| `grafana/grafana` 10.4.0 | `monitoring/docker-compose.yml`, `docker/telemetry/` | `2613b0c2`, `8162fa55` | 11.5.1 in `docker-compose.yml` (the only Grafana); 10.3.3 in `tomb/dark-mirror` |
| `grafana/loki` 3.0.1, 2.9.0 | `monitoring/docker-compose.yml`, `docker/telemetry/` | `2613b0c2`, `8162fa55` | 2.9.17 in `tomb/dark-mirror` only |
| `grafana/promtail` 2.9.17 | `docker/telemetry/` | `8162fa55` | 2.9.17 in `tomb/dark-mirror` only |
| `prom/node-exporter` v1.8.2 | `docker/telemetry/` | `8162fa55` | no image reference left; node_exporter runs as a host unit (`node-exporter.service`) |
| `victoriametrics/victoria-metrics` v1.155.0 (second use), v1.99.0 | `monitoring/docker-compose.yml`, `docker/telemetry/` | `2613b0c2`, `8162fa55` | v1.155.0 once, in `docker-compose.yml` |
| `prom/prometheus` v2.51.0 | `docker/telemetry/` | `8162fa55` | v3.15.1 in `monitoring/docker-compose.yml` (rule evaluator) and `deploy/sophia-eye`; v2.50.1 in `tomb/dark-mirror` |

Counted the way ADR-117 counts (what the default stacks run): the two compose stacks on west go from nine third-party observability containers to six (main stack: Grafana, VictoriaMetrics, ClickHouse, Vector; alert path: Prometheus, Alertmanager). Loki, the second Grafana and the second VictoriaMetrics are gone from the files. ADR-117 says "four images fewer": the fourth is Prometheus, which stays until its rules have another evaluator (ADR-117 note of 2026-11-11), and Promtail was never in the file that ran.

## Not counted by the SBOM gate

`docs/policy/sbom-count-baseline.txt` counts Go, Rust and Python components. Container images are not in it, so this drop does not move those three numbers.

## Stopped at run time

Grafana 11 downloads app plugins from grafana.com at start (`grafana-lokiexplore-app`, `grafana-pyroscope-app` were found in the running container). That is third-party code arriving unreviewed. `GF_PLUGINS_PREINSTALL_DISABLED: "true"` in `docker-compose.yml` (`9c815de8`) stops it. The 10.4 Grafana installed `grafana-piechart-panel` at every start (`GF_INSTALL_PLUGINS`); that goes with the container, and no kept dashboard uses a panel type outside Grafana's built-ins.

## Left for a verdict (Stevie)

- `scripts/bootstrap-llm-lab.sh` pulls `prom/prometheus`, `grafana/grafana`, `grafana/loki`, `grafana/promtail` and `victoriametrics/victoria-metrics` at `:latest`.
- `lxd/telemetry/*.yaml` download Grafana 10.2.2 and Loki 2.9.5 release archives.
- `deploy/sophia-eye` has its own Prometheus, tag only (DP-207: fold into the main store).
- `tomb/dark-mirror` keeps its own Grafana, Prometheus, Loki and Promtail.
