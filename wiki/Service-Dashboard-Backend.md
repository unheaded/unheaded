# Dashboard Backend — Metrics + WebSocket

Real-time metrics aggregation and WebSocket push for the dashboard UI. Bridges eBPF trace data and service metrics to the browser.

| | |
|---|---|
| Container | `unheaded-dashboard` (live on west), port 20000 |
| Pages | overview (req/s, 5xx %, p50/p95/p99 from `/api/v1/metrics/summary`), flow graph, latency, logs (live tail), `compliance.html` |
| Live view | every viewer sees the same live events, one copy per message, no replay on connect |
| Writes to The Well | `ops_writer`: Akira's health state (`service_health_current` / `_transitions`) |
| Memory | `GOMEMLIMIT` set; ~210-235 MiB plateau after the 2026-09 OOM fixes |

---

> **Source:** [cmd/dashboard-backend/](../cmd/dashboard-backend/)
