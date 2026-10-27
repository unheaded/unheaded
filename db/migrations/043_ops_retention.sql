-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — what the retention job did, and what it archived
-- (S-WELL-SPRING Phase 4, M-074; retention policy v1 §11), unheaded_ops.
--
-- retention_runs     one row per class per run of well-retention, dry runs
--                    included: the cutoff, how many rows were eligible,
--                    deleted, archived and held, and for a partition check
--                    the ERDA 5.4 measurement (detail: rows, bytes, oldest,
--                    partition_by, due). A run is written once, when its
--                    class finishes; append-only by trigger like 015/039.
-- retention_archive  the copy a class with an archive keeps before it
--                    purges (kanban-soft-deleted today): the source row as
--                    JSON with every string redacted (pkg/zhencapture
--                    patterns) before it is written here. Its own class
--                    (retention-archive, 365 days) purges it. Never
--                    updated; nobody but the job reads it.
--
-- Partition checks read only the age column of each append-only table
-- (column SELECT below) plus pg_stat/pg_class sizes, which need no grant:
-- never a payload column.
--
-- Writer: retention_runner (042). retention_runs: SELECT, INSERT;
-- ops_reader reads it. retention_archive: SELECT, INSERT, DELETE for
-- retention_runner only. Owner: well_ops_owner.
--
-- Access paths: retention_runs by time for a person (`ORDER BY id DESC`;
-- tens of rows a week, no secondary index); retention_archive by age for
-- its own purge (retention_archive_archived_at_idx).
--
-- Lock plan: two new tables and column grants on existing tables (catalog
-- updates, instant). Rollback: a later numbered migration drops the
-- tables and revokes the grants. Proof: scripts/well/test-retention.sh
-- (throwaway server).

SET lock_timeout = '5s';

CREATE TABLE retention_runs (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id         TEXT NOT NULL CHECK (run_id ~ '^[0-9a-f]{16}$'),
    class          TEXT NOT NULL CHECK (class ~ '^[a-z][a-z0-9-]{0,47}$'),
    action         TEXT NOT NULL
        CHECK (action IN ('purge', 'purge-function', 'archive-purge', 'partition-check', 'held')),
    dry_run        BOOLEAN NOT NULL,
    host           TEXT NOT NULL CHECK (length(host) BETWEEN 1 AND 253),
    started_at     TIMESTAMPTZ NOT NULL,
    finished_at    TIMESTAMPTZ NOT NULL CHECK (finished_at >= started_at),
    cutoff         TIMESTAMPTZ,
    eligible       BIGINT NOT NULL DEFAULT 0 CHECK (eligible >= 0),
    deleted        BIGINT NOT NULL DEFAULT 0 CHECK (deleted >= 0),
    archived       BIGINT NOT NULL DEFAULT 0 CHECK (archived >= 0),
    held           BIGINT NOT NULL DEFAULT 0 CHECK (held >= 0),
    detail         JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(detail) = 'object'),
    error          TEXT NOT NULL DEFAULT '' CHECK (length(error) <= 512),
    CHECK (NOT dry_run OR (deleted = 0 AND archived = 0)),
    UNIQUE (run_id, class)
);

CREATE TABLE retention_archive (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    run_id         TEXT NOT NULL CHECK (run_id ~ '^[0-9a-f]{16}$'),
    class          TEXT NOT NULL CHECK (class ~ '^[a-z][a-z0-9-]{0,47}$'),
    source_db      TEXT NOT NULL CHECK (source_db IN ('unheaded_app', 'unheaded_ops')),
    source_table   TEXT NOT NULL CHECK (source_table ~ '^[a-z][a-z0-9_]{0,62}$'),
    source_key     TEXT NOT NULL CHECK (source_key <> '' AND length(source_key) <= 256),
    row_data       JSONB NOT NULL CHECK (jsonb_typeof(row_data) = 'object'),
    redactions     INTEGER NOT NULL DEFAULT 0 CHECK (redactions >= 0),
    archived_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX retention_archive_archived_at_idx ON retention_archive (archived_at);

CREATE OR REPLACE TRIGGER trg_retention_runs_append_only
    BEFORE UPDATE OR DELETE ON retention_runs
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_retention_runs_no_truncate
    BEFORE TRUNCATE ON retention_runs
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
-- The archive is purged row by row at its age, never edited or truncated.
CREATE OR REPLACE TRIGGER trg_retention_archive_no_update
    BEFORE UPDATE ON retention_archive
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_retention_archive_no_truncate
    BEFORE TRUNCATE ON retention_archive
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT ON retention_runs TO retention_runner;
GRANT SELECT ON retention_runs TO ops_reader;
GRANT SELECT, INSERT, DELETE ON retention_archive TO retention_runner;

-- Partition checks: the age column of each append-only table, nothing else.
GRANT SELECT (occurred_at) ON llm_requests TO retention_runner;
GRANT SELECT (transition_at) ON service_health_transitions TO retention_runner;
GRANT SELECT (occurred_at) ON audit_events TO retention_runner;
GRANT SELECT (changed_at) ON compliance_findings_history TO retention_runner;
GRANT SELECT (occurred_at) ON deploy_history_transitions TO retention_runner;
GRANT SELECT (created_at) ON chaos_journal TO retention_runner;
