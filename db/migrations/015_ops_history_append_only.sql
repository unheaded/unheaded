-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — append-only history in unheaded_ops (W-05).
--
-- audit_events (the Anamnesis log) and service_health_transitions are
-- history: rows are added, never changed. 004 granted ops_writer UPDATE and
-- DELETE on both, and nothing stopped the owner. Same pattern as 014's
-- compliance tables: writers get INSERT/SELECT only, and triggers refuse
-- UPDATE, DELETE and TRUNCATE for everyone, owner included (a superuser can
-- still drop the trigger; that is DDL, visible in the ledger and the log).
-- Retention will drop time partitions (W-15), never delete rows.
--
-- Re-runnable: CREATE OR REPLACE throughout, REVOKE of an absent grant is a
-- no-op.

CREATE OR REPLACE FUNCTION well_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

REVOKE UPDATE, DELETE, TRUNCATE ON audit_events, service_health_transitions FROM ops_writer;

CREATE OR REPLACE TRIGGER trg_audit_events_append_only
    BEFORE UPDATE OR DELETE ON audit_events
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
-- TRUNCATE fires no row trigger: guard it per statement.
CREATE OR REPLACE TRIGGER trg_audit_events_no_truncate
    BEFORE TRUNCATE ON audit_events
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();

CREATE OR REPLACE TRIGGER trg_service_health_transitions_append_only
    BEFORE UPDATE OR DELETE ON service_health_transitions
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_service_health_transitions_no_truncate
    BEFORE TRUNCATE ON service_health_transitions
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
