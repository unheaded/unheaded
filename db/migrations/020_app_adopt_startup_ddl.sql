-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — adopt objects only startup DDL created (W-03, ERDA 3.3/3.4),
-- unheaded_app.
--
-- pkg/champion/pgstore/schema.sql creates three zhen_actions indexes when
-- the champion starts, and the live zhen_conversations (built by 010's
-- shape) has created_at NOT NULL, which 003 lacks. Putting them here makes
-- the ledger the one place that defines them, so a fresh Well and the live
-- one end in the same shape and the startup DDL can be deleted later.
-- zhen_actions and zhen_conversations are small: plain CREATE INDEX under
-- the runner's lock_timeout. No row has a NULL created_at (it defaults to
-- now() everywhere).

CREATE INDEX IF NOT EXISTS zhen_actions_session_id_idx ON zhen_actions (session_id);
CREATE INDEX IF NOT EXISTS zhen_actions_planned_at_idx ON zhen_actions (planned_at DESC);
CREATE INDEX IF NOT EXISTS zhen_actions_type_status_idx ON zhen_actions (action_type, status);

ALTER TABLE zhen_conversations ALTER COLUMN created_at SET NOT NULL;
