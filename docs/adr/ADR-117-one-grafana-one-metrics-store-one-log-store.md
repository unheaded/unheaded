<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-117: One Grafana, one metrics store, one log store

**Status:** Proposed (parked under ADR-112). Stevie asked for this ADR on 2026-11-03 (ADR-115 Q7: "yes, write the store-convergence ADR"). Nothing is built.
**Date:** 2026-11-03
**Grounded on:** `develop` at `222f2fcd`, plus the stopped containers on west.
**Related:** ADR-115 (smallest install; Studio's size depends on this), ADR-113 (SBOM, line count must trend down), ADR-116 (monitor the monitors), S-LOG-HARVEST D1 (Vector → ClickHouse stays, configure only), ADR-100 (alerting), third-party policy of 2026-10-29.

## Context

ADR-115 measured the default stacks and found the same job done twice or three times:

| Store | Main stack (`docker-compose.yml`) | Monitoring stack (`docker/telemetry/docker-compose.telemetry.yml`) | Also |
|---|---|---|---|
| Dashboards | `grafana` 11.5.1, 512 M | `grafana` 10.4.0 (`unheaded-monitoring-grafana`) | `kubernetes/manifests/base/telemetry/grafana.yaml` 10.4.0 |
| Metrics | `victoria` 1.108.1, 512 M; scrapes `config/victoria/scrape.yml` | `prometheus` 2.51.0 (live container runs 3.15.0) and `victoriametrics` 1.99.0 (live container runs 1.108.1) | `deploy/sophia-eye` has its own `prometheus` 3.15.0; k8s has both |
| Logs | `clickhouse` 24.12 (2 G) fed by `vector`; `clickhouse-schema` is the init job on the same image | `loki` 2.9.0 (live 3.0.0) fed by `promtail` | k8s has loki + promtail and clickhouse |

Consequences today:
- Four third-party images exist only because of the duplication (prometheus, loki, promtail, the second grafana), against ADR-113's rule that the SBOM shrinks every sprint.
- The live containers on west drift from the compose pins (prometheus 3.15 vs 2.51, loki 3.0 vs 2.9, victoriametrics 1.108 vs 1.99): the monitoring stack was upgraded by hand and the file never followed.
- Two Grafanas means two logins, two sets of dashboards, two things ADR-114's gate must front, and two places an operator looks.
- About 2.5 GiB of memory ceilings (Prometheus 1 G, the second VictoriaMetrics 1 G, Loki 512 M) on hosts where east has 7 GiB in total.
- Every alert rule exists in whichever store the author happened to use.

S-LOG-HARVEST D1 (2026-11-03) already fixed the log path: Vector → ClickHouse, configure only. That decides the log store.

## Decision direction (Claude's recommendation, nothing decided)

**One Grafana (11.x, the main stack's), VictoriaMetrics as the only metrics store, ClickHouse as the only log store.** Retire Prometheus, the second VictoriaMetrics, Loki, Promtail and the 10.4 Grafana.

Why this way round and not Prometheus + Loki:
- ClickHouse is where the audit and compliance records already land (ADR-092, S-LOG-HARVEST); moving them is a sprint, deleting Loki is an afternoon.
- VictoriaMetrics reads Prometheus scrape configs as they are (`-promscrape.config`), so `prometheus.yml` ports by copying the `scrape_configs` block into `config/victoria/scrape.yml`. It also stores more per byte and already holds the Kingdom's metrics.
- Grafana 11 is the newer of the two; its provisioning tree (`config/grafana/provisioning/`) already has the VictoriaMetrics datasource. The telemetry stack's dashboards move in as provisioned JSON.

Steps, when the freeze lifts (one short sprint, no new third party):
1. Inventory what only the monitoring stack has: its dashboards, Prometheus alert rules, Loki queries in use, node-exporter scrape (node-exporter stays; it is a source, not a store).
2. Port scrape configs to `config/victoria/scrape.yml`; port dashboards to `config/grafana/provisioning/dashboards/`; point every Loki panel at ClickHouse (Vector already ships the same journald stream there).
3. Alert rules: the ones that page go to the native path (alert-mailer, healthprobe, ADR-116's warden), not to a `vmalert` image. Rules that only draw lines become Grafana alerts or are dropped.
4. Retire `docker/telemetry/`, the k8s telemetry manifests for prometheus/loki/promtail/grafana, and `deploy/sophia-eye`'s prometheus (point it at the main VictoriaMetrics). Remove the stopped containers and their volumes on west after a backup of the Grafana database.
5. Re-pin: whatever lives in a compose file is what runs; `check-compose-log-caps.sh` or a sibling gate fails on image drift between the file and the running container.
6. SBOM: four images fewer; record the drop in `docs/policy/third-party-inventory-*.md`.

Studio (ADR-115) is then "Shop + VictoriaMetrics + Grafana + ClickHouse", three images, not seven.

## Alternatives

- **Keep both stacks, document the split.** Costs nothing now, keeps every item in the Context table, and makes Studio seven images. Rejected by ADR-113's direction.
- **Prometheus + Loki as the singles.** Fewer unfamiliar query languages for newcomers; but the audit records would have to leave ClickHouse, and Vector → ClickHouse was just decided (D1).
- **Mimir / Thanos / a long-term store.** New third party, no need at this scale.

## Open questions for the brainstorm

1. Which monitoring-stack dashboards are actually used? (Grafana's own usage stats answer this once the stack is up again.)
2. Do any Prometheus alert rules page today? If so they move to the native path first, before Prometheus goes.
3. The k8s manifests under `kubernetes/manifests/base/telemetry/`: converge them the same way, or delete them with the other k8s material the 2026-11-03 sitting removed (ingress)?
4. `deploy/sophia-eye`'s Prometheus: fold into the main VictoriaMetrics, or keep as a research-only store outside the default stacks?
5. Retention: one number for metrics and one for logs, or per-source (the Pi-hole query log is personal data; S-LOG-HARVEST Step 28)?

## Decision

None yet. Parked under ADR-112.
