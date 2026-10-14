-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — append-only history in unheaded_config (W-05).
--
-- kingdom_config_history is written by 005's track_config_change() trigger,
-- which runs as the role changing kingdom_config (config_admin), so that role
-- keeps INSERT. 005 also granted it UPDATE and DELETE: revoked here, and
-- triggers refuse UPDATE, DELETE and TRUNCATE for everyone, owner included.
-- Same pattern as 013 and 015. Re-runnable.

CREATE OR REPLACE FUNCTION well_append_only()
RETURNS TRIGGER AS $$
BEGIN
    RAISE EXCEPTION '% is append-only', TG_TABLE_NAME;
END;
$$ LANGUAGE plpgsql;

REVOKE UPDATE, DELETE, TRUNCATE ON kingdom_config_history FROM config_admin;

CREATE OR REPLACE TRIGGER trg_kingdom_config_history_append_only
    BEFORE UPDATE OR DELETE ON kingdom_config_history
    FOR EACH ROW EXECUTE FUNCTION well_append_only();
CREATE OR REPLACE TRIGGER trg_kingdom_config_history_no_truncate
    BEFORE TRUNCATE ON kingdom_config_history
    FOR EACH STATEMENT EXECUTE FUNCTION well_append_only();
