-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Zhen's tables in their own database, unheaded_zhen (ADR-149;
-- S-ZHEN-SWITCH D2 (a)). Runs as well_zhen_owner (051).
--
-- This is the state of the eight zhen_* tables in unheaded_app after 049,
-- written from the files that built it, in their order:
--   zhen_conversations     003 (a fresh Well takes 003's table; 010's
--                          CREATE IF NOT EXISTS is then a no-op), 010's
--                          session index, 017's choice of 003's indexes,
--                          020 (created_at NOT NULL), 045 (client)
--   zhen_memories          008
--   zhen_actions           009, 011 (no action_type or triggered_by check),
--                          020/021 (the type/status index; 009's other
--                          indexes kept), 047 (status VARCHAR(40), the gate's
--                          verdicts)
--   zhen_action_snapshots  009
--   zhen_pending_confirmations  031, 046 (teach_write)
--   zhen_agent_runs        032, 037 (scheduled runs)
--   zhen_captures, zhen_capture_labels  033
-- and the grants app_zhen, app_kanban (041's columns) and retention_runner
-- (042's) hold on them after 049. ops_reader's SELECT on zhen_conversations
-- (007) does not follow: nothing that connects as ops_reader reads Zhen's
-- conversations, and the platform's reader has no business in Zhen's data.
--
-- One difference, on purpose: zhen_agent_runs.card_id keeps its type and
-- loses its foreign key, because the board (kanban_tasks) stays in
-- unheaded_app. The worker keeps the link: it claims the run first and the
-- card second, and cancels the run when the card is gone or taken
-- (pkg/zhenagent/sqlstore.go). scripts/well/zhen-schema-fingerprint.sql
-- shows that this foreign key is the only difference between the two.
--
-- Copying rows on a live Well is the attended S-ZHEN-SWITCH Phase 8B, never
-- a migration; 053 refuses to drop unheaded_app's tables while they hold
-- rows that were not copied.

-- LEDGER.sql makes well_append_only() in every database, but db/init.sh runs
-- it after a database's first file, and this is unheaded_zhen's first: the
-- triggers below need it now. Same body as LEDGER.sql's, which replaces it.
DO $do$
BEGIN
    IF to_regprocedure('public.well_append_only()') IS NULL THEN
        CREATE FUNCTION public.well_append_only() RETURNS TRIGGER AS $f$
        BEGIN
            RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
        END;
        $f$ LANGUAGE plpgsql;
    END IF;
END
$do$;

-- ─── zhen_conversations ─────────────────────────────────────────────────────
CREATE TABLE zhen_conversations (
    id             BIGSERIAL PRIMARY KEY,
    session_id     VARCHAR(64) NOT NULL,
    role           VARCHAR(10) NOT NULL
        CHECK (role IN ('user', 'assistant', 'system')),
    content        TEXT NOT NULL,
    sources        JSONB DEFAULT '[]',
    model          VARCHAR(100) DEFAULT '',
    tokens_input   INTEGER DEFAULT 0,
    tokens_output  INTEGER DEFAULT 0,
    elapsed_ms     INTEGER DEFAULT 0,
    feedback       VARCHAR(20) DEFAULT '',
    created_at     TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    search_vector  TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED,
    client         TEXT NOT NULL DEFAULT 'web',
    CONSTRAINT zhen_conversations_client_check CHECK (client IN ('web', 'cli'))
);
CREATE INDEX idx_zhen_session ON zhen_conversations(session_id);
CREATE INDEX idx_zhen_created ON zhen_conversations(created_at DESC);
CREATE INDEX idx_zhen_role ON zhen_conversations(role);
CREATE INDEX idx_zhen_model ON zhen_conversations(model);
CREATE INDEX idx_zhen_search ON zhen_conversations USING GIN(search_vector);
CREATE INDEX idx_zhen_conversations_session_created
    ON zhen_conversations (session_id, created_at DESC);

GRANT SELECT, INSERT ON zhen_conversations TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_conversations_id_seq TO app_zhen;
GRANT SELECT (id, created_at), DELETE ON zhen_conversations TO retention_runner;

-- ─── zhen_memories ──────────────────────────────────────────────────────────
CREATE TABLE zhen_memories (
    id          BIGSERIAL PRIMARY KEY,
    question    TEXT NOT NULL,
    answer      TEXT NOT NULL,
    embedding   BYTEA,
    source      VARCHAR(100) DEFAULT 'user',
    model       VARCHAR(50),
    created_at  TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_zhen_memories_created ON zhen_memories (created_at DESC);

GRANT SELECT, INSERT, UPDATE, DELETE ON zhen_memories TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_memories_id_seq TO app_zhen;

-- ─── zhen_actions, zhen_action_snapshots ────────────────────────────────────
CREATE TABLE zhen_actions (
    id              BIGSERIAL PRIMARY KEY,
    session_id      VARCHAR(64) NOT NULL,
    action_type     VARCHAR(50) NOT NULL,
    status          VARCHAR(40) NOT NULL DEFAULT 'planned',
    intent          TEXT NOT NULL,
    parameters      JSONB DEFAULT '{}',
    result          JSONB DEFAULT '{}',
    result_summary  TEXT DEFAULT '',
    error           TEXT DEFAULT '',
    revertable      BOOLEAN DEFAULT FALSE,
    reverted_by     BIGINT,
    reverts_action  BIGINT,
    triggered_by    VARCHAR(20) DEFAULT 'user',
    model           VARCHAR(50) DEFAULT '',
    confidence      REAL DEFAULT 0.0,
    planned_at      TIMESTAMPTZ DEFAULT NOW(),
    started_at      TIMESTAMPTZ,
    completed_at    TIMESTAMPTZ,
    elapsed_ms      INTEGER DEFAULT 0,
    trace_id        VARCHAR(64) DEFAULT '',
    parent_action   BIGINT,
    CONSTRAINT zhen_actions_status_check
        CHECK (status IN ('planned', 'executing', 'completed', 'failed', 'reverted', 'accepted', 'denied')
               OR status ~ '^denied_[a-z][a-z_]*$')
);
CREATE INDEX idx_zhen_actions_session ON zhen_actions(session_id);
CREATE INDEX idx_zhen_actions_type ON zhen_actions(action_type);
CREATE INDEX idx_zhen_actions_status ON zhen_actions(status);
CREATE INDEX idx_zhen_actions_planned ON zhen_actions(planned_at DESC);
CREATE INDEX idx_zhen_actions_trace ON zhen_actions(trace_id);
CREATE INDEX zhen_actions_type_status_idx ON zhen_actions (action_type, status);

CREATE TABLE zhen_action_snapshots (
    id              BIGSERIAL PRIMARY KEY,
    action_id       BIGINT NOT NULL REFERENCES zhen_actions(id),
    resource_type   VARCHAR(50) NOT NULL
        CHECK (resource_type IN (
            'file', 'kanban_task', 'config', 'memory',
            'corpus_chunk', 'service_state'
        )),
    resource_id     TEXT NOT NULL,
    content_before  TEXT,
    content_after   TEXT,
    metadata        JSONB DEFAULT '{}',
    snapshot_at     TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_zhen_snapshots_action ON zhen_action_snapshots(action_id);
CREATE INDEX idx_zhen_snapshots_resource ON zhen_action_snapshots(resource_type, resource_id);

GRANT SELECT, INSERT, UPDATE ON zhen_actions TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_actions_id_seq TO app_zhen;
GRANT SELECT, INSERT ON zhen_action_snapshots TO app_zhen;
GRANT USAGE, SELECT ON SEQUENCE zhen_action_snapshots_id_seq TO app_zhen;

-- ─── zhen_pending_confirmations ─────────────────────────────────────────────
CREATE TABLE zhen_pending_confirmations (
    id             TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{16}$'),
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    source_service TEXT NOT NULL CHECK (source_service IN ('zhen-claude', 'zhen-agentd')),
    emitted_by     TEXT NOT NULL CHECK (emitted_by <> '' AND length(emitted_by) <= 256),
    project_root   TEXT NOT NULL CHECK (project_root ~ '^/' AND length(project_root) <= 4096),
    tool           TEXT NOT NULL,
    call           JSONB NOT NULL CHECK (jsonb_typeof(call) = 'object'),
    call_sha256    CHAR(64) NOT NULL CHECK (call_sha256 ~ '^[0-9a-f]{64}$'),
    summary        TEXT NOT NULL CHECK (length(summary) <= 512),
    card_id        TEXT CHECK (card_id IS NULL OR length(card_id) <= 128),
    status         TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'denied', 'expired', 'executed', 'failed')),
    decided_at     TIMESTAMPTZ,
    decided_by     TEXT CHECK (decided_by IS NULL OR length(decided_by) <= 256),
    CHECK (expires_at > created_at),
    CHECK ((status = 'pending') = (decided_at IS NULL)),
    CONSTRAINT zhen_pending_confirmations_tool_check
        CHECK (tool IN ('write_file', 'patch_file', 'kanban_create', 'kanban_update', 'teach_write'))
);
CREATE INDEX zhen_pending_confirmations_pending_idx
    ON zhen_pending_confirmations (source_service, created_at) WHERE status = 'pending';

CREATE FUNCTION zhen_confirm_insert() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> 'pending' OR NEW.decided_at IS NOT NULL OR NEW.decided_by IS NOT NULL THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: a new row must be pending and undecided';
    END IF;
    IF (SELECT count(*) FROM zhen_pending_confirmations
         WHERE status = 'pending' AND source_service = NEW.source_service AND expires_at > now()) >= 20 THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: % already has 20 pending confirmations', NEW.source_service;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_confirm_insert BEFORE INSERT ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION zhen_confirm_insert();

CREATE FUNCTION zhen_confirm_transition() RETURNS TRIGGER AS $$
BEGIN
    IF NOT ((OLD.status = 'pending'  AND NEW.status IN ('approved', 'denied', 'expired')) OR
            (OLD.status = 'approved' AND NEW.status IN ('executed', 'failed'))) THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: % -> % refused', OLD.status, NEW.status;
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.created_at IS DISTINCT FROM OLD.created_at
       OR NEW.expires_at IS DISTINCT FROM OLD.expires_at OR NEW.source_service IS DISTINCT FROM OLD.source_service
       OR NEW.emitted_by IS DISTINCT FROM OLD.emitted_by OR NEW.project_root IS DISTINCT FROM OLD.project_root
       OR NEW.tool IS DISTINCT FROM OLD.tool OR NEW.call IS DISTINCT FROM OLD.call
       OR NEW.call_sha256 IS DISTINCT FROM OLD.call_sha256 OR NEW.summary IS DISTINCT FROM OLD.summary
       OR NEW.card_id IS DISTINCT FROM OLD.card_id THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: the call is immutable';
    END IF;
    IF OLD.status = 'pending' AND NEW.status = 'approved' AND now() >= OLD.expires_at THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: % expired at %', OLD.id, OLD.expires_at;
    END IF;
    IF OLD.status = 'approved' AND (NEW.decided_at IS DISTINCT FROM OLD.decided_at OR NEW.decided_by IS DISTINCT FROM OLD.decided_by) THEN
        RAISE EXCEPTION 'zhen_pending_confirmations: the decision is immutable';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_confirm_transition BEFORE UPDATE ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION zhen_confirm_transition();
CREATE TRIGGER trg_zhen_confirm_no_delete BEFORE DELETE ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE TRIGGER trg_zhen_confirm_no_truncate BEFORE TRUNCATE ON zhen_pending_confirmations
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT, UPDATE (status, decided_at, decided_by) ON zhen_pending_confirmations TO app_zhen;

-- ─── zhen_agent_runs (no foreign key to kanban_tasks: see the header) ───────
CREATE TABLE zhen_agent_runs (
    id              TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{16}$'),
    card_id         TEXT NOT NULL,
    claimed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until     TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'claimed'
        CHECK (status IN ('claimed', 'running', 'succeeded', 'failed', 'abandoned', 'cancelled', 'accepted', 'rejected')),
    mode            TEXT NOT NULL CHECK (mode IN ('interactive', 'overnight', 'scheduled')),
    family          TEXT NOT NULL CHECK (family ~ '^[a-z0-9-]{1,64}$'),
    branch          TEXT CHECK (branch ~ '^zhen/[a-z0-9-]{1,64}$'),
    base_commit     CHAR(40) CHECK (base_commit ~ '^[0-9a-f]{40}$'),
    head_commit     CHAR(40) CHECK (head_commit IS NULL OR head_commit ~ '^[0-9a-f]{40}$'),
    route_summary   JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(route_summary) = 'object'),
    claude_nano_usd BIGINT NOT NULL DEFAULT 0 CHECK (claude_nano_usd >= 0),
    review_reason   TEXT CHECK (review_reason IS NULL OR length(review_reason) <= 1000),
    reviewed_at     TIMESTAMPTZ,
    error           TEXT NOT NULL DEFAULT '' CHECK (length(error) <= 1000),
    CHECK (lease_until > claimed_at),
    CHECK ((status IN ('accepted', 'rejected')) = (reviewed_at IS NOT NULL)),
    CONSTRAINT zhen_agent_runs_scheduled_shape CHECK (
        CASE WHEN mode = 'scheduled'
             THEN branch IS NULL AND base_commit IS NULL AND head_commit IS NULL
                  AND status NOT IN ('accepted', 'rejected')
             ELSE branch IS NOT NULL AND base_commit IS NOT NULL
        END)
);
CREATE UNIQUE INDEX zhen_agent_runs_one_active ON zhen_agent_runs (card_id) WHERE status IN ('claimed', 'running');
CREATE INDEX zhen_agent_runs_card_idx ON zhen_agent_runs (card_id, claimed_at DESC);
CREATE INDEX zhen_agent_runs_claimed_at_idx ON zhen_agent_runs (claimed_at DESC);

CREATE FUNCTION zhen_agent_run_insert() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> 'claimed' OR NEW.head_commit IS NOT NULL OR NEW.reviewed_at IS NOT NULL OR NEW.review_reason IS NOT NULL THEN
        RAISE EXCEPTION 'zhen_agent_runs: a new run must be claimed, without a head commit or a review';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_agent_run_insert BEFORE INSERT ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION zhen_agent_run_insert();

CREATE FUNCTION zhen_agent_run_transition() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> OLD.status AND NOT (
         (OLD.status = 'claimed'   AND NEW.status IN ('running', 'abandoned', 'cancelled')) OR
         (OLD.status = 'running'   AND NEW.status IN ('succeeded', 'failed', 'abandoned', 'cancelled')) OR
         (OLD.status = 'succeeded' AND NEW.status IN ('accepted', 'rejected'))) THEN
        RAISE EXCEPTION 'zhen_agent_runs: % -> % refused', OLD.status, NEW.status;
    END IF;
    IF NEW.id IS DISTINCT FROM OLD.id OR NEW.card_id IS DISTINCT FROM OLD.card_id
       OR NEW.branch IS DISTINCT FROM OLD.branch OR NEW.base_commit IS DISTINCT FROM OLD.base_commit
       OR NEW.claimed_at IS DISTINCT FROM OLD.claimed_at OR NEW.mode IS DISTINCT FROM OLD.mode
       OR NEW.family IS DISTINCT FROM OLD.family THEN
        RAISE EXCEPTION 'zhen_agent_runs: identity columns are immutable';
    END IF;
    IF OLD.status NOT IN ('claimed', 'running') AND (
         NEW.lease_until IS DISTINCT FROM OLD.lease_until OR NEW.head_commit IS DISTINCT FROM OLD.head_commit
         OR NEW.route_summary IS DISTINCT FROM OLD.route_summary OR NEW.claude_nano_usd IS DISTINCT FROM OLD.claude_nano_usd
         OR NEW.error IS DISTINCT FROM OLD.error) THEN
        RAISE EXCEPTION 'zhen_agent_runs: the outcome of a finished run is immutable';
    END IF;
    IF NEW.status = OLD.status AND (NEW.reviewed_at IS DISTINCT FROM OLD.reviewed_at OR NEW.review_reason IS DISTINCT FROM OLD.review_reason) THEN
        RAISE EXCEPTION 'zhen_agent_runs: a review is recorded only with the move to accepted or rejected';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_agent_run_transition BEFORE UPDATE ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION zhen_agent_run_transition();
CREATE TRIGGER trg_zhen_agent_run_no_delete BEFORE DELETE ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE TRIGGER trg_zhen_agent_run_no_truncate BEFORE TRUNCATE ON zhen_agent_runs
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT,
      UPDATE (lease_until, status, head_commit, route_summary, claude_nano_usd, review_reason, reviewed_at, error)
   ON zhen_agent_runs TO app_zhen;
GRANT SELECT (id, card_id, claimed_at, status, mode, family, branch, head_commit, route_summary, review_reason, reviewed_at)
   ON zhen_agent_runs TO app_kanban;
GRANT SELECT (card_id, claimed_at) ON zhen_agent_runs TO retention_runner;

-- ─── zhen_captures, zhen_capture_labels ─────────────────────────────────────
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
    value       JSONB NOT NULL DEFAULT '{}'
        CHECK (jsonb_typeof(value) = 'object' AND octet_length(value::text) <= 4096),
    source_ref  TEXT NOT NULL CHECK (source_ref <> '' AND length(source_ref) <= 256),
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    CONSTRAINT zhen_capture_labels_once UNIQUE (capture_id, kind, source_ref)
);

CREATE FUNCTION zhen_captures_insert() RETURNS TRIGGER AS $$
BEGIN
    NEW.purge_after := NEW.captured_at + interval '180 days';
    IF NEW.promoted OR NEW.quarantined THEN
        RAISE EXCEPTION 'zhen_captures: a new capture is neither promoted nor quarantined';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_captures_insert BEFORE INSERT ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_insert();

CREATE FUNCTION zhen_captures_update() RETURNS TRIGGER AS $$
BEGIN
    IF (to_jsonb(NEW) - 'promoted' - 'quarantined' - 'bytes') IS DISTINCT FROM
       (to_jsonb(OLD) - 'promoted' - 'quarantined' - 'bytes') THEN
        RAISE EXCEPTION 'zhen_captures: % is immutable except promoted and quarantined', OLD.id;
    END IF;
    IF OLD.quarantined AND NOT NEW.quarantined THEN
        RAISE EXCEPTION 'zhen_captures: % is quarantined; it is purged, never released', OLD.id;
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_captures_update BEFORE UPDATE ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_update();

CREATE FUNCTION zhen_captures_guard_delete() RETURNS TRIGGER AS $$
BEGIN
    IF coalesce(current_setting('zhen.purge', true), '') <> 'on' THEN
        RAISE EXCEPTION '% rows are deleted only by zhen_captures_purge()', TG_TABLE_NAME;
    END IF;
    RETURN OLD;
END $$ LANGUAGE plpgsql;
CREATE TRIGGER trg_zhen_captures_guard_delete BEFORE DELETE ON zhen_captures
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_guard_delete();
CREATE TRIGGER trg_zhen_captures_no_truncate BEFORE TRUNCATE ON zhen_captures
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
CREATE TRIGGER trg_zhen_capture_labels_guard_delete BEFORE DELETE ON zhen_capture_labels
    FOR EACH ROW EXECUTE FUNCTION zhen_captures_guard_delete();
CREATE TRIGGER trg_zhen_capture_labels_append_only BEFORE UPDATE ON zhen_capture_labels
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE TRIGGER trg_zhen_capture_labels_no_truncate BEFORE TRUNCATE ON zhen_capture_labels
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

CREATE FUNCTION zhen_captures_purge() RETURNS INTEGER
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
GRANT EXECUTE ON FUNCTION zhen_captures_purge() TO app_zhen, retention_runner;

GRANT SELECT, INSERT, UPDATE (promoted, quarantined) ON zhen_captures TO app_zhen;
GRANT SELECT, INSERT ON zhen_capture_labels TO app_zhen;
GRANT SELECT (quarantined, promoted, purge_after) ON zhen_captures TO retention_runner;
