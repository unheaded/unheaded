-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — zhen_conversations.client (ADR-059 Q3, DP-311), unheaded_app.
--
-- zhen-cli sessions join the web UI's in zhen_conversations; client tells
-- them apart. Existing rows are the web UI's, so the default is 'web'.
--
-- Writer: app_zhen, through 003 and 010's grants (no new grant). Reader:
-- unchanged. Access paths: by session_id and created_at, as today; nothing
-- new is indexed (two values, W-14). Volume: a few rows per CLI session.
-- Retention: unchanged (compliance/retention/classes.yaml).
--
-- Lock plan: ADD COLUMN with a constant default is a catalog update under
-- a short ACCESS EXCLUSIVE, bounded by lock_timeout; the CHECK is added NOT
-- VALID and validated in a second statement. Rollback: a later numbered
-- migration drops the column; zhen-agentd then answers 503 for sessions.
-- Applied live with the E1 batch.
-- Proof: scripts/well/test-zhen-conversations.sh.

SET lock_timeout = '5s';

ALTER TABLE zhen_conversations ADD COLUMN IF NOT EXISTS client TEXT NOT NULL DEFAULT 'web';

ALTER TABLE zhen_conversations ADD CONSTRAINT zhen_conversations_client_check
    CHECK (client IN ('web', 'cli')) NOT VALID;
ALTER TABLE zhen_conversations VALIDATE CONSTRAINT zhen_conversations_client_check;
