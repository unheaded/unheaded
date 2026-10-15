-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — zhen_actions duplicate indexes (W-14), unheaded_app.
--
-- 020 adopted the champion's startup indexes, but two are exact copies of
-- 009's: zhen_actions_session_id_idx = idx_zhen_actions_session and
-- zhen_actions_planned_at_idx = idx_zhen_actions_planned. The live Well had
-- both pairs since the champion first started. Drop the copies only where
-- 009's twin exists. zhen_actions_type_status_idx (action_type, status) is
-- a different index and stays. 020 is applied on the live Well, so it is
-- corrected here rather than edited.

DO $$
BEGIN
    IF to_regclass('public.idx_zhen_actions_session') IS NOT NULL THEN
        DROP INDEX IF EXISTS zhen_actions_session_id_idx;
    END IF;
    IF to_regclass('public.idx_zhen_actions_planned') IS NOT NULL THEN
        DROP INDEX IF EXISTS zhen_actions_planned_at_idx;
    END IF;
END $$;
