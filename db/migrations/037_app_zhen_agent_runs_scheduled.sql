-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — scheduled runs in zhen_agent_runs (S-CHAMPION-HARDENING
-- Steps 160-161), unheaded_app.
--
-- zhen-agentd -scheduler runs approved runbooks on a schedule
-- (configs/zhen-schedules.yaml; raft/zhen_scheduler.py is retired, ADR-028
-- superseded). Each run is one row here with mode 'scheduled' and one
-- Kanban card (type 'scheduled-run', owner 'zhen-scheduler', which the
-- worker never takes). A scheduled run has no branch and no base commit,
-- and it is not reviewed: it ends succeeded or failed (a runbook whose
-- steps or verification section fail is failed), or abandoned when its
-- lease runs out because the daemon died mid-run.
--
-- Changes to 032's table:
--   mode       gains 'scheduled';
--   branch, base_commit
--              drop NOT NULL; zhen_agent_runs_scheduled_shape requires
--              them for every other mode and forbids them (and a head
--              commit, and accepted/rejected) for a scheduled run.
-- 032's triggers already hold: a run is born claimed, moves one way, its
-- identity and finished outcome are immutable, it is never deleted.
--
-- Writer: app_zhen, through 032's grants (no new grant).
-- Access paths: the scheduler inserts by id and finishes by id (the
-- primary key); `zhen-claude review list` reads the newest runs
-- (zhen_agent_runs_claimed_at_idx). Volume: a handful of rows per day.
-- Retention: kept, like every run.
--
-- Lock plan: the table is small (one row per card attempt). The new CHECKs
-- are added NOT VALID and validated in a second statement (SHARE UPDATE
-- EXCLUSIVE while it scans); DROP NOT NULL and DROP CONSTRAINT are catalog
-- updates under a short ACCESS EXCLUSIVE, bounded by lock_timeout.
-- Rollback: stop the scheduler (zhen-agentd without -scheduler). Runs are
-- append-only, so once a scheduled row exists 032's narrower CHECKs cannot
-- return; before the first one, a later migration may restore them.
-- Proof: scripts/well/test-zhen-agent-runs.sh (step 9).

SET lock_timeout = '5s';

ALTER TABLE zhen_agent_runs DROP CONSTRAINT zhen_agent_runs_mode_check;
ALTER TABLE zhen_agent_runs ADD CONSTRAINT zhen_agent_runs_mode_check
    CHECK (mode IN ('interactive', 'overnight', 'scheduled')) NOT VALID;
ALTER TABLE zhen_agent_runs VALIDATE CONSTRAINT zhen_agent_runs_mode_check;

ALTER TABLE zhen_agent_runs ALTER COLUMN branch DROP NOT NULL;
ALTER TABLE zhen_agent_runs ALTER COLUMN base_commit DROP NOT NULL;
ALTER TABLE zhen_agent_runs ADD CONSTRAINT zhen_agent_runs_scheduled_shape CHECK (
    CASE WHEN mode = 'scheduled'
         THEN branch IS NULL AND base_commit IS NULL AND head_commit IS NULL
              AND status NOT IN ('accepted', 'rejected')
         ELSE branch IS NOT NULL AND base_commit IS NOT NULL
    END) NOT VALID;
ALTER TABLE zhen_agent_runs VALIDATE CONSTRAINT zhen_agent_runs_scheduled_shape;
