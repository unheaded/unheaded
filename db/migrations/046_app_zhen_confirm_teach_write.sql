-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — teach_write joins the confirmation queue (DP-309), unheaded_app.
--
-- 031 lists the tools a queued call may name. teach_write runs only from a
-- queue approval, so without this no /teach can be queued at all: the row
-- is refused and zhen-agentd answers that it could not be queued.
--
-- Writer: app_zhen, through 031's grants (no new grant). Access paths and
-- volume: 031's. Retention: 031's.
--
-- Lock plan: the table is small (at most 20 pending rows per service, plus
-- decided ones). DROP CONSTRAINT is a catalog update under a short ACCESS
-- EXCLUSIVE, bounded by lock_timeout; the new CHECK is added NOT VALID and
-- validated in a second statement. Rollback: a later numbered migration
-- restores 031's list once no teach_write row remains. Applied live with
-- the E1 batch. Proof: scripts/well/test-zhen-confirm.sh.

SET lock_timeout = '5s';

ALTER TABLE zhen_pending_confirmations DROP CONSTRAINT zhen_pending_confirmations_tool_check;
ALTER TABLE zhen_pending_confirmations ADD CONSTRAINT zhen_pending_confirmations_tool_check
    CHECK (tool IN ('write_file', 'patch_file', 'kanban_create', 'kanban_update', 'teach_write')) NOT VALID;
ALTER TABLE zhen_pending_confirmations VALIDATE CONSTRAINT zhen_pending_confirmations_tool_check;
