# Kanban App — The Meta Moment

The self-hosting proof of concept. Displays Unheaded development progress using Unheaded infrastructure.

| | |
|---|---|
| Container | `unheaded-kanban` (live on west), port 20001 |
| Cards | The Well, `unheaded_app.kanban_tasks` as role `app_kanban` over TLS; a SQLite L1 cache serves reads. The binary carries no cards (see below) |
| Live updates | Wotan streams; editing `references/timeline.md` updates the board with no restart (working in compose since 2026-09-21) |
| Writes | need the database: without it, the board is read-only |
| Login | Reading is public. Writing needs an operator login on the board (`/auth/login` against [unheaded-auth](Service-Auth), HttpOnly session cookie, `AUTH_ISSUER_URL`) or a service token from dashboard-backend, zhen-agentd or micromanager (`AUTH_ALLOW_CALLERS`) |

## API

| Route | What |
|-------|------|
| `GET/POST/PUT/DELETE /api/v1/tasks` | List / create / update / delete cards (`/api/v1/timeline/tasks` is a legacy alias) |
| `GET/PUT/DELETE /api/v1/tasks/{id}` | One card; `POST …/{id}/restore` brings back a deleted card |
| `GET /api/v1/tasks/{deleted,archived,completed}` | Filtered views |
| `GET /api/v1/timeline`, `/api/v1/timeline/cards` | Timeline from Timeguru, as cards |
| `GET /api/v1/runbooks` | The committed runbooks (`runbooks/MANIFEST.sha256`): front matter, `manifest_ok`, latest step progress |
| `POST /api/v1/tasks/from-runbook` | `{"runbook": "category/name"}`: a card of type `runbook`, written to the store like any other |
| `GET /api/v1/stream` (SSE), `/ws` | Live updates to every open board |
| `/health`, `/ready`, `/metrics`, `/api/v1/health` | Probes, Prometheus |

Card changes publish to `tasks.*` on Wotan ([[Fae Chamber Contracts|Fae-Chamber-Contracts]]).

`GET /api/v1/tasks/{id}` also carries `agent_run` (the card's newest `zhen_agent_runs` row) and, for a runbook card, `runbook` and `runbook_progress`.

## Runbooks view and agent runs

The **Runbooks** button lists every runbook in `runbooks/MANIFEST.sha256` (category/name, status, risk, steps; a file changed since the manifest is flagged). **Create task** makes a card of type `runbook` through the same store as every other card; the runbook travels in a fenced `runbook` block in the card description (`kanban_tasks` has no metadata column). The view reads `KANBAN_RUNBOOKS_DIR` (default `runbooks`, `/app/runbooks` in the image) through an `os.Root`, cached 30 s.

**Step progress.** `scripts/run-runbook.py` publishes one event per step on Wotan `runbooks.progress` (`{runbook, step, total, name, state, dry_run, ts}`; state running, ok, failed, manual, skipped, stopped, done) when `RUNBOOK_PROGRESS_WOTAN` names Wotan's HTTP address. It joins as `runbook-runner`, anonymously: with Wotan's auth on (S-IRON-GATE Phase 7) the join is refused until run-runbook.py sends a service token, so step progress is off on a stack with auth; a refused join or a down Wotan turns progress off for that run and never changes its outcome. The Champion's `runbook_execute` sets `RUNBOOK_REF`, so a live run of the private verified copy is still named. The board keeps the newest event per runbook in memory and forwards each as SSE `runbook.progress`: the list and an open card show `step k/n`. Without Wotan the board shows no progress.

**Agent run block.** The card modal shows the newest `zhen_agent_runs` row for the card: run id, mode (interactive, overnight, scheduled), status and check, the `zhen/<card>` branch with a copyable `git log --oneline zhen/<card>` (a scheduled run has none), head commit, the review note's path on the agentd host, and Stevie's review reason. `app_kanban` reads only those columns (migration 041; not the error text, cost or lease). Before 041 is applied the read is refused and cards show no agent run (one warning in the log).

ADR-038's commit-trailer hook is dormant: `zhen_agent_runs` links agent commits to cards; `scripts/kanban-git-link.sh` stays a manual scanner for `Task:` trailers.

## Where cards come from

The Well is the only source of cards. Add or change them through the API above; they persist in `kanban_tasks`.

A fresh Well gets the 65 Genesis cards (milestones, armor, services, tech debt, vision, wishlist) from migration `034_app_kanban_genesis.sql`, which inserts them only when `kanban_tasks` is empty: on a Well that already has a board it is a recorded no-op. The cards are reviewed in `db/seed/kanban-genesis.json`, with the dates git records for each (`scripts/kanban/genesis-timestamps.py`); 034 is rendered from it (`scripts/kanban/render-genesis-sql.py`) and `scripts/check-kanban-genesis.sh` fails in CI when the two differ. To change the Genesis cards for future Wells: edit the JSON, re-render, add the change as a new migration if 034 is already applied anywhere.

kanban-app itself carries no cards (until 2026-10-23 it seeded an empty store from a hardcoded list and invented the dates at startup). Without Postgres it uses SQLite; a new SQLite board starts empty unless `KANBAN_GENESIS_FILE` names a Genesis JSON, which then seeds that empty store once (dev runs only; set nowhere by default). Without any database the board is empty and read-only.

## Hardening

Per-IP token bucket (120 req/min, burst 30), 1 MB request limit, strict field validation, restrictive CORS, CSP and frame headers, `X-Request-ID` on every request. Code: `main.go` (handlers), `runbooks.go` (runbooks, progress, agent runs), `middleware.go`, `pgstore.go` (The Well), `store.go` (SQLite cache), `timeline.go`, `wotan.go`; front end in `static/` (embedded in the binary; `kanban/` at the repo root is an older scaffold).

Decisions: ADR-020 (bug fixes), ADR-025 (mobile), ADR-038 (git audit trail, dormant), ADR-041 (timeline sync), ADR-044 (task detail / status query), ADR-106 §6 (agent runs).

---

> **Source:** [cmd/kanban-app/](../cmd/kanban-app/)
