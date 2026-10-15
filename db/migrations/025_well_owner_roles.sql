-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — one owner role per database, and no connections by default
-- (ERDA 2.2, W-02), cluster-level.
--
-- The superuser owned every database and every object in them. Each
-- database now belongs to a NOLOGIN role that nobody can log in as:
-- well-migrate and db/init.sh SET ROLE to it for every later migration
-- (MANIFEST flag `superuser` opts out), so new objects belong to the owner,
-- and 026 hands over the objects that exist. The superuser stays the
-- cluster's administrator; it no longer owns application data.
--
-- PUBLIC had CONNECT and TEMPORARY on every database (well_health.sql,
-- 2026-10-15). Now each role connects only where it has grants:
--   unheaded_app     app_kanban app_timeguru app_zhen ops_reader
--   unheaded_ops     ops_writer ops_reader
--   unheaded_config  config_admin config_reader app_kanban app_timeguru app_zhen
-- plus huginn_reader (012) on those three and well_backup (023) on every
-- database, since db-backup.sh dumps every database that accepts
-- connections.

DO $$
DECLARE
    r text;
    d text;
BEGIN
    FOREACH r IN ARRAY ARRAY['well_app_owner', 'well_ops_owner', 'well_config_owner'] LOOP
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = r) THEN
            EXECUTE format('CREATE ROLE %I NOLOGIN', r);
        END IF;
    END LOOP;
    FOR d IN SELECT datname FROM pg_database WHERE datallowconn AND NOT datistemplate LOOP
        EXECUTE format('REVOKE CONNECT, TEMPORARY ON DATABASE %I FROM PUBLIC', d);
        EXECUTE format('GRANT CONNECT ON DATABASE %I TO well_backup', d);
    END LOOP;
END $$;

ALTER DATABASE unheaded_app OWNER TO well_app_owner;
ALTER DATABASE unheaded_ops OWNER TO well_ops_owner;
ALTER DATABASE unheaded_config OWNER TO well_config_owner;

GRANT CONNECT ON DATABASE unheaded_app TO app_kanban, app_timeguru, app_zhen, ops_reader, huginn_reader;
GRANT CONNECT ON DATABASE unheaded_ops TO ops_writer, ops_reader, huginn_reader;
GRANT CONNECT ON DATABASE unheaded_config TO config_admin, config_reader, app_kanban, app_timeguru, app_zhen, huginn_reader;
