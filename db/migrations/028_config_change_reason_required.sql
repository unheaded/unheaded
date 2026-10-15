-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — every kingdom_config change says why (ERDA 5.6, W-18),
-- unheaded_config.
--
-- 019 recorded well.change_reason when a writer set it. Now the only
-- writer, pkg/database ConfigStore.Set, sets it in its transaction, so an
-- empty reason is refused: a change by hand in psql needs
--   BEGIN; SET LOCAL well.change_reason = 'why'; UPDATE kingdom_config ...; COMMIT;
-- and a migration that changes kingdom_config sets it the same way.

CREATE OR REPLACE FUNCTION track_config_change()
RETURNS TRIGGER AS $$
DECLARE
    reason TEXT := coalesce(current_setting('well.change_reason', true), '');
BEGIN
    IF btrim(reason) = '' THEN
        RAISE EXCEPTION 'kingdom_config: % of % needs a reason (SET LOCAL well.change_reason)',
            TG_OP, coalesce(NEW.key, OLD.key);
    END IF;
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
