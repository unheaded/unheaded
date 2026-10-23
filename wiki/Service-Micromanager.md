# Micromanager — Execution & QA Service

VP Engineering + CTO + Staff TPM + Senior PM. Hyper detail-oriented. Test EVERYTHING.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `GET /ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |
| `GET`, `POST /api/v1/tasks` | List or create tasks |
| `GET`, `PUT /api/v1/tasks/{id}` | Read or update a task |
| `GET /api/v1/backlog` | Backlog |
| `GET /api/v1/sprint/status` | Sprint counts by status |
| Port: 19003 | |

## State

Tasks live in `micromanager-tasks.json` under `-data-dir` / `MICROMANAGER_DATA_DIR` (compose: the `micromanager-data` volume at `/data`; deb: `/var/lib/unheaded/micromanager`). Same rules as Architect: atomic saves, 503 and undo on a failed save, 503 for every write with no data dir, a damaged file stops startup. Updates apply to a copy, so a request rejected on one field changes nothing. Until 2026-10-23 tasks were in memory only.

---

> **Source:** [services/micromanager/](../services/micromanager/)
