-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — the Kanban board reads zhen_agent_runs (S-CHAMPION-HARDENING
-- Step 223), unheaded_app.
--
-- kanban-app's card modal shows the agent run behind a card: the branch a
-- worker left (zhen/<card>), the review note it wrote, Stevie's review
-- reason, the run's mode and status. Those live in zhen_agent_runs (032,
-- 037), which only app_zhen could read. app_kanban gains SELECT on the
-- columns the modal shows and the two it filters and orders by; nothing
-- else: not the error text, the Claude cost, the lease or the commits.
--
--   SELECT ... FROM zhen_agent_runs WHERE card_id = $1
--   ORDER BY claimed_at DESC LIMIT 1
--
-- served by 032's zhen_agent_runs_card_idx (card_id, claimed_at DESC).
-- One lookup per opened card; no new index.
--
-- Writer: none new (app_zhen through 032's grants). Reader: app_kanban,
-- column SELECT only; no INSERT, UPDATE or DELETE.
-- Retention: unchanged (runs are kept, 032).
--
-- Lock plan: a GRANT is a catalog update, instant; no table lock beyond
-- that. Rollback: a later numbered migration revokes the grant
-- (kanban-app then shows cards without the agent-run block; it treats a
-- refused read as "no run"). Proof: scripts/well/test-zhen-agent-runs.sh
-- (step 10).

SET lock_timeout = '5s';

GRANT SELECT (id, card_id, claimed_at, status, mode, family, branch, head_commit, route_summary, review_reason, reviewed_at)
   ON zhen_agent_runs TO app_kanban;
