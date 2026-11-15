-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Zhen's tables leave unheaded_app (ADR-149; S-ZHEN-SWITCH D2 (a)).
-- Runs as well_app_owner.
--
-- 052 built them in unheaded_zhen. On a fresh Well they are empty here and
-- go at once. On a live Well this refuses while any zhen_* table here holds
-- a row, unless the attended copy (S-ZHEN-SWITCH Phase 8B: pg_dump --data-only
-- of public.zhen_* into unheaded_zhen, counts compared) has recorded itself:
--   ALTER DATABASE unheaded_app SET unheaded.zhen_rows_copied = '<date>';
-- The setting is reset at the end, so it never outlives this migration.
--
-- No CASCADE: a dependency this file did not name (a view, a foreign key
-- from a platform table) must fail the migration, not vanish with it.
-- kanban_zhen_optin (049) stays: the board owns it and is its only writer.

SET lock_timeout = '5s';

DO $$
DECLARE
    t text;
    n bigint;
    total bigint := 0;
BEGIN
    FOR t IN
        SELECT c.relname FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
         WHERE ns.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relname LIKE 'zhen\_%'
    LOOP
        EXECUTE format('SELECT count(*) FROM (SELECT 1 FROM public.%I LIMIT 1) s', t) INTO n;
        total := total + n;
    END LOOP;
    IF total > 0 AND coalesce(current_setting('unheaded.zhen_rows_copied', true), '') = '' THEN
        RAISE EXCEPTION 'zhen tables still hold rows: copy the rows to unheaded_zhen first (S-ZHEN-SWITCH Phase 8B) and set unheaded.zhen_rows_copied';
    END IF;
END $$;

-- Dependents first: snapshots name actions, labels name captures.
DROP TABLE zhen_action_snapshots;
DROP TABLE zhen_actions;
DROP TABLE zhen_capture_labels;
DROP TABLE zhen_captures;
DROP TABLE zhen_agent_runs;
DROP TABLE zhen_pending_confirmations;
DROP TABLE zhen_conversations;
DROP TABLE zhen_memories;

-- Their trigger functions (well_append_only() stays: the platform's
-- append-only tables use it).
DROP FUNCTION zhen_captures_purge();
DROP FUNCTION zhen_captures_guard_delete();
DROP FUNCTION zhen_captures_update();
DROP FUNCTION zhen_captures_insert();
DROP FUNCTION zhen_agent_run_transition();
DROP FUNCTION zhen_agent_run_insert();
DROP FUNCTION zhen_confirm_transition();
DROP FUNCTION zhen_confirm_insert();

-- The database setting, if the copy set it. ALTER DATABASE ... RESET needs
-- the database's owner, which this file runs as.
ALTER DATABASE unheaded_app RESET unheaded.zhen_rows_copied;
