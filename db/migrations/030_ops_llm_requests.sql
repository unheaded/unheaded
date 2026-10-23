-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — per-request LLM telemetry for both brains (ADR-053 §6,
-- ADR-104 §7), unheaded_ops.
--
-- One row per request either brain sends: Claude through pkg/claudebridge
-- (zhen-claude, zhen-eval), the local model through zhen-agentd and
-- zhen-eval. Counts, cost and timing only: never prompt, response or tool
-- input text (the writers have nowhere to put it). Tool-gate decisions do
-- not live here; they go to audit_events (022) through pkg/audit.
--
-- Writer: ops_writer (INSERT, SELECT). Reader: ops_reader. Owner:
-- well_ops_owner (the runner SET ROLEs to it). Append-only by trigger,
-- like audit_events (015): an UPDATE, DELETE or TRUNCATE is refused even
-- for the owner.
--
-- Access paths: time range scans for spend and routing reports
-- (`WHERE occurred_at >= ...`, grouped by backend/source/route_reason),
-- served by llm_requests_occurred_at_idx. Nothing else is indexed: the
-- grouping columns have a handful of values each (W-14).
--
-- Retention: none yet. Rows are small (~200 B); at a few thousand requests
-- a day this is well under 1 GB a year. When it is not, time partitions
-- (W-15) and a dropped-partition retention, never row deletes.
--
-- Lock plan: a new table, no lock on anything that exists. Rollback: none
-- needed for an empty new table; a later numbered migration drops it.
-- Proof: scripts/well/test-llm-requests.sh (throwaway servers).

CREATE TABLE llm_requests (
    id                BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    occurred_at       TIMESTAMPTZ NOT NULL,
    source_service    TEXT NOT NULL
        CHECK (source_service IN ('zhen-claude', 'zhen-agentd', 'zhen-eval', 'zhen-mcp')),
    backend           TEXT NOT NULL CHECK (backend IN ('claude', 'local')),
    model_requested   TEXT NOT NULL CHECK (model_requested <> '' AND length(model_requested) <= 128),
    model_served      TEXT NOT NULL DEFAULT '' CHECK (length(model_served) <= 128),
    conversation_id   TEXT NOT NULL CHECK (conversation_id <> '' AND length(conversation_id) <= 128),
    iteration         INTEGER NOT NULL CHECK (iteration >= 0),
    task_family       TEXT NOT NULL DEFAULT '' CHECK (length(task_family) <= 64),
    route_reason      TEXT NOT NULL DEFAULT ''
        CHECK (route_reason IN ('', 'local_first', 'escalated_check_fail', 'escalated_parse_fail',
                                'escalated_refusal', 'claude_direct', 'local_only', 'manual')),
    stop_reason       TEXT NOT NULL DEFAULT '' CHECK (length(stop_reason) <= 64),
    input_tokens      BIGINT NOT NULL DEFAULT 0 CHECK (input_tokens >= 0),
    output_tokens     BIGINT NOT NULL DEFAULT 0 CHECK (output_tokens >= 0),
    cache_read        BIGINT NOT NULL DEFAULT 0 CHECK (cache_read >= 0),
    cache_write       BIGINT NOT NULL DEFAULT 0 CHECK (cache_write >= 0),
    cost_nano_usd     BIGINT NOT NULL DEFAULT 0 CHECK (cost_nano_usd >= 0),
    reserved_nano_usd BIGINT NOT NULL DEFAULT 0 CHECK (reserved_nano_usd >= 0),
    duration_ms       INTEGER NOT NULL CHECK (duration_ms >= 0),
    redactions        INTEGER NOT NULL DEFAULT 0 CHECK (redactions >= 0),
    sent              BOOLEAN NOT NULL,
    error             TEXT NOT NULL DEFAULT '' CHECK (length(error) <= 512),
    recorded_at       TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX llm_requests_occurred_at_idx ON llm_requests (occurred_at DESC);

CREATE OR REPLACE TRIGGER trg_llm_requests_append_only
    BEFORE UPDATE OR DELETE ON llm_requests
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_llm_requests_no_truncate
    BEFORE TRUNCATE ON llm_requests
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT ON llm_requests TO ops_writer;
GRANT SELECT ON llm_requests TO ops_reader;
