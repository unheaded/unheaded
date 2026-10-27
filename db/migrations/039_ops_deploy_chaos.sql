-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — deploy history and the chaos revert journal, unheaded_ops
-- (S-WELL-SPRING Phase 2, M-072; ADR-099 §4 "a journal of what was
-- applied"; ADR-107 D-19).
--
-- Sword (services/sword) kept its deployment history, and chaos-controller
-- (services/yaldabaoth, `packet-chaos`) its revert journal, in JSON files
-- under their data dirs (pkg/statefile). With WELL_DB set they write here
-- instead, as ops_writer; the files stay only as the explicit no-Well mode.
--
-- deploy_history             one row per deployment: what was asked for
--                            (service, version, strategy, the request as
--                            spec). Never changes.
-- deploy_history_transitions every change to a deployment, newest = current:
--                            from/to status and the whole record after the
--                            change (state). A record's status, messages
--                            and timestamps are read from its newest row.
-- chaos_journal              one row per fault, written BEFORE the fault
--                            lands: its type, target and revert recipe.
-- chaos_journal_transitions  'reverted' once the injector verified the fault
--                            is gone. Open faults = journal rows without one.
--
-- All four are append-only by trigger (as 015/030): an UPDATE, DELETE or
-- TRUNCATE is refused even for the owner. A reverted fault is a new row,
-- never an edit; retention will drop time partitions (W-15), never rows.
-- What is stored is the deploy request and the fault recipe (interface
-- names, tc arguments, instance names): never packet contents or anything
-- a user sent (DoD-STD 8).
--
-- Writer: ops_writer (SELECT, INSERT). Reader: ops_reader. Owner:
-- well_ops_owner (the runner SET ROLEs to it).
--
-- Access paths: Sword loads its own host's newest records at start
-- (deploy_history_host_idx: a Sword deploys through its host's Cuirass, so
-- only those are rollback targets); people list by service
-- (deploy_history_service_idx); a
-- record's newest transition by deployment_id (the transitions index).
-- The controller loads open faults for its own host and kind
-- (chaos_journal_host_kind_idx). Status columns have six values or one and
-- are not indexed (W-14).
--
-- Lock plan: new tables, no lock on anything that exists. Rollback: none
-- needed for empty new tables; a later numbered migration drops them.
-- Proof: scripts/well/test-deploy-journal.sh (throwaway servers).

CREATE TABLE deploy_history (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id  TEXT NOT NULL UNIQUE CHECK (deployment_id ~ '^[a-z0-9][a-z0-9-]{0,63}$'),
    service        TEXT NOT NULL CHECK (service ~ '^[a-z]([a-z0-9-]{0,30}[a-z0-9])?$'),
    version        TEXT NOT NULL CHECK (version ~ '^[a-z0-9]([a-z0-9-]{0,30}[a-z0-9])?$'),
    strategy       TEXT NOT NULL CHECK (strategy IN ('rolling', 'blue_green', 'canary', 'recreate')),
    replicas       INTEGER NOT NULL CHECK (replicas BETWEEN 1 AND 20),
    spec           JSONB NOT NULL CHECK (jsonb_typeof(spec) = 'object'),
    host           TEXT NOT NULL CHECK (length(host) BETWEEN 1 AND 253),
    created_at     TIMESTAMPTZ NOT NULL,
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deploy_history_host_idx ON deploy_history (host, created_at DESC);
CREATE INDEX deploy_history_service_idx ON deploy_history (service, created_at DESC);

CREATE TABLE deploy_history_transitions (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    deployment_id  TEXT NOT NULL REFERENCES deploy_history (deployment_id),
    from_status    TEXT NOT NULL
        CHECK (from_status IN ('', 'pending', 'in_progress', 'succeeded', 'failed', 'rolling_back', 'rolled_back')),
    to_status      TEXT NOT NULL
        CHECK (to_status IN ('pending', 'in_progress', 'succeeded', 'failed', 'rolling_back', 'rolled_back')),
    note           TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 512),
    state          JSONB NOT NULL CHECK (jsonb_typeof(state) = 'object'),
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX deploy_history_transitions_deployment_idx ON deploy_history_transitions (deployment_id, id DESC);

CREATE TABLE chaos_journal (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entry_id       TEXT NOT NULL UNIQUE CHECK (entry_id ~ '^[A-Za-z0-9][A-Za-z0-9._-]{0,127}$'),
    host           TEXT NOT NULL CHECK (length(host) BETWEEN 1 AND 253),
    kind           TEXT NOT NULL CHECK (kind IN ('experiment', 'packet')),
    fault_type     TEXT NOT NULL CHECK (length(fault_type) BETWEEN 1 AND 64),
    target         JSONB NOT NULL CHECK (jsonb_typeof(target) = 'object'),
    recipe         JSONB NOT NULL CHECK (jsonb_typeof(recipe) = 'object'),
    created_at     TIMESTAMPTZ NOT NULL,
    expires_at     TIMESTAMPTZ NOT NULL CHECK (expires_at >= created_at),
    recorded_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE INDEX chaos_journal_host_kind_idx ON chaos_journal (host, kind, created_at);

CREATE TABLE chaos_journal_transitions (
    id             BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    entry_id       TEXT NOT NULL REFERENCES chaos_journal (entry_id),
    to_status      TEXT NOT NULL CHECK (to_status IN ('reverted')),
    note           TEXT NOT NULL DEFAULT '' CHECK (length(note) <= 512),
    occurred_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (entry_id, to_status)
);

CREATE OR REPLACE TRIGGER trg_deploy_history_append_only
    BEFORE UPDATE OR DELETE ON deploy_history
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_deploy_history_no_truncate
    BEFORE TRUNCATE ON deploy_history
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_deploy_history_transitions_append_only
    BEFORE UPDATE OR DELETE ON deploy_history_transitions
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_deploy_history_transitions_no_truncate
    BEFORE TRUNCATE ON deploy_history_transitions
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_chaos_journal_append_only
    BEFORE UPDATE OR DELETE ON chaos_journal
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_chaos_journal_no_truncate
    BEFORE TRUNCATE ON chaos_journal
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_chaos_journal_transitions_append_only
    BEFORE UPDATE OR DELETE ON chaos_journal_transitions
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_chaos_journal_transitions_no_truncate
    BEFORE TRUNCATE ON chaos_journal_transitions
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

-- For people with psql: each deployment as it stands now, and the faults
-- still in place.
CREATE VIEW deploy_history_current AS
SELECT h.deployment_id, h.service, h.version, h.strategy, h.replicas, h.host,
       t.to_status AS status, t.note, h.created_at, t.occurred_at AS updated_at
FROM deploy_history h
JOIN LATERAL (
    SELECT to_status, note, occurred_at
    FROM deploy_history_transitions x
    WHERE x.deployment_id = h.deployment_id
    ORDER BY x.id DESC
    LIMIT 1
) t ON true;

CREATE VIEW chaos_journal_open AS
SELECT j.entry_id, j.host, j.kind, j.fault_type, j.target, j.created_at, j.expires_at
FROM chaos_journal j
WHERE NOT EXISTS (
    SELECT 1 FROM chaos_journal_transitions t
    WHERE t.entry_id = j.entry_id AND t.to_status = 'reverted'
);

GRANT SELECT, INSERT ON deploy_history, deploy_history_transitions, chaos_journal, chaos_journal_transitions TO ops_writer;
GRANT SELECT ON deploy_history_current, chaos_journal_open TO ops_writer;
GRANT SELECT ON deploy_history, deploy_history_transitions, chaos_journal, chaos_journal_transitions,
    deploy_history_current, chaos_journal_open TO ops_reader;
