-- SPDX-License-Identifier: GPL-3.0-or-later
-- One-off: converge the live Well's unheaded_app to the shape a fresh
-- bootstrap has through 020 (ERDA 0.4 / 3.4, 2026-10-15). Not a migration
-- and never run on a fresh Well.
--
-- The live unheaded_app was built by service startup DDL, not by 003:
--   kanban_tasks        guid nullable; no created_at/deleted_at indexes,
--                       no updated_at trigger
--   timeline_milestones missing
--   zhen_conversations  010's shape: session_id text, role varchar(20),
--                       model varchar(64) without default, no feedback, role
--                       CHECK without 'system', 010's index names, and
--                       003's session/role/model indexes missing
--   grants              003's and 007's never ran
--   zhen_actions/_snapshots CHECKs: same meaning, different stored form
-- Columns of zhen_actions, zhen_action_snapshots and zhen_memories match.
-- Checked first on a restore of backup set 2026-10-15/well-123943Z:
-- scripts/well/test-converge-live-app.sh. Afterwards:
--   well-migrate baseline -db unheaded_app -through 020_app_adopt_startup_ddl.sql -yes
-- Data limits checked on live before writing: guid has no NULLs (73/73
-- distinct), max session_id 36 chars, max role 9, max model 31.

BEGIN;
SET LOCAL lock_timeout = '5s';
SET LOCAL statement_timeout = '2min';

CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- kanban_tasks
ALTER TABLE kanban_tasks ALTER COLUMN guid SET NOT NULL;
CREATE INDEX idx_kanban_created_at ON kanban_tasks(created_at DESC);
CREATE INDEX idx_kanban_deleted_at ON kanban_tasks(deleted_at) WHERE deleted_at IS NOT NULL;
CREATE TRIGGER trg_kanban_tasks_updated_at
    BEFORE UPDATE ON kanban_tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- timeline_milestones, as 003
CREATE TABLE timeline_milestones (
    id             BIGSERIAL PRIMARY KEY,
    sprint         VARCHAR(20) NOT NULL,
    phase          VARCHAR(50) DEFAULT '',
    title          TEXT NOT NULL,
    description    TEXT DEFAULT '',
    status         VARCHAR(20) NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'in-progress', 'completed', 'deferred')),
    progress_pct   SMALLINT DEFAULT 0 CHECK (progress_pct BETWEEN 0 AND 100),
    target_date    DATE,
    completed_date DATE,
    depends_on     BIGINT[] DEFAULT '{}',
    tags           TEXT[] DEFAULT '{}',
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    updated_at     TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_timeline_sprint ON timeline_milestones(sprint);
CREATE INDEX idx_timeline_status ON timeline_milestones(status);
CREATE INDEX idx_timeline_phase ON timeline_milestones(phase);
CREATE INDEX idx_timeline_target_date ON timeline_milestones(target_date);
CREATE TRIGGER trg_timeline_milestones_updated_at
    BEFORE UPDATE ON timeline_milestones
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- zhen_conversations, to 003's shape (created_at NOT NULL kept: 020)
ALTER TABLE zhen_conversations
    ALTER COLUMN session_id TYPE VARCHAR(64),
    ALTER COLUMN role TYPE VARCHAR(10),
    ALTER COLUMN model TYPE VARCHAR(100),
    ALTER COLUMN model SET DEFAULT '',
    ADD COLUMN feedback VARCHAR(20) DEFAULT '';
ALTER TABLE zhen_conversations DROP CONSTRAINT zhen_conversations_role_check;
ALTER TABLE zhen_conversations ADD CONSTRAINT zhen_conversations_role_check
    CHECK (role IN ('user', 'assistant', 'system'));
ALTER INDEX idx_zhen_conversations_created RENAME TO idx_zhen_created;
ALTER INDEX idx_zhen_conversations_search RENAME TO idx_zhen_search;
CREATE INDEX idx_zhen_session ON zhen_conversations(session_id);
CREATE INDEX idx_zhen_role ON zhen_conversations(role);
CREATE INDEX idx_zhen_model ON zhen_conversations(model);

-- 003's grants
GRANT USAGE ON SCHEMA public TO app_kanban, app_timeguru, app_zhen;
-- 003 grants ALL SEQUENCES when only its own exist; here the zhen
-- sequences exist too, so name 003's.
GRANT USAGE, SELECT ON SEQUENCE timeline_milestones_id_seq, zhen_conversations_id_seq
    TO app_kanban, app_timeguru, app_zhen;
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_kanban;
GRANT INSERT, UPDATE, DELETE ON kanban_tasks TO app_kanban;
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_timeguru;
GRANT INSERT, UPDATE, DELETE ON timeline_milestones TO app_timeguru;
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_zhen;
GRANT INSERT, UPDATE, DELETE ON zhen_conversations TO app_zhen;

-- 007's unheaded_app grants (never ran here)
GRANT CONNECT ON DATABASE unheaded_app TO ops_reader;
GRANT USAGE ON SCHEMA public TO ops_reader;
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO ops_reader;

-- The champion's startup DDL wrote two CHECKs with the same meaning as
-- 009's but a different stored form; re-create them from 009's text.
ALTER TABLE zhen_actions DROP CONSTRAINT zhen_actions_status_check;
ALTER TABLE zhen_actions ADD CONSTRAINT zhen_actions_status_check
    CHECK (status IN ('planned', 'executing', 'completed', 'failed', 'reverted'));
ALTER TABLE zhen_action_snapshots DROP CONSTRAINT zhen_action_snapshots_resource_type_check;
ALTER TABLE zhen_action_snapshots ADD CONSTRAINT zhen_action_snapshots_resource_type_check
    CHECK (resource_type IN ('file', 'kanban_task', 'config', 'memory', 'corpus_chunk', 'service_state'));

COMMIT;
