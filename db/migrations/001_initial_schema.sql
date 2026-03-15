-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Kingdom Persistent Memory
-- Initial schema for Unheaded services

-- ═══ KANBAN ═══
CREATE TABLE IF NOT EXISTS kanban_tasks (
    id SERIAL PRIMARY KEY,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'todo'
        CHECK (status IN ('backlog', 'todo', 'in-progress', 'review', 'done')),
    priority VARCHAR(5) DEFAULT 'P2'
        CHECK (priority IN ('P0', 'P1', 'P2', 'P3')),
    tags TEXT[] DEFAULT '{}',
    assignee TEXT DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    completed_at TIMESTAMPTZ
);

CREATE INDEX idx_kanban_status ON kanban_tasks(status);
CREATE INDEX idx_kanban_priority ON kanban_tasks(priority);

-- ═══ TIMELINE ═══
CREATE TABLE IF NOT EXISTS timeline_milestones (
    id SERIAL PRIMARY KEY,
    sprint VARCHAR(20) NOT NULL,
    title TEXT NOT NULL,
    description TEXT DEFAULT '',
    status VARCHAR(20) NOT NULL DEFAULT 'planned'
        CHECK (status IN ('planned', 'in-progress', 'completed', 'deferred')),
    target_date DATE,
    completed_date DATE,
    tags TEXT[] DEFAULT '{}',
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_timeline_sprint ON timeline_milestones(sprint);
CREATE INDEX idx_timeline_status ON timeline_milestones(status);

-- ═══ AUDIT LOG (Anamnesis) ═══
CREATE TABLE IF NOT EXISTS audit_events (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    service VARCHAR(50) NOT NULL,
    event_type VARCHAR(50) NOT NULL,
    severity VARCHAR(10) DEFAULT 'info'
        CHECK (severity IN ('debug', 'info', 'warn', 'error', 'critical')),
    actor TEXT DEFAULT '',
    resource TEXT DEFAULT '',
    action TEXT DEFAULT '',
    details JSONB DEFAULT '{}',
    trace_id VARCHAR(64) DEFAULT '',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_timestamp ON audit_events(timestamp DESC);
CREATE INDEX idx_audit_service ON audit_events(service);
CREATE INDEX idx_audit_event_type ON audit_events(event_type);
CREATE INDEX idx_audit_severity ON audit_events(severity);
CREATE INDEX idx_audit_trace_id ON audit_events(trace_id);

-- ═══ SERVICE HEALTH ═══
CREATE TABLE IF NOT EXISTS service_health (
    id SERIAL PRIMARY KEY,
    service_name VARCHAR(50) NOT NULL,
    host VARCHAR(50) DEFAULT 'west',
    status VARCHAR(10) NOT NULL DEFAULT 'unknown'
        CHECK (status IN ('healthy', 'degraded', 'unhealthy', 'unknown')),
    port INTEGER,
    last_check TIMESTAMPTZ DEFAULT NOW(),
    response_time_ms INTEGER,
    details JSONB DEFAULT '{}',
    UNIQUE(service_name, host)
);

-- ═══ LOG AGGREGATION ═══
CREATE TABLE IF NOT EXISTS service_logs (
    id BIGSERIAL PRIMARY KEY,
    timestamp TIMESTAMPTZ DEFAULT NOW(),
    service VARCHAR(50) NOT NULL,
    level VARCHAR(10) NOT NULL DEFAULT 'info',
    message TEXT NOT NULL,
    fields JSONB DEFAULT '{}',
    trace_id VARCHAR(64) DEFAULT '',
    host VARCHAR(50) DEFAULT 'west'
);

CREATE INDEX idx_logs_timestamp ON service_logs(timestamp DESC);
CREATE INDEX idx_logs_service ON service_logs(service);
CREATE INDEX idx_logs_level ON service_logs(level);

-- ═══ ZHEN CONVERSATIONS ═══
CREATE TABLE IF NOT EXISTS zhen_conversations (
    id SERIAL PRIMARY KEY,
    session_id VARCHAR(64) NOT NULL,
    question TEXT NOT NULL,
    answer TEXT NOT NULL,
    sources JSONB DEFAULT '[]',
    tokens_used INTEGER DEFAULT 0,
    elapsed_seconds REAL DEFAULT 0,
    created_at TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_zhen_session ON zhen_conversations(session_id);
CREATE INDEX idx_zhen_created ON zhen_conversations(created_at DESC);

-- ═══ KINGDOM CONFIGURATION ═══
CREATE TABLE IF NOT EXISTS kingdom_config (
    key VARCHAR(100) PRIMARY KEY,
    value JSONB NOT NULL,
    description TEXT DEFAULT '',
    updated_at TIMESTAMPTZ DEFAULT NOW(),
    updated_by TEXT DEFAULT 'system'
);

-- Insert default config
INSERT INTO kingdom_config (key, value, description) VALUES
    ('kingdom.name', '"Unheaded"', 'Kingdom name'),
    ('kingdom.version', '"0.1.0-alpha"', 'Current version'),
    ('kingdom.hosts', '["west", "east"]', 'Active bare metal hosts'),
    ('kingdom.mode', '"development"', 'Operating mode')
ON CONFLICT (key) DO NOTHING;
