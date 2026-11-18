-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — Zhen's tables leave unheaded_app (ADR-149; S-ZHEN-SWITCH D2 (a)).
-- Runs as well_app_owner.
--
-- 052 built them in unheaded_zhen. On a fresh Well they are empty here and
-- go at once. On a live Well this refuses while any zhen_* table here holds
-- a row, unless the attended copy (S-ZHEN-SWITCH Phase 8B,
-- scripts/well/zhen-copy-rows.sh) has recorded the counts it copied and
-- verified on both sides, as the comment on zhen_conversations:
--   COMMENT ON TABLE public.zhen_conversations IS
--     'copied <UTC time>: zhen_actions=<n>,zhen_memories=<n>,...';
-- and every zhen_* table here still holds exactly its recorded count (a
-- table missing from the record, or a row written since the copy, refuses).
-- Before that the attended sitting took a platform backup set and drilled
-- its restore (plan Steps 269-270).
--
-- Where the record lives (changed 2026-11-18, before this file ran on any
-- Well). It was a database setting, ALTER DATABASE unheaded_app SET
-- unheaded.zhen_rows_copied, which this file reset at its end. Since
-- PostgreSQL 15 only a superuser, or a role holding SET on that parameter,
-- may set or reset a custom parameter for a database; this file runs as
-- well_app_owner, so the first full rehearsal
-- (scripts/well/test-zhen-database.sh) stopped at the RESET, after the copy,
-- with "permission denied to set parameter". A fresh Well never saw it: with
-- no setting row for the database the RESET is a no-op that checks nothing.
-- Weighed (the privilege behaviour tried on a throwaway PostgreSQL 16.15):
--   - GRANT SET ON PARAMETER unheaded.zhen_rows_copied TO well_app_owner in
--     051. The RESET then works, but the owner cannot give the privilege
--     back (REVOKE: "no privileges could be revoked"), so it would stay on
--     every Well, fresh ones too, until a superuser revoked it by hand; and
--     scripts/well/batch-rollback.sh would leave it (a cluster-level grant
--     to a role the batch did not create).
--   - The RESET as a superuser's step after this file. The setting then
--     outlives the migration, for good when the step is forgotten.
--   - Chosen: the record is a comment on a table this file drops. The copy
--     script (superuser) writes it, this file reads it as the table's owner,
--     and the drop removes it in this file's transaction: nothing to grant,
--     nothing to reset, and it cannot outlive the migration. When this file
--     fails, the record stands with the tables. A fresh Well has no comment
--     and empty tables, and db/init.sh and well-migrate apply 051 to 053 as
--     before.
--
-- No CASCADE: a dependency this file did not name (a view, a foreign key
-- from a platform table) must fail the migration, not vanish with it.
-- kanban_zhen_optin (049) stays: the board owns it and is its only writer.

SET lock_timeout = '5s';

DO $$
DECLARE
    t text;
    n bigint;
    -- The copy's record, '' when there is none. Only text the copy script
    -- wrote counts: another comment on the table is not a record.
    rec text := coalesce(substring(obj_description(to_regclass('public.zhen_conversations'), 'pg_class') from '^copied .*'), '');
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
        RAISE EXCEPTION 'zhen tables still hold rows: copy the rows to unheaded_zhen first (S-ZHEN-SWITCH Phase 8B, scripts/well/zhen-copy-rows.sh), which records the counts on zhen_conversations';
    END IF;
END $$;

-- Dependents first: snapshots name actions, labels name captures. The copy's
-- record goes with zhen_conversations.
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
