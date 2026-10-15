-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — a read-only role for backups (ERDA 1.4), cluster-level.
--
-- scripts/db-backup.sh dumped as the superuser. well_backup can read every
-- table (pg_read_all_data, PG 14+) and nothing else: no writes, no DDL, no
-- role changes. Backups use pg_dumpall --globals-only --no-role-passwords,
-- which a non-superuser can run; role passwords live in the environment
-- (W-07), so a restore sets them again rather than restoring hashes.
-- No password here: inside the container the backup connects over the local
-- socket; db/init.sh sets one from WELL_BACKUP_PASSWORD if given (TCP use).

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_roles WHERE rolname = 'well_backup') THEN
        CREATE ROLE well_backup LOGIN CONNECTION LIMIT 3;
    END IF;
END $$;
GRANT pg_read_all_data TO well_backup;
