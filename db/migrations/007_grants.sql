-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Cross-Database CONNECT Grants
-- Run against each database to grant CONNECT where needed.

-- All app_* users can CONNECT to unheaded_config (read config)
DO $$
BEGIN
    IF current_database() = 'unheaded_config' THEN
        EXECUTE 'GRANT CONNECT ON DATABASE unheaded_config TO app_kanban';
        EXECUTE 'GRANT CONNECT ON DATABASE unheaded_config TO app_timeguru';
        EXECUTE 'GRANT CONNECT ON DATABASE unheaded_config TO app_zhen';
        -- Grant SELECT on config tables to app users
        GRANT USAGE ON SCHEMA public TO app_kanban, app_timeguru, app_zhen;
        GRANT SELECT ON kingdom_config, service_registry TO app_kanban, app_timeguru, app_zhen;
    END IF;
END $$;

-- ops_reader can CONNECT to unheaded_app (dashboard reads tasks)
DO $$
BEGIN
    IF current_database() = 'unheaded_app' THEN
        EXECUTE 'GRANT CONNECT ON DATABASE unheaded_app TO ops_reader';
        GRANT USAGE ON SCHEMA public TO ops_reader;
        GRANT SELECT ON kanban_tasks, timeline_milestones, zhen_conversations TO ops_reader;
    END IF;
END $$;
