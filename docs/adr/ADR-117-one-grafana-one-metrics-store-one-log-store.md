<!--
SPDX-License-Identifier: GPL-3.0-or-later
Copyright (c) 2024-2026 Stevie Bellis.
-->
# ADR-117: One Grafana, one metrics store, one log store

**Status (2026-11-11):** Accepted for work, in part. Stevie, 2026-11-11, after asking why west has two Grafanas: "go for it in background". Built on branch `sprint/s-one-grafana` and applied to no host: one Grafana, one metrics store, the three duplicates out of the files. Prometheus stays as the rule evaluator; its retirement is an open question. See "Status 2026-11-11" at the end. The next line is the 2026-11-03 status, kept as written.
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

## Decisions 2026-11-03 (decision pack, round 2)

From `docs/battle-plans/DECISION-PACK-2026-11-03.md` (DP ids in parentheses).

- E1 brings both compose stacks up exactly as pinned; this convergence is its own sprint later (DP-19). Q1-Q5 are DP-207, still open.

## Decisions 2026-11-04 (decision pack, round 3)

- DP-207: the leanest, cleanest long-term setup, polish over shortcuts: keep every dashboard worth keeping, rebuild paging rules on the native path before Prometheus retires, fold sophia-eye into the main VictoriaMetrics, retention per retention-policy-v1. Log store: ClickHouse through E1, then a measured week shipping to both, Postgres (the Well) favoured if the numbers are close; this may reopen S-LOG-HARVEST D1.

## Status 2026-11-11

Stevie's word on 2026-11-11 ("go for it in background") took this ADR out of the freeze for the Grafana and metrics halves. Work is on `sprint/s-one-grafana`; applying it to west is `docs/runbooks/one-grafana-cutover.md`. What west ran before: `docs/sweeps/ONE-GRAFANA-INVENTORY-2026-11-11.md`.

Against the steps above:

| Step | State |
|---|---|
| 1 Inventory | Done. The Context table's monitoring column names `docker/telemetry/`; the stack that runs is `monitoring/docker-compose.yml`. Both Grafanas served the same 14 files from the main store; Loki held 0 streams; the second VictoriaMetrics held 0 series |
| 2 Port scrapes and dashboards | Done. `config/victoria/scrape.yml` scrapes both hosts (21 jobs, 27 targets); eight dashboards provisioned under `config/grafana/provisioning/dashboards/kingdom/`; seven with no emitter were not ported. No Loki panel existed to repoint |
| 3 Alert rules to the native path | **Not done: there is no native path to move them to.** See the question below |
| 4 Retire | Done in the files for the second Grafana, Loki, the second VictoriaMetrics and `docker/telemetry/`. `kubernetes/` was already gone. `deploy/sophia-eye`'s Prometheus is untouched. Containers and volumes on west go in the runbook, after a copy of the Grafana database |
| 5 Re-pin | Done: `scripts/check-image-drift.sh` compares running containers with the compose pins |
| 6 Record the drop | Done: `docs/policy/third-party-inventory-2026-11-11-one-grafana.md`. Three duplicates gone, not four: Prometheus stays |

Answers to the open questions: Q1, none of the monitoring stack's dashboards was unique to it. Q2, yes: `policy.yaml` pages on SLO burn and on `HeartbeatMissing`, all evaluated by Prometheus. Q3, moot. Q4 and Q5, unchanged (DP-207).

### Question for Stevie: what evaluates the alert rules

84 alerting and 35 recording rules run in Prometheus. Nothing in the tree can take them: `cmd/healthprobe` is a container health check, `cmd/alert-policy` generates files, ADR-116's warden is not built, and single-node VictoriaMetrics evaluates no alerting rules. DP-207 says paging rules are rebuilt on the native path before Prometheus retires.

- **A. Keep Prometheus as the evaluator until ADR-116's warden exists (recommended).** Nothing new, nothing to build now. One image and its 1 G ceiling stay. Prometheus already forwards only rule results to the store.
- **B. Build a native evaluator now.** First-party Go: read `monitoring/prometheus/rules/*.yml`, query the store, keep `for:` state, post to Alertmanager. Prometheus goes. A sprint of its own, and it is the thing that pages, so it needs ADR-116's own watchdog first.
- **C. `vmalert`.** Least work. A new third-party image, which step 3 above rules out and the 2026-10-29 policy needs confirmed.
- **D. Grafana alerting.** No new image. Rules move into Grafana, `cmd/alert-policy` has to emit Grafana provisioning, and the Grafana whose state is on tmpfs becomes the thing that pages.

### Smaller questions

1. Grafana reports usage and checks for updates at grafana.com by default. Turn both off in `docker-compose.yml`?
2. Log panels in Grafana need the ClickHouse datasource plugin, which is third party. Confirm it, or keep logs in the dashboard's own log page?
3. The seven dashboards with no emitter (eBPF pipeline, LLM inference, services, load balancers, container fleet, firewall, routing): rebuild each when its emitter exists, or leave them in history?
4. `deploy/sophia-eye`'s Prometheus: fold into the main store when that stack next runs (DP-207), or now?
