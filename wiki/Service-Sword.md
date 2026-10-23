# Sword: deploys through Cuirass

Sword runs deployments of a service into LXD, through [[Cuirass|Containers]]: rolling, blue-green, canary and recreate, with automatic rollback when one fails. It never creates or deletes an instance itself; Cuirass does, and Sword counts an instance as up only when Cuirass's own health probe says so. S-REAL-STEEL Phase 8.

**Status:** code and tests landed on `sprint/real-steel`; not run against real LXD yet (the live proof waits on the E2 window, battle plan Phase 9). Cuirass stays on its mock backend until then.

## Running

| | |
|---|---|
| Binary | `cmd/sword` (`services/sword`) |
| Port | 19011 (`pkg/ports.Sword`; was Canary's port, Canary folds into Sword) |
| Account | uid 16746 (`pkg/uids.Sword`) in compose; `unheaded-sword` (sysusers) for the host unit |
| Compose | service `sword`, profile `deploy`: off by default, `docker compose up -d sword` starts it |
| Host unit | `deploy/systemd/unheaded-sword.service` (the fallback if a compose container cannot reach instance IPs); not installed |
| State | `SWORD_DATA_DIR/sword.json`: the deployment history, one JSON file, replaced atomically (`pkg/statefile`) |

## Environment

| Variable | |
|---|---|
| `CUIRASS_URL` | default `http://127.0.0.1:19006` (compose: `http://cuirass:19006`) |
| `CUIRASS_API_KEY` | required; Sword will not start without it. On the host: `/etc/unheaded/sword/sword.env`, 0600, never in the repo |
| `SWORD_DATA_DIR` | unset: read-only, every deployment request answers 503 |
| `VM_URL` | VictoriaMetrics for the SLO guard. Unset: no guard, so canary deployments are refused |
| `SWORD_SLO_POLICY` | the alerting policy (`observability/alerting/policy.yaml`); unreadable with `VM_URL` set: Sword will not start |
| `AUTH_ENABLED`, `AUTH_API_KEYS` | `pkg/auth`. Without `AUTH_ENABLED=true` every POST answers 403 |
| `WOTAN_ADDR`, `WOTAN_GRPC_ADDR` | events on `deploy.*` (best effort; Sword joins as `sword`) |

## API

| | |
|---|---|
| `POST /api/v1/deployments` | start one; 202 with the record. 400 bad request, 403 no auth, 409 the service is busy, 503 no data dir or no Cuirass |
| `GET /api/v1/deployments` | history, oldest first |
| `GET /api/v1/deployments/{id}` | one record: status, reason, message, previous version, the last canary analysis |
| `POST /api/v1/deployments/{id}/rollback` | back to the previous successful deployment of another version; 202 |
| `GET /health` | 200 while the process runs |
| `GET /ready` | 200 only when the data dir takes a write and Cuirass answers |
| `GET /metrics` | `unheaded_http_*`, `sword_deployments_total{strategy,result}`, `sword_rollbacks_total{reason}` |

API keys go in `X-API-Key`.

```json
{"service":"steel-echo","version":"v4","replicas":3,
 "strategy":{"type":"canary","steps":[{"weight":33,"pause":"60s","analysis":true},{"weight":100}],
             "analysis":{"error_rate_threshold":0.01,"min_sample_size":200,"interval":"30s"}},
 "health":{"port":8080,"path":"/health"},
 "proxy":{"listen":"tcp:127.0.0.1:26601","connect":"tcp:127.0.0.1:8080"},
 "labels":{"chaos":"allowed"}}
```

Versions are lowercase letters, digits and hyphens: instance `steel-echo-v4-0` runs image `steel-echo-v4`.

## Strategies

Instances are immutable (`<svc>-<ver>-<n>`): an update is a create and a delete. The service's stable endpoint is a Cuirass-managed proxy device on one instance; moving it is the traffic switch.

| Strategy | What happens |
|---|---|
| `recreate` | delete every old instance, create the new ones, wait until all are healthy |
| `rolling` | `max_surge` new instances at a time; an old one is deleted only while `replicas - max_unavailable` stay healthy |
| `blue_green` | create all new instances, wait until healthy, move the endpoint once, then delete the old ones; a green side that never gets healthy is deleted and blue keeps serving |
| `canary` | start canary instances next to stable; at each analysis step measure them; pass: scale up, move the endpoint, delete stable; fail: endpoint stays on stable, canary deleted, record `rolled_back` / `analysis_failed` |

A deployment that fails (not a canary that rolled itself back) is rolled back to the previous successful deployment: record `rolled_back`, reason `deploy_failed`. With no earlier success it stays `failed`.

## Canary analysis

Each interval Sword reads the canary and stable instances' own `/metrics` twice, one interval apart, and judges the canary on the deltas of `unheaded_http_requests_total` (5xx are errors) and `unheaded_http_request_duration_seconds` (p99 from the bucket deltas). `/health`, `/ready` and `/metrics` requests are left out. Fewer than `min_sample_size` canary requests in an interval holds; three such intervals in a row fail the analysis. The strategy then applies `error_rate_threshold` and `latency_p99_threshold`.

The **Kingdom SLO guard** (`pkg/sloguard`, ADR-100) runs before and after each measurement: every `page: true` SLO's `slo:error_ratio:5m` must be under 14.4 × (1 − objective). A burning SLO, a missing series, or Victoria not answering fails the analysis.

Instances are scraped at the address Cuirass reports, on the health port; loopback, link-local, unspecified and multicast addresses are refused, no redirects are followed, no proxy from the environment is used.

## How rollback picks the version

The newest deployment of the service recorded before the one being rolled back, with status `succeeded` and a different version. The rollback is a rolling update (one at a time, nothing unavailable) through the same driver.

---

> **Source:** [services/sword](../services/sword) · [cmd/sword](../cmd/sword/main.go) · [pkg/deploy/cuirassdriver](../pkg/deploy/cuirassdriver) · [pkg/deploy/strategy](../pkg/deploy/strategy) · [pkg/sloguard](../pkg/sloguard) · [S-REAL-STEEL battle plan](../docs/battle-plans/S-REAL-STEEL-battle-plan.md)
