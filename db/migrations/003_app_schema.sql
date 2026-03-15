-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Application Database Schema (unheaded_app)

-- ═══════════════════════════════════════════════════════════════════════════════
-- UPDATED_AT TRIGGER
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE OR REPLACE FUNCTION update_updated_at()
RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = NOW();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

-- ═══════════════════════════════════════════════════════════════════════════════
-- KANBAN TASKS
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE kanban_tasks (
    id           BIGSERIAL PRIMARY KEY,
    title        TEXT NOT NULL,
    description  TEXT DEFAULT '',
    status       VARCHAR(20) NOT NULL DEFAULT 'todo'
        CHECK (status IN ('backlog', 'todo', 'in-progress', 'review', 'done')),
    priority     VARCHAR(5) DEFAULT 'P2'
        CHECK (priority IN ('P0', 'P1', 'P2', 'P3')),
    tags         TEXT[] DEFAULT '{}',
    assignee     TEXT DEFAULT '',
    sort_order   INTEGER DEFAULT 0,
    created_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ,
    deleted_at   TIMESTAMPTZ
);

CREATE INDEX idx_kanban_status ON kanban_tasks(status);
CREATE INDEX idx_kanban_priority ON kanban_tasks(priority);
CREATE INDEX idx_kanban_assignee ON kanban_tasks(assignee);
CREATE INDEX idx_kanban_sort_order ON kanban_tasks(sort_order);
CREATE INDEX idx_kanban_created_at ON kanban_tasks(created_at DESC);
CREATE INDEX idx_kanban_deleted_at ON kanban_tasks(deleted_at) WHERE deleted_at IS NOT NULL;

CREATE TRIGGER trg_kanban_tasks_updated_at
    BEFORE UPDATE ON kanban_tasks
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ═══════════════════════════════════════════════════════════════════════════════
-- TIMELINE MILESTONES
-- ═══════════════════════════════════════════════════════════════════════════════
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

-- ═══════════════════════════════════════════════════════════════════════════════
-- ZHEN CONVERSATIONS
-- ═══════════════════════════════════════════════════════════════════════════════
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
    created_at     TIMESTAMPTZ DEFAULT NOW(),
    search_vector  TSVECTOR GENERATED ALWAYS AS (to_tsvector('english', content)) STORED
);

CREATE INDEX idx_zhen_session ON zhen_conversations(session_id);
CREATE INDEX idx_zhen_created ON zhen_conversations(created_at DESC);
CREATE INDEX idx_zhen_role ON zhen_conversations(role);
CREATE INDEX idx_zhen_model ON zhen_conversations(model);
CREATE INDEX idx_zhen_search ON zhen_conversations USING GIN(search_vector);

-- ═══════════════════════════════════════════════════════════════════════════════
-- GRANTS
-- ═══════════════════════════════════════════════════════════════════════════════
GRANT USAGE ON SCHEMA public TO app_kanban, app_timeguru, app_zhen;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO app_kanban, app_timeguru, app_zhen;

-- app_kanban: SELECT on all, full CRUD on kanban_tasks
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_kanban;
GRANT INSERT, UPDATE, DELETE ON kanban_tasks TO app_kanban;

-- app_timeguru: SELECT on all, full CRUD on timeline_milestones
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_timeguru;
GRANT INSERT, UPDATE, DELETE ON timeline_milestones TO app_timeguru;

-- app_zhen: SELECT on all, full CRUD on zhen_conversations
GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO app_zhen;
GRANT INSERT, UPDATE, DELETE ON zhen_conversations TO app_zhen;
