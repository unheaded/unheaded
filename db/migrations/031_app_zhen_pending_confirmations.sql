-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — out-of-band approval queue for model-proposed writes
-- (S-TWO-BRAINS Phase 2, ADR-104 open question 2), unheaded_app.
--
-- A mutating tool call that the Champion gate refuses pending confirmation
-- (Rule 2) is queued here by zhen-claude -confirm queue or zhen-agentd.
-- Stevie approves it with `zhen-claude confirm approve <id>`, a separate
-- process under his own login: nothing in this table is ever shown to the
-- model, and the id alone authorises nothing.
--
-- States move one way: pending -> approved | denied | expired, and
-- approved -> executed | failed. The call itself (tool, args, hash, root,
-- who emitted it) never changes after insert, even for the owner; only
-- status, decided_at and decided_by do. An approval after expires_at is
-- refused. At most 20 rows per source_service may be pending at once
-- (queue flooding; the writers enforce the same cap).
--
-- Writer: app_zhen (SELECT, INSERT, UPDATE of status/decided_at/decided_by;
-- no DELETE). Owner: well_app_owner.
--
-- Access paths: `WHERE status = 'pending'` by age (confirm list, the cap),
-- served by the partial index; `WHERE id = $1` (show/approve) by the key.
--
-- Retention: rows are kept. `call` may hold a file body; bodies older than
-- 7 days after a decision are to be blanked by a later maintenance
-- migration (the trigger below will need an exception for it then).
--
-- Lock plan: a new table, no lock on anything that exists. Rollback: a
-- later numbered migration drops it. Proof: scripts/well/test-zhen-confirm.sh.

CREATE TABLE zhen_pending_confirmations (
    id             TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{16}$'),
    -- well:ignore text-surrogate-pk a 16-hex id Stevie types at the CLI (zhen-claude review|confirm), not a join key; Erda accepted it as P3 (S-TWO-BRAINS Steps 198, 202)
    created_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    expires_at     TIMESTAMPTZ NOT NULL,
    source_service TEXT NOT NULL CHECK (source_service IN ('zhen-claude', 'zhen-agentd')),
    emitted_by     TEXT NOT NULL CHECK (emitted_by <> '' AND length(emitted_by) <= 256),
    project_root   TEXT NOT NULL CHECK (project_root ~ '^/' AND length(project_root) <= 4096),
    tool           TEXT NOT NULL CHECK (tool IN ('write_file', 'patch_file', 'kanban_create', 'kanban_update')),
    call           JSONB NOT NULL CHECK (jsonb_typeof(call) = 'object'),
    call_sha256    CHAR(64) NOT NULL CHECK (call_sha256 ~ '^[0-9a-f]{64}$'),
    summary        TEXT NOT NULL CHECK (length(summary) <= 512),
    card_id        TEXT CHECK (card_id IS NULL OR length(card_id) <= 128),
    status         TEXT NOT NULL DEFAULT 'pending'
        CHECK (status IN ('pending', 'approved', 'denied', 'expired', 'executed', 'failed')),
    decided_at     TIMESTAMPTZ,
    decided_by     TEXT CHECK (decided_by IS NULL OR length(decided_by) <= 256),
    CHECK (expires_at > created_at),
    CHECK ((status = 'pending') = (decided_at IS NULL))
);

CREATE INDEX zhen_pending_confirmations_pending_idx
    ON zhen_pending_confirmations (source_service, created_at) WHERE status = 'pending';

-- A row is born pending, under the per-service cap.
CREATE OR REPLACE FUNCTION zhen_confirm_insert() RETURNS TRIGGER AS $$
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

CREATE OR REPLACE TRIGGER trg_zhen_confirm_insert BEFORE INSERT ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION zhen_confirm_insert();

-- One-way transitions; the call is immutable.
CREATE OR REPLACE FUNCTION zhen_confirm_transition() RETURNS TRIGGER AS $$
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

CREATE OR REPLACE TRIGGER trg_zhen_confirm_transition BEFORE UPDATE ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION zhen_confirm_transition();

-- Rows are the record of what was approved: never deleted or truncated.
CREATE OR REPLACE TRIGGER trg_zhen_confirm_no_delete BEFORE DELETE ON zhen_pending_confirmations
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_zhen_confirm_no_truncate BEFORE TRUNCATE ON zhen_pending_confirmations
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT, UPDATE (status, decided_at, decided_by) ON zhen_pending_confirmations TO app_zhen;
