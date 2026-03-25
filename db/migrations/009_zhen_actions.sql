-- Zhen Champion Agent: Action log and snapshot tables
-- Tables: zhen_actions, zhen_action_snapshots in unheaded_app database
-- User: app_zhen (existing, expanded grants)
-- ADR: ADR-019-zhen-champion-agent.md

\connect unheaded_app;

-- ==========================================================================
-- zhen_actions — The Action Log
-- Every operation Zhen performs or plans to perform. Rewindable history.
-- No DELETE allowed. Actions are permanent records.
-- ==========================================================================

CREATE TABLE IF NOT EXISTS zhen_actions (
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

CREATE INDEX IF NOT EXISTS idx_zhen_actions_session ON zhen_actions(session_id);
CREATE INDEX IF NOT EXISTS idx_zhen_actions_type ON zhen_actions(action_type);
CREATE INDEX IF NOT EXISTS idx_zhen_actions_status ON zhen_actions(status);
CREATE INDEX IF NOT EXISTS idx_zhen_actions_planned ON zhen_actions(planned_at DESC);
CREATE INDEX IF NOT EXISTS idx_zhen_actions_trace ON zhen_actions(trace_id);

-- ==========================================================================
-- zhen_action_snapshots — The Before-State Archive
-- Append-only. Stores the state BEFORE Zhen changes something.
-- No UPDATE, no DELETE. The before-state is sacred.
-- ==========================================================================

CREATE TABLE IF NOT EXISTS zhen_action_snapshots (
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

CREATE INDEX IF NOT EXISTS idx_zhen_snapshots_action ON zhen_action_snapshots(action_id);
CREATE INDEX IF NOT EXISTS idx_zhen_snapshots_resource ON zhen_action_snapshots(resource_type, resource_id);

-- ==========================================================================
-- Grants for app_zhen
-- zhen_actions: SELECT, INSERT, UPDATE (no DELETE — actions are permanent)
-- zhen_action_snapshots: SELECT, INSERT (append-only, no UPDATE/DELETE)
-- ==========================================================================

GRANT SELECT, INSERT, UPDATE ON zhen_actions TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_actions_id_seq TO app_zhen;

GRANT SELECT, INSERT ON zhen_action_snapshots TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_action_snapshots_id_seq TO app_zhen;
