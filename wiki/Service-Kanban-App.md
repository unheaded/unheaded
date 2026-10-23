# Kanban App — The Meta Moment

The self-hosting proof of concept. Displays Unheaded development progress using Unheaded infrastructure.

| | |
|---|---|
| Container | `unheaded-kanban` (live on west), port 20001 |
| Cards | The Well, `unheaded_app.kanban_tasks` as role `app_kanban` over TLS; a SQLite L1 cache serves reads. `getInitialTasks()` only seeds Genesis cards |
| Live updates | Wotan streams; editing `references/timeline.md` updates the board with no restart (working in compose since 2026-09-21) |
| Writes | need the database: without it, the board is read-only |

## API

| Route | What |
|-------|------|
| `GET/POST/PUT/DELETE /api/v1/tasks` | List / create / update / delete cards (`/api/v1/timeline/tasks` is a legacy alias) |
| `GET/PUT/DELETE /api/v1/tasks/{id}` | One card; `POST …/{id}/restore` brings back a deleted card |
| `GET /api/v1/tasks/{deleted,archived,completed}` | Filtered views |
| `GET /api/v1/timeline`, `/api/v1/timeline/cards` | Timeline from Timeguru, as cards |
| `GET /api/v1/stream` (SSE), `/ws` | Live updates to every open board |
| `/health`, `/ready`, `/metrics`, `/api/v1/health` | Probes, Prometheus |

Card changes publish to `tasks.*` on Wotan ([[Fae Chamber Contracts|Fae-Chamber-Contracts]]).

## Hardening

Per-IP token bucket (120 req/min, burst 30), 1 MB request limit, strict field validation, restrictive CORS, CSP and frame headers, `X-Request-ID` on every request. Code: `main.go` (handlers), `middleware.go`, `pgstore.go` (The Well), `store.go` (SQLite cache), `timeline.go`, `wotan.go`; front end in `kanban/`.

Decisions: ADR-020 (bug fixes), ADR-025 (mobile), ADR-038 (git audit trail), ADR-041 (timeline sync), ADR-044 (task detail / status query).

---

> **Source:** [cmd/kanban-app/](../cmd/kanban-app/)
