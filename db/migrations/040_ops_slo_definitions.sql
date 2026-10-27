-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — SLO definitions as data (ADR-100 Phase 2, S-WELL-SPRING
-- Phase 3), unheaded_ops.
--
-- observability/alerting/policy.yaml stays the reviewable source: a change
-- to an objective is a reviewed commit. `go run ./cmd/alert-policy
-- -sync-well` copies its SLOs here (one row each, the policy file's sha256
-- with every row), and `alert-policy -check` with WELL_DB set fails when a
-- row and the YAML differ. The dashboard and the compliance `slo` evidence
-- read the rows.
--
-- Columns follow the YAML, in the units the YAML implies once rendered:
--   objective      a fraction (policy 99.5 -> 0.9950); four places, so the
--                  policy allows at most two decimals in percent
--   slo_window     the budget window (policy `window: 28d` -> 28 days);
--                  `window` is a reserved word
--   sli_good_expr  the time-based SLI, whitespace folded to one line
--   sli_total_expr NULL for a time-based SLI (all of them today); a
--                  request-based SLI would set it
--   urgency        page (policy `page: true`) or ticket; none is allowed
--                  for an SLO kept for the record only
--   runbook_url    the URL the burn alerts carry
--
-- History: every INSERT, UPDATE and DELETE adds a row to
-- slo_definitions_history (old and new row as JSON, session user, and
-- well.change_reason when the writer sets it), written by a SECURITY
-- DEFINER trigger so the writer needs no INSERT on the history. The
-- history is append-only like audit_events (015): UPDATE, DELETE and
-- TRUNCATE are refused even for the owner. TRUNCATE of slo_definitions is
-- refused too: it fires no row trigger and would leave no history.
--
-- Writer: ops_writer (SELECT, INSERT, UPDATE, DELETE: -sync-well deletes a
-- row whose SLO left the YAML). Reader: ops_reader. Owner: well_ops_owner.
--
-- Access paths: the whole table (tens of rows) by the drift check and the
-- collector; history by name for a person reading it. No secondary index:
-- both tables stay tiny (one history row per policy change).
--
-- Lock plan: two new tables, no lock on anything that exists. Rollback:
-- none needed for new tables; a later numbered migration drops them.
-- Proof: scripts/well/test-slo-definitions.sh (throwaway server).

CREATE TABLE slo_definitions (
    name           TEXT PRIMARY KEY CHECK (name ~ '^[a-z][a-z0-9-]{0,39}$'),
    service        TEXT NOT NULL CHECK (service <> '' AND length(service) <= 64),
    description    TEXT NOT NULL CHECK (description <> '' AND length(description) <= 512),
    objective      NUMERIC(6, 4) NOT NULL CHECK (objective > 0 AND objective < 1),
    slo_window     INTERVAL NOT NULL CHECK (slo_window > INTERVAL '0'),
    sli_good_expr  TEXT NOT NULL CHECK (sli_good_expr <> '' AND length(sli_good_expr) <= 4096),
    sli_total_expr TEXT CHECK (sli_total_expr <> '' AND length(sli_total_expr) <= 4096),
    urgency        TEXT NOT NULL CHECK (urgency IN ('page', 'ticket', 'none')),
    runbook_url    TEXT NOT NULL CHECK (runbook_url <> '' AND length(runbook_url) <= 512),
    source_sha256  TEXT NOT NULL CHECK (source_sha256 ~ '^[0-9a-f]{64}$'),
    updated_at     TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE slo_definitions_history (
    id            BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    name          TEXT NOT NULL,
    op            TEXT NOT NULL CHECK (op IN ('INSERT', 'UPDATE', 'DELETE')),
    old_row       JSONB,
    new_row       JSONB,
    changed_by    TEXT NOT NULL,
    change_reason TEXT NOT NULL DEFAULT '' CHECK (length(change_reason) <= 512),
    changed_at    TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE OR REPLACE FUNCTION slo_definitions_track() RETURNS TRIGGER
    LANGUAGE plpgsql SECURITY DEFINER SET search_path = pg_catalog, public AS $$
DECLARE
    reason TEXT := left(coalesce(current_setting('well.change_reason', true), ''), 512);
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO public.slo_definitions_history (name, op, old_row, new_row, changed_by, change_reason)
        VALUES (NEW.name, TG_OP, NULL, to_jsonb(NEW), session_user, reason);
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO public.slo_definitions_history (name, op, old_row, new_row, changed_by, change_reason)
        VALUES (NEW.name, TG_OP, to_jsonb(OLD), to_jsonb(NEW), session_user, reason);
    ELSE
        INSERT INTO public.slo_definitions_history (name, op, old_row, new_row, changed_by, change_reason)
        VALUES (OLD.name, TG_OP, to_jsonb(OLD), NULL, session_user, reason);
    END IF;
    RETURN NULL;
END $$;

REVOKE ALL ON FUNCTION slo_definitions_track() FROM PUBLIC;

CREATE OR REPLACE TRIGGER trg_slo_definitions_history
    AFTER INSERT OR DELETE ON slo_definitions
    FOR EACH ROW EXECUTE FUNCTION slo_definitions_track();
-- An UPDATE that changes nothing leaves no history row.
CREATE OR REPLACE TRIGGER trg_slo_definitions_history_update
    AFTER UPDATE ON slo_definitions
    FOR EACH ROW WHEN (OLD.* IS DISTINCT FROM NEW.*)
    EXECUTE FUNCTION slo_definitions_track();
CREATE OR REPLACE TRIGGER trg_slo_definitions_no_truncate
    BEFORE TRUNCATE ON slo_definitions
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

CREATE OR REPLACE TRIGGER trg_slo_definitions_history_append_only
    BEFORE UPDATE OR DELETE ON slo_definitions_history
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_slo_definitions_history_no_truncate
    BEFORE TRUNCATE ON slo_definitions_history
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

GRANT SELECT, INSERT, UPDATE, DELETE ON slo_definitions TO ops_writer;
GRANT SELECT ON slo_definitions_history TO ops_writer;
GRANT SELECT ON slo_definitions, slo_definitions_history TO ops_reader;
