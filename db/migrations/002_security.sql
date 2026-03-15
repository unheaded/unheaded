-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Security Hardening

-- Create read-only role for services that only need SELECT
CREATE ROLE unheaded_readonly;
GRANT CONNECT ON DATABASE unheaded TO unheaded_readonly;
GRANT USAGE ON SCHEMA public TO unheaded_readonly;
GRANT SELECT ON ALL TABLES IN SCHEMA public TO unheaded_readonly;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT ON TABLES TO unheaded_readonly;

-- Create service role for app writes (no DDL)
CREATE ROLE unheaded_service;
GRANT CONNECT ON DATABASE unheaded TO unheaded_service;
GRANT USAGE ON SCHEMA public TO unheaded_service;
GRANT SELECT, INSERT, UPDATE, DELETE ON ALL TABLES IN SCHEMA public TO unheaded_service;
GRANT USAGE, SELECT ON ALL SEQUENCES IN SCHEMA public TO unheaded_service;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT SELECT, INSERT, UPDATE, DELETE ON TABLES TO unheaded_service;
ALTER DEFAULT PRIVILEGES IN SCHEMA public GRANT USAGE, SELECT ON SEQUENCES TO unheaded_service;

-- Enable row-level security on sensitive tables
ALTER TABLE audit_events ENABLE ROW LEVEL SECURITY;
ALTER TABLE zhen_conversations ENABLE ROW LEVEL SECURITY;

-- Policy: services can only see their own audit events
CREATE POLICY audit_service_isolation ON audit_events
    USING (service = current_setting('app.service_name', true));

-- Revoke public schema creation (defense in depth)
REVOKE CREATE ON SCHEMA public FROM PUBLIC;

-- Prevent DROP on critical tables
-- (Can't do this purely in SQL — document for DBA)
COMMENT ON TABLE kanban_tasks IS 'PROTECTED: Do not drop without DBA approval';
COMMENT ON TABLE audit_events IS 'PROTECTED: Append-only audit trail — never delete';
COMMENT ON TABLE kingdom_config IS 'PROTECTED: Kingdom configuration — restricted writes';
