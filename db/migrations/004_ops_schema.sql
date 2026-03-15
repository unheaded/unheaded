-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Operations Database Schema (unheaded_ops)

-- ═══════════════════════════════════════════════════════════════════════════════
-- SERVICE HEALTH — CURRENT STATE (UPSERT target)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE service_health_current (
    service_name    VARCHAR(50) NOT NULL,
    host            VARCHAR(50) NOT NULL DEFAULT 'west',
    status          VARCHAR(10) NOT NULL DEFAULT 'unknown'
        CHECK (status IN ('healthy', 'degraded', 'unhealthy', 'unknown')),
    port            INTEGER,
    response_time_ms INTEGER,
    uptime_seconds  BIGINT DEFAULT 0,
    details         JSONB DEFAULT '{}',
    last_check      TIMESTAMPTZ DEFAULT NOW(),
    PRIMARY KEY (service_name, host)
);

CREATE INDEX idx_health_current_status ON service_health_current(status);

-- ═══════════════════════════════════════════════════════════════════════════════
-- SERVICE HEALTH — STATE TRANSITIONS (append-only log)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE service_health_transitions (
    id              BIGSERIAL PRIMARY KEY,
    service_name    VARCHAR(50) NOT NULL,
    host            VARCHAR(50) NOT NULL DEFAULT 'west',
    old_status      VARCHAR(10) NOT NULL,
    new_status      VARCHAR(10) NOT NULL,
    response_time_ms INTEGER,
    reason          TEXT DEFAULT '',
    details         JSONB DEFAULT '{}',
    transition_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_health_transitions_service ON service_health_transitions(service_name, host);
CREATE INDEX idx_health_transitions_at ON service_health_transitions(transition_at DESC);
CREATE INDEX idx_health_transitions_new_status ON service_health_transitions(new_status);

-- ═══════════════════════════════════════════════════════════════════════════════
-- SERVICE HEALTH — HOURLY AGGREGATES
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE service_health_hourly (
    service_name     VARCHAR(50) NOT NULL,
    host             VARCHAR(50) NOT NULL DEFAULT 'west',
    hour             TIMESTAMPTZ NOT NULL,
    checks_total     INTEGER DEFAULT 0,
    checks_healthy   INTEGER DEFAULT 0,
    checks_degraded  INTEGER DEFAULT 0,
    checks_unhealthy INTEGER DEFAULT 0,
    avg_response_ms  REAL DEFAULT 0,
    max_response_ms  INTEGER DEFAULT 0,
    min_response_ms  INTEGER DEFAULT 0,
    PRIMARY KEY (service_name, host, hour)
);

CREATE INDEX idx_health_hourly_hour ON service_health_hourly(hour DESC);

-- ═══════════════════════════════════════════════════════════════════════════════
-- AUDIT EVENTS (Anamnesis — tamper-evident append-only log)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE audit_events (
    id              VARCHAR(255) PRIMARY KEY,
    event_at        TIMESTAMPTZ DEFAULT NOW(),
    event_type      VARCHAR(50) NOT NULL
        CHECK (event_type IN (
            'auth.login', 'auth.logout', 'auth.failed',
            'config.create', 'config.update', 'config.delete',
            'deploy.start', 'deploy.complete', 'deploy.rollback',
            'service.start', 'service.stop', 'service.restart', 'service.crash',
            'security.alert', 'security.violation', 'security.scan',
            'kingdom.startup', 'kingdom.shutdown', 'kingdom.mode_change',
            'data.export', 'data.import', 'data.purge'
        )),
    severity        VARCHAR(10) NOT NULL DEFAULT 'info'
        CHECK (severity IN ('debug', 'info', 'warn', 'error', 'critical')),
    action          TEXT DEFAULT '',
    outcome         VARCHAR(10) DEFAULT 'success'
        CHECK (outcome IN ('success', 'failure', 'denied', 'error')),
    actor_id        TEXT DEFAULT '',
    actor_type      VARCHAR(20) DEFAULT '',
    actor_name      TEXT DEFAULT '',
    resource_id     TEXT DEFAULT '',
    resource_type   VARCHAR(50) DEFAULT '',
    source_ip       INET,
    source_service  VARCHAR(50) DEFAULT '',
    trace_id        VARCHAR(64) DEFAULT '',
    details         JSONB DEFAULT '{}',
    previous_hash   VARCHAR(128) DEFAULT '',
    hash            VARCHAR(128) DEFAULT '',
    created_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_audit_event_at ON audit_events(event_at DESC);
CREATE INDEX idx_audit_event_type ON audit_events(event_type);
CREATE INDEX idx_audit_severity ON audit_events(severity);
CREATE INDEX idx_audit_actor_id ON audit_events(actor_id);
CREATE INDEX idx_audit_resource_type ON audit_events(resource_type);
CREATE INDEX idx_audit_trace_id ON audit_events(trace_id);
CREATE INDEX idx_audit_source_service ON audit_events(source_service);

-- ═══════════════════════════════════════════════════════════════════════════════
-- GRANTS
-- ═══════════════════════════════════════════════════════════════════════════════
GRANT USAGE ON SCHEMA public TO ops_writer, ops_reader;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO ops_writer;

-- ops_writer: full CRUD on all ops tables
GRANT SELECT, INSERT, UPDATE, DELETE ON
    service_health_current, service_health_transitions, service_health_hourly, audit_events
    TO ops_writer;

-- ops_reader: SELECT only
GRANT SELECT ON
    service_health_current, service_health_transitions, service_health_hourly, audit_events
    TO ops_reader;
