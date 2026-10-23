-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — training captures and their outcome labels (S-TWO-BRAINS
-- Phase 10, the training-data flywheel), unheaded_app.
--
-- zhen-flywheel ingest reads zhen-capture/v1 JSONL (docs/architecture/
-- zhen-capture.md), re-scans every text field for secrets and identifiers,
-- and stores each answer of each turn here once: one row per (model,
-- prompt, response, tool trace), keyed by the sha256 of that content, so a
-- turn ingested twice, or the same answer captured twice, is one row.
-- zhen-flywheel label attaches outcomes recorded elsewhere (checks, judge
-- verdicts, escalations, Stevie's accept/reject and edits) as labels; a
-- label never edits a capture.
--
-- author_kind says who wrote `response`: the local model, Claude, or a
-- human (Stevie's edits). Claude-authored text is stored, tagged, and
-- kept out of every training export unless the export is run with
-- -include-claude-text (off by default; S-TWO-BRAINS Step 339/340).
--
-- Captured text is Kingdom data (T0/T1, redacted). app_zhen writes and
-- reads it; nobody else is granted anything.
--
-- Writer: app_zhen (SELECT, INSERT on both tables; UPDATE of `promoted`
-- and `quarantined` only; EXECUTE on the purge function). Owner:
-- well_app_owner. Text columns are immutable, the owner included.
--
-- Retention: purge_after = captured_at + 180 days, set by the insert
-- trigger (a writer cannot choose it). zhen_captures_purge() deletes
-- captures past purge_after that no export references (promoted = false),
-- and every quarantined capture, with their labels. It is the only delete
-- path: a DELETE outside it is refused, the owner included; TRUNCATE is
-- refused.
--
-- run_id names a zhen_agent_runs row (migration 032, Phase 6, written on
-- another branch). No foreign key here, so this file bootstraps without
-- 032; a later migration adds it NOT VALID once both are in the ledger.
--
-- Access paths: export and label read the whole table (a few thousand
-- rows) ordered by id; by family and time (`WHERE task_family = $1 AND
-- captured_at >= $2`), served by zhen_captures_family_idx; labelling by
-- conversation (`WHERE conversation_id = $1`) and by agent run
-- (`WHERE run_id = $1`), served by the two other indexes. Labels are
-- looked up by capture (the unique index leads with capture_id and serves
-- the foreign key).
--
-- Size: 1-20 KiB per capture, at most 256 KiB (CHECK); a busy month is
-- well under 1 GiB. Revisit with time partitions (W-15) if it grows.
--
-- Lock plan: two new tables, no lock on anything that exists. Rollback: a
-- later numbered migration drops them. Proof: scripts/well/test-zhen-captures.sh
-- (throwaway servers).

CREATE TABLE zhen_captures (
    id                CHAR(64) PRIMARY KEY CHECK (id ~ '^[0-9a-f]{64}$'),
    captured_at       TIMESTAMPTZ NOT NULL,
    ingested_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    source            TEXT NOT NULL CHECK (source IN ('zhen-claude', 'local_llm', 'zhen-agentd', 'zhen-eval')),
    backend           TEXT NOT NULL CHECK (backend IN ('claude', 'local', 'none')),
    model             TEXT NOT NULL CHECK (model <> '' AND length(model) <= 128),
    author_kind       TEXT NOT NULL CHECK (author_kind IN ('local', 'claude', 'human')),
    task_family       TEXT NOT NULL CHECK (task_family ~ '^[a-z0-9-]{1,64}$'),
    conversation_id   TEXT NOT NULL CHECK (conversation_id <> '' AND length(conversation_id) <= 128),
    run_id            TEXT CHECK (run_id IS NULL OR run_id ~ '^[0-9a-f]{16}$'),
    record_id         TEXT NOT NULL CHECK (record_id <> '' AND length(record_id) <= 128),
    schema            TEXT NOT NULL CHECK (schema = 'zhen-capture/v1'),
    redaction_version INTEGER NOT NULL CHECK (redaction_version >= 1),
    prompt            TEXT NOT NULL,
    response          TEXT NOT NULL CHECK (response <> ''),
    tool_trace        JSONB NOT NULL DEFAULT '[]' CHECK (jsonb_typeof(tool_trace) = 'array'),
    truncated         BOOLEAN NOT NULL,
    bytes             INTEGER GENERATED ALWAYS AS
                          (octet_length(prompt) + octet_length(response) + octet_length(tool_trace::text)) STORED
                          CHECK (bytes <= 262144),
    purge_after       TIMESTAMPTZ NOT NULL,
    promoted          BOOLEAN NOT NULL DEFAULT false,
    quarantined       BOOLEAN NOT NULL DEFAULT false,
    -- The brain and the author agree: a human edit has no backend.
    CHECK ((author_kind = 'human') = (backend = 'none')),
    CHECK (author_kind = 'human' OR author_kind = backend)
);

CREATE INDEX zhen_captures_family_idx ON zhen_captures (task_family, captured_at);
CREATE INDEX zhen_captures_conversation_idx ON zhen_captures (conversation_id);
CREATE INDEX zhen_captures_run_idx ON zhen_captures (run_id) WHERE run_id IS NOT NULL;

CREATE TABLE zhen_capture_labels (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    capture_id  CHAR(64) NOT NULL REFERENCES zhen_captures (id) ON DELETE CASCADE,
    kind        TEXT NOT NULL CHECK (kind IN ('check_pass', 'check_fail', 'judge_pass', 'judge_fail',
                                              'escalated', 'accepted', 'rejected', 'human_edit')),
    -- well:ignore open-vocabulary-check the eight kinds are the flywheel's closed set (plan Appendix C); a new kind means new export rules, so it is a schema change on purpose
    value       JSONB NOT NULL DEFAULT '{}'
        CHECK (jsonb_typeof(value) = 'object' AND octet_length(value::text) <= 4096),
    source_ref  TEXT NOT NULL CHECK (source_ref <> '' AND length(source_ref) <= 256),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    -- Re-labelling is idempotent: the same outcome from the same source row
    -- is one label.
    CONSTRAINT zhen_capture_labels_once UNIQUE (capture_id, kind, source_ref)
);

-- Insert: purge_after is the retention rule, not the writer's choice; a
-- capture is born unpromoted and not quarantined.
CREATE OR REPLACE FUNCTION zhen_captures_insert() RETURNS TRIGGER AS $$
BEGIN
    NEW.purge_after := NEW.captured_at + interval '180 days';
    IF NEW.promoted OR NEW.quarantined THEN
        RAISE EXCEPTION 'zhen_captures: a new capture is neither promoted nor quarantined';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_zhen_captures_insert BEFORE INSERT ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_insert();

-- Update: only promoted (both ways: an export is referenced, then retired)
-- and quarantined (one way) change. The text and its provenance never do.
CREATE OR REPLACE FUNCTION zhen_captures_update() RETURNS TRIGGER AS $$
BEGIN
    -- bytes is generated from the text columns (and not yet computed in
    -- NEW inside a BEFORE trigger), so it is left out of the comparison.
    IF (to_jsonb(NEW) - 'promoted' - 'quarantined' - 'bytes') IS DISTINCT FROM
       (to_jsonb(OLD) - 'promoted' - 'quarantined' - 'bytes') THEN
        RAISE EXCEPTION 'zhen_captures: % is immutable except promoted and quarantined', OLD.id;
    END IF;
    IF OLD.quarantined AND NOT NEW.quarantined THEN
        RAISE EXCEPTION 'zhen_captures: % is quarantined; it is purged, never released', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_zhen_captures_update BEFORE UPDATE ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_update();

-- Delete: only inside zhen_captures_purge(), which sets zhen.purge for its
-- own transaction. Labels go with their capture (ON DELETE CASCADE runs
-- this trigger too); a label is never deleted on its own.
CREATE OR REPLACE FUNCTION zhen_captures_guard_delete() RETURNS TRIGGER AS $$
BEGIN
    IF coalesce(current_setting('zhen.purge', true), '') <> 'on' THEN
        RAISE EXCEPTION '% rows are deleted only by zhen_captures_purge()', TG_TABLE_NAME;
    END IF;
    RETURN OLD;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_zhen_captures_guard_delete BEFORE DELETE ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_guard_delete();
CREATE OR REPLACE TRIGGER trg_zhen_captures_no_truncate BEFORE TRUNCATE ON zhen_captures
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_zhen_capture_labels_guard_delete BEFORE DELETE ON zhen_capture_labels
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_guard_delete();
CREATE OR REPLACE TRIGGER trg_zhen_capture_labels_append_only BEFORE UPDATE ON zhen_capture_labels
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_zhen_capture_labels_no_truncate BEFORE TRUNCATE ON zhen_capture_labels
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

-- The retention rule and the quarantine path. Returns the number of
-- captures deleted (their labels go with them).
CREATE OR REPLACE FUNCTION zhen_captures_purge() RETURNS INTEGER
    LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
DECLARE
    n INTEGER;
BEGIN
    PERFORM set_config('zhen.purge', 'on', true);
    DELETE FROM public.zhen_captures
     WHERE quarantined OR (NOT promoted AND purge_after <= now());
    GET DIAGNOSTICS n = ROW_COUNT;
    PERFORM set_config('zhen.purge', 'off', true);
    RETURN n;
END $$;

REVOKE ALL ON FUNCTION zhen_captures_purge() FROM PUBLIC;
GRANT EXECUTE ON FUNCTION zhen_captures_purge() TO app_zhen;

GRANT SELECT, INSERT, UPDATE (promoted, quarantined) ON zhen_captures TO app_zhen;
GRANT SELECT, INSERT ON zhen_capture_labels TO app_zhen;
