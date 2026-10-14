-- SPDX-License-Identifier: GPL-3.0-or-later
-- The Well — huginn_reader monitoring user
-- Read-only access to pg_monitor views for huginn DB health metrics.
-- Run as superuser: psql -U unheaded -f huginn_reader.sql
-- No password here: db/init.sh sets it from HUGINN_READER_PASSWORD.

CREATE USER huginn_reader CONNECTION LIMIT 3;

-- pg_monitor grants access to pg_stat_*, pg_stat_activity, pg_stat_replication
GRANT pg_monitor TO huginn_reader;

-- Allow connecting to each database
GRANT CONNECT ON DATABASE unheaded     TO huginn_reader;
GRANT CONNECT ON DATABASE unheaded_app TO huginn_reader;
GRANT CONNECT ON DATABASE unheaded_ops TO huginn_reader;
GRANT CONNECT ON DATABASE unheaded_config TO huginn_reader;
