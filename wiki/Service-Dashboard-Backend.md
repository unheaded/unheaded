# Dashboard Backend — Metrics + WebSocket

WebSocket push and the API behind the dashboard UI. Bridges eBPF trace data from Wotan and service metrics from VictoriaMetrics to the browser.

| | |
|---|---|
| Container | `unheaded-dashboard` (live on west), port 20000 |
| Pages | overview (req/s, 5xx %, p50/p95/p99 from `/api/v1/metrics/summary`), flow graph, latency, logs (live tail), `compliance.html`, `verdicts.html` (`/verdicts`: the third-party inventory with Stevie's verdict on each item, read-only; data from `GET /api/v1/verdicts`, which is the dated record `docs/policy/third-party-inventory-2026-10-29.md` embedded in the binary and held to it by a test, ADR-113) |
| Live view | every viewer sees the same live events, one copy per message, no replay on connect |
| Reads | every metric from VictoriaMetrics (`--vm-url`, compose `http://victoria:8428` on `observe`; the PromQL is in `internal/server/vmqueries.go`); events from Wotan (the eBPF ingestor holds the one subscription to each `traces.*` topic and feeds both the flow graph and the trace view); host details from each host's huginn `/host-summary` when the page asks; service health from its own monitor, every 15 s, the one poller it runs |
| Keeps | no time series of its own (ADR-147 decision 9): the private scrape store, the metrics aggregator, the 2 s `/health` poller and the host gauges with their VictoriaMetrics push are gone (huginn pushes `host_*` itself) |
| VictoriaMetrics down | the metric endpoints (`/api/v1/metrics`, `/metrics/summary`, `/metrics/query`, `/aggregated`, `/latency` without eBPF) answer 503 `{"error":"victoriametrics unavailable"}`, never 200 with zeros; so does a stack started without `--vm-url` |
| Logged out | a 401 stops the page: it clears every refresh interval, closes its WebSocket without reconnecting and says "Logged out: log in and reload" (it used to poll on: 56% of a week's answers were 401) |
| Writes to The Well | `ops_writer`: Akira's health state (`service_health_current` / `_transitions`) |
| Memory | `GOMEMLIMIT` set; ~210-235 MiB plateau after the 2026-09 OOM fixes |
| Login | `AUTH_ENABLED=true` with `AUTH_ISSUER_URL`: people log in on the page (`/auth/login`, [unheaded-auth](Service-Auth)) and hold an HttpOnly session cookie. Every `/api/` call and WebSocket then needs a login (viewer reads, operator writes) or a service token (`AUTH_ALLOW_CALLERS=unheaded-cli`); the pages themselves stay open |
| Compliance profile switch | `POST /api/v1/compliance/profile` `{host, profile, mode, reason}`: admin or operator login only, actor = the login; writes as `config_admin` (`WELL_CONFIG_ADMIN_PASSWORD`, not set in compose: 503 until it is) |

---

> **Source:** [cmd/dashboard-backend/](../cmd/dashboard-backend/)
