# Sword: deploys through Cuirass

Sword runs deployments of a service into LXD, through [[Cuirass|Containers]]: rolling, blue-green, canary and recreate, with automatic rollback when one fails. It never creates or deletes an instance itself; Cuirass does, and Sword counts an instance as up only when Cuirass's own health probe says so. S-REAL-STEEL Phase 8 (code), Phase 9 and game day 1 (live).

**Status:** live on west since 2026-10-26 (compose, profile `deploy`), against Cuirass on the LXD HTTPS API in project `unheaded`. All four strategies and both rollback paths were run on real instances with the numbers below. East does not run Sword.

## Running

| | |
|---|---|
| Binary | `cmd/sword` (`services/sword`) |
| Port | 19011 (`pkg/ports.Sword`; was Canary's port, Canary folded into Sword and was deleted with E1 G3) |
| Account | uid 16746 (`pkg/uids.Sword`) in compose; `unheaded-sword` (sysusers) for the host unit |
| Compose | service `sword`, profile `deploy`: off by default. Window: `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml up -d --build sword cuirass` |
| Env file | `/etc/unheaded/sword/sword.env`, 0640 root:16746, created by hand on west: `CUIRASS_API_KEY`, `AUTH_API_KEYS`. Never in the repo |
| Host unit | `deploy/systemd/unheaded-sword.service` (fallback if a compose container could not reach instance IPs; H17 was TRUE, so it is not installed) |
| State | `SWORD_DATA_DIR/sword.json` (volume `sword-data`): the deployment history, one JSON file, replaced atomically (`pkg/statefile`). Survives `docker restart unheaded-sword` (6 records listed after the Phase 9 restart) |

**Turning it off:** `sudo docker compose -f docker-compose.yml -f docker/compose.cuirass-lxd.yml stop sword` (the history volume stays), or close the whole window with `... stop`. Instances Sword created are desired state in Cuirass and keep running; remove them with a `recreate` to zero replicas or `DELETE $CUIRASS/api/v1/containers/<id>` (prune deletes the instances).

## Environment

| Variable | |
|---|---|
| `CUIRASS_URL` | default `http://127.0.0.1:19006` (compose: `http://cuirass:19006`) |
| `CUIRASS_API_KEY` | required; Sword will not start without it |
| `SWORD_DATA_DIR` | unset: read-only, every deployment request answers 503 |
| `VM_URL` | VictoriaMetrics for the SLO guard. Unset: no guard, so canary deployments are refused |
| `SWORD_SLO_POLICY` | the alerting policy (`observability/alerting/policy.yaml`, default `/etc/unheaded/sword/policy.yaml`); unreadable with `VM_URL` set: Sword will not start |
| `AUTH_ENABLED`, `AUTH_API_KEYS` | `pkg/auth`. Without `AUTH_ENABLED=true` every POST answers 403 |
| `WOTAN_ADDR`, `WOTAN_GRPC_ADDR` | events on `deploy.started|progress|succeeded|failed|rolled_back` (best effort; Sword joins as `sword` before publishing) |

## API

| | |
|---|---|
| `POST /api/v1/deployments` | start one; 202 with the record. 400 bad request, 401/403 no auth, 409 the service is busy (one operation per service), 503 no data dir or no Cuirass |
| `GET /api/v1/deployments` | history, oldest first |
| `GET /api/v1/deployments/{id}` | one record: `status`, `reason`, `message`, `previous_version`, `rolled_back_to`, the last canary `analysis` |
| `POST /api/v1/deployments/{id}/rollback` | back to the previous successful deployment of another version; 202, reason `manual` |
| `GET /health` | 200 while the process runs |
| `GET /ready` | 200 only when the data dir takes a write and Cuirass answers |
| `GET /metrics` | `unheaded_http_*`, `sword_deployments_total{strategy,result}`, `sword_rollbacks_total{reason}` |

API keys go in `X-API-Key`.

### Request body

```json
{"service":"steel-echo","version":"v3","replicas":3,
 "strategy":{"type":"canary",
             "steps":[{"weight":33,"pause":"60s","analysis":true},{"weight":100}],
             "analysis":{"error_rate_threshold":0.01,"latency_p99_threshold":"100ms",
                         "min_sample_size":100,"interval":"30s"}},
 "health":{"port":8080,"path":"/health"},
 "proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},
 "labels":{"chaos":"allowed"}}
```

`strategy.type` is `recreate`, `rolling` (`max_surge`, `max_unavailable`), `blue_green` or `canary` (`steps[]`, `analysis`); `progress_deadline` caps a wait (default 10 m). `health` is required: it is how a create is confirmed. `proxy` is the stable endpoint (user-apps band 26000-26666 on `127.0.0.1`). Versions are lowercase letters, digits and hyphens: instance `steel-echo-v3-0` runs image `steel-echo-v3`.

### Record statuses

`pending` → `running` → `succeeded`, or `rolling_back` → `rolled_back` with `reason` `analysis_failed` (canary), `deploy_failed` (automatic) or `manual`; `failed` when there is no earlier success to return to. A record left `running` by a restart is marked `failed` with that reason.

## Strategies

Instances are immutable (`<svc>-<ver>-<n>`): an update is a create and a delete. The service's stable endpoint is a Cuirass-managed proxy device on one instance; moving it is the traffic switch (one `POST /api/v1/reconcile` on Cuirass, so a cutover does not wait for the 30 s sync).

| Strategy | What happens | Measured on west, 2026-10-26 (steel-echo x3, 20 rps client meter) |
|---|---|---|
| `recreate` | delete every old instance, create the new ones, wait until all are healthy | baseline v1 x3, `succeeded` "3 instances running v1" |
| `rolling` | `max_surge` new instances at a time; an old one is deleted only while `replicas - max_unavailable` stay healthy | v1→v2 (surge 1, unavailable 0): 75 s; availability 0.9915 (1408/1420), `versions_seen` only v1 / v1;v2 |
| `blue_green` | create all new instances, wait until healthy, move the endpoint once, then delete the old ones; a green side that never gets healthy is deleted and blue keeps serving | v2→v3: version flipped exactly once on the proxy; cutover gap 1 s (one second with `errconn > 0`) |
| `canary` | start canary instances next to stable; at each analysis step measure them; pass: scale up, move the endpoint, delete stable; fail: endpoint stays on stable, canary deleted, record `rolled_back` / `analysis_failed` | v3→v4 (bad build): "Error rate 35.33% exceeds threshold 1.00%" (53/150), canary healthy → rollback complete 54 s. Game day 1: v2→v3 under a 300 ms netem on the canary, "P99 latency 310 ms exceeds 100 ms" (client-observed), fault start → rollback complete 62.4 s |
| manual rollback | `POST .../{id}/rollback` of the blue-green deployment | `rolling_back` 80 s, back on v2 (instances, proxy `/version`, record `rolled_back` reason `manual`) |

A deployment that fails (not a canary that rolled itself back) is rolled back to the previous successful deployment: record `rolled_back`, reason `deploy_failed`. With no earlier success it stays `failed`.

Raw numbers: `docs/gamedays/data/phase9-2026-10-26/{phase9,bg}.csv`, `docs/gamedays/data/gameday1-2026-10-26/gameday.csv`; the per-step record is S-REAL-STEEL Phase 9 and [[Game Days|Game-Days]].

## Canary analysis

Each interval Sword reads the canary and stable instances' own `/metrics` twice, one interval apart, and judges the canary on the deltas of `unheaded_http_requests_total` (5xx are errors) and `unheaded_http_request_duration_seconds` (p99 from the bucket deltas). `/health`, `/ready` and `/metrics` requests are left out. During the interval it also probes each canary's `/health` itself and keeps the round trips: a network fault between Sword and the canary never reaches the instance's own histogram (game day 1 run A1 promoted a canary under a 300 ms netem because the histogram said 5 ms). Since `d90d613d` p99 is the worse of the two readings and run A3 rolled back on 310 ms.

Fewer than `min_sample_size` canary requests in an interval holds; three such intervals in a row fail the analysis (Phase 9 run #5: "inconclusive after 3 intervals: 150 canary requests in 30s, need 200", so size `min_sample_size` to the traffic: a 20 rps meter puts about 5 rps on a 33 % canary). The strategy then applies `error_rate_threshold` and `latency_p99_threshold`.

Fields of `analysis` in the record (`strategy.AnalysisResult`): `pass`, `message`, `should_rollback`, `metrics{error_rate, success_rate, request_count, error_count, latency_p50, latency_p99, client_latency_p99}`. `latency_p99` is the worse of histogram and probes; `client_latency_p99` is the probes alone (absent when nothing was probed).

The **Kingdom SLO guard** (`pkg/sloguard`, ADR-100) runs before and after each measurement: every `page: true` SLO's `slo:error_ratio:5m` must be under 14.4 × (1 − objective). A burning SLO, a missing series, or Victoria not answering fails the analysis (fail closed). The series come from the monitoring compose stack (`monitoring/docker-compose.yml`, Prometheus evaluating `rules/slo.yml`), which nothing starts at boot: with it down, every canary rolls back on the guard (Phase 9 run #4, FND-024 in the ADR-098 register). Bring it up first and wait for `slo:error_ratio:5m` to populate.

Instances are scraped and probed at the address Cuirass reports, on the health port; loopback, link-local, unspecified and multicast addresses are refused, no redirects are followed, no proxy from the environment is used.

## How rollback picks the version

The newest deployment of the service recorded before the one being rolled back, with status `succeeded` and a different version. The rollback is a rolling update (one at a time, nothing unavailable) through the same driver.

---

> **Source:** [services/sword](../services/sword) · [cmd/sword](../cmd/sword/main.go) · [pkg/deploy/cuirassdriver](../pkg/deploy/cuirassdriver) · [pkg/deploy/strategy](../pkg/deploy/strategy) · [pkg/sloguard](../pkg/sloguard) · [S-REAL-STEEL battle plan](../docs/battle-plans/S-REAL-STEEL-battle-plan.md) (Phases 8, 9, 14) · [game day 1](../docs/gamedays/2026-10-26-real-steel-1.md)
