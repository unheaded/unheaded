# zhen-agentd

Long-running HTTP daemon variant of `zhen-agent`. Same agent runtime (`pkg/agent` + `pkg/champion` + cs/vor + llama-server backends), exposed over HTTP for multi-request use:

- `POST /api/v1/agent/ask` — run the agent on a user goal
- `GET /health` — liveness probe (always 200 once the listener is up)
- `GET /ready` — readiness probe (verifies vor + llama-server reachable; cached 5 s)

Default port: **20105** (in the AI Services band 20101-20106 reserved per `CLAUDE.md`'s Port Allocation table).

## Quickstart

```bash
# 1. Build
make build-zhen-agentd

# 2. Make sure vor + llama-server are running

# 3. Run
bin/zhen-agentd -project-root /home/govan/tmp/unheaded

# 4. Ask
curl -s -X POST http://127.0.0.1:20105/api/v1/agent/ask \
    -H 'Content-Type: application/json' \
    -d '{"goal":"Read CONTRIBUTING.md and tell me what sanitized:true means","seed":42,"temperature":0}' \
    | jq

# 5. Shutdown gracefully
kill -TERM <pid>      # or just ^C in foreground
```

## Endpoints

### `POST /api/v1/agent/ask`

**Request body** (JSON, max 64 KiB):

| Field | Type | Default | Notes |
|-------|------|---------|-------|
| `goal` | string | (required) | the user goal |
| `project_root` | string | "" | optional — must equal the daemon's bound `-project-root` if set |
| `session_id` | string | auto-generated | for audit-log correlation |
| `k` | int | 5 | top-K topics for seed retrieval |
| `max_tokens` | int | 600 | LLM max_tokens per turn |
| `max_turns` | int | 8 | agent loop budget |
| `temperature` | float | 0.4 | LLM sampling temperature |
| `seed` | int | 0 | LLM seed; nonzero pins sampling |

**Response (200)**:

```json
{
  "answer": "<final answer>",
  "turns_used": 2,
  "budget_hit": false,
  "trace": [
    {
      "thought": "...",
      "tool": "read_file",
      "args": {"path": "..."},
      "observation": "..."
    },
    {
      "thought": "...",
      "answer": "..."
    }
  ],
  "session_id": "anon-20260502T053640Z"
}
```

When the gate refuses a tool call:

```json
{
  "trace": [
    {
      "thought": "...",
      "tool": "write_file",
      "args": {"path": "..."},
      "refused": true,
      "pending_confirmation": true,
      "pending_token": "<32-hex-char single-use token>",
      "observation": "tool REFUSED (pending user confirmation — write_file): ..."
    },
    ...
  ]
}
```

The `pending_token` can be redeemed via `champion.ConfirmPendingToolCall(token)` (no HTTP endpoint for that yet — direct programmatic call).

**Error responses**:

| Status | Body | Cause |
|--------|------|-------|
| 400 | `{"error":"missing \"goal\""}` | request validation |
| 403 | `{"error":"project_root override (..) must match server's bound root (..)"}` | request specified a different project root |
| 405 | `method not allowed` | non-POST to /ask |
| 500 | `{"error":"agent.Run: ..."}` | LLM/retriever fatal error |
| 503 | `{"ready":false,"detail":"unreachable: vor"}` | (on /ready) backend(s) unreachable |

### `GET /health`

Always returns 200 with `{"status":"ok"}` once the listener is accepting connections. Use this for Kubernetes liveness probes.

### `GET /ready`

Returns 200 + `{"ready":true,...}` when vor and llama-server both pass their `/health` checks. Returns 503 + `{"ready":false,"detail":"unreachable: ..."}` otherwise. Cached 5 s to bound the backend ping rate.

### `GET /metrics`

Prometheus exposition format. Always 200 once the listener is up. Bypasses auth and rate-limit (orchestrator-friendly). Counters / histograms exposed:

| Metric | Labels | What |
|---|---|---|
| `zhen_agentd_http_requests_total` | endpoint, status | per-endpoint HTTP request counter |
| `zhen_agentd_http_request_duration_seconds` | endpoint | histogram of request duration |
| `zhen_agentd_agent_runs_total` | outcome (answer / budget_hit / error) | agent loop completions |
| `zhen_agentd_agent_turns_used` | (histogram) | turns consumed per run |
| `zhen_agentd_champion_actions_total` | action_type, status | every Champion gate decision (incl. denied_destructive / denied_untrusted_justification / denied_path) |
| `zhen_agentd_confirm_tokens_issued_total` | (counter) | pending-confirm tokens produced |
| `zhen_agentd_confirm_tokens_redeemed_total` | outcome (ok / expired / unknown / used / denied / error) | confirm-flow outcomes |
| `zhen_agentd_rate_limited_requests_total` | (counter) | 429 responses |

### `POST /api/v1/agent/ask/stream`

Same request body as `/ask` — but the response is a Server-Sent Events stream of per-turn updates rather than a single JSON blob. Use this when you want to render the agent's reasoning live (e.g., in a chat UI) instead of waiting for the full loop to complete.

Stream format:

```
event: turn
data: {<TraceEntry json>}

event: turn
data: {<TraceEntry json>}

event: done
data: {"answer":"...","turns_used":N,"budget_hit":false,"session_id":"..."}
```

On error mid-stream:

```
event: error
data: {"error":"..."}
```

Client disconnects (closing the TCP connection or aborting the fetch) cause the agent loop to bail at the next turn boundary — the daemon notices via the request context and stops at that point. Trace entries up to that point are NOT delivered to a future request; the stream is one-shot.

Response headers:

```
Content-Type: text/event-stream
Cache-Control: no-cache
Connection: keep-alive
X-Accel-Buffering: no   # disables nginx buffering
```

Smoke recipe:

```bash
curl -sN -X POST http://127.0.0.1:20105/api/v1/agent/ask/stream \
    -H 'Content-Type: application/json' \
    -d '{"goal":"...","seed":42}'
```

### `POST /api/v1/agent/confirm`

Redeems a single-use pending-confirmation token from a prior `/api/v1/agent/ask` response (look for `Trace[i].pending_token`). The bound tool call is re-run with Rule 2 (untrusted justification) suppressed; Rules 1 (path-allowlist) and 3 (destructive verb) still fire — users can authorize an external source, they cannot authorize destruction.

**Request body**:

| Field | Type | Notes |
|-------|------|-------|
| `token` | string | (required) |
| `project_root` | string | (required if multi-tenant) — must match the daemon's allow-list |

**Response (200)**: `{"result": <tool-output>, "status": "ok"}`. The `result` shape mirrors the underlying tool — string for `read_file`, null for `write_file`/`patch_file`, array for `kanban_list`, etc.

**Error responses**:

| Status | Body | Cause |
|---|---|---|
| 400 | `{"status":"unknown","reason":"unknown confirmation token"}` | bogus / never-issued token |
| 400 | `{"status":"used","reason":"confirmation token already used"}` | token redeemed previously (single-use) |
| 400 | `{"status":"expired","reason":"confirmation token expired"}` | token >5min old |
| 403 | `{"status":"denied","reason":"destructive shell verbs cannot be confirmed; refused"}` | Rule 3 fires post-confirm |
| 403 | `{"status":"denied","reason":"path-allowlist (still enforced after confirm): ..."}` | Rule 1 fires post-confirm |

## Flags

| Flag | Default | Purpose |
|------|---------|---------|
| `-port <int>` | 20105 | HTTP listen port |
| `-host <ip>` | 127.0.0.1 | listen host (use `0.0.0.0` for non-loopback) |
| `-project-root <path>` | cwd | default Champion sandbox root used when a request omits `project_root` |
| `-allowed-roots <list>` | (just `-project-root`) | comma-separated list of additional roots requests may target |
| `-action-store <kind>` | `stderr` | audit-log backend: `stderr` (dev) or `pg` (production; requires `WELL_DSN` env) |
| `-kanban-store <kind>` | `memory` | kanban-task backend for `kanban_create` / `kanban_update`: `memory` (no persistence — these calls fail at dispatch with "no kanban store configured") or `pg` (persists to The Well's `kanban_tasks` table; requires `WELL_DSN`) |
| `-rate-limit <rps>` | 0 | per-IP requests/sec; 0 disables. Recommended ~5 unauthenticated, higher behind auth |
| `-rate-burst <n>` | 10 | per-IP burst capacity (only used when `-rate-limit > 0`) |

Env: `VOR_URL` (default `http://127.0.0.1:9876`), `LLAMA_URL` (default `http://127.0.0.1:8081`), `RAG_MODEL` (default `qwen2.5-coder-7b-instruct`).

## Graceful shutdown

`SIGINT` (Ctrl-C) or `SIGTERM`: stops accepting new requests, waits up to 10 seconds for in-flight requests to complete, then exits 0. In-flight requests beyond that timeout get cut.

## Multi-tenant scoping

A single daemon can serve multiple project roots. Allowed roots are an explicit allow-list:

- `-project-root <path>` is the **default** root used when a request omits `project_root`.
- `-allowed-roots <p1,p2,p3>` adds additional roots requests may target.
- A request's `project_root` must exactly match one of the allowed roots after `filepath.Abs` normalization. Non-matching → 403.

Each unique allowed root gets its own `*champion.Champion` instance (cached for the daemon's lifetime). Caching means repeat requests against the same root reuse Champion's snapshot/revert state and audit thread.

Example: serve two projects from one daemon:

```bash
bin/zhen-agentd \
    -project-root /home/govan/tmp/unheaded \
    -allowed-roots /home/govan/tmp/projects/cs

# Request can target either root:
curl -X POST http://127.0.0.1:20105/api/v1/agent/ask \
    -d '{"goal":"...","project_root":"/home/govan/tmp/projects/cs"}'
```

Per-session sandboxing (different sandbox-root per `session_id`) is NOT yet implemented — sessions are an audit-log concept, not a sandbox boundary. The trust gate's contract is per-Champion (per-root), not per-session. If you need sessions to enforce hard boundaries, deploy one daemon per session.

## Auth

Auth is **disabled by default** (`AUTH_ENABLED=false`) for back-compat with the dev-mode CLI flow. For production, opt in via env:

```bash
export AUTH_ENABLED=true
export AUTH_API_KEYS="prod-key-1,prod-key-2"            # static API keys (X-API-Key header)
# OR / AND
export AUTH_JWT_KEY="$(openssl rand -hex 32)"           # JWT signing key (Authorization: Bearer)
export AUTH_JWT_ISSUER="https://auth.unheaded.dev"
export AUTH_JWT_AUDIENCE="zhen-agentd"
```

Wires `pkg/auth.SetupMiddleware` (see `pkg/auth/README.md` for the full auth design). When enabled:

- `/api/v1/agent/ask` and `/api/v1/agent/confirm` require valid auth → 401 without
- `/health`, `/ready`, `/metrics` always bypass auth (orchestrator-friendly)
- Rate-limit and auth chain: rate-limit is OUTERMOST (drops floods before the auth path-skip check), then auth, then the mux

Smoke verified:

```
without auth header  → 401
wrong API key        → 401
correct API key      → 200 (or 400 if request body invalid, etc.)
/health and /metrics → 200 regardless of auth state
```

## Rate limiting

Per-IP token-bucket. Set via `-rate-limit <rps>` (default 0 = disabled) and `-rate-burst <n>` (default 10).

```bash
bin/zhen-agentd -rate-limit 5 -rate-burst 10
# Each client IP: 5 sustained req/s, burst up to 10.
```

429s expose `Retry-After: 1` and increment `zhen_agentd_rate_limited_requests_total`. `/health`, `/ready`, `/metrics` are excluded.

`X-Forwarded-For` is honored when present (right-most entry — closest trusted proxy's view of the source). Trust this only behind a proxy that injects it; otherwise spoofable from the network.

## Audit log

Every Champion decision (tool-call attempt, accept, refuse, file op completion) is logged via `champion.ActionStore`. The daemon supports two backends:

### `-action-store=stderr` (default — development)

Prints each action to the daemon's stderr. No persistence. Good for local development and seeing the gate's reasoning live.

```
[champion] log #1: tool_call_attempt — ToolCall: write_file
[champion] log #1: accepted
[champion] log #2: file.write — Write file: foo.go (243 bytes)
[champion] log #2: completed
```

### `-action-store=pg` (production — The Well)

Persists to PostgreSQL via `pkg/champion/pgstore`. Requires `WELL_DSN` env var:

```bash
export WELL_DSN="host=well-pg.internal port=5432 user=zhen password=$ZHEN_PW dbname=unheaded_app sslmode=require"
bin/zhen-agentd -action-store=pg -port 20105
# zhen-agentd: connected to PostgreSQL audit store; schema migrated
# zhen-agentd listening on http://127.0.0.1:20105 — ...
```

Schema is applied automatically on startup (idempotent — `CREATE TABLE IF NOT EXISTS`). The table:

```sql
CREATE TABLE zhen_actions (
    id              BIGSERIAL    PRIMARY KEY,
    session_id      TEXT         NOT NULL,
    action_type     TEXT         NOT NULL,
    status          TEXT         NOT NULL,
    intent          TEXT,
    parameters      TEXT,
    result          TEXT,
    result_summary  TEXT,
    error           TEXT,
    triggered_by    TEXT,
    planned_at      TIMESTAMPTZ  NOT NULL DEFAULT NOW(),
    completed_at    TIMESTAMPTZ,
    elapsed_ms      INTEGER
);
-- + indexes on session_id, planned_at DESC, (action_type, status)
```

Common forensic queries:

```sql
-- "what did session X do, newest first?"
SELECT * FROM zhen_actions WHERE session_id = $1 ORDER BY id DESC LIMIT 50;

-- "every refused tool call in the last 24h"
SELECT * FROM zhen_actions
 WHERE action_type = 'tool_call_attempt'
   AND status LIKE 'denied_%'
   AND planned_at > NOW() - INTERVAL '24 hours';

-- "every untrusted-justification refusal — pending-confirm pattern"
SELECT * FROM zhen_actions
 WHERE status = 'denied_untrusted_justification'
 ORDER BY planned_at DESC;
```

The daemon-side connection pool: 10 max open, 5 idle, 5-minute lifetime. Tune via custom DSN options if needed.

Startup-failure modes (each exits non-zero with a clear message):
- `WELL_DSN` not set → `action-store=pg requires WELL_DSN env var`
- `WELL_DSN` malformed → `open postgres: <err>`
- PG unreachable → `ping postgres: <err>`
- Insufficient privileges → `apply schema: <err>` (typically: `permission denied for relation`)

## Difference from zhen-agent

| | `zhen-agent` (CLI) | `zhen-agentd` (daemon) |
|-|-|-|
| Lifecycle | one shot | long-running |
| Input | `-q` flag or stdin | HTTP POST body |
| Output | stdout | HTTP response |
| Trace | `-show-trace` to stderr | always in response body |
| Audit | stderr | stderr (PG planned) |
| Concurrency | one request | many concurrent |
| Sandbox | per-process | bound at startup |

## Code map

```
cmd/zhen-agentd/main.go       — HTTP server, handlers, ready tracker, vorRetriever / llamaLLM (duplicated from zhen-agent)
pkg/agent/agent.go            — agent runtime (shared with cmd/zhen-agent)
pkg/champion/                 — gate + dispatch + confirm (shared)
```

## Testing

```bash
# Smoke against a running daemon
bin/zhen-agentd -port 20105 &
curl -s -X POST http://127.0.0.1:20105/api/v1/agent/ask \
    -H 'Content-Type: application/json' \
    -d '{"goal":"What is the right tag for a clickable button?","seed":42,"temperature":0}'
kill %1
```

Unit tests for the daemon-specific HTTP layer are out-of-scope for this first cut — the underlying agent + Champion are tested in `pkg/agent` and `pkg/champion`. Add daemon-level tests when wiring in a real `ActionStore` (PostgreSQL) and multi-session scoping.
