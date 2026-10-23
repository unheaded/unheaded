# Zhen Audit

Both brains (Claude through `zhen-claude`, the local model through `zhen-agentd`) record what they do in the same two places. Records carry counts, cost, timing and hashes, **never prompt, response or tool input text**.

## Where records go

| What | Where | Writer |
|---|---|---|
| One row per model request (either brain) | `unheaded_ops.llm_requests` (migration 030, append-only) | `ops_writer` |
| One event per tool-gate decision | `unheaded_ops.audit_events` (migration 022, pkg/audit taxonomy, hash-chained) | `ops_writer` |
| Live view of both | Wotan topics `zhen.llm.requests`, `zhen.audit.toolcalls` (JSON, at most 4 KiB) | `zhen-claude`, `zhen-agentd`, `zhen-eval` (allowlisted in `configs/wotan.yaml`) |
| Champion's own action log | `unheaded_app.zhen_actions` (ADR-019), unchanged | `app_zhen` |

The Well is the record; Wotan is the live view (its store is memory only).

## Turning it on

| Binary | Flag | Environment (never a flag value) |
|---|---|---|
| `zhen-claude` | `-audit-well` | `WELL_OPS_DSN` (unheaded_ops as `ops_writer`) |
| `zhen-claude` | `-audit-wotan` | `WOTAN_ADDR` (Wotan HTTP `host:port`, e.g. `127.0.0.1:18000`) |
| `zhen-agentd` | `-audit-well`, `-audit-wotan` | same |

A flag without its variable stops at startup (exit 2 for `zhen-claude`).

**Well down = read-only.** If `zhen-claude -audit-well` cannot reach the Well (or its 022/030 tables), the run continues read-only: `write_file`, `patch_file` and the Kanban writes are not offered, and a `read_only` log line says why (ADR-019: no actions without an audit trail). `zhen-agentd` warns loudly and keeps its `zhen_actions` trail.

Writes are queued (1024 records per sink) and never block a model call. A full queue drops and counts (`zhen_audit_dropped_total{sink}`); a refused write is counted (`zhen_audit_errors_total{sink}`) and logged.

## Fields

`llm_requests`: `occurred_at`, `source_service` (zhen-claude, zhen-agentd, zhen-eval, zhen-mcp), `backend` (claude, local), `model_requested`, `model_served`, `conversation_id`, `iteration`, `task_family`, `route_reason` (local_first, escalated_check_fail, escalated_parse_fail, escalated_refusal, claude_direct, local_only, manual), `stop_reason`, token counts, `cost_nano_usd` (0 for local), `reserved_nano_usd`, `duration_ms`, `redactions`, `sent`, `error` (512 B max).

`audit_events` for a tool call: `event_type='authorization'`, `action='tool_call'` (Phase 2 adds `confirm_issue`, `confirm_approve`, `confirm_deny`, `confirm_expire`), `outcome` success / pending / failure, `actor_id` `claude-api/<model>` or `zhen-local/<model>`, `actor_type='model'`, `resource_type='tool'`, `resource_id` = tool name (or `invalid` if it is not `^[a-z0-9_.-]{1,64}$`), `source_service`, `request_id` = conversation. `details`: provider, decision, reason (512 B), `input_sha256`, input/output bytes, truncated, pending, redactions.

## Queries

```sql
-- spend and volume per brain
SELECT backend, count(*), sum(cost_nano_usd)/1e9 AS usd FROM llm_requests GROUP BY 1;
-- why requests reached Claude, last week
SELECT route_reason, count(*) FROM llm_requests
 WHERE backend = 'claude' AND occurred_at > now() - interval '7 days' GROUP BY 1;
-- refused or pending tool calls
SELECT occurred_at, actor_id, resource_id, outcome, details->>'reason'
  FROM audit_events WHERE action = 'tool_call' AND outcome <> 'success' ORDER BY occurred_at DESC LIMIT 20;
```

## Tests

`scripts/well/test-llm-requests.sh` (throwaway server, never the real Well): grants, append-only triggers (owner included), CHECKs, then one bridge turn against the fake Claude API writing through the real sinks; a `pg_dump` must hold no trace of the prompt marker. Registered in `scripts/check-gates-can-fail.sh`.

---

> **Source:** [ADR-104](../docs/adr/ADR-104-zhenai-claude-api-bridge.md) · [S-TWO-BRAINS plan](../docs/battle-plans/S-TWO-BRAINS-battle-plan.md) Phase 1 · `db/migrations/030_ops_llm_requests.sql` · `pkg/claudebridge/audit_sinks.go`
