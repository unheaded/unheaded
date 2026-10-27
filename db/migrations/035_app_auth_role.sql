-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — app_auth, the login role for API keys (S-IRON-GATE Phase 3,
-- ADR-103 §5 and Q4), cluster-level.
--
-- app_auth reads and writes unheaded_app.api_keys (036) and nothing else:
-- unheaded-cli creates and revokes keys as it, and a service that checks
-- uhk_ keys against the Well (AUTH_APIKEY_STORE=well) reads them as it.
-- Its own role, not app_kanban's or app_zhen's, so a leaked Kanban or Zhen
-- password cannot mint API keys.
--
-- No password here: db/init.sh and scripts/well/set-role-passwords.sh set
-- it from APP_AUTH_PASSWORD (W-07). Connection limit 40: each process opens
-- at most 2 connections for its key store (pkg/auth WellStore).
--
-- Lock plan: a role and a database grant; no table is touched. Rollback: a
-- later numbered migration revokes CONNECT and drops the role once 036's
-- grants are gone.

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'app_auth') THEN
        CREATE ROLE app_auth LOGIN CONNECTION LIMIT 40;
    END IF;
END $$;

GRANT CONNECT ON DATABASE unheaded_app TO app_auth;
