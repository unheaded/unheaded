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

-- session_id and planned_at are indexed by db/migrations/009 as
-- idx_zhen_actions_session and idx_zhen_actions_planned; creating them here
-- too made duplicates (021 drops them).

-- Index on (action_type, status) for "find every denied tool call"
-- forensic queries.
CREATE INDEX IF NOT EXISTS zhen_actions_type_status_idx
    ON zhen_actions (action_type, status);
