-- 011_zhen_actions_relax_constraints.sql
--
-- Migration 009 set CHECK constraints on zhen_actions.action_type and
-- .triggered_by enumerating the action-type vocabulary as it stood
-- when the table was first designed. The cmd/zhen-agentd daemon and
-- pkg/champion gate emit a wider set:
--
--   action_type:
--     tool_call_attempt
--     tool_call_user_confirmed
--     tool_call_confirmed_destructive_blocked
--     tool_call_confirmed_path_denied
--     runbook.execute        (was named differently in 009 — was missing entirely)
--     denied_destructive
--     denied_untrusted_justification
--     ... (the list grows as new tools are added in pkg/champion/dispatch.go)
--
--   triggered_by:
--     direct       (from /api/v1/tool/exec, browser-clicked actions)
--     zhen-agent   (CLI binary)
--     zhen-agentd  (daemon)
--
-- The daemon's pkg/champion code is the authoritative source of valid
-- values; rigid CHECK constraints in PG cause INSERTs to silently fail
-- because pgstore.LogAction wraps the error in fmt.Errorf and the
-- caller (champion.logAction) intentionally doesn't surface it (so that
-- audit-log failures don't break the main mutation path).
--
-- Decision: drop the CHECK constraints on action_type and triggered_by.
-- Keep the status CHECK (status values are stable + small).
--
-- Idempotent: ALTER TABLE ... DROP CONSTRAINT IF EXISTS.

\set ON_ERROR_STOP on
\c unheaded_app

ALTER TABLE zhen_actions
    DROP CONSTRAINT IF EXISTS zhen_actions_action_type_check;

ALTER TABLE zhen_actions
    DROP CONSTRAINT IF EXISTS zhen_actions_triggered_by_check;

-- The status check stays — completed/failed/planned/etc are stable.

\echo '011_zhen_actions_relax_constraints.sql applied'
