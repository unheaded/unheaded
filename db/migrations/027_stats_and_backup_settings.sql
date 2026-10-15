-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — statement statistics and the backup role's session setting
-- (ERDA 4.1), cluster-level.
--
-- db/postgresql.conf preloads pg_stat_statements; the view lives in
-- `postgres` and shows every database's statements to pg_monitor members
-- (huginn_reader). Without the preload (a test container with the image's
-- own config) the extension still installs; reading it then errors.
--
-- idle_in_transaction_session_timeout is 60 s server-wide. db-backup.sh
-- holds one transaction open to export the snapshot every dump reads, idle
-- while pg_dump runs: well_backup is exempt.

CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
ALTER ROLE well_backup SET idle_in_transaction_session_timeout = 0;
