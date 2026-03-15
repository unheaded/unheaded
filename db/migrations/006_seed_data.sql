-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Seed Data (unheaded_config)

-- ═══════════════════════════════════════════════════════════════════════════════
-- KINGDOM CONFIG DEFAULTS
-- ═══════════════════════════════════════════════════════════════════════════════
INSERT INTO kingdom_config (key, value, value_type, description, category, is_sensitive) VALUES
    ('kingdom.name', '"Unheaded"', 'string', 'Kingdom name', 'general', FALSE),
    ('kingdom.version', '"0.1.0-alpha"', 'string', 'Current version', 'general', FALSE),
    ('kingdom.hosts', '["west", "east"]', 'array', 'Active bare metal hosts', 'general', FALSE),
    ('kingdom.mode', '"development"', 'string', 'Operating mode (development, staging, production)', 'general', FALSE),
    ('kingdom.wire_format_version', '"0x01"', 'string', 'Monad wire format version (FROZEN)', 'general', FALSE),
    -- Network settings
    ('network.bridge', '"lxdbr0"', 'string', 'Primary bridge interface', 'network', FALSE),
    ('network.subnet', '"10.10.10.0/24"', 'string', 'Internal network subnet', 'network', FALSE),
    ('network.gateway_ip', '"10.10.10.100"', 'string', 'Gateway IP address', 'network', FALSE),
    ('network.wotan_ip', '"10.10.10.10"', 'string', 'Wotan message bus IP', 'network', FALSE),
    ('network.port_range_start', '16666', 'integer', 'Doom Range start port', 'network', FALSE),
    ('network.port_range_end', '26666', 'integer', 'Doom Range end port', 'network', FALSE),
    -- Security settings
    ('security.tls_min_version', '"1.3"', 'string', 'Minimum TLS version for external traffic', 'security', FALSE),
    ('security.auth_default', '"noop"', 'string', 'Default authenticator (noop, apikey, jwt)', 'security', FALSE),
    ('security.secrets_backend', '"sops"', 'string', 'Secrets management backend', 'security', TRUE),
    -- Observability settings
    ('observability.metrics_backend', '"prometheus"', 'string', 'Metrics collection backend', 'observability', FALSE),
    ('observability.logging_backend', '"zerolog"', 'string', 'Structured logging backend', 'observability', FALSE),
    ('observability.tracing_backend', '"opentelemetry"', 'string', 'Distributed tracing backend', 'observability', FALSE),
    ('observability.log_level', '"info"', 'string', 'Default log level', 'observability', FALSE),
    ('observability.metrics_interval_sec', '15', 'integer', 'Metrics scrape interval in seconds', 'observability', FALSE)
ON CONFLICT (key) DO NOTHING;

-- ═══════════════════════════════════════════════════════════════════════════════
-- SERVICE REGISTRY — 10 Core Services
-- ═══════════════════════════════════════════════════════════════════════════════
INSERT INTO service_registry (service_name, display_name, description, port, grpc_port, protocol, host_default, layer, dependencies, health_endpoint, enabled) VALUES
    ('wotan', 'Wotan', 'Message bus — ring buffer + event bus + protocol RAM', 18000, 18001, 'both', 'localhost', 3, '{}', '/health', TRUE),
    ('unheaded-daemon', 'Unheaded Daemon', 'Control plane — orchestration and reconciliation', 17000, 17001, 'both', 'localhost', 2, '{wotan}', '/health', TRUE),
    ('timeguru', 'Timeguru', 'Timeline tracking service', 19000, NULL, 'http', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('architect', 'Architect', 'ADR and design service', 19001, NULL, 'http', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('captain', 'Captain', 'Strategy service', 19002, NULL, 'http', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('micromanager', 'Micromanager', 'Task execution service', 19003, NULL, 'http', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('monad', 'Monad', 'Unified state management', 19004, NULL, 'grpc', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('sophia', 'Sophia', 'Knowledge graph service', 19005, NULL, 'grpc', 'localhost', 4, '{wotan}', '/health', TRUE),
    ('dashboard-backend', 'Dashboard', 'Observability dashboard backend', 20000, NULL, 'http', 'localhost', 5, '{wotan}', '/health', TRUE),
    ('kanban-app', 'Kanban', 'Self-hosting proof — the meta moment', 20001, NULL, 'http', 'localhost', 5, '{wotan,timeguru}', '/health', TRUE)
ON CONFLICT (service_name) DO NOTHING;
