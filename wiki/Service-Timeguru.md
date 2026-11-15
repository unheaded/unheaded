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
| `WELL_HOST`, `WELL_PORT`, `WELL_USER`, `WELL_PASSWORD`, `WELL_DB`, `WELL_SSLMODE`, `WELL_SSLROOTCERT` | `localhost`, `5432`, `app_timeguru`, none, `unheaded_app`, `require`, none |
| `TIMELINE_PATH` | `./references/timeline.md` |
| `SYNC_DIR` | directory of `TIMELINE_PATH` |

A missing timeline returns 404 and `/ready` reports not ready.

**One home, the Well (ADR-147).** The timeline lives in memory, built from `TIMELINE_PATH` at start and on change, and is written to `unheaded_app.timeline_milestones` (migration 050, one row per milestone) as `app_timeguru`. A milestone changed through the API after the file's modification time keeps its status and progress over the file at the next start. Without the Well timeguru serves the file read-only: updates and imports answer 503 `{"error":"the Well is unavailable"}`, `/health` says `"well":"waiting"`, and it retries every 1 s, doubling to 30 s. A Well without migration 050 stops it. There is no SQLite file any more (it lived on a tmpfs, so every restart lost the API's changes).

---

> **Sources:** [services/timeguru/](../services/timeguru/) · [references/timeline.md](../references/timeline.md)
