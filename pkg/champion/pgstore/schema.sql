-- pkg/champion/pgstore — schema for the zhen_actions table.
-- Embedded into the binary via go:embed; applied by Migrate(ctx).
--
-- Idempotent: safe to apply on every daemon startup.

CREATE TABLE IF NOT EXISTS zhen_actions (
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

-- Index on session_id for the common audit query: "what did this
-- session do?" — uses a B-tree because session_id is high-cardinality
-- and the query is exact match.
CREATE INDEX IF NOT EXISTS zhen_actions_session_id_idx
    ON zhen_actions (session_id);

-- Index on planned_at for time-range queries ("what happened in the
-- last hour?"). DESC because most ops want recent-first.
CREATE INDEX IF NOT EXISTS zhen_actions_planned_at_idx
    ON zhen_actions (planned_at DESC);

-- Index on (action_type, status) for "find every denied tool call"
-- forensic queries.
CREATE INDEX IF NOT EXISTS zhen_actions_type_status_idx
    ON zhen_actions (action_type, status);
