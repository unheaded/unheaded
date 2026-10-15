-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — duplicate indexes on zhen_conversations (W-14), unheaded_app.
--
-- 003 creates idx_zhen_created and idx_zhen_search; 010's CREATE TABLE IF
-- NOT EXISTS is skipped but its CREATE INDEX IF NOT EXISTS lines are not, so
-- every bootstrap ends with idx_zhen_conversations_created and
-- idx_zhen_conversations_search as exact copies. Every insert pays for both.
-- Drop 010's copy only where 003's is present, so a Well created some
-- other way never loses its only index. Small table: a plain DROP under the
-- runner's lock_timeout is fine.

DO $$
BEGIN
    IF to_regclass('public.idx_zhen_created') IS NOT NULL THEN
        DROP INDEX IF EXISTS idx_zhen_conversations_created;
    END IF;
    IF to_regclass('public.idx_zhen_search') IS NOT NULL THEN
        DROP INDEX IF EXISTS idx_zhen_conversations_search;
    END IF;
END $$;
