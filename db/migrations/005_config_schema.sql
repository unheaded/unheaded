-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Configuration Database Schema (unheaded_config)

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
-- KINGDOM CONFIGURATION (key-value with typed values)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE kingdom_config (
    key          VARCHAR(200) PRIMARY KEY,
    value        JSONB NOT NULL,
    value_type   VARCHAR(20) DEFAULT 'string'
        CHECK (value_type IN ('string', 'integer', 'float', 'boolean', 'array', 'object')),
    description  TEXT DEFAULT '',
    category     VARCHAR(50) DEFAULT 'general'
        CHECK (category IN ('general', 'network', 'security', 'observability', 'services', 'runtime')),
    is_sensitive BOOLEAN DEFAULT FALSE,
    updated_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_by   TEXT DEFAULT 'system'
);

CREATE INDEX idx_config_category ON kingdom_config(category);
CREATE INDEX idx_config_sensitive ON kingdom_config(is_sensitive) WHERE is_sensitive = TRUE;

CREATE TRIGGER trg_kingdom_config_updated_at
    BEFORE UPDATE ON kingdom_config
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ═══════════════════════════════════════════════════════════════════════════════
-- KINGDOM CONFIG HISTORY (change tracking — append-only)
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE kingdom_config_history (
    id           BIGSERIAL PRIMARY KEY,
    key          VARCHAR(200) NOT NULL,
    old_value    JSONB,
    new_value    JSONB,
    changed_by   TEXT DEFAULT 'system',
    change_reason TEXT DEFAULT '',
    changed_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_config_history_key ON kingdom_config_history(key);
CREATE INDEX idx_config_history_changed_at ON kingdom_config_history(changed_at DESC);

-- Config change tracking trigger
CREATE OR REPLACE FUNCTION track_config_change()
RETURNS TRIGGER AS $$
BEGIN
    INSERT INTO kingdom_config_history (key, old_value, new_value, changed_by, changed_at)
    VALUES (OLD.key, OLD.value, NEW.value, NEW.updated_by, NOW());
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

CREATE TRIGGER trg_kingdom_config_history
    AFTER UPDATE ON kingdom_config
    FOR EACH ROW
    WHEN (OLD.value IS DISTINCT FROM NEW.value)
    EXECUTE FUNCTION track_config_change();

-- ═══════════════════════════════════════════════════════════════════════════════
-- SERVICE REGISTRY
-- ═══════════════════════════════════════════════════════════════════════════════
CREATE TABLE service_registry (
    service_name    VARCHAR(50) PRIMARY KEY,
    display_name    TEXT NOT NULL,
    description     TEXT DEFAULT '',
    port            INTEGER NOT NULL,
    grpc_port       INTEGER,
    protocol        VARCHAR(10) DEFAULT 'http'
        CHECK (protocol IN ('http', 'grpc', 'both')),
    host_default    VARCHAR(100) DEFAULT 'localhost',
    layer           SMALLINT DEFAULT 4
        CHECK (layer BETWEEN 0 AND 5),
    dependencies    TEXT[] DEFAULT '{}',
    health_endpoint VARCHAR(100) DEFAULT '/health',
    config          JSONB DEFAULT '{}',
    enabled         BOOLEAN DEFAULT TRUE,
    registered_at   TIMESTAMPTZ DEFAULT NOW(),
    updated_at      TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX idx_registry_layer ON service_registry(layer);
CREATE INDEX idx_registry_enabled ON service_registry(enabled);
CREATE INDEX idx_registry_protocol ON service_registry(protocol);

CREATE TRIGGER trg_service_registry_updated_at
    BEFORE UPDATE ON service_registry
    FOR EACH ROW EXECUTE FUNCTION update_updated_at();

-- ═══════════════════════════════════════════════════════════════════════════════
-- GRANTS
-- ═══════════════════════════════════════════════════════════════════════════════
GRANT USAGE ON SCHEMA public TO config_admin, config_reader;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO config_admin;

-- config_admin: full CRUD on all config tables
GRANT SELECT, INSERT, UPDATE, DELETE ON
    kingdom_config, kingdom_config_history, service_registry
    TO config_admin;

-- config_reader: SELECT only
GRANT SELECT ON
    kingdom_config, kingdom_config_history, service_registry
    TO config_reader;
