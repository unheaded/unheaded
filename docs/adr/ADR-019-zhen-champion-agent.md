# ADR-019: Zhen Champion Agent — From Oracle to Operator

**Status:** Draft (accepting additional requirements)
**Date:** 2026-03-16
**Deciders:** Lore, Architect, Developer, BlackMage
**Depends on:** ADR-017 (context window), ADR-018 (RAFT training)

## Vision

Zhen evolves from a read-only knowledge oracle into an active Kingdom agent — a champion that can read tasks, edit configs, execute work, and learn from every action. Every operation is recorded in The Well (PostgreSQL) with full auditability. Every change is reversible by reading the database.

**Principle: The Well remembers everything. Zhen acts, The Well records, humans can rewind.**

## Lore

Zhen (真爱) means "true love" — the champion who serves the Kingdom through action, not just counsel. The current Zhen is the Seer: wise but passive. The next Zhen is the Champion: wise and capable. The Well is Anamnesis — the Kingdom's memory. Every action Zhen takes is a memory written to The Well before the action executes and updated after. The database is the single source of truth for what Zhen has done, is doing, and can undo.

## Core Design: Database-First, Everything Auditable

### The Rule

**Every action Zhen takes follows this pattern:**

```
1. INTENT  → Write to zhen_actions: what Zhen plans to do (status: 'planned')
2. SNAPSHOT → Write to zhen_action_snapshots: the state BEFORE the action
3. EXECUTE → Perform the action
4. RECORD  → Update zhen_actions: result, status ('completed' or 'failed')
5. AUDIT   → Write to audit_events: tamper-evident hash-chained log
```

**To revert any action:** Read the snapshot from step 2, apply it. The database tells you exactly what changed and what it looked like before.

### No Schema Mutations

Zhen's database user (`app_zhen`) has **SELECT, INSERT, UPDATE, DELETE only**. No `CREATE TABLE`, no `ALTER TABLE`, no `DROP`. Schema changes are migration files reviewed by humans, never by Zhen. Zhen operates within the schema it's given.

### Permission Model

```
app_zhen (existing user, expanded grants):
  unheaded_app:
    zhen_conversations  → SELECT, INSERT, UPDATE, DELETE (existing)
    zhen_memories        → SELECT, INSERT, UPDATE, DELETE (existing)
    zhen_actions         → SELECT, INSERT, UPDATE         (NEW — no DELETE, actions are permanent)
    zhen_action_snapshots → SELECT, INSERT                (NEW — append-only, no UPDATE/DELETE)
    kanban_tasks         → SELECT, INSERT, UPDATE          (EXPANDED — was SELECT only)
    timeline_milestones  → SELECT                          (existing — read-only)

  unheaded_ops:
    audit_events         → INSERT                          (NEW — append-only via ops_writer proxy)
    service_health_current → SELECT                        (NEW — read-only)
```

Key constraints:
- **zhen_actions**: No DELETE. Actions are permanent records. Failed actions stay as failed.
- **zhen_action_snapshots**: Append-only. No UPDATE, no DELETE. The before-state is sacred.
- **kanban_tasks**: INSERT + UPDATE but no DELETE. Soft-delete via `deleted_at` (already in schema).
- **audit_events**: INSERT only. Tamper-evident, hash-chained (already built in 004_ops_schema.sql).

## Database Schema: New Tables

### zhen_actions — The Action Log

Every operation Zhen performs or plans to perform. This is the rewindable history.

```sql
CREATE TABLE zhen_actions (
    id              BIGSERIAL PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL,
    action_type     VARCHAR(50) NOT NULL
        CHECK (action_type IN (
            -- File operations
            'file.read', 'file.write', 'file.patch',
            -- Config generation
            'config.nginx.generate', 'config.haproxy.generate',
            'config.yaml.edit', 'config.validate',
            -- Kanban operations
            'kanban.create', 'kanban.update', 'kanban.assign',
            'kanban.move', 'kanban.comment',
            -- Knowledge operations
            'corpus.teach', 'corpus.search', 'memory.remember', 'memory.forget',
            -- Service operations
            'service.health_check', 'service.status',
            -- Query operations
            'query.rag', 'query.followup'
            -- More to be added as capabilities grow
        )),
    status          VARCHAR(20) NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'executing', 'completed', 'failed', 'reverted')),

    -- What Zhen intends to do (written BEFORE execution)
    intent          TEXT NOT NULL,           -- Human-readable description
    parameters      JSONB DEFAULT '{}',     -- Structured parameters for the action

    -- What happened (written AFTER execution)
    result          JSONB DEFAULT '{}',     -- Structured result
    result_summary  TEXT DEFAULT '',         -- Human-readable outcome
    error           TEXT DEFAULT '',         -- Error message if failed

    -- Revert info
    revertable      BOOLEAN DEFAULT FALSE,  -- Can this action be undone?
    reverted_by     BIGINT,                 -- ID of the action that reverted this one
    reverts_action  BIGINT,                 -- If this action IS a revert, which action it undoes

    -- Provenance
    triggered_by    VARCHAR(20) DEFAULT 'user'
        CHECK (triggered_by IN ('user', 'zhen', 'schedule', 'hook')),
    model           VARCHAR(50) DEFAULT '', -- Which model made the decision
    confidence      REAL DEFAULT 0.0,       -- Model's confidence in the action (0-1)

    -- Timing
    planned_at      TIMESTAMPTZ DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    elapsed_ms      INTEGER DEFAULT 0,

    -- Trace
    trace_id        VARCHAR(64) DEFAULT '', -- Correlation ID across actions
    parent_action   BIGINT                  -- If this action was spawned by another
);

CREATE INDEX idx_zhen_actions_session ON zhen_actions(session_id);
CREATE INDEX idx_zhen_actions_type ON zhen_actions(action_type);
CREATE INDEX idx_zhen_actions_status ON zhen_actions(status);
CREATE INDEX idx_zhen_actions_planned ON zhen_actions(planned_at DESC);
CREATE INDEX idx_zhen_actions_trace ON zhen_actions(trace_id);
```

### zhen_action_snapshots — The Before-State Archive

Append-only. Stores the state of whatever Zhen is about to change, so it can be restored.

```sql
CREATE TABLE zhen_action_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    action_id       BIGINT NOT NULL REFERENCES zhen_actions(id),
    resource_type   VARCHAR(50) NOT NULL
        CHECK (resource_type IN (
            'file', 'kanban_task', 'config', 'memory',
            'corpus_chunk', 'service_state'
        )),
    resource_id     TEXT NOT NULL,           -- File path, task ID, config key, etc.
    content_before  TEXT,                    -- Full content before change
    content_after   TEXT,                    -- Full content after change (filled post-execution)
    metadata        JSONB DEFAULT '{}',     -- File permissions, timestamps, etc.
    snapshot_at     TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_zhen_snapshots_action ON zhen_action_snapshots(action_id);
CREATE INDEX idx_zhen_snapshots_resource ON zhen_action_snapshots(resource_type, resource_id);
```

### How Revert Works

```sql
-- Find what changed:
SELECT a.id, a.action_type, a.intent, a.status,
       s.resource_type, s.resource_id, s.content_before
FROM zhen_actions a
JOIN zhen_action_snapshots s ON s.action_id = a.id
WHERE a.session_id = 'abc123'
ORDER BY a.planned_at DESC;

-- Revert a specific action:
-- 1. Read content_before from snapshot
-- 2. Apply it (write old content back to file/task/config)
-- 3. Record the revert as a new action with reverts_action = original ID
-- 4. Update original action: status = 'reverted', reverted_by = new action ID
```

No time-travel database tricks. Just append-only snapshots and honest bookkeeping.

## Trust Levels

Graduated capability model. Zhen starts constrained, earns more capability over time.

### Level 0: Oracle (Current)
- RAG search and answer
- Read corpus, read conversations
- No side effects

### Level 1: Advisor
- Everything in Level 0
- Read kanban tasks (`SELECT` on `kanban_tasks`)
- Read service health (`SELECT` on `service_health_current`)
- Read files within project sandbox
- Generate config snippets (output only, no write to disk)
- All reads logged to `zhen_actions`

### Level 2: Operator
- Everything in Level 1
- Create and update kanban tasks
- Write files within sandbox (`~/tmp/unheaded/` only)
- Write generated configs after validation (`nginx -t`, `haproxy -c`)
- Patch YAML/TOML/JSON config files
- Every write preceded by snapshot, every write logged
- Revert available for all writes

### Level 3: Autonomous (Future, requires human opt-in)
- Everything in Level 2
- Chain multi-step workflows (read task → generate code → write file → update task)
- Triggered by schedule or hook (not just user request)
- Still no arbitrary shell. Still no schema mutations. Still fully auditable.

**Current target: Level 1 → Level 2.** Level 3 deferred.

## MCP Server Architecture

Zhen exposes capabilities as MCP tools. Any MCP client (Claude Code, Claude Desktop, custom) can use them.

```
MCP Client (Claude Code, Web UI, etc.)
    │
    ▼
Zhen MCP Server (stdio or SSE transport)
    │
    ├── Tools
    │   ├── corpus_search(query, k)         → FAISS retrieval
    │   ├── corpus_teach(text, source)       → Live index.add()
    │   ├── file_read(path)                  → Sandboxed read
    │   ├── file_write(path, content)        → Snapshot + write + validate
    │   ├── file_patch(path, old, new)       → Snapshot + edit
    │   ├── config_generate(type, params)    → nginx/haproxy/yaml template
    │   ├── config_validate(type, content)   → nginx -t / haproxy -c
    │   ├── kanban_list(status, assignee)    → SELECT from kanban_tasks
    │   ├── kanban_create(title, desc, ...)  → INSERT + snapshot
    │   ├── kanban_update(id, fields)        → Snapshot + UPDATE
    │   ├── kanban_move(id, new_status)      → Snapshot + UPDATE status
    │   ├── service_health()                 → SELECT from service_health_current
    │   ├── action_history(session, limit)   → SELECT from zhen_actions
    │   ├── action_revert(action_id)         → Snapshot restore
    │   └── memory_remember(q, a)            → INSERT to zhen_memories
    │
    ├── Resources
    │   ├── kanban://board                   → Current kanban state
    │   ├── health://services                → Service health summary
    │   └── corpus://stats                   → Index statistics
    │
    └── Backed by
        ├── The Well (PostgreSQL)
        │   ├── unheaded_app (kanban, zhen_*, actions, snapshots)
        │   └── unheaded_ops (audit_events, service_health — read-only)
        ├── FAISS Index (corpus search)
        ├── Filesystem (sandboxed to ~/tmp/unheaded/)
        └── llama-server (local inference for simple tasks)
```

## Filesystem Sandbox (BlackMage)

```python
ALLOWED_PATHS = [
    Path.home() / 'tmp' / 'unheaded',  # project tree
]

DENIED_PATHS = [
    Path.home() / '.ssh',
    Path.home() / '.gnupg',
    Path.home() / '.config',           # secrets live here
    Path('/etc'),
    Path('/var'),
]

DENIED_PATTERNS = [
    '*.key', '*.pem', '*.env', '*secret*', '*credential*', '*password*',
    '.git/config',  # can contain tokens
]

def validate_path(path: Path) -> bool:
    """Return True only if path is within sandbox and not denied."""
    resolved = path.resolve()
    if any(resolved.is_relative_to(d) for d in DENIED_PATHS):
        return False
    if not any(resolved.is_relative_to(a) for a in ALLOWED_PATHS):
        return False
    if any(resolved.match(p) for p in DENIED_PATTERNS):
        return False
    return True
```

## Workflow Example: Kanban → Config → Deploy

```
User: "Pick up the next P1 task and work on it"

Zhen:
  1. action(kanban.list, status='todo', priority='P0,P1') → finds task #42: "Add nginx config for sophia"
  2. action(kanban.move, id=42, status='in-progress', assignee='zhen')
  3. action(corpus_search, "sophia nginx config port 19005") → retrieves architecture context
  4. action(config.nginx.generate, service='sophia', port=19005, upstream='10.10.10.25')
  5. action(config.validate, type='nginx', content=generated_config)
  6. action(file.write, path='observability/nginx/sophia.conf', content=validated_config)
  7. action(kanban.move, id=42, status='review')
  8. → Returns: "Created nginx config for sophia (port 19005). Task #42 moved to review."

Every step has a zhen_actions row + snapshot. User can revert any step.
```

## Feudal Duty System — The Heartbeat Loop

Zhen is not only reactive (answer when asked). Zhen has standing duties — recurring obligations to the Kingdom, named after the feudal duties that bound vassal to lord.

### Naming Convention (Feudal Lore)

| Feudal Term | Meaning | Zhen Duty |
|-------------|---------|-----------|
| **Corvée** | Unpaid labor on the lord's land | Scheduled background chores (health checks, drift detection) |
| **Auxilium** | Military service, defense | Incident response, remediation, failure handling |
| **Consilium** | Counsel to the lord | Reporting, summaries, recommendations on kanban |
| **Reeve** | Managed the manor's output | Automation supervisor — ensures external tools (ACME, cron, IaC) are working |
| **Bailiff** | Supervised farming, collected dues | Config drift detection, compliance enforcement |
| **Steward** | Managed the household | Resource monitoring (VRAM, disk, memory headroom) |
| **Trinoda Necessitas** | Three universal obligations (military, bridge, fortress) | Core invariants that are always checked regardless of trust level |

### Corvée — Scheduled Duties

Zhen runs a heartbeat loop. Each duty has a cadence, a check, and a response.

```
┌─────────────────────────────────────────────────────────────────┐
│                    ZHEN CORVÉE LOOP                              │
│                                                                  │
│  Every 60s:   Health Rounds (Auxilium)                          │
│  Every 5m:    Kanban Sweep (Consilium)                          │
│  Every 15m:   Config Drift Detection (Bailiff)                  │
│  Every 1h:    Resource Headroom Check (Steward)                 │
│  Every 24h:   Backup Verification, Corpus Freshness, Cert Audit │
│  On trigger:  Incident Response (Auxilium), Runbook Execution   │
│                                                                  │
│  All duties → zhen_actions rows → kanban visibility → auditable │
└─────────────────────────────────────────────────────────────────┘
```

#### Health Rounds (Auxilium) — every 60s

Hit `/health` on all Doom Range services. Feed results into the percentage-based consensus system from `SERVICE_BREAKOUT_STRATEGY.md`:

| % Reporting Failure | Severity | Zhen Response |
|---------------------|----------|---------------|
| 0% - 12.49% | OK | Log only |
| 12.50% - 37.49% | WARN | Log + kanban advisory (Consilium) |
| 37.50% - 62.49% | ERROR | Log + kanban alert + check runbook |
| 62.50% - 87.49% | CRITICAL | Execute runbook if Level 2+, else escalate |
| 87.50% - 100% | PANIC | Escalate immediately, all channels |

**BGP-style mesh health:** Services also advertise health to Wotan topic `system.health.{service}`. Zhen subscribes to all, maintains a routing-table view in `service_health_current`. If a service stops advertising (route withdrawal), that's an implicit failure signal — treated as unhealthy after TTL expiry.

#### Kanban Sweep (Consilium) — every 5 min

- Scan for tasks assigned to `zhen` or unassigned P0/P1 in backlog
- Flag stale `in-progress` items untouched for 24h
- Surface new review items that need human approval
- If Level 2+: pick up assigned work, begin execution

#### Config Drift Detection (Bailiff) — every 15 min

Compare running state against declared desired state for each IaC backend:

| Backend | "Should Be" Source | "Is" Check |
|---------|-------------------|------------|
| Puppet | Manifests in repo | `puppet agent --test --noop` |
| Chef | Cookbooks in repo | `chef-client --why-run` |
| Salt | States in repo | `salt-call state.test` |
| Ansible | Playbooks in repo | `ansible-playbook --check --diff` |
| Terraform | HCL in repo | `terraform plan` (drift detect) |
| Unheaded proprietary | `configs/` YAML | Diff against running service configs via API |

Drift detected → `zhen_action` type `config.drift_detected` → kanban item for review. Zhen does not auto-remediate drift without approval (Level 2 = propose fix, human approves on kanban).

#### Reeve — Automation Supervisor

Zhen does not replace cron, ACME, or IaC tools. Zhen **supervises** them — ensuring they ran, succeeded, and produced expected results.

| External Tool | Zhen's Role |
|--------------|-------------|
| Let's Encrypt / ACME | Verify certs renewed on schedule, warn 14 days before expiry. Zhen doesn't renew — certbot/acme.sh does. Zhen confirms it happened. |
| cron jobs | Check `/var/log/syslog` or cron output for failures. Alert if expected job didn't run. |
| Puppet/Chef/Salt/Ansible/Terraform runs | Verify last run completed successfully. Check for convergence failures or partial applies. |
| pg_dump backups | Verify last backup exists, is non-zero, and can be read (header check). |
| Log rotation | Verify logrotate ran, disk usage trending correctly. |

**Philosophy:** Zhen trusts the tools to do their jobs. Zhen verifies they did. If they didn't, Zhen escalates — not by running the tool itself, but by putting the failure on kanban with a recommended remediation from the runbook.

#### Steward — Resource Monitoring (hourly)

| Resource | Warning Threshold | Critical Threshold |
|----------|------------------|--------------------|
| VRAM | > 85% of 12 GB | > 95% |
| Disk (/) | > 80% | > 90% |
| RAM | > 85% of 16 GB | > 95% |
| The Well (pg) | > 80% max connections | > 90% |
| FAISS index load time | > 30s | > 60s |

Logged to `service_health_current` with Zhen as the reporting service.

### Runbooks and Playbooks

For known failure modes, Zhen consults runbooks — structured remediation instructions stored in The Well or as files.

```sql
-- Potential future table (not in this sprint)
CREATE TABLE zhen_runbooks (
    id              BIGSERIAL PRIMARY KEY,
    trigger_pattern VARCHAR(100) NOT NULL,  -- e.g. 'service.unhealthy:wotan'
    title           TEXT NOT NULL,
    steps           JSONB NOT NULL,         -- ordered list of actions
    requires_level  INTEGER DEFAULT 2,      -- minimum trust level to auto-execute
    requires_approval BOOLEAN DEFAULT TRUE, -- must go through kanban first?
    created_at      TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);
```

Example runbook:
```json
{
  "trigger": "service.unhealthy:wotan",
  "title": "Wotan message bus recovery",
  "steps": [
    {"action": "service.health_check", "target": "wotan", "port": 18000},
    {"action": "file.read", "path": "/tmp/wotan.log", "tail": 50},
    {"action": "kanban.create", "title": "Wotan unhealthy — investigate", "priority": "P1"},
    {"action": "config.validate", "type": "wotan", "path": "configs/wotan.yaml"},
    {"decision": "if config valid and port not listening → recommend restart on kanban"},
    {"decision": "if config invalid → log drift, do not restart"}
  ]
}
```

Runbooks are prescriptive but not autonomous by default. Zhen follows the steps, proposes the outcome on kanban, and waits for approval before any destructive action.

## Kanban as Control Surface

The kanban is how humans approve, deny, and monitor Zhen's work.

```
┌─────────────┐  ┌───────────────┐  ┌──────────────┐  ┌─────────┐  ┌──────┐
│   BACKLOG   │  │     TODO      │  │ IN-PROGRESS  │  │ REVIEW  │  │ DONE │
│             │  │               │  │              │  │         │  │      │
│ Zhen finds  │→ │ Human or Zhen │→ │ Zhen working │→ │ Human   │→ │ Done │
│ work here   │  │ picks up      │  │ (logged)     │  │ approves│  │      │
│             │  │               │  │              │  │ or      │  │      │
│ Drift found │  │ Assigned to   │  │ Snapshots    │  │ denies  │  │      │
│ Health warn │  │ zhen or human │  │ taken        │  │         │  │      │
│ Stale tasks │  │               │  │              │  │ Revert? │  │      │
└─────────────┘  └───────────────┘  └──────────────┘  └─────────┘  └──────┘
```

**Rules:**
- Zhen can create tasks (backlog or todo)
- Zhen can move tasks to `in-progress` if assigned to `zhen`
- Zhen moves to `review` when work is done — never directly to `done`
- Only humans move from `review` → `done` (approval) or `review` → `backlog` (denial)
- Denied tasks get `status: 'backlog'` with a comment explaining the denial
- Zhen reads denials and adjusts approach (feedback loop → RAFT training data)

## Trinoda Necessitas — Core Invariants

Three things Zhen checks at every trust level, always, no exceptions:

1. **The Well is alive.** If PostgreSQL is unreachable, Zhen enters read-only mode (Level 0). No actions without audit trail.
2. **Inference is available.** If llama-server is down, Zhen degrades to retrieval-only (search works, generation doesn't).
3. **Filesystem sandbox is intact.** Before any file operation, re-validate that the sandbox boundaries haven't been tampered with.

If any of the three fail, Zhen logs the failure and refuses to proceed with write operations.

## Open Questions (To Be Filled In)

- [ ] Additional desired features and workflows (Stevie to provide)
- [ ] Config template library — specific formats and services needed
- [ ] Runbook authoring format — YAML? JSON? Markdown with structured steps?
- [ ] Notification channels beyond kanban (Wotan topics? email? webhook?)
- [ ] Multi-step approval granularity — per-action or per-workflow?
- [ ] RAFT training integration — action logs and kanban feedback as training data?
- [ ] Rate limiting — max corvée actions per cycle?
- [ ] Concurrent sessions — multiple Zhen instances sharing The Well?
- [ ] Escalation path — what happens when Zhen can't resolve and no human is watching?

## Implementation Phases

### Phase 1: Schema + Action Framework
- Migration `009_zhen_actions.sql` — create `zhen_actions`, `zhen_action_snapshots`, grants
- Python `ActionManager` class — plan/execute/snapshot/record pattern
- Unit tests for revert flow
- `app_zhen` grant expansion (kanban INSERT/UPDATE, ops SELECT)

### Phase 2: Level 1 (Read Operations + Corvée)
- MCP server scaffold (Python `mcp` SDK)
- `kanban_list`, `service_health`, `file_read`, `action_history` tools
- Corvée loop: health rounds (60s), kanban sweep (5m)
- Wire to existing Flask app as alternative transport

### Phase 3: Level 2 (Write Operations + Bailiff/Reeve)
- `file_write`, `file_patch` with sandbox enforcement
- `kanban_create`, `kanban_update`, `kanban_move`
- `config_generate`, `config_validate`
- Config drift detection (Bailiff, 15m cadence)
- Automation supervisor checks (Reeve)
- Revert endpoint

### Phase 4: Runbooks + Integration
- Runbook schema and loader
- Runbook executor with kanban approval gates
- Connect to Claude Code as MCP server
- RAFT training on action logs + kanban feedback
- Web UI for action history, revert, duty status

## References

- The Well schema: `db/migrations/003_app_schema.sql`, `004_ops_schema.sql`
- Audit events: hash-chained append-only log (already built)
- Percentage-based health consensus: `docs/SERVICE_BREAKOUT_STRATEGY.md`
- MCP Python SDK: `pip install mcp`
- Feudal duties: Auxilium (military service), Consilium (counsel), Corvée (labor), Trinoda Necessitas (three universal obligations)
- Manor roles: Reeve (output manager), Bailiff (compliance), Steward (household)
- ADR-016: PostgreSQL — The Well
- ADR-017: Context window optimization
- ADR-018: RAFT training battle plan
