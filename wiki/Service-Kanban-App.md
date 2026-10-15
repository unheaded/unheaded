# Kanban App — The Meta Moment

The self-hosting proof of concept. Displays Unheaded development progress using Unheaded infrastructure.

| | |
|---|---|
| Container | `unheaded-kanban` (live on west), port 20001 |
| Cards | The Well, `unheaded_app.kanban_tasks` as role `app_kanban` over TLS; a SQLite L1 cache serves reads. `getInitialTasks()` only seeds Genesis cards |
| Live updates | Wotan streams; editing `references/timeline.md` updates the board with no restart (working in compose since 2026-09-21) |
| Writes | need the database: without it, the board is read-only |

---

> **Source:** [cmd/kanban-app/](../cmd/kanban-app/)
