-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — retention_runner, the login role of the retention job
-- (S-WELL-SPRING Phase 4, M-074; retention policy v1 §11), cluster-level
-- and unheaded_app.
--
-- well-retention (cmd/well-retention, timer unheaded-well-retention) is
-- the only thing that deletes rows the retention policy ages out. It logs
-- in as retention_runner, its own role: no service's leaked password can
-- delete history, and retention_runner can delete from nothing but the
-- tables its classes own (compliance/retention/classes.yaml).
--
-- Applied twice (MANIFEST), like 007: in `postgres` it creates the role
-- and grants CONNECT; in unheaded_app it grants what the app-side classes
-- need. 043 does unheaded_ops.
--
-- unheaded_app grants, per class:
--   zhen-conversations   DELETE, and SELECT of the key and age columns only
--   zhen-captures        EXECUTE zhen_captures_purge() (033, the only
--                        delete path; the age is 033's trigger, 180 days)
--                        and SELECT of the three columns its dry-run count
--                        reads; no DELETE: 033's trigger refuses one anyway
--   kanban-soft-deleted  SELECT, DELETE: the archive copies the whole row
--                        (redacted) before the purge deletes it
--   zhen-agent-runs      SELECT (card_id, claimed_at): a card any run names
--                        is held, never purged (032's foreign key would
--                        refuse it too), and the run table's age is
--                        measured; runs are never deleted
--
-- No password here: db/init.sh and scripts/well/set-role-passwords.sh set
-- it from RETENTION_RUNNER_PASSWORD. Connection limit 4: one job, one
-- connection per database.
--
-- Lock plan: a role, database grants and table grants (catalog updates,
-- instant); no table is rewritten. Rollback: a later numbered migration
-- revokes the grants and drops the role once 043's grants are revoked too.
-- Proof: scripts/well/test-retention.sh (throwaway server).

SET lock_timeout = '5s';

DO $$
BEGIN
    IF current_database() = 'postgres' THEN
        IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'retention_runner') THEN
            CREATE ROLE retention_runner LOGIN CONNECTION LIMIT 4;
        END IF;
        GRANT CONNECT ON DATABASE unheaded_app TO retention_runner;
        GRANT CONNECT ON DATABASE unheaded_ops TO retention_runner;
    END IF;
END $$;

DO $$
BEGIN
    IF current_database() = 'unheaded_app' THEN
        GRANT SELECT (id, created_at), DELETE ON zhen_conversations TO retention_runner;
        GRANT SELECT (quarantined, promoted, purge_after) ON zhen_captures TO retention_runner;
        GRANT EXECUTE ON FUNCTION zhen_captures_purge() TO retention_runner;
        GRANT SELECT, DELETE ON kanban_tasks TO retention_runner;
        GRANT SELECT (card_id, claimed_at) ON zhen_agent_runs TO retention_runner;
    END IF;
END $$;
