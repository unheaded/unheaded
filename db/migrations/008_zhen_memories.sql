-- Zhen memories: cached good answers for future similarity matching
-- Table: zhen_memories in unheaded_app database
-- User: app_zhen (existing)

\connect unheaded_app;

CREATE TABLE IF NOT EXISTS zhen_memories (
    id          BIGSERIAL PRIMARY KEY,
    question    TEXT NOT NULL,
    answer      TEXT NOT NULL,
    embedding   BYTEA,
    source      VARCHAR(100) DEFAULT 'user',
    model       VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_zhen_memories_created
    ON zhen_memories (created_at DESC);

-- Grant access to app_zhen
GRANT SELECT, INSERT, UPDATE, DELETE ON zhen_memories TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_memories_id_seq TO app_zhen;
