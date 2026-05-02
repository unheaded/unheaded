# Zhenai — Web UI for the Unheaded Kingdom

Zhen runs at **http://localhost:20103** as the operator's browser-facing
chat + runbook console. WAVE15 (commit `45d7aeb7`-era) retargeted its
backends:

- **Inference**: shared `llama-server` on `:8081` (Qwen-Coder-7B-Instruct
  q4_k_m on ROCm GPU). Same binary the Go agent stack uses.
- **Retrieval**: `vor` (`cs serve`) on `:9876` — 1847+ Unheaded markdown
  topics + curated cheatsheets, source-trust labeled. Replaces the
  legacy 1.76M-vector Mistral-era FAISS index.
- **Memory**: `sentence-transformers/all-MiniLM-L6-v2` (local, ~80 MB)
  embeds new questions for cosine recall against `zhen_memories`.
  Memory recall is **display-only** (T1 closure): cached matches surface
  as side-channel data; the live LLM call always runs.
- **Mutations**: chat-driven kingdom-altering actions
  (`/api/v1/runbooks/<n>/execute`, future kanban-from-chat) route
  through `cmd/zhen-agentd /api/v1/tool/exec` for `pkg/champion` gating
  (T6b closure). Without the daemon up, mutation endpoints return 503.

See [`docs/battle-plans/WAVE15-ZHENAI-REWIRE.md`](../docs/battle-plans/WAVE15-ZHENAI-REWIRE.md)
for the rewire's full plan + acceptance evidence.

## For Claude Code agents

Before working on any task, query Zhen for context:

```bash
curl -s http://localhost:20103/api/v1/context \
  -H "Content-Type: application/json" \
  -d '{"task": "describe your task here", "k": 10}'
```

Returns the top-10 relevant chunks from vor's index — Unheaded markdown
prose, ADRs, battle plans, embedded cheatsheets. Source-trust labels on
each chunk distinguish `canonical` (built-in cheatsheets) from `local`
(user-edited) from `external` (user-symlinked content that could be
poisoned — see [`eval/coding-gate/probe-2026-05-02/A1-source-poison.md`](../eval/coding-gate/probe-2026-05-02/A1-source-poison.md)).

## Available endpoints

| Endpoint | Method | Purpose |
|---|---|---|
| `/health` | GET | liveness + RAG readiness + Well connectivity |
| `/api/v1/query` | POST | RAG question answering (chat) |
| `/api/v1/search` | POST | semantic search via vor (no LLM) |
| `/api/v1/context` | POST | context retrieval for Claude agents |
| `/api/v1/conversations` | GET | list recent conversations |
| `/api/v1/conversations/search` | GET | full-text search via tsvector |
| `/api/v1/remember` | POST | save Q/A to `zhen_memories` |
| `/api/v1/forget` | POST | delete memory by id |
| `/api/v1/runbooks` | GET | list runbooks |
| `/api/v1/runbooks/<n>` | GET | runbook YAML content |
| `/api/v1/runbooks/<n>/execute` | POST | **gated** runbook exec via `cmd/zhen-agentd` |
| `/api/v1/champion/read` | POST | sandboxed file read |
| `/api/v1/skills` | GET | list Kingdom skills |
| `/api/v1/skill/<name>` | GET | get skill content |
| `/api/v1/source` | GET | view a vor topic in full (used by viewer page) |
| `/view/source` | GET | HTML page for the source viewer |
| `/api/v1/stats` | GET | backend + memory-embedder stats |
| `/api/v1/corpus/stats` | GET | **deprecated** post-WAVE15 (returns deprecation message; was the 6+ min Werkzeug deadlock cause) |
| `/api/v1/teach` | POST | **deprecated** (returns 410); to add content drop a markdown file under `~/.config/cs/sources/<source>/` |

## Security posture

The browser-facing endpoint set is documented in
[`docs/security/application-threat-model.md`](../docs/security/application-threat-model.md)
with per-threat status (T1-T10) and explicit residual-risk rationale
where defenses aren't fully closed today. Notable:

- **T1** (memory replay → tool injection): CLOSED via display-only
  recall semantics.
- **T6** (UI bypasses Champion): SPLIT — chat path is OPEN-DOCUMENTED
  (theoretical-only on current surface), mutation path is CLOSED via
  the new `/api/v1/tool/exec` direct-dispatch endpoint.
- **T8** (CSRF on browser-served state-changing endpoints): OPEN-
  DOCUMENTED. Compensating control is the LAN-only deployment posture.
  Full closure planned with the future Go port (Stevie's solo
  post-gate exercise).

When adding new mutation endpoints, route through the daemon at
`/api/v1/tool/exec` with a `direct-user` justification chain. That
keeps Champion's three rules in the path (path-allowlist + untrusted-
justification + destructive-verb).

## Startup

`./start-zhen.sh`        — UI only (read-only on runbook surface)
`./start-zhen.sh -gated` — UI + zhen-agentd (mutation gating ON)

Both forms require `llama-server :8081` and `vor :9876` already up.
The script fails fast and prints the bring-up commands if either is
missing. No more launching our own llama-server from this script.

## Stopping

```bash
pkill -f zhen_app.py
pkill -f bin/zhen-agentd
```
