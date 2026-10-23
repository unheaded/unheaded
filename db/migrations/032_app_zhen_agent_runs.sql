-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — card claims, leases and outcomes of zhen-agentd's worker
-- (S-TWO-BRAINS Phase 6), unheaded_app.
--
-- `zhen-agentd -worker` picks an eligible Kanban card (owner 'zhen', a
-- ```zhen block in its description), claims it with one row here, works
-- it in its own zhen/<card> worktree and leaves a branch for Stevie to
-- review; `zhen-claude review accept|reject` records his decision on the
-- same row. Nothing is pushed.
--
-- One active run per card: a partial unique index over the active states
-- (claimed, running), so two workers racing for one card cannot both win.
-- A lease (lease_until, 30 min, renewed every 5 min by the worker) that
-- runs out turns the run 'abandoned'; its worktree is kept for inspection.
--
-- States move one way:
--   claimed   -> running | abandoned | cancelled
--   running   -> succeeded | failed | abandoned | cancelled
--   succeeded -> accepted | rejected
-- The run's identity (id, card, branch, base commit, mode, family, when
-- it was claimed) never changes after insert, the owner included. A new
-- row must be 'claimed'. reviewed_at and review_reason are set only by
-- the move to accepted/rejected.
--
-- kanban_tasks: app_zhen already reads it (003); it gains UPDATE of the
-- columns the worker moves (status, progress, commits, updated_at). Those
-- writes are the daemon acting on Stevie's behalf, audited as
-- actor_type 'service' in unheaded_ops.audit_events; they are not model
-- tool calls. The column grant is also what lets the worker take
-- `SELECT ... FOR UPDATE SKIP LOCKED` on a card.
--
-- The card reference is a plain foreign key (RESTRICT): a card with run
-- history cannot be hard-deleted; kanban-app's delete is a soft delete
-- (deleted_at), which is unaffected.
--
-- Writer: app_zhen (SELECT, INSERT, UPDATE of the mutable columns; no
-- DELETE). Owner: well_app_owner.
--
-- Access paths: the claim and the attempt count look up runs by card
-- (card_id, claimed_at), served by zhen_agent_runs_card_idx, which also
-- serves the foreign key; `zhen-claude review list` reads the newest runs
-- (claimed_at DESC); expiry scans the active rows, a handful at most.
--
-- Retention: rows are kept (one per card attempt; low volume).
--
-- Lock plan: a new table, no lock on anything that exists except the
-- GRANT on kanban_tasks (a catalog update, instant). Rollback: a later
-- numbered migration drops the table and revokes the grant. Proof:
-- scripts/well/test-zhen-agent-runs.sh.

CREATE TABLE zhen_agent_runs (
    id              TEXT PRIMARY KEY CHECK (id ~ '^[0-9a-f]{16}$'),
    -- well:ignore text-surrogate-pk a 16-hex id Stevie types at the CLI (zhen-claude review|confirm), not a join key; Erda accepted it as P3 (S-TWO-BRAINS Steps 198, 202)
    card_id         TEXT NOT NULL REFERENCES kanban_tasks (id),
    claimed_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    lease_until     TIMESTAMPTZ NOT NULL,
    status          TEXT NOT NULL DEFAULT 'claimed'
        CHECK (status IN ('claimed', 'running', 'succeeded', 'failed', 'abandoned', 'cancelled', 'accepted', 'rejected')),
    mode            TEXT NOT NULL CHECK (mode IN ('interactive', 'overnight')),
    family          TEXT NOT NULL CHECK (family ~ '^[a-z0-9-]{1,64}$'),
    branch          TEXT NOT NULL CHECK (branch ~ '^zhen/[a-z0-9-]{1,64}$'),
    base_commit     CHAR(40) NOT NULL CHECK (base_commit ~ '^[0-9a-f]{40}$'),
    head_commit     CHAR(40) CHECK (head_commit IS NULL OR head_commit ~ '^[0-9a-f]{40}$'),
    route_summary   JSONB NOT NULL DEFAULT '{}' CHECK (jsonb_typeof(route_summary) = 'object'),
    claude_nano_usd BIGINT NOT NULL DEFAULT 0 CHECK (claude_nano_usd >= 0),
    review_reason   TEXT CHECK (review_reason IS NULL OR length(review_reason) <= 1000),
    reviewed_at     TIMESTAMPTZ,
    error           TEXT NOT NULL DEFAULT '' CHECK (length(error) <= 1000),
    CHECK (lease_until > claimed_at),
    CHECK ((status IN ('accepted', 'rejected')) = (reviewed_at IS NOT NULL))
);

CREATE UNIQUE INDEX zhen_agent_runs_one_active ON zhen_agent_runs (card_id) WHERE status IN ('claimed', 'running');
CREATE INDEX zhen_agent_runs_card_idx ON zhen_agent_runs (card_id, claimed_at DESC);
CREATE INDEX zhen_agent_runs_claimed_at_idx ON zhen_agent_runs (claimed_at DESC);

-- A run is born claimed.
CREATE OR REPLACE FUNCTION zhen_agent_run_insert() RETURNS TRIGGER AS $$
BEGIN
    IF NEW.status <> 'claimed' OR NEW.head_commit IS NOT NULL OR NEW.reviewed_at IS NOT NULL OR NEW.review_reason IS NOT NULL THEN
        RAISE EXCEPTION 'zhen_agent_runs: a new run must be claimed, without a head commit or a review';
    END IF;
    RETURN NEW;
END $$ LANGUAGE plpgsql;

CREATE OR REPLACE TRIGGER trg_zhen_agent_run_insert BEFORE INSERT ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION zhen_agent_run_insert();

-- One-way transitions; the identity columns are immutable.
CREATE OR REPLACE FUNCTION zhen_agent_run_transition() RETURNS TRIGGER AS $$
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
    -- Only an active run's lease moves; a finished run's outcome is fixed,
    -- except the review fields on the way to accepted/rejected.
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

CREATE OR REPLACE TRIGGER trg_zhen_agent_run_transition BEFORE UPDATE ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION zhen_agent_run_transition();

-- Runs are the record of what the agent did: never deleted or truncated.
CREATE OR REPLACE TRIGGER trg_zhen_agent_run_no_delete BEFORE DELETE ON zhen_agent_runs
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_zhen_agent_run_no_truncate BEFORE TRUNCATE ON zhen_agent_runs
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT,
      UPDATE (lease_until, status, head_commit, route_summary, claude_nano_usd, review_reason, reviewed_at, error)
   ON zhen_agent_runs TO app_zhen;
-- SELECT on kanban_tasks is 003's; the worker moves these columns only.
GRANT UPDATE (status, progress, commits, updated_at) ON kanban_tasks TO app_zhen;
