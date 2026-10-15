-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — kingdom_config history records every change (W-18),
-- unheaded_config.
--
-- 005's trigger fired on UPDATE only: a key created or deleted left no
-- trace. Now INSERT, UPDATE (when the value changes) and DELETE each add a
-- history row with the operation. The reason comes from the session
-- setting well.change_reason when the writer sets it
-- (SET LOCAL well.change_reason = '...'); making it required waits until
-- every writer sets it (pkg/database/config_store.go does not yet).
-- Adding `op` with a constant default is a catalog-only change on PG 11+.

ALTER TABLE kingdom_config_history
    ADD COLUMN IF NOT EXISTS op TEXT NOT NULL DEFAULT 'UPDATE'
        CHECK (op IN ('INSERT', 'UPDATE', 'DELETE'));

CREATE OR REPLACE FUNCTION track_config_change()
RETURNS TRIGGER AS $$
DECLARE
    reason TEXT := coalesce(current_setting('well.change_reason', true), '');
BEGIN
    IF TG_OP = 'INSERT' THEN
        INSERT INTO kingdom_config_history (key, old_value, new_value, changed_by, change_reason, op, changed_at)
        VALUES (NEW.key, NULL, NEW.value, NEW.updated_by, reason, TG_OP, now());
        RETURN NEW;
    ELSIF TG_OP = 'UPDATE' THEN
        INSERT INTO kingdom_config_history (key, old_value, new_value, changed_by, change_reason, op, changed_at)
        VALUES (OLD.key, OLD.value, NEW.value, NEW.updated_by, reason, TG_OP, now());
        RETURN NEW;
    END IF;
    INSERT INTO kingdom_config_history (key, old_value, new_value, changed_by, change_reason, op, changed_at)
    VALUES (OLD.key, OLD.value, NULL, current_user, reason, TG_OP, now());
    RETURN OLD;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_kingdom_config_history ON kingdom_config;
CREATE TRIGGER trg_kingdom_config_history
    AFTER UPDATE ON kingdom_config
    FOR EACH ROW
    WHEN (OLD.value IS DISTINCT FROM NEW.value)
    EXECUTE FUNCTION track_config_change();
CREATE OR REPLACE TRIGGER trg_kingdom_config_history_insert_delete
    AFTER INSERT OR DELETE ON kingdom_config
    FOR EACH ROW
    EXECUTE FUNCTION track_config_change();
