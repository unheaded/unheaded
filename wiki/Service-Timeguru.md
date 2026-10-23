# Timeguru — Timeline Tracking Service

The living roadmap. Tracks the Unheaded journey through Ages, Epochs, Features, and Tasks.

## Endpoints

| Endpoint | Purpose |
|----------|---------|
| `GET /health` | Health check |
| `GET /ready` | Readiness probe |
| `GET /metrics` | Prometheus metrics |
| `GET /api/v1/timeline`, `/timeline` | Current timeline; `?format=` or `Accept` picks JSON, YAML, TOML or Markdown |
| `POST /api/v1/timeline/sync` | Write the JSON/YAML/TOML mirrors to `SYNC_DIR` (503 if unset) |
| `POST /api/v1/timeline/import` | Replace the timeline from a JSON/YAML/TOML body |
| `GET /api/v1/timeline/tasks` | Milestones as Kanban tasks |
| `GET /api/v1/timeline/stream` | SSE stream of timeline updates |
| `GET /api/v1/milestones`, `/milestones` | Milestones |
| `/milestones/{id}/update` | Update one milestone |
| Port: 19000 (`PORT`) | |

## Configuration

| Env | Default |
|-----|---------|
| `PORT` | `19000` |
| `WOTAN_ADDR` | `localhost:18000` (HTTP) |
| `WOTAN_GRPC_ADDR` | `localhost:18001` |
| `DB_PATH` | `./data/timeguru.db` |
| `TIMELINE_PATH` | `./references/timeline.md` |
| `SYNC_DIR` | directory of `TIMELINE_PATH` |

A missing timeline returns 404 and `/ready` reports not ready.

---

> **Sources:** [services/timeguru/](../services/timeguru/) · [references/timeline.md](../references/timeline.md)
