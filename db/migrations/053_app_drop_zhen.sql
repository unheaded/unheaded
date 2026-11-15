-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Zhen's tables leave unheaded_app (ADR-149; S-ZHEN-SWITCH D2 (a)).
-- Runs as well_app_owner.
--
-- 052 built them in unheaded_zhen. On a fresh Well they are empty here and
-- go at once. On a live Well this refuses while any zhen_* table here holds
-- a row, unless the attended copy (S-ZHEN-SWITCH Phase 8B,
-- scripts/well/zhen-copy-rows.sh) has recorded the counts it copied and
-- verified on both sides:
--   ALTER DATABASE unheaded_app SET unheaded.zhen_rows_copied =
--     'copied <UTC time>: zhen_actions=<n>,zhen_memories=<n>,...';
-- and every zhen_* table here still holds exactly its recorded count (a
-- table missing from the record, or a row written since the copy, refuses).
-- Before that the attended sitting took a platform backup set and drilled
-- its restore (plan Steps 269-270). The setting is reset at the end, so it
-- never outlives this migration.
--
-- No CASCADE: a dependency this file did not name (a view, a foreign key
-- from a platform table) must fail the migration, not vanish with it.
-- kanban_zhen_optin (049) stays: the board owns it and is its only writer.

SET lock_timeout = '5s';

DO $$
DECLARE
    t text;
    n bigint;
    rec text := coalesce(current_setting('unheaded.zhen_rows_copied', true), '');
    m text[];
    total bigint := 0;
BEGIN
    FOR t IN
        SELECT c.relname FROM pg_class c JOIN pg_namespace ns ON ns.oid = c.relnamespace
         WHERE ns.nspname = 'public' AND c.relkind IN ('r', 'p') AND c.relname LIKE 'zhen\_%'
         ORDER BY 1
    LOOP
        EXECUTE format('SELECT count(*) FROM public.%I', t) INTO n;
        total := total + n;
        IF rec <> '' THEN
            m := regexp_match(rec, '(?:^|[ ,:])' || t || '=([0-9]+)(?:,|$)');
            IF m IS NULL OR m[1]::bigint <> n THEN
                RAISE EXCEPTION 'zhen tables: % holds % row(s), the copy recorded %: copy the rows again (S-ZHEN-SWITCH Phase 8B, scripts/well/zhen-copy-rows.sh)',
                    t, n, coalesce(m[1], 'nothing');
            END IF;
        END IF;
    END LOOP;
    IF total > 0 AND rec = '' THEN
        RAISE EXCEPTION 'zhen tables still hold rows: copy the rows to unheaded_zhen first (S-ZHEN-SWITCH Phase 8B, scripts/well/zhen-copy-rows.sh), which sets unheaded.zhen_rows_copied';
    END IF;
END $$;

-- Dependents first: snapshots name actions, labels name captures.
DROP TABLE zhen_action_snapshots; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_actions; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_capture_labels; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_captures; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_agent_runs; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_pending_confirmations; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_conversations; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise
DROP TABLE zhen_memories; -- well:ignore destructive rows copied to unheaded_zhen and counted on both sides first (zhen-copy-rows.sh, Phase 8B Steps 276-280, after the drilled backup set of Steps 269-270); the DO block above refuses otherwise

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
