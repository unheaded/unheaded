-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — zhen_actions.status takes the Champion gate's verdicts, unheaded_app.
--
-- 009 allows planned/executing/completed/failed/reverted at VARCHAR(20).
-- pkg/champion also writes 'accepted', 'denied' and 'denied_<reason>'
-- (denied_path, denied_teach, denied_destructive, denied_unknown_tool,
-- denied_untrusted_justification: 30 characters). Every such UPDATE failed
-- on a real Well and was only logged, so a refused tool call stayed
-- 'planned' in the action log: the denials, the rows an operator looks
-- for, were the ones lost (S-ZHEN-POLISH E8; found again 2026-11-08 by
-- pkg/champion's integration test against a throwaway Well).
--
-- A pattern and not a list: 011 dropped the action_type CHECK because a
-- fixed list refused each new tool; a new gate reason is likewise one more
-- 'denied_<reason>' (lower case and underscores). VARCHAR(40) bounds it.
--
-- Writer: app_zhen, through 009's grants (no new grant). Access paths,
-- volume and retention: 009's.
--
-- Lock plan: widening a VARCHAR is a catalog update, no rewrite and no
-- index rebuild (no view depends on the column). DROP CONSTRAINT and the
-- type change take a short ACCESS EXCLUSIVE, bounded by lock_timeout; the
-- new CHECK is added NOT VALID and validated in a second statement.
-- Rollback: a later numbered migration restores 009's CHECK once no row
-- holds a gate verdict (narrowing back to VARCHAR(20) rewrites the table).
-- Applied live with the E1 batch. Proof: scripts/well/test-zhen-agent-runs.sh
-- (the CHECK, and pkg/champion's integration tests, build tag integration).

SET lock_timeout = '5s';

ALTER TABLE zhen_actions DROP CONSTRAINT zhen_actions_status_check;
ALTER TABLE zhen_actions
    -- well:ignore type-change-rewrite VARCHAR(20) to VARCHAR(40) is binary-coercible: on PG 16 the table's and idx_zhen_actions_status's relfilenodes are unchanged after it (measured on a throwaway Well, 2026-11-08)
    ALTER COLUMN status TYPE VARCHAR(40);
ALTER TABLE zhen_actions ADD CONSTRAINT zhen_actions_status_check
    CHECK (status IN ('planned', 'executing', 'completed', 'failed', 'reverted', 'accepted', 'denied')
           OR status ~ '^denied_[a-z][a-z_]*$') NOT VALID;
ALTER TABLE zhen_actions VALIDATE CONSTRAINT zhen_actions_status_check;
