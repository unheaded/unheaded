# Dashboard Backend — Metrics + WebSocket

Real-time metrics aggregation and WebSocket push for the dashboard UI. Bridges eBPF trace data and service metrics to the browser.

| | |
|---|---|
| Container | `unheaded-dashboard` (live on west), port 20000 |
| Pages | overview (req/s, 5xx %, p50/p95/p99 from `/api/v1/metrics/summary`), flow graph, latency, logs (live tail), `compliance.html` |
| Live view | every viewer sees the same live events, one copy per message, no replay on connect |
| Writes to The Well | `ops_writer`: Akira's health state (`service_health_current` / `_transitions`) |
| Memory | `GOMEMLIMIT` set; ~210-235 MiB plateau after the 2026-09 OOM fixes |
| Login | `AUTH_ENABLED=true` with `AUTH_ISSUER_URL`: people log in on the page (`/auth/login`, [unheaded-auth](Service-Auth)) and hold an HttpOnly session cookie. Every `/api/` call and WebSocket then needs a login (viewer reads, operator writes) or a service token (`AUTH_ALLOW_CALLERS=unheaded-cli`); the pages themselves stay open |
| Compliance profile switch | `POST /api/v1/compliance/profile` `{host, profile, mode, reason}`: admin or operator login only, actor = the login; writes as `config_admin` (`WELL_CONFIG_ADMIN_PASSWORD`, not set in compose: 503 until it is) |

---

> **Source:** [cmd/dashboard-backend/](../cmd/dashboard-backend/)
