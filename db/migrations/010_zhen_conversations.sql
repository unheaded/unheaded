-- 010_zhen_conversations.sql
-- WAVE15 Phase 3: explicit migration for zhen_conversations.
--
-- The Python web UI (raft/zhen_app.py:_pg_log) has been writing against
-- this table since pre-WAVE15 but no migration existed; the table was
-- assumed to be created out-of-band or it silently failed (graceful-
-- degrade: zhen_app.py keeps working without PG). Phase 3 codifies the
-- schema so the writes are durable + queryable + indexed.
--
-- Schema mirrors the column list the Python's INSERT statement uses:
--   session_id, role, content, sources (jsonb), model,
--   tokens_input, tokens_output, elapsed_ms
-- + computed search_vector (tsvector) for full-text recall used by
--   /api/v1/conversations/search and the "recall <topic>" command.

\set ON_ERROR_STOP on

-- Connect to the app database (per 002_create_databases.sql convention).
\c unheaded_app

CREATE TABLE IF NOT EXISTS zhen_conversations (
    id              BIGSERIAL PRIMARY KEY,
    session_id      TEXT NOT NULL,
    role            VARCHAR(20) NOT NULL CHECK (role IN ('user', 'assistant')),
    content         TEXT NOT NULL,
    sources         JSONB DEFAULT '[]'::jsonb,
    model           VARCHAR(64),
    tokens_input    INTEGER DEFAULT 0,
    tokens_output   INTEGER DEFAULT 0,
    elapsed_ms      INTEGER DEFAULT 0,
    -- Generated FTS column. websearch_to_tsquery() in /api/v1/conversations/search
    -- queries this directly. Generated-stored is automatic + indexed.
    search_vector   tsvector
                    GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

-- GIN on the tsvector — required for websearch_to_tsquery to scale
-- beyond a few hundred rows.
CREATE INDEX IF NOT EXISTS idx_zhen_conversations_search
    ON zhen_conversations USING GIN (search_vector);

-- Recent-conversations queries (the "list last N" UI pattern) hit this.
CREATE INDEX IF NOT EXISTS idx_zhen_conversations_session_created
    ON zhen_conversations (session_id, created_at DESC);

-- Time-range queries for `recall <topic> [--days=N]` and similar.
CREATE INDEX IF NOT EXISTS idx_zhen_conversations_created
    ON zhen_conversations (created_at DESC);

-- Permissions: app_zhen is the writer + reader (the Python UI).
-- ops_zhen + config_zhen don't touch this table; do not grant.
GRANT SELECT, INSERT ON zhen_conversations TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_conversations_id_seq TO app_zhen;

-- Phase 3 H3 testing: the test_memory_poison fixture seeds and cleans
-- up zhen_memories rows tagged source='poison'. We don't add anything
-- to zhen_memories here; just note the pattern.

-- Idempotent: re-running this migration is a no-op (CREATE IF NOT EXISTS).

\echo '010_zhen_conversations.sql applied'
