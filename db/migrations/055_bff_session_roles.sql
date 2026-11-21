-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — one login role per login front, for browser sessions kept in
-- the Well (ADR-114 N5, decision pack 2026-11-18 row 44), cluster-level.
--
-- A login front (pkg/bff: dashboard-backend, kanban-app, wiki-server) that
-- runs with AUTH_SESSION_STORE=well keeps its sessions in
-- unheaded_app.bff_sessions (056). Each front logs in as its own role,
-- named after it: bff_ and the front's name with - as _. 056 grants these
-- roles one view, which shows a role the sessions of the front it is named
-- after and no others. So the password of one front cannot read, forge or
-- end the sessions of another: a session row is what makes a browser a
-- logged-in admin, and a front that could write rows in another front's
-- name could log itself in there.
--
-- A front added later gets its role in a later numbered migration, with the
-- same grant as 056 gives these three.
--
-- No password here: db/init.sh and scripts/well/set-role-passwords.sh set
-- each from its variable (BFF_DASHBOARD_BACKEND_PASSWORD,
-- BFF_KANBAN_APP_PASSWORD, BFF_WIKI_SERVER_PASSWORD; W-07). Without one the
-- role cannot log in. Connection limit 3: a front's pool is 2
-- (pkg/bff/sessions WellStoreConns) and one more lets a restart overlap.
-- <ROLE>_CONNECTION_LIMIT and scripts/well/set-role-limits.sh change it on
-- a deployment.
--
-- Lock plan: three roles and a database grant; no table is touched.
-- Rollback: a later numbered migration revokes CONNECT and drops the roles
-- once 056's grants are gone.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bff_dashboard_backend') THEN
        CREATE ROLE bff_dashboard_backend LOGIN CONNECTION LIMIT 3;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bff_kanban_app') THEN
        CREATE ROLE bff_kanban_app LOGIN CONNECTION LIMIT 3;
    END IF;
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'bff_wiki_server') THEN
        CREATE ROLE bff_wiki_server LOGIN CONNECTION LIMIT 3;
    END IF;
END $$;

GRANT CONNECT ON DATABASE unheaded_app TO bff_dashboard_backend, bff_kanban_app, bff_wiki_server;
